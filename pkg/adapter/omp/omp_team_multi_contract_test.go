package omp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The team and provider axes must stay orthogonal and native on every OMP
// surface: `--team` picks an execution topology, `--multi` picks review
// breadth, and neither may imply the other or a second DAG owner. The exact
// sentences that carried this moved between the entrypoint and its resources,
// so this pins the vocabulary each surface must still expose, not its prose.
func TestGeneratedOMPTeamMultiContract(t *testing.T) {
	surfaces := generatedOMPTeamMultiSurfaces(t)
	required := []string{"--team", "--multi", "--solo"}

	for _, surface := range surfaces {
		t.Run(surface.name, func(t *testing.T) {
			for _, contract := range required {
				assert.Contains(t, surface.body, contract)
			}
			assertOMPTeamConflictIsStated(t, surface.body)
		})
	}
}

// `--team` and `--solo` are mutually exclusive topologies. A surface that never
// says so lets a run silently pick one and report the other.
func assertOMPTeamConflictIsStated(t *testing.T, body string) {
	t.Helper()
	lowered := strings.ToLower(body)
	assert.True(t,
		strings.Contains(lowered, "conflict") || strings.Contains(lowered, "mutually exclusive"),
		"the surface must state that the topology flags conflict")
}

func TestGeneratedOMPTeamMultiContractRejectsForeignCoordination(t *testing.T) {
	// The defect this guards is an OMP surface telling the runtime to call a
	// tool it does not have, or to open a skill file OMP never installs. Naming
	// another product in factual prose is not that defect: the generated bodies
	// legitimately describe what a flag means on Codex, and stripping the name
	// would make the sentence wrong rather than native.
	forbidden := []string{
		".omp/skills/agent-teams",
		"spawn_agent",
		"send_message",
		"followup_task",
		"wait_agent",
		"interrupt_agent",
		"list_agents",
		"get_goal",
		"create_goal",
		"update_goal",
	}

	for _, surface := range generatedOMPTeamMultiSurfaces(t) {
		t.Run(surface.name, func(t *testing.T) {
			for _, token := range forbidden {
				assert.NotContains(t, surface.body, token)
			}
		})
	}
}

type ompTeamMultiSurface struct {
	name string
	body string
}

func generatedOMPTeamMultiSurfaces(t *testing.T) []ompTeamMultiSurface {
	t.Helper()
	root := generateOMPOnly(t)
	names := []string{"auto-go", "agent-pipeline"}
	surfaces := make([]ompTeamMultiSurface, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, ".omp", "skills", name, "SKILL.md"))
		require.NoError(t, err)
		_, body := splitEmittedFrontmatter(t, string(data))
		require.NotEmpty(t, strings.TrimSpace(body))
		surfaces = append(surfaces, ompTeamMultiSurface{name: name, body: body})
	}
	return surfaces
}
