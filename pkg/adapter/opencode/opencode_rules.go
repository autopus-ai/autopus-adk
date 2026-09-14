package opencode

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

const openCodeDeferredToolsRule = `# OpenCode Deferred Tool Compatibility

Use only tools exposed by the current OpenCode runtime. If a requested tool is
absent, stop that route with an unsupported-tool diagnostic. Persistent Claude
team lifecycle is unavailable here; use the ordinary OpenCode task pipeline.
`

func (a *Adapter) prepareRuleMappings() ([]adapter.FileMapping, error) {
	entries, err := contentfs.FS.ReadDir("rules")
	if err != nil {
		return nil, fmt.Errorf("rules 디렉터리 읽기 실패: %w", err)
	}

	files := make([]adapter.FileMapping, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, readErr := fs.ReadFile(contentfs.FS, pkgcontent.EmbeddedPath("rules", entry.Name()))
		if readErr != nil {
			return nil, fmt.Errorf("rule 파일 읽기 실패 %s: %w", entry.Name(), readErr)
		}
		content := pkgcontent.ReplacePlatformReferences(string(data), "opencode")
		if entry.Name() == "deferred-tools.md" {
			content = openCodeDeferredToolsRule
		}
		relPath := filepath.Join(".opencode", "rules", "autopus", entry.Name())
		files = append(files, adapter.FileMapping{
			TargetPath:      relPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(content),
			Content:         []byte(content),
		})
	}
	return files, nil
}

func managedRulePaths() (active, deferred []string, err error) {
	entries, err := contentfs.FS.ReadDir("rules")
	if err != nil {
		return nil, nil, err
	}
	active = make([]string, 0, len(entries))
	deferred = make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, readErr := contentfs.FS.ReadFile("rules/" + entry.Name())
		if readErr != nil {
			return nil, nil, readErr
		}
		rule, parseErr := rulecond.ParseRule(entry.Name(), raw)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		path := toSlash(filepath.Join(".opencode", "rules", "autopus", entry.Name()))
		if rulecond.Classify(rule) == rulecond.ClassSkillScoped {
			deferred = append(deferred, path)
		} else {
			active = append(active, path)
		}
	}
	return active, deferred, nil
}
