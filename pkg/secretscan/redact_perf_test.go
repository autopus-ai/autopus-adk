package secretscan

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linearRatio bounds the time Redact takes for twice the headers over the time
// for the headers. The linear scan takes 2.0 times as long, plain and under
// the race detector, and 1.6 to 2.6 times with 96 copies of this test sharing
// 24 cores; the quadratic footer search of review round 2 took 3.7 to 4.2
// times as long, from 16 KB up to 128 KB.
const linearRatio = 3.0

// linearCap bounds the larger input, times the race slowdown. It is far above
// the 0.08 s that input takes on an Apple M-series laptop, so only a
// pathological slowdown fails it.
const linearCap = 10 * time.Second

// TestRedact_PrivateKeyHeadersWithoutFooter_StayLinear pins review round 2
// finding 1: every private key header searched for its footer from itself to
// the end of the text, so headers that never meet a footer took quadratic
// time. A fixed bound on one size failed on a loaded CI runner (2.49 s for
// 128 KB in run 37871017866), so the test compares sizes instead: the fastest
// of two runs of 16 KB and of 32 KB, in the order 16, 32, 32, 16 so that a
// load drifting during the test reaches both sizes alike. It measures process
// CPU time, which waiting for a CPU does not add to: with 96 copies of the
// test sharing 24 cores, the wall time ratio ranged from 0.7 to 19. The
// outputs are pinned too, so a ratio met by redacting less (or everything)
// fails. It runs alone, before the parallel tests of the package, so their
// work does not count against the ratio.
func TestRedact_PrivateKeyHeadersWithoutFooter_StayLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test of 16 KB and 32 KB")
	}
	// The PEM armor is joined from fragments so no private-key header is
	// committed as one literal; the bytes are unchanged.
	const period = "-----BEGIN " + "PRIVATE" + " KEY-----" + "-----END "
	periods := (16 << 10) / len(period)
	inputs := [2]string{strings.Repeat(period, periods), strings.Repeat(period, 2*periods)}
	Redact(inputs[0]) // the first call pays for warming up, not for the scan

	var fastest [2]time.Duration
	var outputs [2]string
	for _, size := range []int{0, 1, 1, 0} {
		runtime.GC()
		start := processTime()
		out, changed := Redact(inputs[size])
		elapsed := processTime() - start
		assert.True(t, changed)
		outputs[size] = out
		if fastest[size] == 0 || elapsed < fastest[size] {
			fastest[size] = elapsed
		}
	}

	for size, out := range outputs {
		want := strings.Repeat(PlaceholderSecret+"-----END ", periods<<size)
		assert.Equal(t, want, out, "each header is redacted alone; no footer follows it")
	}
	require.Positive(t, fastest[0])
	ratio := float64(fastest[1]) / float64(fastest[0])
	t.Logf("Redact took %s for %d bytes of headers and %s for %d bytes (ratio %.2f)",
		fastest[0], len(inputs[0]), fastest[1], len(inputs[1]), ratio)
	assert.Less(t, ratio, linearRatio, "twice the headers take at most %.1f times as long", linearRatio)
	assert.Less(t, fastest[1], linearCap*slowdown, "Redact of %d bytes of headers", len(inputs[1]))
}
