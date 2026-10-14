package config

import (
	"crypto/rand"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// placeholderLineBreaks are the characters yaml.v3 reads as a line break.
const placeholderLineBreaks = "\r\n\u0085  "

const placeholderShapeRule = "a placeholder value may only replace the scalar it stands in"

// expandConfigPlaceholders expands the ${NAME} placeholders of a raw
// autopus.yaml for every reader that decodes it, and refuses an expansion
// that does more than fill the scalars the placeholders stand in (security
// re-check NEW-1). expandEnvVars works on the raw text, so a value could
// otherwise add a key, an item, or a block, or turn a scalar into an alias,
// an anchor, a tagged value, or a merge key, even on one line. The file is
// parsed once with every placeholder replaced by a token and once expanded,
// and the two trees must have one shape. A value with a line break is
// refused before either parse, and so is an explicit tag other than !!str on
// a placeholder (noteSecrets). Every refusal names variables, never a value.
// The tokens of a file with a placeholder come back with the expanded bytes,
// for the checks that read where a placeholder lands and for the errors that
// would quote a value it put there (spellDecodeError, spellError).
func expandConfigPlaceholders(raw []byte) ([]byte, *placeholderTokens, error) {
	if !envVarPattern.Match(raw) {
		return raw, nil, nil
	}
	tokens := tokenizePlaceholders(raw)
	for _, name := range tokens.names {
		if value, set := os.LookupEnv(name); set && strings.ContainsAny(value, placeholderLineBreaks) {
			return nil, nil, fmt.Errorf("config.placeholder_line_break: ${%s} expands to a value with a line break, "+
				"and a placeholder in autopus.yaml takes a one-line value", name)
		}
	}
	expanded := []byte(expandEnvVars(string(raw)))
	var tokenized, parsed yaml.Node
	if err := yaml.Unmarshal(tokens.text, &tokenized); err != nil {
		return nil, nil, fmt.Errorf("config.placeholder_shape_changed: autopus.yaml does not parse with %s unexpanded (%s), "+
			"so the shape the values give it cannot be checked", placeholderList(tokens.names), tokens.restore(err.Error()))
	}
	// The expanded parse error is never shown: it can quote the value.
	if yaml.Unmarshal(expanded, &parsed) != nil {
		return nil, nil, fmt.Errorf("config.placeholder_shape_changed: expanding %s breaks the YAML of autopus.yaml; %s",
			placeholderList(tokens.names), placeholderShapeRule)
	}
	if node, path := tokens.mismatch(&tokenized, &parsed, ""); node != nil {
		return nil, nil, fmt.Errorf("config.placeholder_shape_changed: expanding %s changes the YAML shape of autopus.yaml at %s; %s",
			placeholderList(tokens.namesIn(node)), placeholderPath(path), placeholderShapeRule)
	}
	if err := tokens.noteSecrets(&tokenized, &parsed); err != nil {
		return nil, nil, err
	}
	return expanded, tokens, nil
}

// placeholderTokens is a raw autopus.yaml in which every placeholder
// expandEnvVars expands is replaced by a token naming its variable. A token
// is one alphanumeric word, so YAML parses the text around it as it parses
// the text around a short plain value, and its random prefix keeps the file
// itself from spelling one.
type placeholderTokens struct {
	text    []byte
	names   []string
	prefix  string
	secrets []placeholderSecret
}

func tokenizePlaceholders(raw []byte) *placeholderTokens {
	tokens := &placeholderTokens{prefix: "AutopusEnv" + rand.Text() + "n"}
	index := map[string]int{}
	tokens.text = envVarPattern.ReplaceAllFunc(raw, func(match []byte) []byte {
		name := string(match[2 : len(match)-1])
		position, seen := index[name]
		if !seen {
			position = len(tokens.names)
			index[name] = position
			tokens.names = append(tokens.names, name)
		}
		return []byte(tokens.prefix + strconv.Itoa(position) + "x")
	})
	return tokens
}

func (p *placeholderTokens) holds(text string) bool {
	return strings.Contains(text, p.prefix)
}

// replace returns text with every token replaced by what spell returns for
// its variable, calling spell in the order the tokens appear.
func (p *placeholderTokens) replace(text string, spell func(name string) string) string {
	var out strings.Builder
	for {
		start := strings.Index(text, p.prefix)
		if start < 0 {
			out.WriteString(text)
			return out.String()
		}
		rest := text[start+len(p.prefix):]
		digits := 0
		for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
			digits++
		}
		position, err := strconv.Atoi(rest[:digits])
		if err != nil || digits == len(rest) || rest[digits] != 'x' || position >= len(p.names) {
			out.WriteString(text[:start+len(p.prefix)])
			text = rest
			continue
		}
		out.WriteString(text[:start] + spell(p.names[position]))
		text = rest[digits+1:]
	}
}

// restore spells every token of text as the placeholder it replaced.
func (p *placeholderTokens) restore(text string) string {
	return p.replace(text, func(name string) string { return "${" + name + "}" })
}

// namesIn lists the variables whose tokens node or a node under it holds in
// a value, a key, a tag, or an anchor, or every variable when it holds none.
func (p *placeholderTokens) namesIn(node *yaml.Node) []string {
	seen := map[string]bool{}
	var names []string
	var walk func(*yaml.Node)
	walk = func(current *yaml.Node) {
		for _, text := range []string{current.Tag, current.Anchor, current.Value} {
			p.replace(text, func(name string) string {
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
				return ""
			})
		}
		for _, child := range current.Content {
			walk(child)
		}
	}
	walk(node)
	if len(names) == 0 {
		return p.names
	}
	return names
}

// mismatch returns the node of the tokenized tree where the expanded tree
// stops having its shape, with the node's path, or nil when the trees have
// one shape: the same kinds, anchors, alias names, explicit tags, mapping
// keys, and sequence lengths. Only a scalar holding a token may read
// differently, and a key holding one may take no value another key of its
// mapping takes. An alias is not followed: the node it names is compared
// where it is anchored.
func (p *placeholderTokens) mismatch(tokenized, expanded *yaml.Node, path string) (*yaml.Node, string) {
	if tokenized.Kind != expanded.Kind || tokenized.Anchor != expanded.Anchor ||
		len(tokenized.Content) != len(expanded.Content) || !p.sameLabel(tokenized, expanded) ||
		tokenized.Kind == yaml.MappingNode && !p.sameKeys(tokenized, expanded) {
		return tokenized, path
	}
	for i := range tokenized.Content {
		child := path
		switch tokenized.Kind {
		case yaml.SequenceNode:
			child = fmt.Sprintf("%s[%d]", path, i)
		case yaml.MappingNode:
			child = joinPlaceholderPath(path, p.restore(tokenized.Content[i-i%2].Value))
		}
		if node, at := p.mismatch(tokenized.Content[i], expanded.Content[i], child); node != nil {
			return node, at
		}
	}
	return nil, ""
}

// sameLabel compares the value and tag of two nodes. A scalar holding a
// token takes the expanded value and the tag that value resolves to, but an
// explicit tag belongs to the file and stays the file's.
func (p *placeholderTokens) sameLabel(tokenized, expanded *yaml.Node) bool {
	tagged := tokenized.Style&yaml.TaggedStyle != 0
	if tagged != (expanded.Style&yaml.TaggedStyle != 0) {
		return false
	}
	if tokenized.Kind == yaml.ScalarNode && p.holds(tokenized.Value) {
		return !tagged || tokenized.Tag == expanded.Tag
	}
	return tokenized.Value == expanded.Value && tokenized.Tag == expanded.Tag
}

// sameKeys reports whether the expanded keys of a mapping stay one to one
// with its tokenized keys: no key turns into a merge key or out of one, and
// a key holding a token takes no value another key of the mapping takes.
func (p *placeholderTokens) sameKeys(tokenized, expanded *yaml.Node) bool {
	keys := map[string]string{}
	for i := 0; i+1 < len(tokenized.Content); i += 2 {
		key, expandedKey := tokenized.Content[i], expanded.Content[i]
		if isMergeKey(key) != isMergeKey(expandedKey) {
			return false
		}
		if expandedKey.Kind != yaml.ScalarNode {
			continue
		}
		if previous, taken := keys[expandedKey.Value]; taken && (previous != key.Value || p.holds(key.Value)) {
			return false
		}
		keys[expandedKey.Value] = key.Value
	}
	return true
}

func placeholderList(names []string) string {
	spelled := make([]string, len(names))
	for i, name := range names {
		spelled[i] = "${" + name + "}"
	}
	return strings.Join(spelled, ", ")
}
