package rulecond

import "testing"

// The sticky hook normally answers in well under a tenth of a second, but the
// first run after `auto update --self` pays macOS's assessment of a freshly
// replaced binary, and a loaded shared machine stretches that further. Claude
// Code drops a hook that overruns its timeout for that turn and never retries
// it, so the bound has to cover a cold start, not the warm path.
func TestStickyHook_TimeoutCoversAColdBinaryStart(t *testing.T) {
	if got := stickyHook().Timeout; got < 15 {
		t.Fatalf("sticky hook timeout = %ds, want at least 15s so a cold binary start is not dropped", got)
	}
}
