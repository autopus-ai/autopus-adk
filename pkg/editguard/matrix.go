package editguard

// EnforcementState is a platform's state in the edit-guard enforcement matrix
// (REQ-EG-16).
type EnforcementState string

const (
	// Enforced: the platform's native hook blocks a denied edit, verified by
	// a probe against the real host.
	Enforced EnforcementState = "enforced"
	// AdvisoryOnly: Autopus hooks run on the platform but cannot block.
	AdvisoryOnly EnforcementState = "advisory-only"
	// NotEnforced: no guard runs on the platform.
	NotEnforced EnforcementState = "none"
)

// Lane is one platform row of the enforcement matrix. Hook generation
// registers the guard exactly on the enforced lanes, and the docs and doctor
// tasks publish the same rows.
type Lane struct {
	// Platform is the id `auto guard edit --platform` and the adapters use.
	Platform string
	// Name is the platform's display name.
	Name string
	// State is the lane's matrix state.
	State EnforcementState
	// Evidence names the probe or code fact that decided State, with the
	// caveats a user of the lane needs.
	Evidence string
}

// lanes is the matrix in publication order. Probe evidence lives in the
// SPEC-EDITGUARD-001 evidence directory.
var lanes = [...]Lane{
	{PlatformClaudeCode, "Claude Code", Enforced,
		"A1 PASS on Claude Code 2.1.289 (evidence/t0-probes.txt)"},
	{PlatformOpenCode, "OpenCode", Enforced,
		"A2 PASS on the V2 plugin API, OpenCode 2.0.10, and the generated V2 plugin on the same host " +
			"(evidence/t0-probes.txt, evidence/t11-probes.txt); the V1 plugin API lane is generated but " +
			"host-unverified (A2 V1 not-run, CD-1 open)"},
	{PlatformCodex, "Codex", Enforced,
		"A3 PASS on Codex CLI 0.160.0 and the T11 apply_patch matcher probe (evidence/t11-probes.txt); " +
			"Codex runs project hooks only after the user trusts them, and offers apply_patch only to models " +
			"with bundled metadata"},
	{PlatformGemini, "Gemini CLI", Enforced,
		"T11 BeforeTool probe PASS on Gemini CLI 0.52.0 for write_file and replace (evidence/t11-probes.txt); " +
			"project hooks run only in a trusted folder"},
	{"antigravity-cli", "Antigravity", AdvisoryOnly,
		"PreToolUse hooks run through the always-allow wrapper in pkg/content/hooks_antigravity.go, so " +
			".agents/hooks.json gets no guard"},
	{"omp", "OMP", NotEnforced,
		"the OMP adapter's SupportsHooks() is false"},
}

// laneAliases maps the other adapter spellings of a platform to its lane.
var laneAliases = map[string]string{
	"claude":     PlatformClaudeCode,
	"gemini-cli": PlatformGemini,
}

// Lanes returns a copy of the enforcement matrix in publication order.
func Lanes() []Lane {
	return append([]Lane(nil), lanes[:]...)
}

// LaneFor returns the lane of a platform id or one of its adapter spellings.
func LaneFor(platform string) (Lane, bool) {
	if canonical, ok := laneAliases[platform]; ok {
		platform = canonical
	}
	for _, lane := range lanes {
		if lane.Platform == platform {
			return lane, true
		}
	}
	return Lane{}, false
}
