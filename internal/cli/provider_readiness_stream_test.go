package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// countingReader serves size bytes of fill and counts how many were served.
type countingReader struct {
	remaining int
	fill      byte
	served    int
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	count := min(len(buffer), reader.remaining)
	for index := range count {
		buffer[index] = reader.fill
	}
	reader.remaining -= count
	reader.served += count
	return count, nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("read /dev/fd/3: input/output error")
}

func TestReadReadinessStream_BoundaryAtKeepBytes(t *testing.T) {
	tests := []struct {
		name          string
		size          int
		wantKept      int
		wantOversized bool
		wantServed    int
	}{
		{name: "empty stream", size: 0, wantKept: 0, wantServed: 0},
		{name: "exactly 65,536 bytes is kept whole", size: 65536, wantKept: 65536, wantServed: 65536},
		{name: "65,537 bytes is oversized", size: 65537, wantKept: 65536, wantOversized: true, wantServed: 65537},
		{name: "70 KiB is read only to 65,537 bytes", size: 70 * 1024, wantKept: 65536, wantOversized: true, wantServed: 65537},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			reader := &countingReader{remaining: tt.size, fill: 'x'}

			stream := readReadinessStream(reader)

			assert.Len(t, stream.Data, tt.wantKept)
			assert.Equal(t, tt.wantOversized, stream.Oversized)
			assert.Equal(t, tt.wantServed, reader.served)
			assert.NoError(t, stream.Err)
		})
	}
}

func TestReadReadinessStream_NilAndFailingReaders(t *testing.T) {
	t.Parallel()
	assert.Equal(t, readinessStream{}, readReadinessStream(nil))

	stream := readReadinessStream(failingReader{})
	assert.Error(t, stream.Err)
	assert.False(t, stream.Oversized)
}

func TestProbeProviderReadiness_StreamBounds_DecideClassification(t *testing.T) {
	const loggedIn = `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"dev@example.com","orgId":"org-123"}`
	padded := loggedIn + strings.Repeat(" ", readinessStreamKeepBytes-len(loggedIn))
	require.Len(t, padded, 65536)

	// Given: S16, a valid R1 output padded to exactly 65,536 bytes
	result := probeSingleReadiness(t, orchestra.ProviderConfig{Name: "claude"}, replyWith(0, padded, ""), nil)
	assert.Equal(t, "ready", result.Token())

	// Given: a 70 KiB claude stdout behind a counting reader
	counter := &countingReader{remaining: 70 * 1024, fill: ' '}
	result = probeSingleReadiness(t, orchestra.ProviderConfig{Name: "claude"},
		func(context.Context, providerReadinessCommand) (providerReadinessProcess, error) {
			return providerReadinessProcess{Stdout: counter, Stderr: strings.NewReader(""),
				Wait: func() (int, error) { return 0, nil }}, nil
		}, nil)
	assert.Equal(t, "unknown(oversized_output)", result.Token())
	assert.Equal(t, 65537, counter.served)

	// Given: codex exits 0, which alone means ready, but floods stderr
	result = probeSingleReadiness(t, orchestra.ProviderConfig{Name: "codex"},
		replyWith(0, "", strings.Repeat("Logged in using ChatGPT\n", 3000)), nil)
	assert.Equal(t, "unknown(oversized_output)", result.Token())
}

func TestProbeProviderReadiness_OversizedStream_StopsProbeWithoutWaitingForTimeout(t *testing.T) {
	// Given: stdout floods while stderr stays held open and the process never exits on its own
	installReadinessRunner(t, func(ctx context.Context, _ providerReadinessCommand) (providerReadinessProcess, error) {
		release := make(chan struct{})
		return providerReadinessProcess{
			Stdout: bytes.NewReader(bytes.Repeat([]byte("y"), 70*1024)),
			Stderr: heldReader{release},
			Wait: func() (int, error) {
				<-ctx.Done()
				close(release)
				return -1, ctx.Err()
			},
		}, nil
	})

	// When
	started := time.Now()
	results := probeProviderReadiness(context.Background(),
		[]orchestra.ProviderConfig{{Name: "claude"}}, providerReadinessOptions{Env: []string{}})

	// Then
	require.Len(t, results, 1)
	assert.Equal(t, "unknown(oversized_output)", results[0].Token())
	assert.Less(t, time.Since(started), time.Second)
}

// installMarkedExecutables puts scripts alone on PATH; each leaves a marker once it runs.
func installMarkedExecutables(t *testing.T, scripts map[string]string) (string, func(string) bool) {
	t.Helper()
	dir, markers := t.TempDir(), t.TempDir()
	for name, body := range scripts {
		script := "#!/bin/sh\n: > '" + filepath.Join(markers, name) + "'\n" + body + "\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))
	}
	t.Setenv("PATH", dir)
	return dir, func(name string) bool {
		_, err := os.Stat(filepath.Join(markers, name))
		return err == nil
	}
}

func readinessArgv(argv ...string) providerReadinessCommand {
	return providerReadinessCommand{Argv: argv, Env: []string{"HOME=/nonexistent"}}
}

func TestStartProviderReadinessProcess_RefusesEveryArgvOutsideTheContract(t *testing.T) {
	dir, ran := installMarkedExecutables(t, map[string]string{"claude": "exit 0", "codex": "exit 0", "omp": "exit 0", "sh": "exit 0"})
	ompPath, ompIdentity, err := canonicalPipelineOMPExecutable(filepath.Join(dir, "omp"))
	require.NoError(t, err)
	_, claudeIdentity, err := canonicalPipelineOMPExecutable(filepath.Join(dir, "claude"))
	require.NoError(t, err)
	pinned := func(identity pipelineOMPExecutableIdentity, argv ...string) providerReadinessCommand {
		command := readinessArgv(argv...)
		command.ompIdentity = &identity
		return command
	}
	tests := map[string]providerReadinessCommand{
		"empty argv":                    readinessArgv(),
		"claude login":                  readinessArgv("claude", "auth", "login"),
		"claude logout":                 readinessArgv("claude", "auth", "logout"),
		"claude status without json":    readinessArgv("claude", "auth", "status"),
		"claude status with extra flag": readinessArgv("claude", "auth", "status", "--json", "--verbose"),
		"claude model call":             readinessArgv("claude", "--print", "hello"),
		"claude without arguments":      readinessArgv("claude"),
		"claude by absolute path":       readinessArgv(filepath.Join(dir, "claude"), "auth", "status", "--json"),
		"codex login":                   readinessArgv("codex", "login"),
		"codex logout":                  readinessArgv("codex", "logout"),
		"codex status with extra flag":  readinessArgv("codex", "login", "status", "--with-api-key"),
		"codex model call":              readinessArgv("codex", "exec", "hi"),
		"shell wrapper":                 readinessArgv("sh", "-c", "claude auth status --json"),
		"bare shell":                    readinessArgv("sh"),
		"omp by name":                   readinessArgv("omp", "usage", "--json", "--redact"),
		"omp path without identity":     readinessArgv(ompPath, "usage", "--json", "--redact"),
		"omp usage invalidate":          pinned(ompIdentity, ompPath, "usage", "invalidate"),
		"omp usage without redact":      pinned(ompIdentity, ompPath, "usage", "--json"),
		"omp usage with profile":        pinned(ompIdentity, ompPath, "usage", "--json", "--redact", "--profile", "work"),
		"omp token":                     pinned(ompIdentity, ompPath, "token"),
		"omp relative with identity":    pinned(ompIdentity, "omp", "usage", "--json", "--redact"),
		"identity of another file":      pinned(claudeIdentity, ompPath, "usage", "--json", "--redact"),
	}
	for name, command := range tests {
		t.Run(name, func(t *testing.T) {
			process, err := startProviderReadinessProcess(context.Background(), command)
			assert.Error(t, err)
			assert.Nil(t, process.Wait)
		})
	}
	for _, name := range []string{"claude", "codex", "omp", "sh"} {
		assert.False(t, ran(name), "%s must never start", name)
	}
}

func TestProbeProviderReadiness_RealRunner_ExecutesContractArgvAndExitCodes(t *testing.T) {
	// Given: executables that fail unless they receive exactly their contract arguments
	_, ran := installMarkedExecutables(t, map[string]string{
		"claude": `[ "$*" = "auth status --json" ] || exit 9` + "\n" + `printf '%s' '{"loggedIn":true}'`,
		"codex":  `[ "$*" = "login status" ] || exit 9` + "\n" + `printf 'Not logged in\n' >&2; exit 1`,
		"omp": `[ "$*" = "usage --json --redact" ] || exit 9` + "\n" +
			`[ "$AUTOPUS_OMP_MANAGED_INNER" = 1 ] || exit 9` + "\n" +
			`printf '%s' '` + ompUsageJSON(`[{"provider":"anthropic"}]`, "[]", "[]") + `'`,
	})
	providers := []orchestra.ProviderConfig{
		{Name: "claude"}, {Name: "codex"}, {Name: "gemini", Backend: "omp", Model: ompAnthropicModel},
	}

	// When
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{"HOME=/nonexistent"}})

	// Then
	require.Len(t, results, 3)
	assert.Equal(t, []string{"ready", "not_ready(logged_out)", "ready"},
		[]string{results[0].Token(), results[1].Token(), results[2].Token()})
	assert.True(t, ran("claude") && ran("codex") && ran("omp"))
}

func TestStartProviderReadinessProcess_OMPReplacedAfterResolution_IsRefused(t *testing.T) {
	dir, ran := installMarkedExecutables(t, map[string]string{"omp": "exit 0"})
	command, err := ompReadinessCommand([]string{"HOME=/nonexistent"}, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "omp"), []byte("#!/bin/sh\nexit 0\n# swapped\n"), 0o755))

	_, err = startProviderReadinessProcess(context.Background(), command)

	assert.Error(t, err)
	assert.False(t, ran("omp"))
}

func TestProbeProviderReadiness_RealRunnerOversizedOrMissing_EndsWithoutWaiting(t *testing.T) {
	installMarkedExecutables(t, map[string]string{"claude": "printf '%71680s' ''\nexec /bin/sleep 30"})
	providers := []orchestra.ProviderConfig{{Name: "claude"}, {Name: "codex"}}

	started := time.Now()
	results := probeProviderReadiness(context.Background(), providers, providerReadinessOptions{Env: []string{"HOME=/nonexistent"}})

	assert.Equal(t, "unknown(oversized_output)", results[0].Token())
	assert.Equal(t, "unknown(probe_failed)", results[1].Token(), "codex is not installed")
	assert.Less(t, time.Since(started), 3*time.Second)
}

func TestRedactReadinessText_ScrubsAccountIdentifiersAndKeepsContractLines(t *testing.T) {
	t.Parallel()
	redacted := map[string]string{
		"account dev@example.com is disabled":                 "account [REDACTED] is disabled",
		`{"email":"first.last+ci@mail.example.co.uk"}`:        `{"email":"[REDACTED]"}`,
		"token sk-ant-oat01-abc123 expired":                   "token [REDACTED] expired",
		"key sk-proj-AbC_123-xyz":                             "key [REDACTED]",
		"orgId org-123 and org_9f8e":                          "orgId [REDACTED] and [REDACTED]",
		"Authorization: Bearer abc.def-ghi":                   "Authorization: [REDACTED]",
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln tail":  "jwt [REDACTED] tail",
		"account 6f1c2a3b-1234-4abc-9def-0123456789ab linked": "account [REDACTED] linked",
	}
	for input, want := range redacted {
		assert.Equal(t, want, redactReadinessText(input), input)
	}
	for _, line := range []string{
		`preflight: codex not_ready(logged_out) - run "codex login"`,
		`omp openai-codex: 1 of 2 accounts unusable (auth_expired); run "omp usage --redact" for details`,
		`preflight: claude not_ready(auth_expired) - run "omp login anthropic" (same PI_CODING_AGENT_DIR as this review)`,
		"disk-usage task-list ask-me",
	} {
		assert.Equal(t, line, redactReadinessText(line))
	}
}

func TestProbeProviderReadiness_StreamReadErrorOrAbnormalExit_IsProbeFailed(t *testing.T) {
	result := probeSingleReadiness(t, orchestra.ProviderConfig{Name: "codex"},
		func(context.Context, providerReadinessCommand) (providerReadinessProcess, error) {
			return providerReadinessProcess{Stdout: failingReader{}, Stderr: strings.NewReader(""),
				Wait: func() (int, error) { return 0, nil }}, nil
		}, nil)
	assert.Equal(t, "unknown(probe_failed)", result.Token())

	result = probeSingleReadiness(t, orchestra.ProviderConfig{Name: "codex"},
		func(context.Context, providerReadinessCommand) (providerReadinessProcess, error) {
			return providerReadinessProcess{Stdout: strings.NewReader(""), Stderr: strings.NewReader("Not logged in"),
				Wait: func() (int, error) { return -1, errors.New("signal: killed") }}, nil
		}, nil)
	assert.Equal(t, "unknown(probe_failed)", result.Token())
}
