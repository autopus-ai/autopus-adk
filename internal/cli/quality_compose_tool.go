package cli

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeToolText is the change-line wording of a tool tab in one locale.
// Only the terminal sees it; nothing written depends on it.
type composeToolText struct {
	provider   string // provider, before, after
	none       string
	givesUp    string // preset name
	oldReaders string
}

var composeToolTexts = map[string]composeToolText{
	"ko": {
		provider:   "quality.providers.%s %s → %s",
		none:       "없음",
		givesUp:    "%s은 balanced 표준 배치를 받지 않음",
		oldReaders: "quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다",
	},
	"en": {
		provider:   "quality.providers.%s %s → %s",
		none:       "none",
		givesUp:    "%s does not get the balanced standard placement",
		oldReaders: "auto older than v0.50.88 does not read quality.providers",
	},
}

func composeToolTextFor(lang string) composeToolText {
	if lang == "ko" {
		return composeToolTexts["ko"]
	}
	return composeToolTexts["en"]
}

// composeToolPlanner turns a Claude Code or Codex tab's edits into
// Per-Tool Binding Contract writes (SPEC-MODELUX-002 REQ-MUX2-07). It is a
// composeDriverPlanner, so the form driver runs both kinds of tab alike.
type composeToolPlanner struct {
	run  *composeSessionRun
	tool qualityModelTool
}

func (p composeToolPlanner) Start() map[string]string { return p.run.toolStart(p.tool) }

// PlanEdit plans edited: nothing when it equals the start, otherwise
// <provider>-custom holding every canonical agent's tier and the tool's
// binding to it. quality.default and every other preset stay.
func (p composeToolPlanner) PlanEdit(edited map[string]string) (composeDriverPlan, error) {
	if maps.Equal(edited, p.Start()) {
		return composeDriverPlan{NoChange: true}, nil
	}
	source, err := p.run.source()
	if err != nil {
		return composeDriverPlan{}, err
	}
	name := qualityToolPresetName(p.tool)
	agents := maps.Clone(edited)
	candidate := p.run.toolCandidate(p.tool)(agents)
	return composeDriverPlan{
		Candidate:   candidate,
		Composition: maps.Clone(agents),
		Changes:     p.changes(name, candidate, source),
		Apply:       func() error { return p.persist(name, agents) },
	}, nil
}

// PlanRefresh has no meaning on a tool tab: `u` refreshes the built-in
// presets every tool shares.
func (p composeToolPlanner) PlanRefresh() (composeDriverPlan, error) {
	return composeDriverPlan{}, errors.New("a tool tab does not refresh the built-in presets")
}

// changes lists the preview lines of the write: the preset write, the
// binding when it moves, the Codex default session when it moves, what a
// balanced tool gives up, the block rewrite of a file write, and the
// version that started reading quality.providers.
func (p composeToolPlanner) changes(name string, candidate *config.HarnessConfig, source []byte) []string {
	quality := p.run.cfg.Quality
	bind := composeBindTextFor(p.run.lang)
	text := composeToolTextFor(p.run.lang)
	kind := composePresetNew
	if _, exists := quality.Presets[name]; exists {
		kind = composePresetOverwrite
		if quality.EffectiveMode(p.tool.provider) == name {
			kind = composePresetRewrite
		}
	}
	lines := []string{fmt.Sprintf(bind.presetKinds[kind], name, len(config.CanonicalAgentNames()))}
	if before := strings.TrimSpace(quality.Providers[p.tool.provider]); before != name {
		if before == "" {
			before = text.none
		}
		lines = append(lines, fmt.Sprintf(text.provider, p.tool.provider, terminalSafe(before), name))
	}
	if from, to, moved := composeSupervisorMove(p.run.view(), candidate, p.run.snapshot); moved {
		lines = append(lines, fmt.Sprintf(bind.supervisor, from, to))
	}
	if quality.EffectiveMode(p.tool.provider) == "balanced" {
		lines = append(lines, fmt.Sprintf(text.givesUp, name))
	}
	if source != nil {
		if comments, ok := qualityBlockCommentLines(source); ok {
			lines = append(lines, fmt.Sprintf(bind.blockRewrite, comments))
		} else {
			lines = append(lines, bind.blockUnreadable)
		}
	}
	return append(lines, text.oldReaders)
}

// persist writes the confirmed tiers: through saveQualityToolPreset into
// autopus.yaml, which refuses and keeps the file and cfg as they were on any
// failure, or for init into its config, which init then saves whole.
func (p composeToolPlanner) persist(name string, agents map[string]string) error {
	if p.run.writer == composeSessionMemory {
		setQualityToolPreset(&p.run.cfg.Quality, p.tool, name, agents)
		return nil
	}
	return saveQualityToolPreset(p.run.dir, p.run.cfg, p.tool, name, agents)
}

// toolStart is the starting composition of tool's tab (SPEC-MODELUX-002
// Per-Tool Binding Contract): every canonical agent at the tier the tool's
// provider view projects, its override preset when it has one and the
// global binding otherwise, with the fallback tier its embedded source
// declares.
func (r *composeSessionRun) toolStart(tool qualityModelTool) map[string]string {
	view := r.cfg.Quality.ForProvider(tool.provider)
	agents := config.CanonicalAgentNames()
	start := make(map[string]string, len(agents))
	for _, agent := range agents {
		start[agent] = view.EffectiveAgentTier(agent, r.sources[agent].FallbackTier)
	}
	return start
}

// toolCandidate returns the candidate function of tool's tab: a composition
// equal to the start leaves the loaded config as it is, and any other binds
// the tool to <provider>-custom holding it, in a detached copy built by the
// per-tool editor's withQualityToolPreset.
func (r *composeSessionRun) toolCandidate(tool qualityModelTool) func(map[string]string) *config.HarnessConfig {
	start := r.toolStart(tool)
	return func(composition map[string]string) *config.HarnessConfig {
		if maps.Equal(composition, start) {
			return r.view()
		}
		return withQualityToolPreset(r.view(), tool, qualityToolPresetName(tool), composition)
	}
}
