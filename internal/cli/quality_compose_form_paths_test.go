package cli

// SPEC-MODELUX-002 REQ-MUX2-10, REQ-MUX2-14: Ctrl+C in any form of the
// flow ends the step with nothing further written, and the OMP tab's back,
// capability filter, single-level, and unpin paths keep the OMP editor's
// rules.

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Every form position of the flow: the nth form returns huh.ErrUserAborted
// after the answers that reach it.
func TestComposeForms_InterruptAtEveryFormEndsTheStep(t *testing.T) {
	cases := []struct {
		name    string
		fixture func(*testing.T) string
		lines   []string
		nth     int
	}{
		{"method", composeD, []string{"2\n", "1\n"}, 3},
		{"role group", composeD, []string{"2\n", "1\n", "\n"}, 4},
		{"agent list", composeD, []string{"2\n", "2\n", "2\n"}, 4},
		{"agent value", composeD, []string{"2\n", "2\n", "2\n", "6\n"}, 5},
		{"default preset", composeD, []string{"3\n"}, 2},
		{"override question", writeComposeProjectP, composeFormS16, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composeSessionCatalog(t, "CAT-A")
			dir := tc.fixture(t)
			before := composeSessionHash(t, dir)
			deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
			abortComposeFormAt(t, tc.nth)
			cmd, out := newComposeTestCommand()
			cmd.SetIn(formInput(tc.lines...))

			_, err := newComposeFormSession(t, dir).runForms(cmd, composeFormOptions{Presets: true, OMP: &composeOMPTab{deps: deps}})

			assert.True(t, errors.Is(err, errComposeInterrupted), "%v", err)
			assert.Equal(t, 1, strings.Count(out.String(), composeFormTextKO.Interrupted+"\n"))
			assert.Equal(t, before, composeSessionHash(t, dir))
			assert.Zero(t, activations.calls)
		})
	}
}

func composeD(t *testing.T) string {
	t.Helper()
	return writeComposeFixture(t, "claude-code", "codex")
}
