package procreap_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/procreap"
)

const (
	me    = 501
	other = 502
)

// fixturePS mirrors `ps -A -ww -o pid=,ppid=,uid=,etime=,command=` on a busy
// machine: one agent session (100) owning several background shells.
const fixturePS = `    1     0     0 10-00:00:00 /sbin/launchd
  100     1   501 12:00:00 claude
  200   100   501 05:00:00 /bin/zsh -c source /home/u/.claude/shell-snapshots/snap.sh && eval 'until ! pgrep -f "heavy-lock.sh foo" >/dev/null; do sleep 10; done; echo finished'
  201   200   501    00:04 sleep 10
  300   100   501 08:30:00 /bin/zsh -c eval 'until grep -q "READY" /tmp/out.log; do sleep 10; done'
  301   300   501    00:02 sleep 10
  400   100   501 06:00:00 /bin/bash -c while ! test -f done.flag; do go test ./...; sleep 5; done
  401   400   501 01:00:00 go test ./...
  500   100   501    10:00 /bin/zsh -c until grep -q DONE log; do sleep 10; done
  501   500   501    00:01 sleep 10
  600     1   502 09:00:00 /bin/bash -c while pgrep -f "build.sh"; do sleep 15; done
  601   600   502    00:03 sleep 15
  700   100   501 07:00:00 /bin/zsh -c until ! pgrep -f "[h]eavy-lock.sh bar"; do sleep 10; done
  800   100   501 07:00:00 python -c while True: do sleep
  900   100   501 03:00:00 /bin/zsh -c while true; do sleep 30; done
  901   900   501 03:00:00 /usr/bin/env node agent.js
  902   901   501 03:00:00 sleep 30
`

func fixture(t *testing.T) []procreap.Process {
	t.Helper()
	procs, err := procreap.ParsePS(fixturePS)
	require.NoError(t, err)
	require.Len(t, procs, 17)
	return procs
}

func byPID(cands []procreap.Candidate) map[int]procreap.Candidate {
	out := map[int]procreap.Candidate{}
	for _, c := range cands {
		out[c.PID] = c
	}
	return out
}

func defaults() procreap.Options {
	return procreap.Options{UID: me, SelfPID: 999999, OlderThan: 2 * time.Hour}
}

func TestFind_SelectsOnlyIdlePollingShells(t *testing.T) {
	got := byPID(procreap.Find(fixture(t), defaults()))

	require.Contains(t, got, 200, "self-matching pgrep loop")
	assert.Equal(t, procreap.ReasonSelfMatchingPgrep, got[200].Reason)
	require.Len(t, got[200].Descendants, 1)
	assert.Equal(t, 201, got[200].Descendants[0].PID)

	require.Contains(t, got, 300, "grep wait with no deadline")
	assert.Equal(t, "no deadline, waiting 8h30m", got[300].Reason)

	assert.NotContains(t, got, 400, "a shell running go test is working, not waiting")
	assert.NotContains(t, got, 500, "a young loop is not stale")
	assert.NotContains(t, got, 600, "another user's process is never touched")
	assert.NotContains(t, got, 800, "a non-shell process is never touched")
	assert.NotContains(t, got, 900, "a non-sleep grandchild disqualifies the shell")

	require.Contains(t, got, 700)
	assert.True(t, strings.HasPrefix(got[700].Reason, "no deadline"),
		"the bracket trick does not match its own command line")
	assert.Len(t, got, 3)
}

func TestFind_OrdersOldestFirst(t *testing.T) {
	cands := procreap.Find(fixture(t), defaults())
	require.NotEmpty(t, cands)
	assert.Equal(t, 300, cands[0].PID)
}

func TestFind_ExcludesTheCallerAndItsAncestors(t *testing.T) {
	opts := defaults()
	opts.SelfPID = 201 // the caller runs as the loop's own child
	got := byPID(procreap.Find(fixture(t), opts))
	assert.NotContains(t, got, 200)
	assert.Contains(t, got, 300)
}

func TestFind_SessionPIDRestrictsToDescendants(t *testing.T) {
	opts := defaults()
	opts.SessionPID = 300 // a loop has no polling shell below it
	assert.Empty(t, procreap.Find(fixture(t), opts))

	opts.SessionPID = 100
	assert.Len(t, procreap.Find(fixture(t), opts), 3)
}

func TestFind_YoungThresholdAdmitsTheYoungLoop(t *testing.T) {
	opts := defaults()
	opts.OlderThan = 0
	assert.Contains(t, byPID(procreap.Find(fixture(t), opts)), 500)
}

func TestSelfMatchingPgrep(t *testing.T) {
	cases := map[string]bool{
		`while pgrep -f "name"; do sleep 15; done`:                   true,
		`until ! pgrep -fl 'heavy job' >/dev/null; do sleep 1; done`: true,
		`while pgrep -f "[n]ame"; do sleep 15; done`:                 false,
		`while pgrep name; do sleep 15; done`:                        false,
		`while kill -0 123; do sleep 1; done`:                        false,
	}
	for cmd, want := range cases {
		assert.Equal(t, want, procreap.SelfMatchingPgrep(cmd), cmd)
	}
}

func TestParseEtime(t *testing.T) {
	cases := map[string]time.Duration{
		"00:04":       4 * time.Second,
		"12:00:00":    12 * time.Hour,
		"2-03:04:05":  51*time.Hour + 4*time.Minute + 5*time.Second,
		"10-00:00:00": 240 * time.Hour,
	}
	for in, want := range cases {
		got, err := procreap.ParseEtime(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := procreap.ParseEtime("abc")
	assert.Error(t, err)
}

func TestShortCommand(t *testing.T) {
	got := procreap.ShortCommand(`/bin/zsh -c source x && eval 'until grep -q A f; do sleep 10; done'`, 20)
	assert.Equal(t, "until grep -q A f; …", got)
}
