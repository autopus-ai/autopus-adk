package cli

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/config"
)

// composeBindText is the preview wording of a binding plan in one locale.
// Only the terminal sees it; nothing written to autopus.yaml depends on it.
type composeBindText struct {
	presetKinds    map[composePresetWriteKind]string // name, agent count
	presetUpdate   string                            // name, detail
	effortsMoved   string                            // name, agent count (SPEC-MODELUX-003 REQ-MUX3-20)
	noModelChange  string
	added, removed string
	defaultChange  string // from, to
	noDefault      string
	supervisor     string // before, after
	givesUp        map[string]string
	blockRewrite   string // comment lines
	// blockUnreadable is the notice for a file whose lines cannot be located,
	// which every writer refuses; it replaces blockRewrite.
	blockUnreadable string
}

var composeBindTexts = map[string]composeBindText{
	"ko": {
		presetKinds: map[composePresetWriteKind]string{
			composePresetNew:       "quality.presets.%s 새로 씀 (%d개 에이전트)",
			composePresetRewrite:   "quality.presets.%s 고쳐 씀 (%d개 에이전트)",
			composePresetOverwrite: "quality.presets.%s 덮어씀 (%d개 에이전트)",
			composePresetRefresh:   "quality.presets.%s 최신 기본값으로 갱신 (%d개 에이전트)",
		},
		presetUpdate:  "quality.presets.%s 최신 기본값으로 갱신: %s",
		effortsMoved:  "quality.presets.%s.efforts 옮겨 씀 (%d개 에이전트)",
		noModelChange: "모델 변화 없음, YAML만 정리",
		added:         "추가",
		removed:       "제거",
		defaultChange: "quality.default %s → %s",
		noDefault:     "(없음)",
		supervisor:    "Codex 기본 세션: %s → %s",
		givesUp: map[string]string{
			"balanced": "custom은 balanced 표준 배치를 받지 않음",
			"ultra":    "custom은 ultra effort 사다리를 받지 않음",
		},
		blockRewrite: "quality 블록을 다시 씁니다: 블록 안 주석 %d줄과 서식은 유지되지 않습니다(값은 그대로)",
		blockUnreadable: "quality 블록의 줄 범위를 읽을 수 없어(단독 CR, U+0085, U+2028, U+2029 같은 줄바꿈 문자) " +
			"확인해도 쓰지 않습니다",
	},
	"en": {
		presetKinds: map[composePresetWriteKind]string{
			composePresetNew:       "quality.presets.%s written new (%d agents)",
			composePresetRewrite:   "quality.presets.%s rewritten (%d agents)",
			composePresetOverwrite: "quality.presets.%s overwritten (%d agents)",
			composePresetRefresh:   "quality.presets.%s refreshed to the current default (%d agents)",
		},
		presetUpdate:  "quality.presets.%s refreshed to the current default: %s",
		effortsMoved:  "quality.presets.%s.efforts carried (%d agents)",
		noModelChange: "no model change, YAML cleanup only",
		added:         "added",
		removed:       "removed",
		defaultChange: "quality.default %s → %s",
		noDefault:     "(none)",
		supervisor:    "Codex default session: %s → %s",
		givesUp: map[string]string{
			"balanced": "custom does not get the balanced standard placement",
			"ultra":    "custom does not get the ultra effort ladder",
		},
		blockRewrite: "the quality block is rewritten: its %d comment lines and layout are not kept (values are)",
		blockUnreadable: "the quality block's lines cannot be located (a line break such as a lone CR, U+0085, " +
			"U+2028, or U+2029), so confirming writes nothing",
	},
}

// composeBindTextFor returns the wording for lang as tui.DetectSystemLang
// reports it: Korean for "ko", English otherwise.
func composeBindTextFor(lang string) composeBindText {
	if lang == "ko" {
		return composeBindTexts["ko"]
	}
	return composeBindTexts["en"]
}

// describeComposeBinding returns the preview lines of plan (REQ-MUX-06): each
// autopus.yaml key the write changes, then each effect outside the table —
// the Codex default session when quality manages it and it moves, what a
// custom binding gives up (C6), and for a block write the comment lines of
// source, the current autopus.yaml bytes, that the rewrite drops. A source
// holding a line break the splice cannot see gets the refusal notice instead,
// whichever writer the plan takes. A nil source means no file is rewritten
// (init saves its in-memory config whole), so neither notice is shown.
// snapshot is the run's Codex catalog. A plan that writes nothing has no
// lines.
func describeComposeBinding(
	cfg *config.HarnessConfig,
	plan composeBindingPlan,
	snapshot driftGenerationSnapshot,
	source []byte,
	lang string,
) []string {
	if plan.Writer == composeWriterNone {
		return nil
	}
	text := composeBindTextFor(lang)
	var lines []string
	for _, write := range plan.Presets {
		if write.Kind != composePresetUpdate {
			lines = append(lines, fmt.Sprintf(text.presetKinds[write.Kind], write.Name, len(write.Preset.Agents)))
			if write.Carried > 0 {
				lines = append(lines, fmt.Sprintf(text.effortsMoved, write.Name, write.Carried))
			}
			continue
		}
		detail := text.noModelChange
		if write.ModelChange {
			detail = composeAgentChangeText(write.From, write.Preset.Agents, text)
		}
		lines = append(lines, fmt.Sprintf(text.presetUpdate, write.Name, detail))
	}
	if plan.Default != "" {
		from := terminalSafe(cfg.Quality.Default)
		if from == "" {
			from = text.noDefault
		}
		lines = append(lines, fmt.Sprintf(text.defaultChange, from, plan.Default))
	}
	if before, after, moved := composeSupervisorMove(cfg, plan.Candidate, snapshot); moved {
		lines = append(lines, fmt.Sprintf(text.supervisor, before, after))
	}
	if notice, ok := text.givesUp[plan.GivesUp]; ok {
		lines = append(lines, notice)
	}
	switch {
	case source != nil && hasYAMLOnlyLineBreak(source):
		// Every writer, the line write included, refuses such a file.
		lines = append(lines, text.blockUnreadable)
	case plan.Writer == composeWriterBlock && source != nil:
		if comments, ok := qualityBlockCommentLines(source); ok {
			lines = append(lines, fmt.Sprintf(text.blockRewrite, comments))
		} else {
			lines = append(lines, text.blockUnreadable)
		}
	}
	return lines
}

// composeSupervisorMove resolves the Codex default session before and after
// the write with the run's snapshot, as generation will. It reports a move
// only when Codex is installed, quality manages the session model, and the
// effective profile changes.
func composeSupervisorMove(
	before, after *config.HarnessConfig,
	snapshot driftGenerationSnapshot,
) (string, string, bool) {
	if after == nil || !containsPlatform(before.Platforms, "codex") {
		return "", "", false
	}
	from, managed := codex.ResolveSupervisorProfile(before.Quality, snapshot.codexCatalog)
	to, _ := codex.ResolveSupervisorProfile(after.Quality, snapshot.codexCatalog)
	if !managed || from.Effective == to.Effective {
		return "", "", false
	}
	return composeBindProfileText(from.Effective), composeBindProfileText(to.Effective), true
}

// composeBindProfileText writes a Codex profile the way the table does:
// without the gpt- prefix, `-` for a missing effort.
func composeBindProfileText(profile config.CodexProfile) string {
	effort := profile.Effort
	if effort == "" {
		effort = "-"
	}
	return strings.TrimPrefix(profile.Model, "gpt-") + " · " + effort
}

// composeAgentChangeText renders a preset's per-agent diff (REQ-MUX-11):
// changed agents grouped by transition (`architect, planner opus→fable`),
// each group in the order of its first agent, then added and removed agents.
func composeAgentChangeText(from, to map[string]string, text composeBindText) string {
	type transition struct{ from, to string }
	agents := make([]string, 0, len(from)+len(to))
	for agent := range from {
		agents = append(agents, agent)
	}
	for agent := range to {
		if _, seen := from[agent]; !seen {
			agents = append(agents, agent)
		}
	}
	slices.Sort(agents)

	groups := map[transition][]string{}
	var order []transition
	var added, removed []string
	for _, agent := range agents {
		old, had := from[agent]
		next, has := to[agent]
		switch {
		case !had:
			added = append(added, agent+"→"+next)
		case !has:
			removed = append(removed, agent)
		case old != next:
			key := transition{old, next}
			if _, seen := groups[key]; !seen {
				order = append(order, key)
			}
			groups[key] = append(groups[key], agent)
		}
	}
	parts := make([]string, 0, len(order)+2)
	for _, key := range order {
		parts = append(parts, strings.Join(groups[key], ", ")+" "+key.from+"→"+key.to)
	}
	if len(added) > 0 {
		parts = append(parts, text.added+" "+strings.Join(added, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, text.removed+" "+strings.Join(removed, ", "))
	}
	return strings.Join(parts, " · ")
}

// qualityBlockCommentLines counts the comment lines inside the top-level
// quality block of data, whole-line and trailing alike, by the boundary rule
// saveQualityBlock splices with — exactly the comments a block rewrite drops.
// Unparsable data or data without the block counts 0. ok is false when the
// block's lines cannot be located, a file the writer refuses (F2).
func qualityBlockCommentLines(data []byte) (count int, ok bool) {
	if hasYAMLOnlyLineBreak(data) {
		return 0, false
	}
	var doc yaml.Node
	if yaml.Unmarshal(data, &doc) != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return 0, true
	}
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "quality" {
			continue
		}
		lines := strings.SplitAfter(string(data), "\n")
		start, end, err := qualityBlockRange(lines, root, i)
		if err != nil {
			return 0, false
		}
		commented := map[int]bool{}
		for line := start; line < end; line++ {
			if strings.HasPrefix(strings.TrimSpace(lines[line]), "#") {
				commented[line] = true
			}
		}
		markQualityLineComments(root.Content[i], start, end, commented)
		markQualityLineComments(root.Content[i+1], start, end, commented)
		return len(commented), true
	}
	return 0, true
}

// markQualityLineComments records the lines in [start, end) that carry a
// trailing comment of node or a node below it. An alias is a leaf here, so
// the walk terminates.
func markQualityLineComments(node *yaml.Node, start, end int, commented map[int]bool) {
	if line := node.Line - 1; node.LineComment != "" && line >= start && line < end {
		commented[line] = true
	}
	for _, child := range node.Content {
		markQualityLineComments(child, start, end, commented)
	}
}
