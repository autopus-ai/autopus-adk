package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

var (
	createTempQualityConfig = os.CreateTemp
	chmodQualityConfig      = func(file *os.File, mode os.FileMode) error { return file.Chmod(mode) }
	writeQualityConfig      = func(file *os.File, data []byte) (int, error) { return file.Write(data) }
	syncQualityConfig       = func(file *os.File) error { return file.Sync() }
	closeQualityConfig      = func(file *os.File) error { return file.Close() }
	renameQualityConfig     = os.Rename
)

func saveQualityDefault(dir string, cfg *config.HarnessConfig, preset string) error {
	cfg.Quality.Default = preset
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	return saveQualityScalar(dir, cfg, "default", preset)
}

func saveQualitySupervisorPolicy(dir string, cfg *config.HarnessConfig, policy string) error {
	cfg.Quality.SupervisorModelPolicy = policy
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	return saveQualityScalar(dir, cfg, "supervisor_model_policy", policy)
}

func saveQualityScalar(dir string, cfg *config.HarnessConfig, key, value string) error {
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
	// The retired orchestra keys never ride along (SPEC-PANERM-001 REQ-10).
	if data, _, err = pruneRetiredConfig(data); err != nil {
		return err
	}

	updated, err := updateQualityScalar(data, key, value)
	if err != nil {
		return err
	}
	if err := verifyQualityLineEdit(data, updated, "quality."+key); err != nil {
		return err
	}
	if err := validateQualityYAML(updated, cfg); err != nil {
		return err
	}
	if err := atomicWriteQualityConfig(path, updated); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// @AX:WARN [AUTO]: This YAML-preserving scalar update contains ten format and placement decision branches. @AX:SPEC SPEC-PROVIDER-QUALITY-001
// @AX:REASON [AUTO]: Encoding, comments, indentation, and insertion order must remain stable while rejecting unsupported YAML shapes.
func updateQualityScalar(data []byte, key, value string) ([]byte, error) {
	encodedValue, err := encodeQualityScalar(value)
	if err != nil {
		return nil, err
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
		return appendQualitySection(data, key, encodedValue), nil
	}
	if quality.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("quality must be a YAML mapping")
	}
	if fieldKey, field := yamlMappingPair(quality, key); field != nil {
		line := field.Line
		if fieldKey.Line > 0 {
			line = fieldKey.Line
		}
		return replaceQualityScalarLine(data, line-1, key, value)
	}

	lines := strings.SplitAfter(string(data), "\n")
	newline := detectConfigNewline(data)
	indent := strings.Repeat(" ", max(qualityKey.Column-1, 0)+2)
	insertAt := qualityKey.Line
	if len(quality.Content) > 0 {
		firstKey := quality.Content[0]
		insertAt = firstKey.Line - 1
		indent = lineIndent(lines, insertAt, indent)
	}
	if key != "default" {
		if defaultKey, defaultValue := yamlMappingPair(quality, "default"); defaultValue != nil {
			insertAt = defaultKey.Line
			indent = lineIndent(lines, defaultKey.Line-1, indent)
		}
	}
	return insertConfigLine(lines, insertAt, indent+key+": "+encodedValue+newline), nil
}

func findQualityDefaultLine(data []byte) (int, bool) {
	line, ok := findQualityScalarLine(data, "default")
	return line, ok
}

func findQualityScalarLine(data []byte, key string) (int, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return 0, false
	}
	_, quality := yamlMappingPair(doc.Content[0], "quality")
	_, field := yamlMappingPair(quality, key)
	if field == nil || field.Line <= 0 {
		return 0, false
	}
	return field.Line - 1, true
}

func yamlMappingPair(node *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i], node.Content[i+1]
		}
	}
	return nil, nil
}

func replaceQualityDefaultLine(data []byte, lineIdx int, preset string) ([]byte, error) {
	return replaceQualityScalarLine(data, lineIdx, "default", preset)
}

func replaceQualityScalarLine(data []byte, lineIdx int, key, value string) ([]byte, error) {
	encodedValue, err := encodeQualityScalar(value)
	if err != nil {
		return nil, err
	}
	lines := strings.SplitAfter(string(data), "\n")
	if lineIdx < 0 || lineIdx >= len(lines) {
		return nil, fmt.Errorf("quality.%s line index out of range", key)
	}
	line := lines[lineIdx]
	newline := ""
	if strings.HasSuffix(line, "\n") {
		newline = "\n"
		line = strings.TrimSuffix(line, "\n")
	}
	carriage := ""
	if strings.HasSuffix(line, "\r") {
		carriage = "\r"
		line = strings.TrimSuffix(line, "\r")
	}
	quotedKey := regexp.QuoteMeta(key)
	lineRE := regexp.MustCompile(`^(\s*(?:` + quotedKey + `|"` + quotedKey + `"|'` + quotedKey + `')\s*:\s*)([^#\r\n]*?)(\s*(?:#.*)?)$`)
	matches := lineRE.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("quality.%s line has unsupported format", key)
	}
	// Only the scalar content is replaced. The value's anchor and tag stay, so
	// a later alias of the anchor keeps binding to this node instead of an
	// earlier anchor of the same name; verifyQualityLineEdit refuses the edit
	// when that alias would change.
	properties := qualityValuePropertiesRE.FindString(matches[2])
	if properties != "" && strings.TrimRight(properties, " \t") == properties {
		properties += " "
	}
	lines[lineIdx] = matches[1] + properties + encodedValue + matches[3] + carriage + newline
	return []byte(strings.Join(lines, "")), nil
}

// qualityValuePropertiesRE matches the node properties that lead the value of
// a one-line mapping entry: at most one anchor and one tag, in either order,
// each followed by blanks or the end of the value.
var qualityValuePropertiesRE = regexp.MustCompile(`^(?:(?:&[0-9A-Za-z_-]+|!\S*)(?:[ \t]+|$)){1,2}`)

func encodeQualityScalar(value string) (string, error) {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode quality scalar: %w", err)
	}
	scalar := strings.TrimSuffix(string(encoded), "\n")
	if strings.ContainsAny(scalar, "\r\n") {
		return "", fmt.Errorf("quality scalar must encode on one line")
	}
	return scalar, nil
}

func appendQualitySection(data []byte, key, encodedValue string) []byte {
	content := string(data)
	newline := detectConfigNewline(data)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += newline
	}
	return []byte(content + "quality:" + newline + "  " + key + ": " + encodedValue + newline)
}

func insertConfigLine(lines []string, index int, line string) []byte {
	if index < 0 {
		index = 0
	}
	if index > len(lines) {
		index = len(lines)
	}
	if index > 0 && !strings.HasSuffix(lines[index-1], "\n") {
		lines[index-1] += detectConfigNewline([]byte(strings.Join(lines, "")))
	}
	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[:index]...)
	updated = append(updated, line)
	updated = append(updated, lines[index:]...)
	return []byte(strings.Join(updated, ""))
}

func lineIndent(lines []string, index int, fallback string) string {
	if index < 0 || index >= len(lines) {
		return fallback
	}
	line := strings.TrimSuffix(strings.TrimSuffix(lines[index], "\n"), "\r")
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}

func detectConfigNewline(data []byte) string {
	if strings.Contains(string(data), "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// @AX:WARN [AUTO]: This atomic persistence path contains nine filesystem and cleanup decision branches. @AX:SPEC SPEC-PROVIDER-QUALITY-001
// @AX:REASON [AUTO]: File mode preservation, sync, close, rename, and temporary-file cleanup ordering must remain fail-closed.
func atomicWriteQualityConfig(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	} else if !os.IsNotExist(err) {
		return err
	}

	tmp, err := createTempQualityConfig(filepath.Dir(path), ".autopus-quality-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := chmodQualityConfig(tmp, mode); err != nil {
		_ = closeQualityConfig(tmp)
		return err
	}
	if _, err := writeQualityConfig(tmp, data); err != nil {
		_ = closeQualityConfig(tmp)
		return err
	}
	if err := syncQualityConfig(tmp); err != nil {
		_ = closeQualityConfig(tmp)
		return err
	}
	if err := closeQualityConfig(tmp); err != nil {
		return err
	}
	if err := renameQualityConfig(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
