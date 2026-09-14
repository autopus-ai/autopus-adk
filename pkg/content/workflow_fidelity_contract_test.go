package content

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

// assertFidelityContract verifies the JS code matches the faithful-dispatch specification.
// Returns an error if any invariant is violated.
func assertFidelityContract(js string) error {
	// 1. PLAN_SCHEMA capture
	if !strings.Contains(js, "const PLAN_SCHEMA = {") {
		return fmt.Errorf("missing PLAN_SCHEMA constant")
	}
	planningBlock := phaseJSBlock(js, "planning")
	if !strings.Contains(planningBlock, "schema: PLAN_SCHEMA") {
		return fmt.Errorf("planning phase does not capture with schema: PLAN_SCHEMA")
	}

	// 1b. Bare generic skeleton check (negative). The thin "Execute <role> agent"
	// prompt is forbidden (REQ-005), regardless of quoting. Runs early so the
	// negative-control fixtures genuinely exercise it rather than tripping a later
	// check first.
	for _, skel := range []string{
		"Execute planner agent", "Execute executor agent", "Execute tester agent",
		"Execute annotator agent", "Execute reviewer agent", "Execute security auditor",
	} {
		if strings.Contains(js, skel) {
			return fmt.Errorf("contains generic skeleton agent prompt %q", skel)
		}
	}

	// 2. Specialized agentType per phase
	agentPhases := map[string]string{
		"planning":       "planner",
		"test_scaffold":  "tester",
		"implementation": "executor",
		"testing":        "tester",
	}
	for phase, agentType := range agentPhases {
		block := phaseJSBlock(js, phase)
		expectedOpt := fmt.Sprintf("agentType: '%s'", agentType)
		if !strings.Contains(block, expectedOpt) {
			return fmt.Errorf("phase %s missing expected agentType option '%s'", phase, agentType)
		}
	}

	// 3. Review phase dual-role (reviewer + security-auditor)
	reviewBlock := phaseJSBlock(js, "review")
	if !strings.Contains(reviewBlock, "agentType: 'reviewer'") {
		return fmt.Errorf("review phase missing reviewer role")
	}
	if !strings.Contains(reviewBlock, "agentType: 'security-auditor'") {
		return fmt.Errorf("review phase missing security-auditor role")
	}

	// 4. Executor fan-out uses parallel shared-tree calls with disjoint ownership.
	implBlock := phaseJSBlock(js, "implementation")
	if !strings.Contains(implBlock, "parallel(") {
		return fmt.Errorf("implementation phase missing parallel execution")
	}
	if strings.Contains(implBlock, "isolation:") || !strings.Contains(implBlock, "shared working tree") {
		return fmt.Errorf("executor must use disjoint ownership in the shared working tree")
	}
	if !strings.Contains(implBlock, "Math.min(") || !strings.Contains(implBlock, "plan.tasks") {
		return fmt.Errorf("executor fan-out count not dynamically bounded over plan.tasks via Math.min")
	}

	// 4b. parallel() must take an array of deferred thunks, not spread
	// already-invoked promises. The real Workflow runtime contract is
	// parallel(thunks: Array<() => Promise>); parallel(...promises) would crash.
	if strings.Contains(js, "parallel(...") {
		return fmt.Errorf("parallel must take an array of thunks, not spread promises ('parallel(...')")
	}
	if !strings.Contains(implBlock, "push(() => agent(") {
		return fmt.Errorf("executor fan-out must push deferred thunks '() => agent(' for parallel()")
	}

	// 4c. Degenerate floor (FIDELITY-001 F2): an empty/failed plan.tasks must fall
	// back to at least one executor instead of silently no-op'ing the phase.
	if !strings.Contains(implBlock, "length === 0") {
		return fmt.Errorf("implementation phase missing degenerate floor for empty plan.tasks")
	}

	return nil
}

func TestFidelityContract_RouteTeam(t *testing.T) {
	contentDir := repoContentDir(t)
	schema, err := workflow.LoadSchema(filepath.Join(contentDir, "workflows", "route_team.schema.json"))
	if err != nil {
		t.Fatalf("load team schema: %v", err)
	}
	js := deriveTeamWorkflowJS(schema)

	// Should pass the fidelity contract
	if err := assertFidelityContract(js); err != nil {
		t.Errorf("real generated JS failed fidelity contract: %v\nGenerated JS:\n%s", err, js)
	}
}

func TestFidelityContract_NegativeControl(t *testing.T) {
	fixtures := []struct {
		name string
		js   string
	}{
		{
			name: "missing_plan_schema",
			js:   "// phase('planning')\nawait phase('planning');\nawait agent('Plan SPEC', { agentType: 'planner' });",
		},
		{
			name: "missing_agent_type",
			js: `const PLAN_SCHEMA = {};
// phase('planning')
await phase('planning');
await agent('Plan SPEC', { schema: PLAN_SCHEMA });`,
		},
		{
			name: "missing_dual_role_review",
			js: `const PLAN_SCHEMA = {};
// phase('planning')
await phase('planning');
await agent('Plan', { agentType: 'planner', schema: PLAN_SCHEMA });
// phase('review')
await phase('review');
await agent('Review changes', { agentType: 'reviewer' });`,
		},
		{
			name: "missing_parallel_execution",
			js: `const PLAN_SCHEMA = {};
// phase('planning')
await phase('planning');
await agent('Plan', { agentType: 'planner', schema: PLAN_SCHEMA });
// phase('implementation')
await phase('implementation');
for(let i=0; i<5; i++) {
  await agent('Implement', { agentType: 'executor', isolation: 'worktree' });
}`,
		},
		{
			name: "missing_worktree_isolation",
			js: `const PLAN_SCHEMA = {};
// phase('planning')
await phase('planning');
await agent('Plan', { agentType: 'planner', schema: PLAN_SCHEMA });
// phase('implementation')
await phase('implementation');
await parallel(agent('Implement', { agentType: 'executor' }));`,
		},
		{
			name: "skeleton_prompt_violation",
			js: `const PLAN_SCHEMA = {};
// phase('planning')
await phase('planning');
await agent('Plan', { agentType: 'planner', schema: PLAN_SCHEMA });
// phase('implementation')
await phase('implementation');
await parallel(agent('Execute executor agent for spec spec', { agentType: 'executor', isolation: 'worktree' }));`,
		},
	}

	for _, tc := range fixtures {
		t.Run(tc.name, func(t *testing.T) {
			if err := assertFidelityContract(tc.js); err == nil {
				t.Errorf("fixture %s did not fail fidelity contract as expected", tc.name)
			}
		})
	}
}
