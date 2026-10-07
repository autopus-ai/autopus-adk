package opencode

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func (a *Adapter) prepareConfigMapping() (adapter.FileMapping, error) {
	configDoc, err := a.renderConfigDocument(nil)
	if err != nil {
		return adapter.FileMapping{}, err
	}
	return adapter.FileMapping{
		TargetPath:      configFile,
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        adapter.Checksum(configDoc),
		Content:         []byte(configDoc),
	}, nil
}

func (a *Adapter) renderConfigDocument(extraPlugins []string) (string, error) {
	if err := a.validateRuntime(); err != nil {
		return "", err
	}
	path := filepath.Join(a.root, configFile)
	doc, err := readJSONObject(path)
	if err != nil {
		return "", fmt.Errorf("%s 파싱 실패: %w", configFile, err)
	}
	rulePaths, deferred, err := managedRulePaths()
	if err != nil {
		return "", fmt.Errorf("rule 경로 생성 실패: %w", err)
	}
	doc["$schema"] = "https://opencode.ai/config.json"
	existing := jsonStringSlice(doc["instructions"])
	kept := existing[:0]
	for _, path := range existing {
		if !slices.Contains(deferred, filepath.ToSlash(filepath.Clean(path))) {
			kept = append(kept, path)
		}
	}
	doc["instructions"] = uniqueStrings(kept, rulePaths)
	plugins := managedPluginPaths(extraPlugins)
	if err := mergePluginConfig(doc, plugins, a.isV2(), a.root); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%s 직렬화 실패: %w", configFile, err)
	}
	return string(data) + "\n", nil
}

// retractStaleCompletionPlugins rewrites the opencode.json mapping of an
// update without the plugin entries that load a group S script and returns
// those scripts, so the same transaction can delete them (SPEC-PANERM-001
// REQ-13). Only Update calls it: Generate and InjectOrchestraPlugin keep the
// entries, and the next update retracts them together with their scripts.
func (a *Adapter) retractStaleCompletionPlugins(files []adapter.FileMapping) ([]string, error) {
	for i := range files {
		if filepath.ToSlash(files[i].TargetPath) != configFile {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(files[i].Content, &doc); err != nil {
			return nil, fmt.Errorf("%s 파싱 실패: %w", configFile, err)
		}
		scripts, err := retractStalePluginEntries(doc, a.isV2(), a.root)
		if err != nil || len(scripts) == 0 {
			return nil, err
		}
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("%s 직렬화 실패: %w", configFile, err)
		}
		content := string(data) + "\n"
		files[i].Content = []byte(content)
		files[i].Checksum = adapter.Checksum(content)
		return scripts, nil
	}
	return nil, nil
}

func managedPluginPaths(extraPlugins []string) []string {
	base := []string{toSlash(filepath.Join(".opencode", "plugins", "autopus-hooks.js"))}
	return uniqueStrings(base, extraPlugins)
}

// InjectOrchestraPlugin preserves legacy external callers by appending a plugin path.
func (a *Adapter) InjectOrchestraPlugin(scriptPath string) error {
	doc, err := a.renderConfigDocument([]string{toSlash(scriptPath)})
	if err != nil {
		return err
	}
	return writeMapping(a.root, adapter.FileMapping{
		TargetPath:      configFile,
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        adapter.Checksum(doc),
		Content:         []byte(doc),
	})
}
