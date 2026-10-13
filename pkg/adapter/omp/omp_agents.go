package omp

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// prepareOMPAgentMappings renders one .omp/agents definition per canonical
// agent for autopus mode. routed names the agents this generation's
// task.agentModelOverrides binds: their files declare no thinkingLevel, so
// the override's thinking is the only one OMP can see. Every other agent
// declares its source effort and inherits the parent session model.
func prepareOMPAgentMappings(routed map[string]bool) ([]adapter.FileMapping, error) {
	sources, err := pkgcontent.LoadAgentSourcesFromFS(contentfs.FS, "agents")
	if err != nil {
		return nil, fmt.Errorf("agent source 로드 실패: %w", err)
	}
	bySource := make(map[string]pkgcontent.AgentSource, len(sources))
	for _, src := range sources {
		bySource[src.Meta.Name] = src
	}
	files := make([]adapter.FileMapping, 0, len(sources))
	for _, agent := range config.CanonicalAgentNames() {
		src, ok := bySource[agent]
		if !ok {
			return nil, fmt.Errorf("agent source %s 없음", agent)
		}
		options := pkgcontent.OMPAgentDefinitionOptions{}
		if !routed[agent] {
			options.ThinkingLevel = src.Meta.Effort
		}
		rendered, err := pkgcontent.RenderOMPAgentDefinition(src, options)
		if err != nil {
			return nil, err
		}
		files = append(files, adapter.FileMapping{
			TargetPath:      OMPAgentDefinitionPath(agent),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        adapter.Checksum(rendered),
			Content:         []byte(rendered),
		})
	}
	return files, nil
}

// OMPAgentDefinitionPath is the project definition OMP discovers for agent.
func OMPAgentDefinitionPath(agent string) string {
	return filepath.Join(ompAgentDir, agent+".md")
}

// isOMPAgentDefinitionPath reports whether path is a generated agent
// definition, the one surface where an unrecorded file is the user's.
func isOMPAgentDefinitionPath(path string) bool {
	path = filepath.ToSlash(path)
	return strings.HasPrefix(path, ompAgentDir+"/") && strings.HasSuffix(path, ".md") &&
		!strings.Contains(strings.TrimPrefix(path, ompAgentDir+"/"), "/")
}

// ompRoutedAgentNames lists the agents the generation's overrides bind.
func ompRoutedAgentNames(integration *ompModelIntegration) map[string]bool {
	if integration == nil {
		return nil
	}
	routed := make(map[string]bool, len(integration.projection.Agents))
	for _, agent := range integration.projection.Agents {
		routed[agent.Agent] = true
	}
	return routed
}

// ompUserAgentFileAt reports whether an agent definition exists on disk
// without a manifest record. OMP resolves agents by name, so ADK must neither
// overwrite nor adopt such a file: it is skipped and stays the user's.
func ompUserAgentFileAt(
	workspace *ompRootedWorkspace,
	file adapter.FileMapping,
	manifest *adapter.Manifest,
) (bool, error) {
	if !isOMPAgentDefinitionPath(file.TargetPath) {
		return false, nil
	}
	if manifest != nil {
		if _, managed := manifest.Files[file.TargetPath]; managed {
			return false, nil
		}
	}
	if _, err := workspace.lstat(file.TargetPath); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect OMP agent definition %s: %w", file.TargetPath, err)
	}
	return true, nil
}

// expectedOMPAgentMappings is the agent surface Validate expects: every
// generated definition except one the user owns, which an unrecorded file on
// disk is. A recorded definition that went missing stays expected.
func (a *Adapter) expectedOMPAgentMappings(
	cfg *config.HarnessConfig,
	integration *ompModelIntegration,
) ([]adapter.FileMapping, error) {
	if cfg.RoleModelPolicy.OMPAgentSource() != config.OMPAgentSourceAutopus {
		return nil, nil
	}
	mappings, err := prepareOMPAgentMappings(ompRoutedAgentNames(integration))
	if err != nil {
		return nil, err
	}
	workspace, err := openOMPRootedWorkspace(a.root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = workspace.Close() }()
	manifest, err := loadOMPManifestAt(workspace, adapterName)
	if err != nil {
		return nil, err
	}
	expected := mappings[:0]
	for _, mapping := range mappings {
		userFile, userErr := ompUserAgentFileAt(workspace, mapping, manifest)
		if userErr != nil {
			return nil, userErr
		}
		if !userFile {
			expected = append(expected, mapping)
		}
	}
	return expected, nil
}
