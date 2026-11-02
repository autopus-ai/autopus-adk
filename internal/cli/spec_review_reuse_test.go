package cli

// SPEC-REVIEWREUSE-001 S9, S10, S12 (REQ-RR-REUSE-02 to REQ-RR-REUSE-07): an
// unchanged input returns the previous result with no call and no side
// effect, any change or damage triggers a paid review that names why, and
// --force-review re-runs against the same session budget.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// runReviewOutput runs one invocation and returns its stdout and stderr.
func runReviewOutput(t *testing.T, fixture *readOnlyReviewFixture, opts specReviewOptions) (string, string) {
	t.Helper()
	var stdout string
	stderr := captureSpecReviewStderr(t, func() {
		stdout = captureStdout(t, func() {
			require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, opts))
		})
	})
	return stdout, stderr
}

func TestRunSpecReview_UnchangedInputReturnsThePreviousResult(t *testing.T) {
	cases := []struct {
		name, reply, verdict, state string
	}{
		{"revise", majorCorrectnessRevise, "REVISE", "awaiting_changes"},
		{"reject", rejectNoFinding, "REJECT", "revisions_exhausted"},
		{"pass", passNoFinding, "PASS", "converged"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, nil)
			backend.replies["claude/reviewer"] = []scriptedReply{{output: tc.reply}}
			backend.replies["codex/reviewer"] = []scriptedReply{{output: tc.reply}}
			runReviewOutput(t, fixture, specReviewOptions{})
			require.Equal(t, 2, backend.count())
			first := readSpecReviewReceipt(t, fixture.specDir)
			files := map[string][]byte{}
			for _, name := range []string{"review-receipt.json", "review-findings.json", "review.md", "spec.md"} {
				files[name] = readFixtureFile(t, filepath.Join(fixture.specDir, name))
			}
			probes := countSpecReviewReadinessProbes(t)

			stdout, _ := runReviewOutput(t, fixture, specReviewOptions{})

			assert.Equal(t, 2, backend.count(), "a reused result makes no call")
			assert.Zero(t, probes.Load(), "a reused result probes nothing")
			assert.Contains(t, stdout, "SPEC 리뷰 재사용: "+fixture.specID+" — 입력이 "+first.FinishedAt+" 리뷰(run "+first.RunID+")와 같아 이전 결과를 반환합니다\n")
			assert.Contains(t, stdout, "판정: "+tc.verdict+"\n")
			assert.Contains(t, stdout, "리뷰 상태: "+tc.state+"\n")
			assert.Contains(t, stdout, "모델 호출 0회 — 다시 리뷰하려면 --force-review (같은 세션 예산 사용)\n")
			for name, data := range files {
				assert.Equal(t, string(data), string(readFixtureFile(t, filepath.Join(fixture.specDir, name))), name)
			}
			session := readTestSpecReviewSession(t, fixture.specDir)
			last := session.Attempts[len(session.Attempts)-1]
			assert.Equal(t, specReviewEndReasonReused, last.EndReason)
			for _, call := range session.Calls {
				assert.NotEqual(t, last.AttemptID, call.AttemptID, "the reused attempt made no call")
			}
		})
	}
}

func editFixtureConfig(t *testing.T, root string, edit func(*config.HarnessConfig)) {
	t.Helper()
	cfg, err := config.Load(root)
	require.NoError(t, err)
	edit(cfg)
	require.NoError(t, config.Save(root, cfg))
}

func editProviderArgs(t *testing.T, root, provider, old, replacement string) {
	t.Helper()
	editFixtureConfig(t, root, func(cfg *config.HarnessConfig) {
		entry := cfg.Orchestra.Providers[provider]
		for i, arg := range entry.Args {
			if arg == old {
				entry.Args[i] = replacement
			}
		}
		cfg.Orchestra.Providers[provider] = entry
	})
}

func rewriteFixtureReceipt(t *testing.T, specDir string, edit func(map[string]any)) {
	t.Helper()
	path := filepath.Join(specDir, "review-receipt.json")
	var receipt map[string]any
	require.NoError(t, json.Unmarshal(readFixtureFile(t, path), &receipt))
	edit(receipt)
	data, err := json.MarshalIndent(receipt, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

func appendFixtureFile(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, append(readFixtureFile(t, path), []byte(text)...), 0o644))
}

func TestRunSpecReview_AnyChangeOrDamageInvalidatesTheResult(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, fixture *readOnlyReviewFixture)
		text   string
	}{
		{"plan.md", func(t *testing.T, f *readOnlyReviewFixture) {
			appendFixtureFile(t, filepath.Join(f.specDir, "plan.md"), "\nOne more line.\n")
		}, "documents: plan.md"},
		{"new commit", func(t *testing.T, f *readOnlyReviewFixture) {
			appendFixtureFile(t, filepath.Join(f.root, "internal", "x.go"), "// changed\n")
			commitFixtureRepo(t, f.root, "change")
		}, "code: head"},
		{"uncommitted edit", func(t *testing.T, f *readOnlyReviewFixture) {
			appendFixtureFile(t, filepath.Join(f.root, "internal", "x.go"), "// changed\n")
		}, "code: worktree"},
		{"pass criteria", func(t *testing.T, f *readOnlyReviewFixture) {
			editFixtureConfig(t, f.root, func(cfg *config.HarnessConfig) { cfg.Spec.ReviewGate.PassCriteria = "CUSTOM_RULE" })
		}, "policy: pass_criteria"},
		{"checklist", func(t *testing.T, f *readOnlyReviewFixture) {
			original := specReviewChecklistDigest
			specReviewChecklistDigest = func() (string, error) { return strings.Repeat("c", 64), nil }
			t.Cleanup(func() { specReviewChecklistDigest = original })
		}, "checklist"},
		{"codex model", func(t *testing.T, f *readOnlyReviewFixture) {
			editProviderArgs(t, f.root, "codex", "gpt-5.6-sol", "gpt-5.6-sol-pro")
		}, "providers: codex"},
		{"claude effort", func(t *testing.T, f *readOnlyReviewFixture) {
			editProviderArgs(t, f.root, "claude", "max", "high")
		}, "providers: claude"},
		{"corrupt receipt", func(t *testing.T, f *readOnlyReviewFixture) {
			require.NoError(t, os.WriteFile(filepath.Join(f.specDir, "review-receipt.json"), []byte("{"), 0o644))
		}, "receipt: 손상"},
		{"incomplete fingerprint", func(t *testing.T, f *readOnlyReviewFixture) {
			rewriteFixtureReceipt(t, f.specDir, func(r map[string]any) { r["fingerprint"].(map[string]any)["complete"] = false })
		}, "fingerprint: 불완전"},
		{"findings byte", func(t *testing.T, f *readOnlyReviewFixture) {
			appendFixtureFile(t, filepath.Join(f.specDir, "review-findings.json"), " ")
		}, "evidence: review-findings.json"},
		{"older than 168 hours", func(t *testing.T, f *readOnlyReviewFixture) {
			rewriteFixtureReceipt(t, f.specDir, func(r map[string]any) {
				r["finished_at"] = time.Now().Add(-169 * time.Hour).UTC().Format(time.RFC3339Nano)
			})
		}, "168시간 초과"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, nil)
			backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
			backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
			runReviewOutput(t, fixture, specReviewOptions{})
			require.Equal(t, 2, backend.count())

			tc.change(t, fixture)
			_, stderr := runReviewOutput(t, fixture, specReviewOptions{})

			assert.Equal(t, 4, backend.count(), "the changed input is reviewed again")
			assert.Contains(t, stderr, "리뷰 재사용 안 함:")
			assert.Contains(t, stderr, tc.text)
		})
	}
}

// S10 last line and REQ-RR-REUSE-04: a PASS reviewed by an unpinned model is
// reviewed again; a REJECT from the same model is still reused.
func TestRunSpecReview_UnpinnedModelBlocksOnlyAReusedPass(t *testing.T) {
	for _, tc := range []struct {
		reply string
		calls int
	}{{passNoFinding, 4}, {rejectNoFinding, 2}} {
		fixture, backend := reuseFixture(t, config.ReviewBudgetConf{}, func(cfg *config.HarnessConfig) {
			// A user-pinned policy keeps the configured alias; otherwise the
			// quality profile replaces it with a versioned model.
			entry := cfg.Orchestra.Providers["claude"]
			entry.Args = []string{"--print", "--model", "opus", "--effort", "max"}
			entry.ModelPolicy = "pinned"
			cfg.Orchestra.Providers["claude"] = entry
		})
		backend.replies["claude/reviewer"] = []scriptedReply{{output: tc.reply}}
		backend.replies["codex/reviewer"] = []scriptedReply{{output: tc.reply}}
		runReviewOutput(t, fixture, specReviewOptions{})
		_, stderr := runReviewOutput(t, fixture, specReviewOptions{})

		assert.Equal(t, tc.calls, backend.count())
		if tc.calls == 4 {
			assert.Contains(t, stderr, "PASS: 모델 alias (claude: opus)")
		}
	}
}

// S12: --force-review re-runs against the same session budget.
func TestRunSpecReview_ForceReviewSpendsTheSameBudget(t *testing.T) {
	fixture, backend := reuseFixture(t, config.ReviewBudgetConf{MaxCalls: 4}, nil)
	backend.replies["claude/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	backend.replies["codex/reviewer"] = []scriptedReply{{output: majorCorrectnessRevise}}
	runReviewOutput(t, fixture, specReviewOptions{})
	first := readTestSpecReviewSession(t, fixture.specDir)

	runReviewOutput(t, fixture, specReviewOptions{forceReview: true})

	assert.Equal(t, 4, backend.count())
	session := readTestSpecReviewSession(t, fixture.specDir)
	assert.Equal(t, first.SessionID, session.SessionID)
	assert.True(t, session.Attempts[len(session.Attempts)-1].ForcedReview)
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	require.NotNil(t, receipt.Session)
	assert.True(t, receipt.Session.ForcedReview)

	runReviewOutput(t, fixture, specReviewOptions{forceReview: true})
	assert.Equal(t, 4, backend.count(), "the spent session makes no call")
	assert.Equal(t, "budget_exhausted", readSpecReviewReceipt(t, fixture.specDir).LoopStatus)
}
