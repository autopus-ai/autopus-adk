package orchestra

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Security L2: a provider stream keeps only the head up to its limit while
// the process runs, and every write still reports success so the provider
// is drained to its end instead of failing on a full pipe.
func TestFastFailBuffer_KeepsTheHeadWithinItsLimit(t *testing.T) {
	t.Parallel()
	buf := newFastFailBuffer(&fastFailDetector{}, nil, func(string) {})
	buf.limit = 10
	for range 5 {
		n, err := buf.Write([]byte("abcdef"))
		require.NoError(t, err)
		assert.Equal(t, 6, n)
	}
	assert.Equal(t, "abcdefabcd", buf.String())
	assert.Equal(t, 64<<20, fastFailBufferCap, "a stream without its own limit is still bounded")
}

// A real subprocess that prints 3 MB keeps exactly MaxOutputBytes of it.
func TestRunProvider_MaxOutputBytesBoundsTheCapture(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell")
	}
	provider := ProviderConfig{
		Name: "flood", Binary: "sh", PromptViaArgs: true, MaxOutputBytes: 4096,
		Args: []string{"-c", `head -c 3000000 /dev/zero | tr '\000' x`},
	}
	resp, err := runProvider(context.Background(), provider, "prompt")
	require.NoError(t, err)
	assert.Len(t, resp.Output, 4096)
	assert.Equal(t, 0, resp.ExitCode)
}
