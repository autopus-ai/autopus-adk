package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

func saveQualityProvider(dir string, cfg *config.HarnessConfig, provider, preset string) error {
	if cfg.Quality.Providers == nil {
		cfg.Quality.Providers = make(map[string]string)
	}
	cfg.Quality.Providers[provider] = preset
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	return persistQualityProvider(dir, cfg, provider, preset, false)
}

func removeQualityProvider(dir string, cfg *config.HarnessConfig, provider string) error {
	delete(cfg.Quality.Providers, provider)
	if len(cfg.Quality.Providers) == 0 {
		cfg.Quality.Providers = nil
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	return persistQualityProvider(dir, cfg, provider, "", true)
}

// @AX:WARN [AUTO]: This persistence path contains eight read, validation, equality, and atomic-write decision branches. @AX:SPEC SPEC-PROVIDER-QUALITY-001
// @AX:REASON [AUTO]: The in-memory provider override and preserved YAML must stay equivalent before any on-disk replacement occurs.
func persistQualityProvider(
	dir string,
	cfg *config.HarnessConfig,
	provider string,
	preset string,
	remove bool,
) error {
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data, marshalErr := yaml.Marshal(cfg)
		if marshalErr != nil {
			return fmt.Errorf("marshal config: %w", marshalErr)
		}
		if writeErr := atomicWriteQualityConfig(path, data); writeErr != nil {
			return fmt.Errorf("write config: %w", writeErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	// Like saveQualityScalar: the retired orchestra keys never ride along.
	if data, _, err = pruneRetiredConfig(data); err != nil {
		return err
	}

	updated, err := updateQualityProvider(data, provider, preset, remove)
	if err != nil {
		return err
	}
	if err := verifyQualityLineEdit(data, updated, "quality.providers."+provider); err != nil {
		return err
	}
	if err := validateQualityYAML(updated, cfg); err != nil {
		return err
	}
	if bytes.Equal(data, updated) {
		return nil
	}
	if err := atomicWriteQualityConfig(path, updated); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// @AX:WARN [AUTO]: This YAML-preserving provider update contains fourteen shape, insertion, replacement, and removal decision branches. @AX:SPEC SPEC-PROVIDER-QUALITY-001
// @AX:REASON [AUTO]: Provider overrides must persist without rewriting unrelated YAML or accepting ambiguous flow and multiline mappings.
func updateQualityProvider(data []byte, provider, preset string, remove bool) ([]byte, error) {
	encodedPreset := ""
	if !remove {
		var err error
		encodedPreset, err = encodeQualityScalar(preset)
		if err != nil {
			return nil, err
		}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config root must be a YAML mapping")
	}

	root := doc.Content[0]
	qualityKey, quality := yamlMappingPair(root, "quality")
	if quality == nil {
		if remove {
			return data, nil
		}
		return appendQualityProviderSection(data, provider, encodedPreset), nil
	}
	if quality.Kind != yaml.MappingNode || quality.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("quality must be a block YAML mapping")
	}

	providersKey, providers := yamlMappingPair(quality, "providers")
	if providers == nil {
		if remove {
			return data, nil
		}
		return insertQualityProvidersMapping(data, qualityKey, quality, provider, encodedPreset), nil
	}
	if providers.Kind != yaml.MappingNode || providers.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("quality.providers must be a block YAML mapping")
	}

	providerKey, providerValue := yamlMappingPair(providers, provider)
	if providerValue != nil {
		if remove {
			return removeQualityProviderEntry(data, providersKey, providers, providerKey, providerValue), nil
		}
		if providerKey.Line != providerValue.Line ||
			providerValue.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 ||
			strings.ContainsAny(providerValue.Value, "\r\n") {
			return nil, fmt.Errorf("quality.providers.%s must use a single-line scalar", provider)
		}
		return replaceQualityScalarLine(data, providerValue.Line-1, provider, preset)
	}
	if remove {
		return data, nil
	}
	return insertQualityProviderEntry(data, providersKey, providers, provider, encodedPreset), nil
}

func appendQualityProviderSection(data []byte, provider, encodedPreset string) []byte {
	content := string(data)
	newline := detectConfigNewline(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += newline
	}
	return []byte(content +
		"quality:" + newline +
		"  providers:" + newline +
		"    " + provider + ": " + encodedPreset + newline)
}

func insertQualityProvidersMapping(
	data []byte,
	qualityKey *yaml.Node,
	quality *yaml.Node,
	provider string,
	encodedPreset string,
) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	newline := detectConfigNewline(data)
	childIndent := strings.Repeat(" ", max(qualityKey.Column-1, 0)+2)
	insertAt := qualityKey.Line

	if defaultKey, defaultValue := yamlMappingPair(quality, "default"); defaultValue != nil {
		insertAt = defaultValue.Line
		childIndent = lineIndent(lines, defaultKey.Line-1, childIndent)
	} else if len(quality.Content) > 0 {
		firstKey := quality.Content[0]
		insertAt = firstKey.Line - 1
		childIndent = lineIndent(lines, insertAt, childIndent)
	}
	block := childIndent + "providers:" + newline +
		childIndent + "  " + provider + ": " + encodedPreset + newline
	return insertConfigLine(lines, insertAt, block)
}

func insertQualityProviderEntry(
	data []byte,
	providersKey *yaml.Node,
	providers *yaml.Node,
	provider string,
	encodedPreset string,
) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	newline := detectConfigNewline(data)
	indent := strings.Repeat(" ", max(providersKey.Column-1, 0)+2)
	insertAt := providersKey.Line
	if len(providers.Content) > 0 {
		lastValue := providers.Content[len(providers.Content)-1]
		insertAt = lastValue.Line
		indent = lineIndent(lines, lastValue.Line-1, indent)
	}
	return insertConfigLine(lines, insertAt, indent+provider+": "+encodedPreset+newline)
}

func removeQualityProviderEntry(
	data []byte,
	providersKey *yaml.Node,
	providers *yaml.Node,
	providerKey *yaml.Node,
	providerValue *yaml.Node,
) []byte {
	startLine := providerKey.Line - 1
	endLine := providerValue.Line
	if len(providers.Content) == 2 {
		startLine = providersKey.Line - 1
	}
	return removeConfigLines(data, startLine, endLine)
}

func removeConfigLines(data []byte, start, end int) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	if start < 0 || start >= len(lines) || end <= start {
		return data
	}
	if end > len(lines) {
		end = len(lines)
	}
	updated := make([]string, 0, len(lines)-(end-start))
	updated = append(updated, lines[:start]...)
	updated = append(updated, lines[end:]...)
	return []byte(strings.Join(updated, ""))
}
