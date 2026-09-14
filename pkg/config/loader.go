package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFileName = "autopus.yaml"

// Load는 autopus.yaml을 로드한다. 파일이 없으면 기본 설정을 반환한다.
func Load(dir string) (*HarnessConfig, error) {
	cfg, _, err := loadConfig(dir, true)
	return cfg, err
}

// LoadPreview는 autopus.yaml을 로드하되, 파일 정규화 결과를 디스크에 쓰지 않는다.
func LoadPreview(dir string) (*HarnessConfig, error) {
	cfg, _, err := loadConfig(dir, false)
	return cfg, err
}

// LoadPreviewWithMetadata는 preview 로드와 함께 정규화 필요 여부를 반환한다.
func LoadPreviewWithMetadata(dir string) (*HarnessConfig, bool, error) {
	return loadConfig(dir, false)
}

// MissingTopLevelKey reports whether an existing autopus.yaml lacks a top-level key.
func MissingTopLevelKey(dir string, key string) (bool, error) {
	path, err := checkedConfigPath(dir)
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read config: %w", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, fmt.Errorf("parse config: %w", err)
	}
	_, ok := raw[key]
	return !ok, nil
}

func loadConfig(dir string, persistNormalization bool) (*HarnessConfig, bool, error) {
	path, err := checkedConfigPath(dir)
	if err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			name := filepath.Base(dir)
			return DefaultFullConfig(name), false, nil
		}
		return nil, false, fmt.Errorf("read config: %w", err)
	}

	expanded := expandEnvVars(string(data))

	var cfg HarnessConfig
	// Strict: an unrecognised key is a typo, and Save re-marshals the struct,
	// so accepting it silently loses the user's line while looking like a
	// working setting. Keys that are deliberately tolerated are named in
	// removedConfigKeys and reservedConfigKeys, never inferred from absence.
	if err := decodeStrict([]byte(expanded), &cfg); err != nil {
		return nil, false, fmt.Errorf("parse config %s: %w", path, err)
	}
	applyMissingDefaults(&cfg, []byte(expanded))

	normalized := MigratePlatformNames(&cfg)
	if persistNormalization && normalized {
		// Persist the corrected config so subsequent loads don't repeat the migration.
		if corrected, marshalErr := marshalConfigPreservingPlaceholders(&cfg, data); marshalErr == nil {
			if err := rejectSymlinkComponents(path); err != nil {
				return nil, false, err
			}
			_ = os.WriteFile(path, corrected, 0644)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, false, fmt.Errorf("validate config: %w", err)
	}
	return &cfg, normalized, nil
}

// @AX:NOTE [AUTO]: Missing design block is backfilled for backward compatibility; explicit design config keeps caller intent.
func applyMissingDefaults(cfg *HarnessConfig, data []byte) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}
	defaults := DefaultFullConfig(cfg.ProjectName)
	if _, ok := raw["design"]; !ok {
		cfg.Design = defaults.Design
	}
	// Backfill the workflow section key by key. An absent (or empty) section
	// takes the whole default; a present section that omits coverage_threshold
	// still inherits the floor, because a zero there reaches the runner as
	// "no gate" and is indistinguishable from an explicit opt-out.
	section, ok := raw["workflow"].(map[string]any)
	if !ok {
		cfg.Workflow = defaults.Workflow
		return
	}
	if _, set := section["coverage_threshold"]; !set {
		cfg.Workflow.CoverageThreshold = defaults.Workflow.CoverageThreshold
	}
}

// Save validates and writes the config to autopus.yaml.
func Save(dir string, cfg *HarnessConfig) error {
	path, err := checkedConfigPath(dir)
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		data, err = preserveRawPlaceholders(raw, data)
		if err != nil {
			return fmt.Errorf("preserve config placeholders: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read existing config: %w", err)
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return match
	})
}

func marshalConfigPreservingPlaceholders(cfg *HarnessConfig, raw []byte) ([]byte, error) {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return preserveRawPlaceholders(raw, data)
}

func preserveRawPlaceholders(raw, marshaled []byte) ([]byte, error) {
	if !envVarPattern.Match(raw) {
		return marshaled, nil
	}

	var rawNode, marshaledNode yaml.Node
	if err := yaml.Unmarshal(raw, &rawNode); err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(marshaled, &marshaledNode); err != nil {
		return nil, err
	}
	restoreRawPlaceholders(&rawNode, &marshaledNode)
	return yaml.Marshal(&marshaledNode)
}

func restoreRawPlaceholders(raw, marshaled *yaml.Node) {
	if raw.Kind != marshaled.Kind {
		return
	}
	switch raw.Kind {
	case yaml.DocumentNode:
		if len(raw.Content) == 1 && len(marshaled.Content) == 1 {
			restoreRawPlaceholders(raw.Content[0], marshaled.Content[0])
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(marshaled.Content); i += 2 {
			for j := 0; j+1 < len(raw.Content); j += 2 {
				if yamlMapKeysMatch(raw.Content[j], marshaled.Content[i]) {
					restoreRawPlaceholders(raw.Content[j], marshaled.Content[i])
					restoreRawPlaceholders(raw.Content[j+1], marshaled.Content[i+1])
					break
				}
			}
		}
	case yaml.SequenceNode:
		for i := 0; i < len(raw.Content) && i < len(marshaled.Content); i++ {
			restoreRawPlaceholders(raw.Content[i], marshaled.Content[i])
		}
	case yaml.ScalarNode:
		if envVarPattern.MatchString(raw.Value) && expandEnvVars(raw.Value) == marshaled.Value {
			marshaled.Value = raw.Value
			marshaled.Tag = raw.Tag
			marshaled.Style = raw.Style
		}
	}
}

func yamlMapKeysMatch(raw, marshaled *yaml.Node) bool {
	if raw.Kind != yaml.ScalarNode || marshaled.Kind != yaml.ScalarNode {
		return raw.Value == marshaled.Value
	}
	return raw.Value == marshaled.Value ||
		envVarPattern.MatchString(raw.Value) && expandEnvVars(raw.Value) == marshaled.Value
}

func checkedConfigPath(dir string) (string, error) {
	absoluteDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	info, err := os.Lstat(absoluteDir)
	if os.IsNotExist(err) {
		return filepath.Join(absoluteDir, configFileName), nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect config directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("config path crosses symlink: %s", absoluteDir)
	}
	canonicalDir, err := filepath.EvalSymlinks(absoluteDir)
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	path := filepath.Join(canonicalDir, configFileName)
	if err := rejectSymlinkComponents(path); err != nil {
		return "", err
	}
	return path, nil
}

func rejectSymlinkComponents(path string) error {
	for _, component := range []string{filepath.Dir(path), path} {
		info, err := os.Lstat(component)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect config path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("config path crosses symlink: %s", component)
		}
	}
	return nil
}

// Exists reports whether dir holds an autopus.yaml. Load synthesizes a default
// config for a missing file rather than returning an error, so a caller that
// must not act on a synthesized platform list checks this first.
func Exists(dir string) bool {
	path, err := checkedConfigPath(dir)
	if err != nil {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
