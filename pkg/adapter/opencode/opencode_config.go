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
	if len(plugins) > 0 {
		doc["plugin"] = uniqueStrings(jsonPluginSlice(doc["plugin"]), plugins)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%s 직렬화 실패: %w", configFile, err)
	}
	return string(data) + "\n", nil
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
