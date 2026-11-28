package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/debt"
)

func debtStopRepo(t *testing.T) string {
	t.Helper()
	isolateStop(t)
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return a + b }\n")
	items, err := debt.Scan(context.Background(), dir, 0)
	require.NoError(t, err)
	_, _, err = debt.WriteBaseline(dir, items, false)
	require.NoError(t, err)
	cmd := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "add", "-A")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	return dir
}

func gate(t *testing.T, session, dir string) (*stopHookOutput, string) {
	t.Helper()
	return debtGate(context.Background(), session, dir, []string{"add.go", "big.go"})
}

func TestDebtGate_BlocksOnceThenWarnsThenBlocksOnANewItem(t *testing.T) {
	dir := debtStopRepo(t)
	session := "debt-gate"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "debt")) })
	write := func(body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body), 0o644))
	}
	write("package stopgate\n\n// TODO handle overflow\nfunc Add(a, b int) int { return a + b }\n")
	block, _ := gate(t, session, dir)
	require.NotNil(t, block)
	assert.Contains(t, block.Reason, "add.go:3 marker TODO handle overflow")
	assert.Contains(t, block.Reason, "auto debt baseline --force")

	block, warning := gate(t, session, dir)
	assert.Nil(t, block, "the same items only warn")
	assert.Contains(t, warning, "add.go:3")

	write("package stopgate\n\n// TODO handle overflow\n// FIXME and underflow\nfunc Add(a, b int) int { return a + b }\n")
	block, _ = gate(t, session, dir)
	require.NotNil(t, block, "an item added after the block blocks again")
	assert.Contains(t, block.Reason, "add.go:4 marker FIXME and underflow")

	t.Setenv("AUTOPUS_DEBT_STOP", "off")
	block, warning = gate(t, "debt-off", dir)
	assert.Nil(t, block)
	assert.Empty(t, warning)
}

func TestDebtGate_NoBaselineSuggestsOnce(t *testing.T) {
	isolateStop(t)
	dir := newStopRepo(t, "package stopgate\n\n// TODO x\nfunc Add(a, b int) int { return a + b }\n")
	session := "debt-nobase"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "debt-nobase")) })
	block, warning := gate(t, session, dir)
	assert.Nil(t, block)
	assert.Contains(t, warning, "auto debt baseline")
	block, warning = gate(t, session, dir)
	assert.Nil(t, block)
	assert.Empty(t, warning, "suggested once a session")
}

func TestDebtGate_NearLimitOnlyWarns(t *testing.T) {
	dir := debtStopRepo(t)
	session := "debt-near"
	t.Cleanup(func() { resetStopState(session) })
	var b strings.Builder
	b.WriteString("package stopgate\n")
	for i := 0; i < 280; i++ {
		fmt.Fprintf(&b, "var v%d = %d\n", i, i)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.go"), []byte(b.String()), 0o644))
	block, warning := gate(t, session, dir)
	assert.Nil(t, block)
	assert.Contains(t, warning, "big.go:1 near-limit (281 of 300 lines)")
}
