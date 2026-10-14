package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// minSpelledSecret is the shortest secret spelled where an error holds it
// bare: a shorter one is no secret and would match ordinary words.
const minSpelledSecret = 4

// placeholderSecret is a text a placeholder put into the expanded file that
// the file does not spell itself: the value of a set variable, or a whole
// scalar holding a placeholder. The decoder and Validate quote the key or
// value they reject, and the retired-key notice names the paths it pruned,
// so a failing load could otherwise print a variable of the environment
// (security re-check NEW-1). line and where locate a scalar: the line the
// decoder names for it, and the path of its value or, for a key, of its
// mapping. A variable's value has neither.
type placeholderSecret struct {
	value   string
	spelled string
	line    int
	where   string
}

// noteSecrets refuses a placeholder scalar with an explicit tag other than
// !!str, since such a tag makes the decoder convert or retype the value:
// !!binary decodes it into text no error could be checked for, and !!int
// quotes it whole when it does not resolve. It then notes the secrets of the
// file, longest first. tokenized and expanded have one shape.
func (p *placeholderTokens) noteSecrets(tokenized, expanded *yaml.Node) error {
	for _, name := range p.names {
		if value, set := os.LookupEnv(name); set {
			p.noteSecret(placeholderSecret{value: value, spelled: "${" + name + "}"})
		}
	}
	if err := p.noteScalars(tokenized, expanded, "", ""); err != nil {
		return err
	}
	sort.SliceStable(p.secrets, func(i, j int) bool { return len(p.secrets[i].value) > len(p.secrets[j].value) })
	return nil
}

func (p *placeholderTokens) noteScalars(tokenized, expanded *yaml.Node, path, where string) error {
	if tokenized.Kind == yaml.ScalarNode && p.holds(tokenized.Value) {
		if tokenized.Style&yaml.TaggedStyle != 0 && tokenized.Tag != "!!str" {
			return fmt.Errorf("config.placeholder_tag_unsupported: %s at %s carries the tag %s, and a placeholder "+
				"takes no tag but !!str: another tag makes the decoder convert or retype the value",
				placeholderList(p.namesIn(tokenized)), placeholderPath(path), tokenized.Tag)
		}
		p.noteSecret(placeholderSecret{value: expanded.Value, spelled: p.restore(tokenized.Value),
			line: tokenized.Line, where: where})
	}
	for i := range tokenized.Content {
		child, childWhere := path, path
		switch tokenized.Kind {
		case yaml.SequenceNode:
			child = fmt.Sprintf("%s[%d]", path, i)
			childWhere = child
		case yaml.MappingNode:
			child = joinPlaceholderPath(path, p.restore(tokenized.Content[i-i%2].Value))
			if i%2 == 1 {
				childWhere = child
			}
		}
		if err := p.noteScalars(tokenized.Content[i], expanded.Content[i], child, childWhere); err != nil {
			return err
		}
	}
	return nil
}

func (p *placeholderTokens) noteSecret(secret placeholderSecret) {
	if secret.value == "" || secret.value == secret.spelled || bytes.Contains(p.text, []byte(secret.value)) {
		return
	}
	p.secrets = append(p.secrets, secret)
}

// spelling is how an error shows a secret: as the file spells it, with its
// line breaks escaped so the error keeps its lines.
func (s placeholderSecret) spelling() string {
	return strings.NewReplacer("\r", `\r`, "\n", `\n`).Replace(s.spelled)
}

// spell replaces every secret text holds, quoted as %q quotes it or bare, by
// its spelling.
func (p *placeholderTokens) spell(text string) string {
	for _, secret := range p.secrets {
		text = strings.ReplaceAll(text, strconv.Quote(secret.value), secret.spelling())
		if len(secret.value) >= minSpelledSecret {
			text = strings.ReplaceAll(text, secret.value, secret.spelling())
		}
	}
	return text
}

// spellItem spells the secrets of one yaml.TypeError item. The item names
// the line its node starts on; there the decoder quotes a value longer than
// ten bytes cut to its first seven, and the item gains the path of the
// scalar it quotes.
func (p *placeholderTokens) spellItem(item string) string {
	var line int
	head, rest, found := strings.Cut(item, ": ")
	if _, err := fmt.Sscanf(head, "line %d", &line); err != nil || !found {
		return p.spell(item)
	}
	where, located := "", false
	for _, secret := range p.secrets {
		if secret.line != line {
			continue
		}
		cut := secret.value
		if len(cut) > 10 {
			cut = cut[:7] + "..."
		}
		quoted := " `" + cut + "`"
		if !strings.Contains(rest, quoted) && !strings.Contains(rest, secret.value) {
			continue
		}
		rest = strings.ReplaceAll(rest, quoted, " `"+secret.spelling()+"`")
		if !located {
			where, located = secret.where, true
		}
	}
	rest = p.spell(rest)
	if where != "" {
		rest = where + ": " + rest
	}
	return head + ": " + rest
}

// spellDecodeError spells the secrets an error from decoding the expanded
// file holds. A yaml.TypeError stays one, spelled item by item.
func (p *placeholderTokens) spellDecodeError(err error) error {
	if p == nil || len(p.secrets) == 0 || err == nil {
		return err
	}
	if typeErr, ok := err.(*yaml.TypeError); ok {
		items := make([]string, len(typeErr.Errors))
		for i, item := range typeErr.Errors {
			items[i] = p.spellItem(item)
		}
		return &yaml.TypeError{Errors: items}
	}
	return p.spellError(err)
}

// spellError spells the secrets the message of err holds; an error that
// holds none is returned as it is.
func (p *placeholderTokens) spellError(err error) error {
	if p == nil || len(p.secrets) == 0 || err == nil {
		return err
	}
	if message := p.spell(err.Error()); message != err.Error() {
		return errors.New(message)
	}
	return err
}

// spellPaths spells the secrets each of paths holds.
func (p *placeholderTokens) spellPaths(paths []string) []string {
	if p == nil || len(p.secrets) == 0 {
		return paths
	}
	spelled := make([]string, len(paths))
	for i, path := range paths {
		spelled[i] = p.spell(path)
	}
	return spelled
}

func joinPlaceholderPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func placeholderPath(path string) string {
	if path == "" {
		return "the document root"
	}
	return path
}
