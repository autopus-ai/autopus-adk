package agentexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type execFunc = func(ctx context.Context, argv []string, dir, stdin string) (string, string, int, error)

// fakeRunner reads only env, never the real environment, so a developer's own
// AUTOPUS_QA_AGENT_ARGV cannot leak into the test.
func fakeRunner(t *testing.T, env map[string]string, run execFunc) Runner {
	t.Helper()
	return Runner{
		LookPath: func(name string) (string, error) { return name, nil },
		Exec:     run,
		Getenv:   func(k string) string { return env[k] },
		TempDir:  func() (string, error) { return os.MkdirTemp(t.TempDir(), "agent-*") },
	}
}

func mustNotExec(t *testing.T) execFunc {
	return func(context.Context, []string, string, string) (string, string, int, error) {
		t.Error("Exec ran although the run should have been refused")
		return "", "", 0, nil
	}
}

// AC-QALOOP-008: a missing binary yields qa_agent_cli_missing, not a failure.
func TestRun_MissingBinary_ReportsSetupGap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		target Target
		env    map[string]string
		binary string
	}{
		{"claude", TargetClaude, nil, "claude"},
		{"codex", TargetCodex, nil, "codex"},
		{"gemini", TargetGemini, nil, "agy"},
		{"opencode", TargetOpenCode, nil, "opencode"},
		{"override", TargetClaude, map[string]string{OverrideEnv: `["/opt/missing-agent","--x"]`}, "/opt/missing-agent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := fakeRunner(t, tc.env, mustNotExec(t))
			var looked string
			r.LookPath = func(name string) (string, error) { looked = name; return "", exec.ErrNotFound }

			resp, err := r.Run(context.Background(), Request{Target: tc.target, Mode: ModeGenerate, Prompt: "p"})

			var gap *SetupGapError
			require.ErrorAs(t, err, &gap)
			assert.Equal(t, "qa_agent_cli_missing", gap.Code)
			assert.Equal(t, tc.binary, gap.Binary)
			assert.Equal(t, tc.binary, looked)
			assert.Equal(t, tc.binary, resp.Argv[0])
		})
	}
}

func TestRun_Success_PassesWorkDirStdinAndDefaultTimeout(t *testing.T) {
	t.Parallel()
	var gotArgv []string
	var gotDir, gotStdin string
	var deadline time.Time
	r := fakeRunner(t, nil, func(ctx context.Context, argv []string, dir, stdin string) (string, string, int, error) {
		gotArgv, gotDir, gotStdin = argv, dir, stdin
		deadline, _ = ctx.Deadline()
		return "scenario yaml", "warn", 0, nil
	})
	start := time.Now()

	resp, err := r.Run(context.Background(), Request{Target: TargetClaude, Mode: ModeEdit, Prompt: "heal it", WorkDir: "/repo"})

	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "-p", "--permission-mode", "acceptEdits"}, gotArgv)
	assert.Equal(t, "/repo", gotDir)
	assert.Equal(t, "heal it", gotStdin)
	assert.WithinDuration(t, start.Add(15*time.Minute), deadline, time.Minute)
	assert.Equal(t, Response{Stdout: "scenario yaml", Stderr: "warn", Argv: gotArgv, Duration: resp.Duration}, resp)
}

func TestRun_NonZeroExit_ReturnsAgentErrorWithResponse(t *testing.T) {
	t.Parallel()
	r := fakeRunner(t, nil, func(context.Context, []string, string, string) (string, string, int, error) {
		return "partial output", "rate limited", 3, nil
	})

	resp, err := r.Run(context.Background(), Request{Target: TargetOpenCode, Mode: ModeGenerate, Prompt: "p"})

	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, "qa_agent_failed", agentErr.Code)
	assert.Equal(t, 3, agentErr.ExitCode)
	assert.Equal(t, "partial output", resp.Stdout)
	assert.Equal(t, "rate limited", resp.Stderr)
	assert.Equal(t, 3, resp.ExitCode)
	assert.Equal(t, []string{"opencode", "run", "p"}, resp.Argv)
}

func TestRun_StartFailure_IsAgentFailureWrappingCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("exec format error")
	r := fakeRunner(t, nil, func(context.Context, []string, string, string) (string, string, int, error) {
		return "", "", -1, cause
	})
	_, err := r.Run(context.Background(), Request{Target: TargetGemini, Mode: ModeGenerate, Prompt: "p"})
	assert.Equal(t, CodeFailed, ErrorCode(err))
	assert.ErrorIs(t, err, cause)
}

func TestRun_Deadline_ReportsTimeout(t *testing.T) {
	t.Parallel()
	r := fakeRunner(t, nil, func(ctx context.Context, _ []string, _, _ string) (string, string, int, error) {
		<-ctx.Done()
		return "half", "", -1, ctx.Err()
	})

	resp, err := r.Run(context.Background(), Request{Target: TargetClaude, Mode: ModeGenerate, Prompt: "p", Timeout: 20 * time.Millisecond})

	var agentErr *AgentError
	require.ErrorAs(t, err, &agentErr)
	assert.Equal(t, "qa_agent_timeout", agentErr.Code)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "half", resp.Stdout)
}

func TestRun_CallerCancel_IsFailureWrappingCanceled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := fakeRunner(t, nil, func(ctx context.Context, _ []string, _, _ string) (string, string, int, error) {
		return "", "", -1, ctx.Err()
	})
	_, err := r.Run(ctx, Request{Target: TargetClaude, Mode: ModeGenerate, Prompt: "p"})
	assert.Equal(t, CodeFailed, ErrorCode(err))
	assert.ErrorIs(t, err, context.Canceled)
}

func TestRun_Codex_ReadsLastMessageFromOutputFile(t *testing.T) {
	t.Parallel()
	for name, written := range map[string]string{"file wins": "final answer\n", "empty file falls back": "  \n"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var outPath, stdinSeen string
			r := fakeRunner(t, nil, func(_ context.Context, argv []string, _, stdin string) (string, string, int, error) {
				i := slices.Index(argv, "-o")
				require.GreaterOrEqual(t, i, 0)
				outPath, stdinSeen = argv[i+1], stdin
				require.NoError(t, os.WriteFile(outPath, []byte(written), 0o600))
				return "progress noise", "", 0, nil
			})

			resp, err := r.Run(context.Background(), Request{Target: TargetCodex, Mode: ModeGenerate, Prompt: "p", Timeout: time.Minute})

			require.NoError(t, err)
			assert.Equal(t, "p", stdinSeen)
			if written == "final answer\n" {
				assert.Equal(t, "final answer\n", resp.Stdout)
			} else {
				assert.Equal(t, "progress noise", resp.Stdout)
			}
			_, statErr := os.Stat(outPath)
			assert.True(t, os.IsNotExist(statErr), "run must remove its output dir, got %v", statErr)
		})
	}
}

// AC-QALOOP-008: AUTOPUS_QA_AGENT_ARGV replaces argv, prompt on stdin.
func TestRun_OverrideEnv_ReplacesArgvAndSendsPromptOnStdin(t *testing.T) {
	t.Parallel()
	var gotArgv []string
	var gotStdin string
	r := fakeRunner(t, map[string]string{OverrideEnv: `["/fake/agent","--json"]`},
		func(_ context.Context, argv []string, _, stdin string) (string, string, int, error) {
			gotArgv, gotStdin = argv, stdin
			return "ok", "", 0, nil
		})
	r.TempDir = func() (string, error) { t.Error("override runs need no codex output dir"); return "", nil }

	resp, err := r.Run(context.Background(), Request{Target: TargetCodex, Mode: ModeEdit, Prompt: "fix it"})

	require.NoError(t, err)
	assert.Equal(t, []string{"/fake/agent", "--json"}, gotArgv)
	assert.Equal(t, "fix it", gotStdin)
	assert.Equal(t, gotArgv, resp.Argv)
}

func TestRun_InvalidRequest_SpawnsNothing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		env  map[string]string
		req  Request
		code string
	}{
		{"empty prompt", nil, Request{Target: TargetCodex, Mode: ModeGenerate}, CodeRequestInvalid},
		{"unknown target", nil, Request{Target: "cursor", Mode: ModeGenerate, Prompt: "p"}, CodeTargetUnknown},
		{"invalid override", map[string]string{OverrideEnv: "{bad"}, Request{Target: TargetClaude, Mode: ModeGenerate, Prompt: "p"}, CodeOverrideInvalid},
	}
	for _, tc := range cases {
		r := fakeRunner(t, tc.env, mustNotExec(t))
		r.TempDir = func() (string, error) { t.Error("invalid request created a temp dir"); return "", nil }
		_, err := r.Run(context.Background(), tc.req)
		assert.Equal(t, tc.code, ErrorCode(err), tc.name)
	}
}

func TestRun_TempDirFailure_IsReported(t *testing.T) {
	t.Parallel()
	r := fakeRunner(t, nil, mustNotExec(t))
	r.TempDir = func() (string, error) { return "", os.ErrPermission }
	_, err := r.Run(context.Background(), Request{Target: TargetCodex, Mode: ModeGenerate, Prompt: "p"})
	assert.ErrorIs(t, err, os.ErrPermission)
}
