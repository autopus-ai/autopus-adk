package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/debt"
)

// A teammate copies a block out of add.go into other.go (merged without a
// baseline update). The session then edits add.go far from that block.
func TestDebtGateRegression2_TeammateCopyBlocksSessionTouchingOriginal(t *testing.T) {
	isolateStop(t)
	dir := newStopRepo(t, "")
	write := func(name, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	var blk strings.Builder
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&blk, "\tr%d := normalizeIncomingPayload(input%d, cfg.Threshold, cfg.Limit, cfg.Mode)\n", i, i)
	}
	add := "package stopgate\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Normalize() {\n" + blk.String() + "}\n"
	write("add.go", add)
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "add")
	items, err := debt.Scan(context.Background(), dir, 0)
	require.NoError(t, err)
	_, _, err = debt.WriteBaseline(dir, items, false)
	require.NoError(t, err)
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "baseline")
	write("other.go", "package stopgate\n\nfunc Other() {\n"+blk.String()+"}\n")
	rvGitC(t, dir, "add", "-A")
	rvGitC(t, dir, "commit", "-qm", "teammate copy merged without a baseline update")
	// The session's change: one line at the top of add.go.
	write("add.go", strings.Replace(add, "func Add(a, b int) int { return a + b }", "func Add(a, b int) int { return b + a }", 1))
	session := "rv2-teammate-copy"
	t.Cleanup(func() {
		resetStopState(session)
		_ = os.Remove(stopStateFile(session, "debt"))
		_ = os.Remove(stopStateFile(session, "debt-blocks"))
	})
	block, warning := debtGate(context.Background(), stopHookInput{SessionID: session}, dir, []string{"add.go"})
	if block != nil {
		t.Errorf("blocked for a teammate's copy because add.go was touched elsewhere: %s", strings.SplitN(block.Reason, "\n", 2)[0])
	} else {
		t.Logf("no block; warning=%q", warning)
	}
}
