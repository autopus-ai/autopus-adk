package cli

import (
	"crypto/rand"
	"fmt"
	"reflect"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// qualityPlaceholderPattern is the ${NAME} placeholder config.Load expands
// across the whole autopus.yaml text before it decodes (envVarPattern in
// pkg/config/loader.go). TestQualityPlaceholderPattern_MatchesTheLoader holds
// the two together.
var qualityPlaceholderPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// verifyQualityPlaceholders refuses, with errQualityWriteRefused, a block
// rewrite that activates a placeholder (SPEC-MODELUX-001 REQ-MUX-08, security
// review F1). The raw decode resolves YAML escapes and tags, so a value
// spelled "\x24{NAME}" or `!!binary` holds the text ${NAME} while the file
// spells no placeholder; re-encoding spells it out, and config.Load would
// expand it on its next read. original is the autopus.yaml bytes on disk,
// before any retired-key prune: no placeholder may land in updated where the
// loader did not already expand that variable in original
// (placeholdersActivated), and updated, loaded as config.Load loads it in
// this environment, must hold exactly candidate.
func verifyQualityPlaceholders(original, updated []byte, candidate config.QualityConf) error {
	if activated, err := placeholdersActivated(original, updated); err != nil || activated {
		return errQualityWriteRefused
	}
	loaded, err := config.ParseConfig(updated)
	if err != nil {
		return fmt.Errorf("validate written config: %w", err)
	}
	normalizeQualityConf(&loaded.Quality)
	if !reflect.DeepEqual(loaded.Quality, candidate) {
		return errQualityWriteRefused
	}
	return nil
}

// yamlPlaceholder is one ${NAME} placeholder the loader expands into data:
// the variable name and the path of the key or value it lands in.
type yamlPlaceholder struct{ path, name string }

// placeholdersActivated reports whether updated holds, at some path, more
// placeholders of a variable that config.Load expands than original holds
// there. Paths keep a placeholder from moving: one active in a key the edit
// drops cannot vouch for one the rewrite spells out in another value.
func placeholdersActivated(original, updated []byte) (bool, error) {
	marker := rand.Text()
	before, err := activeYAMLPlaceholders(original, marker)
	if err != nil {
		return false, err
	}
	after, err := activeYAMLPlaceholders(updated, marker)
	if err != nil {
		return false, err
	}
	for placeholder, count := range after {
		if count > before[placeholder] {
			return true, nil
		}
	}
	return false, nil
}

// activeYAMLPlaceholders counts the placeholders of data that config.Load
// expands into the document's keys and values, by path. Like the loader with
// every variable set, it replaces each placeholder of the text, here by
// marker-NAME-marker, then parses the result and finds those stand-ins in
// every scalar. marker is random per check, so no escape in the file can
// spell a stand-in. A placeholder in a comment never reaches the data, and
// one spelled through an escape or a tag is not one the loader expands, so
// neither counts. A key segment of a path is quoted, so no two paths collide.
func activeYAMLPlaceholders(data []byte, marker string) (map[yamlPlaceholder]int, error) {
	quoted := regexp.QuoteMeta(marker)
	standIn := regexp.MustCompile(quoted + `-([A-Za-z_][A-Za-z0-9_]*)-` + quoted)
	substituted := qualityPlaceholderPattern.ReplaceAllString(string(data), marker+"-${1}-"+marker)
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(substituted), &doc); err != nil {
		return nil, err
	}
	counts := map[yamlPlaceholder]int{}
	var walk func(node *yaml.Node, path string)
	walk = func(node *yaml.Node, path string) {
		for _, match := range standIn.FindAllStringSubmatch(node.Value, -1) {
			counts[yamlPlaceholder{path: path, name: match[1]}]++
		}
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				child := path + "/" + strconv.Quote(node.Content[i].Value)
				walk(node.Content[i], child+"#key")
				walk(node.Content[i+1], child)
			}
		case yaml.SequenceNode:
			for i, item := range node.Content {
				walk(item, path+"/"+strconv.Itoa(i))
			}
		case yaml.DocumentNode:
			for _, item := range node.Content {
				walk(item, path)
			}
		}
	}
	walk(&doc, "")
	return counts, nil
}
