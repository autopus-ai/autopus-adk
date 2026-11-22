package verifygate

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// busyDeadlockSuite is RG3: a t.Parallel test deadlocks inside a package
// whose other parallel tests keep finishing, so the package never goes quiet.
func busyDeadlockSuite() string {
	var b strings.Builder
	b.WriteString("package m\n\nimport (\n\t\"sync\"\n\t\"testing\"\n\t\"time\"\n)\n\n" +
		"func TestDead(t *testing.T) {\n\tt.Parallel()\n\tvar mu sync.Mutex\n\tmu.Lock()\n\tmu.Lock()\n}\n\n")
	for i := 0; i < 960; i++ {
		// Sleeps spread over ~10s keep pass events coming for most of the window.
		fmt.Fprintf(&b, "func TestQ%d(t *testing.T) { t.Parallel(); time.Sleep(%d * time.Millisecond) }\n", i, (i%240)*40)
	}
	return b.String()
}

// RG3: the test-level and package-level analysers are joined, and the
// evidence is ordered by idle time, so the deadlocked test is named first.
func TestRun_DeadlockInABusyPackageIsSuspectedAndNamedFirst(t *testing.T) {
	step := goPackageStep(t, map[string]string{"m_test.go": busyDeadlockSuite()})
	// Every test gets a slot at once: with -parallel 2 (a small CI runner)
	// TestDead's start depended on which paused test the scheduler resumed
	// first, and it often never started inside the window -- a correct
	// budget skip, but not the scenario under test. Compile first so the
	// window is spent running.
	step.Argv = []string{"go", "test", "-json", "-parallel=2048", "."}
	warm := exec.Command("go", "test", "-count=1", "-run", "^$", ".")
	warm.Dir = step.Dir
	out, err := warm.CombinedOutput()
	require.NoError(t, err, string(out))

	failure, skipped := Run(context.Background(), []Step{step}, 14*time.Second)

	require.NotNil(t, failure, "skipped: %v", skipped)
	assert.True(t, failure.Hung)
	assert.True(t, strings.HasPrefix(failure.Reason, "m.TestDead "), failure.Reason)
}

// F3: a package-level fail that go prints after the step was killed is the
// kill, not a failure, when no test had failed.
func TestRun_PackageFailAfterTheDeadlineIsNotAFailure(t *testing.T) {
	dir := t.TempDir()
	late := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	events := eventsAt(`{"Action":"pass","Package":"x","Test":"TestOK"}`) + "\n" +
		`{"Time":"` + late + `","Action":"fail","Package":"x"}`

	failure, skipped := Run(context.Background(), []Step{sleepyStep(dir, events, true)}, 2*time.Second)

	require.Nil(t, failure, "the package fail came after the deadline")
	require.Len(t, skipped, 1)
	assert.True(t, skipped[0].Budget)
}

// A package whose tests all finished but that never ended (teardown after
// m.Run) is not described as waiting before its tests.
func TestDescribeStalled_TeardownIsNotCalledTestMain(t *testing.T) {
	t.Parallel()
	old := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	out := `{"Time":"` + old + `","Action":"start","Package":"m"}` + "\n" +
		`{"Time":"` + old + `","Action":"pass","Package":"m","Test":"TestA"}`
	now := time.Now()
	prog := goTestProgress([]byte(out), now)

	stalled := prog.stalled(now, time.Second)

	require.Len(t, stalled, 1)
	text := describeStalled(stalled, now)
	assert.NotContains(t, text, "TestMain or package init is waiting")
	assert.Contains(t, text, "after its tests")
}
