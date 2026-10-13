package omp

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

const ompDeferredToolsRule = `# OMP Deferred Tool Compatibility

Use only tools exposed by the current oh-my-pi runtime. If a requested tool is
absent, stop that route with an unsupported-tool diagnostic. Persistent Claude
team lifecycle is unavailable here; use the ordinary OMP task pipeline.
`

const (
	// ompRuleDir is the only directory oh-my-pi scans for project rules. Rule
	// discovery is non-recursive (measured against omp 17.3.5: rules written to
	// a nested namespace directory reached zero sessions), so generated rules
	// live directly in the shared directory.
	ompRuleDir = ".omp/rules"
	// ompRuleFilePrefix namespaces ADK-owned rule files inside that shared
	// directory. The prefix, not a subdirectory, is what separates generated
	// rules from the user's own files in .omp/rules; ownership checks and
	// pruning key off it.
	ompRuleFilePrefix = "autopus-"
)

// prepareRuleMappings renders every embedded rule for OMP in cfg's agent
// source, the mode skills and workflows render dispatch text in, so native
// rules stay what v0.50.125 wrote.
func (a *Adapter) prepareRuleMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
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
		content, renderErr := renderOMPRule(entry.Name(), data, cfg)
		if renderErr != nil {
			return nil, renderErr
		}

		relPath := filepath.Join(filepath.FromSlash(ompRuleDir), ompRuleFilePrefix+entry.Name())
		files = append(files, adapter.FileMapping{
			TargetPath:      relPath,
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(content),
			Content:         []byte(content),
		})
	}
	return files, nil
}

// renderOMPRule renders one embedded rule for OMP in cfg's agent source.
func renderOMPRule(name string, data []byte, cfg *config.HarnessConfig) (string, error) {
	content := pkgcontent.ReplacePlatformReferencesWith(string(data), "omp", ompPlatformReferences(cfg))
	if name == "deferred-tools.md" {
		content = ompDeferredToolsRule
	}
	transformed, err := pkgcontent.TransformRuleForOMP(content)
	if err != nil {
		return "", fmt.Errorf("rule 변환 실패 %s: %w", name, err)
	}
	return transformed, nil
}
