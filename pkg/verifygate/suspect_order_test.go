package verifygate

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The evidence is ordered by idle time, longest first, whatever the order
// the tests were seen in; tests under the limit are not suspects.
func TestSuspects_OrderedByIdleLongestFirst(t *testing.T) {
	p := testProgress{Running: []runningTest{
		{Name: "m.TestBusy", Idle: 2 * time.Second},
		{Name: "m.TestSlow", Idle: 9 * time.Second},
		{Name: "m.TestDead", Idle: 13 * time.Second},
		{Name: "m.TestWarm", Idle: 7 * time.Second},
	}}

	got := p.suspects(time.Now(), 7*time.Second)

	require.Len(t, got, 3)
	assert.True(t, strings.HasPrefix(got[0].Label, "m.TestDead "), got[0].Label)
	assert.True(t, strings.HasPrefix(got[1].Label, "m.TestSlow "), got[1].Label)
	assert.True(t, strings.HasPrefix(got[2].Label, "m.TestWarm "), got[2].Label)
}
