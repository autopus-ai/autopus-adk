package opencode

import (
	"fmt"
	"path/filepath"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// prepareAgentMappings renders every agent source for OpenCode. An agent's skill
// list is narrowed to the skills this configuration installs on the OpenCode
// surface, so no agent points at guidance that was never written.
func (a *Adapter) prepareAgentMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	sources, err := pkgcontent.LoadAgentSourcesFromFS(contentfs.FS, "agents")
	if err != nil {
		return nil, fmt.Errorf("agent source 로드 실패: %w", err)
	}
	files := make([]adapter.FileMapping, 0, len(sources))
	for _, src := range sources {
		src.Meta.Skills = pkgcontent.FilterInstalledSkillNames(src.Meta.Skills, "opencode", cfg)
		content := pkgcontent.TransformAgentForOpenCode(src)
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(".opencode", "agents", src.Meta.Name+".md"),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(content),
			Content:         []byte(content),
		})
	}
	return files, nil
}
