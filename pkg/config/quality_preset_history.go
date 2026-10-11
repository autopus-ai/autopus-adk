package config

import "maps"

// BuiltinQualityPresetClass is the GAP 1 class of one written built-in preset
// (SPEC-MODELUX-001 REQ-MUX-10). Classification is exact agents-map equality;
// descriptions never take part.
type BuiltinQualityPresetClass string

const (
	// BuiltinQualityPresetAbsent means the file does not write the preset.
	BuiltinQualityPresetAbsent BuiltinQualityPresetClass = "absent"
	// BuiltinQualityPresetCurrent means the agents map equals the current code default.
	BuiltinQualityPresetCurrent BuiltinQualityPresetClass = "current"
	// BuiltinQualityPresetHistorical means the agents map equals a registry row
	// of the same preset: a complete layout an older release shipped.
	BuiltinQualityPresetHistorical BuiltinQualityPresetClass = "historical"
	// BuiltinQualityPresetCustomized covers every other written map, including
	// partial maps, extra agents, and invalid tiers. Its values belong to the user.
	BuiltinQualityPresetCustomized BuiltinQualityPresetClass = "customized"
)

// BuiltinQualityPresetStatus reports how one written built-in preset relates to
// the shipped defaults.
type BuiltinQualityPresetStatus struct {
	Class BuiltinQualityPresetClass
	// Row names the matching registry row (HB1 ... HU2) of a historical preset.
	Row string
	// RuntimeStandard is true for a historical row that NativeBalancedAgentCandidate
	// keeps resolving to the standard balanced placement.
	RuntimeStandard bool
	// Differs counts canonical agents whose written tier, absence included,
	// differs from the current code default.
	Differs int
}

// historicalQualityPreset is one complete agents map that release tags shipped
// as a built-in default, restored with `git log -p -- pkg/config/defaults.go`
// and `git tag --contains`.
type historicalQualityPreset struct {
	id      string
	preset  string
	commit  string // commit that introduced the layout
	shipped string // release tags that carried it
	// runtimeStandard keeps the layout on the standard balanced placement at
	// runtime. Only HB2 and HB5 carry it; widening it would change the models
	// existing projects already run.
	runtimeStandard bool
	agents          map[string]string
}

// historicalQualityPresets is the Historical Preset Registry. 0cb6296b (an
// eleven-agent balanced with executor promoted to opus) is deliberately absent:
// aaeae52d replaced it the same day and no release tag carried it alone, so a
// file holding it is a user customization.
var historicalQualityPresets = []historicalQualityPreset{
	{
		id: "HB1", preset: "balanced", commit: "c95806e2", shipped: "v0.4.3-v0.40.14",
		agents: map[string]string{
			"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
			"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet",
			"devops": "sonnet", "validator": "haiku", "explorer": "haiku",
		},
	},
	{
		id: "HB2", preset: "balanced", commit: "c21f7316", shipped: "v0.40.15-v0.50.103", runtimeStandard: true,
		agents: map[string]string{
			"planner": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
			"executor": "sonnet", "tester": "sonnet", "reviewer": "sonnet", "debugger": "sonnet",
			"devops": "sonnet", "validator": "sonnet", "explorer": "sonnet",
		},
	},
	{
		id: "HB4", preset: "balanced", commit: "aaeae52d", shipped: "v0.50.105-v0.50.114",
		agents: map[string]string{
			"architect": "opus", "deep-worker": "opus", "executor": "opus",
			"planner": "opus", "security-auditor": "opus", "spec-writer": "opus",
			"annotator": "sonnet", "debugger": "sonnet", "devops": "sonnet", "explorer": "sonnet",
			"frontend-specialist": "sonnet", "perf-engineer": "sonnet", "reviewer": "sonnet",
			"tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
		},
	},
	{
		id: "HB5", preset: "balanced", commit: "001830fd", shipped: "v0.50.115", runtimeStandard: true,
		agents: map[string]string{
			"architect": "fable", "planner": "fable", "security-auditor": "fable",
			"debugger": "opus", "deep-worker": "opus", "executor": "opus",
			"reviewer": "opus", "spec-writer": "opus",
			"annotator": "sonnet", "devops": "sonnet", "explorer": "sonnet",
			"frontend-specialist": "sonnet", "perf-engineer": "sonnet",
			"tester": "sonnet", "ux-validator": "sonnet", "validator": "sonnet",
		},
	},
	{
		id: "HU1", preset: "ultra", commit: "c95806e2", shipped: "v0.4.3-v0.50.103",
		agents: map[string]string{
			"planner": "opus", "executor": "opus", "validator": "opus", "tester": "opus",
			"reviewer": "opus", "architect": "opus", "spec-writer": "opus", "security-auditor": "opus",
			"debugger": "opus", "explorer": "opus", "devops": "opus",
		},
	},
	{
		id: "HU2", preset: "ultra", commit: "aaeae52d", shipped: "v0.50.105-v0.50.114",
		agents: map[string]string{
			"annotator": "opus", "architect": "opus", "debugger": "opus", "deep-worker": "opus",
			"devops": "opus", "executor": "opus", "explorer": "opus", "frontend-specialist": "opus",
			"perf-engineer": "opus", "planner": "opus", "reviewer": "opus", "security-auditor": "opus",
			"spec-writer": "opus", "tester": "opus", "ux-validator": "opus", "validator": "opus",
		},
	},
}

// ClassifyBuiltinQualityPreset classifies the written `balanced` or `ultra`
// preset of quality. The second result is false for any other name; preset
// names are matched exactly, like the YAML keys they come from.
func ClassifyBuiltinQualityPreset(quality QualityConf, name string) (BuiltinQualityPresetStatus, bool) {
	current, ok := currentBuiltinQualityAgents(name)
	if !ok {
		return BuiltinQualityPresetStatus{}, false
	}
	preset, written := quality.Presets[name]
	status := BuiltinQualityPresetStatus{Differs: differingCanonicalAgents(preset.Agents, current)}
	switch {
	case !written:
		status.Class = BuiltinQualityPresetAbsent
	case maps.Equal(preset.Agents, current):
		status.Class = BuiltinQualityPresetCurrent
	default:
		status.Class = BuiltinQualityPresetCustomized
		for _, row := range historicalQualityPresets {
			if row.preset == name && maps.Equal(preset.Agents, row.agents) {
				status.Class = BuiltinQualityPresetHistorical
				status.Row, status.RuntimeStandard = row.id, row.runtimeStandard
				break
			}
		}
	}
	return status, true
}

// currentBuiltinQualityAgents returns the agents map a fresh config writes for
// a built-in preset, so the current default has exactly one source.
func currentBuiltinQualityAgents(name string) (map[string]string, bool) {
	if name != "balanced" && name != "ultra" {
		return nil, false
	}
	return DefaultFullConfig("").Quality.Presets[name].Agents, true
}

func differingCanonicalAgents(written, current map[string]string) int {
	differs := 0
	for _, agent := range canonicalAgentNames {
		got, inWritten := written[agent]
		want, inCurrent := current[agent]
		if inWritten != inCurrent || got != want {
			differs++
		}
	}
	return differs
}
