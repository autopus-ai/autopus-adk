package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
	"github.com/insajin/autopus-adk/pkg/orchestra"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

// reviewHeavyTranscript is a session that spends two thirds of its time in
// one review.
func reviewHeavyTranscript(t *testing.T, dir, name string) string {
	t.Helper()
	at := func(s int) string {
		return time.Date(2026, 11, 27, 9, 0, 0, 0, time.UTC).Add(time.Duration(s) * time.Second).Format(time.RFC3339)
	}
	lines := []string{
		fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"role":"user","content":"fix it"}}`, at(0)),
		fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"content":[{"type":"tool_use","id":"r1","name":"Agent","input":{"subagent_type":"reviewer"}}]}}`, at(1)),
		fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"content":[{"type":"tool_result","tool_use_id":"r1","content":"ok"}]}}`, at(1000)),
		fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"role":"user","content":"next"}}`, at(1500)),
	}
	path := filepath.Join(dir, name+".jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
	return path
}

func harnessRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	out, err := exec.Command("git", "init", "-q", dir).CombinedOutput()
	require.NoError(t, err, string(out))
	t.Chdir(dir)
	return dir
}

func stubHarnessProviders(t *testing.T, verdict string) *[]string {
	t.Helper()
	var calls []string
	oldP, oldA := harnessProviders, harnessAsker
	harnessProviders = func() ([]string, error) { return []string{"claude", "codex"}, nil }
	harnessAsker = func(string) harnessfb.Asker {
		return func(_ context.Context, provider, prompt string) (harnessfb.Reply, error) {
			calls = append(calls, provider)
			if strings.Contains(prompt, "LESSON:") {
				return harnessfb.Reply{Output: "LESSON: Review once per change.\nRATIONALE: measured.", Seconds: 4, Tokens: 900}, nil
			}
			return harnessfb.Reply{Output: "VERDICT: " + verdict + "\nREASON: fine", Seconds: 2, Tokens: 300}, nil
		}
	}
	t.Cleanup(func() { harnessProviders, harnessAsker = oldP, oldA })
	return &calls
}

// The loop through the commands: three observed sessions make a repeated
// pattern; the observation drafts and refutes a candidate on its own; the
// rule file changes only when a person approves.
func TestHarnessCommands_EndToEnd(t *testing.T) {
	dir := harnessRepo(t)
	t.Setenv(harnessFeedbackEnv, "")
	calls := stubHarnessProviders(t, "survived")
	tdir := t.TempDir()

	out, err := runAuto(t, "harness", "report")
	require.NoError(t, err)
	assert.Contains(t, out, "분석할 세션 기록이 없습니다")

	for i, id := range []string{"s1", "s2"} {
		_, err := runAuto(t, "harness", "observe", "--session-id", id, "--transcript", reviewHeavyTranscript(t, tdir, id))
		require.NoError(t, err, "session %d", i)
	}
	assert.Empty(t, *calls, "two sessions are not a repeated pattern: no model call")
	out, err = runAuto(t, "harness", "report")
	require.NoError(t, err)
	assert.Contains(t, out, "반복 패턴이 없습니다")
	assert.Contains(t, out, "review-heavy(세션 2개)")

	_, err = runAuto(t, "harness", "observe", "--session-id", "s3", "--transcript", reviewHeavyTranscript(t, tdir, "s3"))
	require.NoError(t, err)
	assert.Equal(t, []string{"claude", "codex"}, *calls, "drafted by one provider, refuted by the other")
	assert.Equal(t, "off", os.Getenv(harnessFeedbackEnv), "the providers' own sessions do not feed the loop")
	t.Setenv(harnessFeedbackEnv, "")

	out, err = runAuto(t, "harness", "report")
	require.NoError(t, err)
	assert.Contains(t, out, "review-heavy: 리뷰가 세션 시간의 30% 이상 — 세션 3개")
	assert.Contains(t, out, "16.6분, 세션 시간의 66.6%")
	assert.Contains(t, out, "리뷰            67%")

	out, err = runAuto(t, "harness", "status")
	require.NoError(t, err)
	assert.Contains(t, out, "[승인 대기]")
	assert.Contains(t, out, "HL-001 승인 대기 — review-heavy (세션 3개)")
	assert.Contains(t, out, "codex: 반증 실패 — fine")
	assert.Contains(t, out, "모델 호출 2회, 6초, 토큰 1200")
	assert.NoFileExists(t, filepath.Join(dir, harnessfb.RuleFile), "nothing changes before a person approves")

	out, err = runAuto(t, "harness", "approve", "HL-001")
	require.NoError(t, err)
	assert.Contains(t, out, filepath.Join(filepath.Base(dir), harnessfb.RuleFile)+"를 다시 썼습니다")
	rule, err := os.ReadFile(filepath.Join(dir, harnessfb.RuleFile))
	require.NoError(t, err)
	assert.Contains(t, string(rule), "HL-001 (review-heavy, 3 sessions): Review once per change.")

	*calls = nil
	_, err = runAuto(t, "harness", "observe", "--session-id", "s4", "--transcript", reviewHeavyTranscript(t, tdir, "s4"))
	require.NoError(t, err)
	assert.Empty(t, *calls, "a pattern with a candidate is not drafted again")

	_, err = runAuto(t, "harness", "retire", "HL-001")
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(dir, harnessfb.RuleFile))
}

func TestHarnessCommands_RefutedStaysOutOfTheQueue(t *testing.T) {
	harnessRepo(t)
	t.Setenv(harnessFeedbackEnv, "")
	stubHarnessProviders(t, "refuted")
	tdir := t.TempDir()
	for _, id := range []string{"s1", "s2", "s3"} {
		_, err := runAuto(t, "harness", "observe", "--session-id", id, "--transcript", reviewHeavyTranscript(t, tdir, id))
		require.NoError(t, err)
	}
	out, err := runAuto(t, "harness", "status")
	require.NoError(t, err)
	assert.NotContains(t, out, "[승인 대기]")
	assert.Contains(t, out, "[반증됨]")
	assert.Contains(t, out, "codex: 반증됨 — fine")
	_, err = runAuto(t, "harness", "approve", "HL-001")
	assert.Error(t, err)
}

// The Stop hook starts the observation in the background and its own
// answer does not depend on it.
func TestVerifyStop_StartsHarnessObservation(t *testing.T) {
	dir := t.TempDir()
	var spawned []string
	old := harnessSpawn
	harnessSpawn = func(session, transcript, d string) error {
		spawned = append(spawned, session+" "+transcript+" "+d)
		return fmt.Errorf("spawn failed")
	}
	t.Cleanup(func() { harnessSpawn = old })
	t.Setenv("AUTOPUS_VERIFY_STOP", "off")
	t.Setenv("AUTOPUS_DISK_AUTO_RECLAIM", "off")

	stop := func() string {
		cmd := newVerifyStopCmd()
		payload, _ := json.Marshal(stopHookInput{SessionID: "s1", Cwd: dir, TranscriptPath: "/x/s1.jsonl"})
		cmd.SetIn(bytes.NewReader(payload))
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.Execute())
		return out.String()
	}
	t.Setenv(harnessFeedbackEnv, "")
	on := stop()
	assert.Equal(t, []string{"s1 /x/s1.jsonl " + dir}, spawned)

	t.Setenv(harnessFeedbackEnv, "off")
	off := stop()
	assert.Len(t, spawned, 1, "off starts nothing")
	assert.Equal(t, off, on, "a failed spawn does not change the hook's answer")
}

// A failed draft makes propose fail and say so, instead of reporting that
// nothing was left to propose.
func TestHarnessPropose_ReportsAFailedDraft(t *testing.T) {
	harnessRepo(t)
	t.Setenv(harnessFeedbackEnv, "")
	oldP, oldA := harnessProviders, harnessAsker
	harnessProviders = func() ([]string, error) { return []string{"claude"}, nil }
	harnessAsker = func(string) harnessfb.Asker {
		return func(context.Context, string, string) (harnessfb.Reply, error) {
			return harnessfb.Reply{Output: "no idea", Tokens: -1}, nil
		}
	}
	t.Cleanup(func() { harnessProviders, harnessAsker = oldP, oldA })
	tdir := t.TempDir()
	for _, id := range []string{"s1", "s2", "s3"} {
		_, _ = runAuto(t, "harness", "observe", "--session-id", id, "--transcript", reviewHeavyTranscript(t, tdir, id))
		t.Setenv(harnessFeedbackEnv, "")
	}
	out, err := runAuto(t, "harness", "propose")
	assert.Error(t, err)
	assert.Contains(t, out, "후보 작성 실패")
	assert.NotContains(t, out, "새로 후보로 만들 반복 패턴이 없습니다")
}

func TestUsageTokens(t *testing.T) {
	in, out := int64(100), int64(20)
	assert.Equal(t, int64(-1), usageTokens(&orchestra.ProviderResponse{}))
	assert.Equal(t, int64(120), usageTokens(&orchestra.ProviderResponse{Usage: []telemetry.UsageEnvelope{{InputTokensTotal: &in, OutputTokensTotal: &out}}}))
}
