package rulecond_test

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// TestStickyFire_ConcurrentFirstPromptsOfOneCounter_AllInject covers prompts
// that share one counter and start together on a project with no sticky state
// yet: every payload without a session_id reaches the digest of the empty
// string, so concurrent sessions of such a host race to create the same
// counter. On darwin an os.Root (openat) O_CREAT open that races another
// creator of the same name can fail with ENOENT although the directory handle
// is live (SPEC-EDITGUARD-001 T15 measured the same open in the edit guard's
// store lock). Before the bounded retry such a prompt took the silent
// unusable-state path and dropped its injection. At cadence 1 every prompt
// that is counted is due, so each one must inject.
func TestStickyFire_ConcurrentFirstPromptsOfOneCounter_AllInject(t *testing.T) {
	const rounds, prompts = 30, 8
	payload := `{"hook_event_name":"UserPromptSubmit","prompt":"hi"}`
	for round := range rounds {
		f := newStickyFixture(t)
		f.writeShippedStickyPair()
		outs := make([]string, prompts)
		errs := make([]string, prompts)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range prompts {
			wg.Go(func() {
				var out, errOut bytes.Buffer
				<-start
				_ = rulecond.StickyFire(f.root, stickyEvent, 1, strings.NewReader(payload), &out, &errOut)
				outs[i], errs[i] = out.String(), errOut.String()
			})
		}
		close(start)
		wg.Wait()
		for i := range prompts {
			if stickyContext(t, outs[i]) == "" || errs[i] != "" {
				t.Fatalf("round %d, prompt %d of a new counter: stdout %q, stderr %q; want an injection",
					round, i, outs[i], errs[i])
			}
		}
		if got := f.counter(""); got < 1 || got > prompts {
			t.Fatalf("round %d: counter %d, want 1 to %d", round, got, prompts)
		}
	}
}
