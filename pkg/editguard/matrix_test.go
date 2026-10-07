package editguard

import "testing"

// REQ-EG-16 and S12: one state per platform, in matrix order, decided by the
// probes A1 to A3 and T11; Antigravity is advisory-only and OMP has no hooks.
func TestLanes_EnforcementMatrix(t *testing.T) {
	t.Parallel()
	// The state spellings are the ones REQ-EG-16 publishes.
	want := []struct{ platform, name, state string }{
		{"claude-code", "Claude Code", "enforced"},
		{"opencode", "OpenCode", "enforced"},
		{"codex", "Codex", "enforced"},
		{"gemini", "Gemini CLI", "enforced"},
		{"antigravity-cli", "Antigravity", "advisory-only"},
		{"omp", "OMP", "none"},
	}
	lanes := Lanes()
	if len(lanes) != len(want) {
		t.Fatalf("Lanes() has %d rows, want %d: %+v", len(lanes), len(want), lanes)
	}
	for i, row := range want {
		got := lanes[i]
		if got.Platform != row.platform || got.Name != row.name || string(got.State) != row.state || got.Evidence == "" {
			t.Errorf("row %d = %+v, want %s %q %s with evidence", i, got, row.platform, row.name, row.state)
		}
	}
	lanes[0].State = NotEnforced
	if Lanes()[0].State != Enforced {
		t.Error("Lanes() must return a copy the caller cannot use to rewrite the matrix")
	}
}

// An enforced lane needs a codec for its hook payload; a lane without
// enforcement has none, so `auto guard edit` cannot answer for it.
func TestLanes_EnforcedExactlyWhereADialectExists(t *testing.T) {
	t.Parallel()
	for _, lane := range Lanes() {
		_, ok := DialectFor(lane.Platform)
		if ok != (lane.State == Enforced) {
			t.Errorf("%s: state %s but DialectFor ok = %v", lane.Platform, lane.State, ok)
		}
	}
}

// The adapter spellings of a platform resolve to its lane; anything else has
// no lane.
func TestLaneFor_ResolvesAdapterSpellings(t *testing.T) {
	t.Parallel()
	for alias, platform := range map[string]string{
		"claude": "claude-code", "claude-code": "claude-code", "gemini": "gemini", "gemini-cli": "gemini",
		"opencode": "opencode", "codex": "codex", "antigravity-cli": "antigravity-cli", "omp": "omp",
	} {
		lane, ok := LaneFor(alias)
		if !ok || lane.Platform != platform {
			t.Errorf("LaneFor(%q) = %+v, %v; want platform %s", alias, lane, ok, platform)
		}
	}
	for _, unknown := range []string{"", "Claude-Code", "cursor"} {
		if lane, ok := LaneFor(unknown); ok {
			t.Errorf("LaneFor(%q) = %+v; want no lane", unknown, lane)
		}
	}
}
