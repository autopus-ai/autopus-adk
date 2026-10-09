package cli

import (
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// verifyQualityLineEdit is the whole-document check of the quality line
// writers, which edit raw lines of autopus.yaml instead of re-encoding a node
// tree. A line edit may change only the quality block: with the root quality
// entry set aside, edited must parse to the same data as original, every alias
// compared by the data it resolves to, and it must load as config.Load would
// load it. validateQualityYAML compares the Quality fields alone, so without
// this check an anchor the edit drops or keeps could rebind or change an alias
// elsewhere in the file. path names the edited entry for the error.
func verifyQualityLineEdit(original, edited []byte, path string) error {
	var before, after yaml.Node
	if err := yaml.Unmarshal(original, &before); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if err := yaml.Unmarshal(edited, &after); err != nil {
		return fmt.Errorf("validate written config: %w", err)
	}
	dropRootMappingKey(&before, "quality")
	dropRootMappingKey(&after, "quality")
	if !sameYAMLData(&before, &after) {
		return fmt.Errorf("refuse to write %s: the edit would also change data outside the quality block "+
			"(a YAML alias there resolves to an anchor the edit touches); edit autopus.yaml by hand", path)
	}
	if _, err := config.ParseConfig(edited); err != nil {
		return fmt.Errorf("validate written config: %w", err)
	}
	return nil
}

// dropRootMappingKey removes the root mapping entries named key from a parsed
// document it owns. The removed nodes stay reachable through any alias that
// resolves into them, so sameYAMLData still compares such an alias by its
// data.
func dropRootMappingKey(doc *yaml.Node, key string) {
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return
	}
	root := doc.Content[0]
	kept := make([]*yaml.Node, 0, len(root.Content))
	for i := 0; i+1 < len(root.Content); i += 2 {
		if entry := root.Content[i]; entry.Kind == yaml.ScalarNode && entry.Value == key {
			continue
		}
		kept = append(kept, root.Content[i], root.Content[i+1])
	}
	root.Content = kept
}

// validateQualityYAML checks that the bytes a quality writer is about to write
// decode to expected's Quality fields, and that those fields validate.
func validateQualityYAML(data []byte, expected *config.HarnessConfig) error {
	var parsed config.HarnessConfig
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("validate written config: %w", err)
	}
	candidate := *expected
	candidate.Quality = parsed.Quality
	if err := candidate.Validate(); err != nil {
		return fmt.Errorf("validate written config: %w", err)
	}
	if !reflect.DeepEqual(candidate.Quality, expected.Quality) {
		return fmt.Errorf("validate written config: quality fields changed unexpectedly")
	}
	return nil
}
