package cli

// SPEC-TUIUX-001 T9: a line that comes from a decision prints once per run,
// however often the init loop settles the decision again after back moves.
// The REA Node notice goes through the run's REA line sink, and the
// analysis-rung fallback lines through the run's fallback notice.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/adapter"
)

func TestInitInteractive_DecisionLinesPrintOncePerRun(t *testing.T) {
	newTUIUXEnv(t, true, "")
	original := reaNodeExec
	t.Cleanup(func() { reaNodeExec = original })
	node := &reaNodeExecRecorder{startErr: errors.New("node: not found")}
	reaNodeExec = node.exec
	tuiuxCatalogs(t, "CAT-B")

	// 뒤로 twice from the review: each time the wizard resumes, the stage
	// draws its table again, and the REA decision is settled again.
	run := runTUIUXIN(t, []tui.InitWizardResult{tuiuxW()}, "2\n", "2\n", "\n")

	require.Len(t, *run.calls, 3)
	require.Equal(t, 3, strings.Count(run.out, tuiuxReview+"\n"))
	notice := "  " + reaTextFor("ko").NodeAbsent + "\n"
	assert.Equal(t, 1, strings.Count(run.out, notice), run.out)
	assert.Len(t, node.commands, 1, "the Node probe runs once per run")
	// CAT-B has no gpt-6.1-sol, so each of the three tables would report the
	// qa-explorer fallback; the run's fallback notice prints it once.
	fallback := reaFallbackLine(adapter.ReaFallback{Catalog: "Codex", Agent: "qa-explorer", Requested: "gpt-6.1-sol/xhigh"}, "ko")
	assert.Equal(t, 1, strings.Count(run.out, fallback+"\n"), run.out)
	_, err := os.Stat(run.dir + "/autopus.yaml")
	assert.NoError(t, err, "the third review applied")
}
