package secretscan

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// linearBound is the time 128 KB of adversarial input may take, times
// slowdown under the race detector. The linear scan takes about a sixth of it
// (0.3 s on an Apple M-series laptop); the quadratic footer search of review
// round 2 took 6.6 s.
const linearBound = 2 * time.Second

// TestRedact_PrivateKeyHeadersWithoutFooter_StayLinear pins review round 2
// finding 1: every private key header searched for its footer from itself to
// the end of the text, so 128 KB of headers that never meet a footer took
// seconds. The output is pinned too, so a bound met by redacting less (or
// everything) fails. It runs alone, before the parallel tests of the package,
// so their load does not count against the bound.
func TestRedact_PrivateKeyHeadersWithoutFooter_StayLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test of 128 KB")
	}
	const period = "-----BEGIN PRIVATE KEY----------END "
	in := strings.Repeat(period, (128<<10)/len(period))

	start := time.Now()
	out, changed := Redact(in)
	elapsed := time.Since(start)

	want := strings.Repeat(PlaceholderSecret+"-----END ", (128<<10)/len(period))
	assert.True(t, changed)
	assert.Equal(t, want, out, "each header is redacted alone; no footer follows it")
	assert.Less(t, elapsed, linearBound*slowdown, "Redact of %d bytes of headers", len(in))
}
