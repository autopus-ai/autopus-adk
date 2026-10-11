package cli

import (
	"crypto/rand"
	"fmt"
	"reflect"
	"regexp"

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
// expand it on its next read. So every placeholder the loader expands into
// the written quality section must be one it already expands into the
// original section, counted per name, and updated, loaded as config.Load
// loads it in this environment, must hold exactly candidate.
func verifyQualityPlaceholders(original, updated []byte, candidate config.QualityConf) error {
	marker := rand.Text()
	before, err := activeQualityPlaceholders(original, marker)
	if err != nil {
		return errQualityWriteRefused
	}
	after, err := activeQualityPlaceholders(updated, marker)
	if err != nil {
		return errQualityWriteRefused
	}
	for name, count := range after {
		if count > before[name] {
			return errQualityWriteRefused
		}
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

// activeQualityPlaceholders counts, per variable name, the placeholders of
// data that config.Load expands into the top-level quality section. Like the
// loader with every variable set, it replaces each placeholder of the text,
// here by marker-NAME-marker, then parses the result and counts those
// stand-ins in every key and value of the quality subtree. marker is random
// per write, so no escape in the file can spell a stand-in. A placeholder in
// a comment never reaches a value, and one spelled through an escape or a
// tag is not one the loader expands, so neither counts.
func activeQualityPlaceholders(data []byte, marker string) (map[string]int, error) {
	quoted := regexp.QuoteMeta(marker)
	standIn := regexp.MustCompile(quoted + `-([A-Za-z_][A-Za-z0-9_]*)-` + quoted)
	substituted := qualityPlaceholderPattern.ReplaceAllString(string(data), marker+"-${1}-"+marker)
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(substituted), &doc); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	if len(doc.Content) == 0 {
		return counts, nil
	}
	_, quality := yamlMappingPair(doc.Content[0], "quality")
	var walk func(*yaml.Node)
	walk = func(node *yaml.Node) {
		if node == nil {
			return
		}
		for _, match := range standIn.FindAllStringSubmatch(node.Value, -1) {
			counts[match[1]]++
		}
		for _, child := range node.Content {
			walk(child)
		}
	}
	walk(quality)
	return counts, nil
}
