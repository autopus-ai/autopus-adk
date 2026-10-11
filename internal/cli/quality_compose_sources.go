package cli

import (
	"fmt"
	"io/fs"

	contentfs "github.com/insajin/autopus-adk/content"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// composeAgentSource holds what one embedded agent source feeds the generators.
// FallbackTier is the frontmatter `model:` tier: Claude reads it as the
// current model of applyClaudeAgentProfile, and generate-templates writes it
// into the Codex agent template as the fallback tier. CodexEffort is the
// declared effort that template passes, MapEffort(effort, "codex").
type composeAgentSource struct {
	FallbackTier string
	CodexEffort  string
}

// loadComposeAgentSources reads the embedded agent sources, keyed by agent
// name, so the composition table hands each resolver exactly the arguments
// generation does instead of keeping a second copy of them.
func loadComposeAgentSources() (map[string]composeAgentSource, error) {
	return loadComposeAgentSourcesFrom(contentfs.FS)
}

// loadComposeAgentSourcesFrom derives the arguments the way
// pkgcontent.TransformAgentForCodex writes them into the Codex templates.
func loadComposeAgentSourcesFrom(fsys fs.FS) (map[string]composeAgentSource, error) {
	sources, err := pkgcontent.LoadAgentSourcesFromFS(fsys, "agents")
	if err != nil {
		return nil, fmt.Errorf("embedded agent sources: %w", err)
	}
	bySource := make(map[string]composeAgentSource, len(sources))
	for _, src := range sources {
		bySource[src.Meta.Name] = composeAgentSource{
			FallbackTier: src.Meta.Model,
			CodexEffort:  pkgcontent.MapEffort(src.Meta.Effort, "codex"),
		}
	}
	return bySource, nil
}
