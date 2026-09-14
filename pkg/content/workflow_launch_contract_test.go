// Launch-contract oracle for SPEC-HARNESS-WORKFLOW-RUNTIME-001 (S1/S2/S3, S11,
// REQ-007): asserts the generated route_a/route_team workflow JS conforms to the
// real Claude Code Workflow runtime API (single export const meta + top-level
// body, no export default/env/agent.exec) and the segment-dispatch guards.
package content

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

func assertLaunchContractCommon(t *testing.T, js string, name string) {
	t.Helper()
	// Must begin with export const meta (ignoring comments/whitespace at start)
	trimmed := strings.TrimSpace(js)
	// Remove leading generated comments if any
	for strings.HasPrefix(trimmed, "//") {
		idx := strings.Index(trimmed, "\n")
		if idx < 0 {
			break
		}
		trimmed = strings.TrimSpace(trimmed[idx+1:])
	}
	if !strings.HasPrefix(trimmed, "export const meta") {
		t.Errorf("[%s] JS must start with 'export const meta', got: %s", name, js)
	}

	// Contains no second export
	if count := strings.Count(js, "export"); count != 1 {
		t.Errorf("[%s] expected exactly 1 'export' token, got %d", name, count)
	}

	// Contains no export default
	if strings.Contains(js, "export default") {
		t.Errorf("[%s] must not contain 'export default'", name)
	}

	// Contains no function run(
	if strings.Contains(js, "function run(") {
		t.Errorf("[%s] must not contain 'function run('", name)
	}

	// Contains no env(
	if strings.Contains(js, "env(") {
		t.Errorf("[%s] must not contain 'env('", name)
	}

	// Contains no agent.exec(
	if strings.Contains(js, "agent.exec(") {
		t.Errorf("[%s] must not contain 'agent.exec('", name)
	}
}

// assertSegmentGuards validates S11: the generated JS must contain the SEGMENT
// preamble and exactly one segment-A guard block (ending with gate_build_test)
// and one segment-B guard block. For route_team, the first phase in segment B
// must be annotation.
func assertSegmentGuards(t *testing.T, js, name string, firstSegBPhase string) {
	t.Helper()

	// args normalization: the runtime delivers args as a JSON string, so the JS
	// must JSON.parse it before reading .segment/.spec/.quality (RUNTIME-001 gap
	// surfaced by the FIDELITY-001 live e2e). Without this, segment B never runs.
	if !strings.Contains(js, "const ARGV = (typeof args === 'string')") || !strings.Contains(js, "JSON.parse(args)") {
		t.Errorf("[%s] missing ARGV string-args normalization preamble", name)
	}

	// SEGMENT preamble (reads the normalized ARGV, not the raw string args)
	if !strings.Contains(js, "const SEGMENT = (ARGV && ARGV.segment) || 'A'") {
		t.Errorf("[%s] missing SEGMENT preamble", name)
	}

	// Exactly one segment A guard and one segment B guard
	if count := strings.Count(js, "if (SEGMENT === 'A')"); count != 1 {
		t.Errorf("[%s] expected exactly 1 segment-A guard, got %d", name, count)
	}
	if count := strings.Count(js, "if (SEGMENT === 'B')"); count != 1 {
		t.Errorf("[%s] expected exactly 1 segment-B guard, got %d", name, count)
	}

	// Locate segment A and B blocks
	segAIdx := strings.Index(js, "if (SEGMENT === 'A')")
	segBIdx := strings.Index(js, "if (SEGMENT === 'B')")
	if segAIdx < 0 || segBIdx < 0 || segAIdx >= segBIdx {
		t.Errorf("[%s] segment guard blocks not in expected order", name)
		return
	}
	segABlock := js[segAIdx:segBIdx]
	segBBlock := js[segBIdx:]

	// Last phase('...') call in segment A must be gate_build_test
	lastPhaseA := lastPhaseInBlock(segABlock)
	if lastPhaseA != gateBuildTestID {
		t.Errorf("[%s] last phase in segment A must be %s, got %q", name, gateBuildTestID, lastPhaseA)
	}

	// First phase('...') in segment B must match the expected first phase
	if firstSegBPhase != "" {
		firstPhaseB := firstPhaseInBlock(segBBlock)
		if firstPhaseB != firstSegBPhase {
			t.Errorf("[%s] first phase in segment B must be %q, got %q", name, firstSegBPhase, firstPhaseB)
		}
	}
}

// lastPhaseInBlock returns the last phase ID referenced via phase('...') in the
// given JS block.
func lastPhaseInBlock(block string) string {
	last := ""
	rest := block
	for {
		idx := strings.Index(rest, "phase('")
		if idx < 0 {
			break
		}
		rest = rest[idx+len("phase('"):]
		end := strings.Index(rest, "'")
		if end < 0 {
			break
		}
		last = rest[:end]
		rest = rest[end:]
	}
	return last
}

// firstPhaseInBlock returns the first phase ID referenced via phase('...') in
// the given JS block, skipping the guard keyword itself.
func firstPhaseInBlock(block string) string {
	idx := strings.Index(block, "phase('")
	if idx < 0 {
		return ""
	}
	rest := block[idx+len("phase('"):]
	end := strings.Index(rest, "'")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestLaunchContract_RouteA(t *testing.T) {
	contentDir := repoContentDir(t)
	schema, err := workflow.LoadSchema(filepath.Join(contentDir, "workflows", "route_a.schema.json"))
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	js := deriveWorkflowJS(schema)
	assertLaunchContractCommon(t, js, "route_a")

	// S11: segment guards — route_a segment B starts with release_hygiene
	assertSegmentGuards(t, js, "route_a", releaseHygieneID)

	// S1/REQ-007: gate_build_test and release_hygiene must have phase markers and logs, but no agent.exec
	for _, pid := range []string{"gate_build_test", "release_hygiene"} {
		block := phaseJSBlock(js, pid)
		if block == "" {
			t.Errorf("Route A block %s must exist", pid)
		}
		if !strings.Contains(block, "log(") {
			t.Errorf("Route A block %s must contain log call, got:\n%s", pid, block)
		}
	}
	// S1: planning is single-argument phase
	if !strings.Contains(js, "phase('planning')") || strings.Contains(js, "phase('planning',") {
		t.Errorf("Route A planning phase must be single-argument, got: %s", js)
	}
}

func TestLaunchContract_RouteTeam(t *testing.T) {
	contentDir := repoContentDir(t)
	schema, err := workflow.LoadSchema(filepath.Join(contentDir, "workflows", "route_team.schema.json"))
	if err != nil {
		t.Fatalf("load team schema: %v", err)
	}
	js := deriveTeamWorkflowJS(schema)
	assertLaunchContractCommon(t, js, "route_team")

	// Interpose the deterministic checks between execution segments.
	assertTeamMultiSegment(t, js, "route_team")

	// S2: Specific assertions for Route Team
	if strings.Contains(js, "JSON.parse(env") {
		t.Errorf("Route Team must not contain 'JSON.parse(env'")
	}
	if strings.Contains(js, "AUTOPUS_WORKFLOW_QUALITY") {
		t.Errorf("Route Team must not contain 'AUTOPUS_WORKFLOW_QUALITY'")
	}

	// S3: agent calls must use template literal referring to ctx/args and carry opts with model
	// We can check each agent-driven phase
	agentPhases := []string{"planning", "test_scaffold", "implementation", "testing", "review"}
	for _, pid := range agentPhases {
		block := phaseJSBlock(js, pid)
		if pid == "implementation" {
			if !strings.Contains(block, "agent(taskPrompt") {
				t.Errorf("Route Team phase implementation must call agent with taskPrompt, got:\n%s", block)
			}
		} else {
			if !strings.Contains(block, "agent(`") {
				t.Errorf("Route Team phase %s must call agent with template literal, got:\n%s", pid, block)
			}
		}
		if strings.Contains(block, "agent('") || strings.Contains(block, "agent(\"") {
			t.Errorf("Route Team phase %s must not call agent with role-only string, got:\n%s", pid, block)
		}
		// check opts has model
		if !strings.Contains(block, "model:") {
			t.Errorf("Route Team phase %s agent call missing model option, got:\n%s", pid, block)
		}
	}

	// preamble binds ctx and RT to the normalized ARGV (not raw string args)
	if !strings.Contains(js, "const ctx = ARGV") || !strings.Contains(js, "ARGV.quality") {
		t.Errorf("Route Team missing normalized ARGV preamble (ctx/quality)")
	}
}

func TestLaunchContract_NegativeControl(t *testing.T) {
	// Violation fixtures must fail assertLaunchContractCommon
	fixtures := []struct {
		name string
		js   string
	}{
		{"no_meta", "const meta = {};"},
		{"second_export", "export const meta = {}; export const other = 1;"},
		{"export_default", "export const meta = {}; export default async function run() {}"},
		{"function_run", "export const meta = {}; function run() {}"},
		{"env_call", "export const meta = {}; env('VAR');"},
		{"agent_exec", "export const meta = {}; agent.exec(['ls']);"},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			fakeT := &testing.T{}
			assertLaunchContractCommon(fakeT, tc.js, tc.name)
			if !fakeT.Failed() {
				t.Errorf("fixture %s did not fail as expected", tc.name)
			}
		})
	}
}
