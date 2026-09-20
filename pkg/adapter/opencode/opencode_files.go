package opencode

import (
	"context"
	"fmt"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func (a *Adapter) prepareFiles(_ context.Context, cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	if cfg == nil {
		return nil, fmt.Errorf("하네스 설정이 필요합니다")
	}

	if err := a.validateRuntime(); err != nil {
		return nil, err
	}

	var files []adapter.FileMapping
	appendFiles := func(items []adapter.FileMapping, err error) error {
		if err != nil {
			return err
		}
		files = append(files, items...)
		return nil
	}

	agentsMapping, err := a.prepareAgentsMapping(cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, agentsMapping)

	if err := appendFiles(a.prepareRuleMappings()); err != nil {
		return nil, err
	}
	if err := appendFiles(a.prepareSkillMappings(cfg)); err != nil {
		return nil, err
	}
	if err := appendFiles(a.prepareAgentMappings()); err != nil {
		return nil, err
	}
	if err := appendFiles(a.prepareCommandMappings(cfg)); err != nil {
		return nil, err
	}
	if err := appendFiles(a.preparePluginMappings(cfg)); err != nil {
		return nil, err
	}
	if err := appendFiles(a.prepareGitHookMappings(cfg)); err != nil {
		return nil, err
	}

	configMapping, err := a.prepareConfigMapping()
	if err != nil {
		return nil, err
	}
	files = append(files, configMapping)

	return a.adaptRuntimeMappings(sanitizeUnsupportedClaudeTeamMappings(files)), nil
}
