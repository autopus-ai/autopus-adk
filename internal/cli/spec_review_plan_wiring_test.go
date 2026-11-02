package cli

// SPEC-REVIEWPOLICY-001 S7 and S8 at the CLI boundary: the printed plan names
// the reviewers and judge the fake backend actually receives, the recorded
// calls never exceed the printed bound, and refused combinations make no
// readiness probe and no model call.

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// markFixtureLean rewrites the fixture SPEC's frontmatter to declare class.
func markFixtureLean(t *testing.T, specDir, class string) {
	t.Helper()
	path := filepath.Join(specDir, "spec.md")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	lean := "status: draft\nprofile: lean\nchange_class: " + class + "\nsurface: internal/cli/spec_timing_test.go\n"
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(body), "status: draft\n", lean, 1)), 0o644))
}

var specReviewAttemptBoundRe = regexp.MustCompile(`이번 실행 상한 (\d+)`)

func TestRunSpecReview_PrintedPlanMatchesCalls(t *testing.T) {
	cases := []struct {
		name    string
		class   string
		opts    specReviewOptions
		lines   []string
		calls   map[string]int
		warning string
	}{
		{"lean default", "test_only", specReviewOptions{},
			[]string{"  profile: lean", "  리뷰어: claude (", "  fallback: codex, gemini", "  judge: 없음 — lean profile 기본값", "  quorum: 1\n"},
			map[string]int{"claude/reviewer": 1}, ""},
		{"single provider override", "test_only", specReviewOptions{providers: []string{"codex"}},
			[]string{"  profile: lean", "  리뷰어: codex (--providers)", "  judge: 없음", "  quorum: 1\n"},
			map[string]int{"codex/reviewer": 1}, ""},
		{"multiple provider override", "test_only", specReviewOptions{providers: []string{"codex", "gemini"}},
			[]string{"  profile: lean", "  리뷰어: codex (--providers)", "  fallback: gemini", "  judge: 없음", "  quorum: 1\n"},
			map[string]int{"codex/reviewer": 1}, "--providers codex,gemini"},
		{"explicit judge", "test_only", specReviewOptions{judge: "codex", judgeSet: true},
			[]string{"  profile: lean", "  리뷰어: claude (", "  judge: codex (always) — --judge", "  quorum: 1\n"},
			map[string]int{"claude/reviewer": 1, "codex/judge": 1}, "--judge codex"},
		{"high-risk lean", "feature", specReviewOptions{},
			[]string{"  profile: full", "  리뷰어: claude, codex, gemini (", "  judge: claude (always) — spec.review_gate.judge", "  quorum: 2"},
			map[string]int{"claude/reviewer": 1, "codex/reviewer": 1, "gemini/reviewer": 1, "claude/judge": 1}, "lean profile 조건"},
		{"raised profile", "test_only", specReviewOptions{profile: "full"},
			[]string{"  profile: full — --profile full", "  리뷰어: claude, codex, gemini (", "  judge: claude (always)", "  quorum: 2"},
			map[string]int{"claude/reviewer": 1, "codex/reviewer": 1, "gemini/reviewer": 1, "claude/judge": 1}, "--profile full"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, nil)
			writeGPTReviewDocuments(t, fixture.root)
			markFixtureLean(t, fixture.specDir, tc.class)
			backend := fixture.useFakeBackend(t)

			var runErr error
			stderr := captureSpecReviewStderr(t, func() {
				runErr = runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, tc.opts)
			})
			require.NoError(t, runErr, stderr)

			planStart := strings.Index(stderr, "SPEC 리뷰 계획 (")
			require.GreaterOrEqual(t, planStart, 0, stderr)
			plan := stderr[planStart:]
			for _, line := range tc.lines {
				assert.Contains(t, plan, line)
			}
			total := 0
			for key, want := range tc.calls {
				parts := strings.SplitN(key, "/", 2)
				assert.Equal(t, want, backend.calls(parts[0], parts[1]), key)
				total += want
			}
			assert.Equal(t, total, backend.total(), "no call outside the plan")
			match := specReviewAttemptBoundRe.FindStringSubmatch(plan)
			require.Len(t, match, 2)
			bound, err := strconv.Atoi(match[1])
			require.NoError(t, err)
			assert.LessOrEqual(t, backend.total(), bound)
			if tc.warning != "" {
				assert.Contains(t, stderr[:planStart], "경고: "+tc.warning)
			}
		})
	}
}

func TestRunSpecReview_InvalidCombinationsMakeNoProbeOrCall(t *testing.T) {
	cases := map[string]struct {
		opts specReviewOptions
		want []string
	}{
		"unknown profile":     {specReviewOptions{profile: "quick"}, []string{"--profile", "auto, full, lean"}},
		"lean on a full SPEC": {specReviewOptions{profile: "lean"}, []string{"--profile lean"}},
		"empty judge":         {specReviewOptions{judge: "", judgeSet: true}, []string{"--judge"}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, nil)
			backend := fixture.useFakeBackend(t)
			var probes atomic.Int32
			original := reviewReadinessProbe
			reviewReadinessProbe = func(ctx context.Context, providers []orchestra.ProviderConfig, opts providerReadinessOptions) []providerReadinessResult {
				probes.Add(1)
				return original(ctx, providers, opts)
			}
			t.Cleanup(func() { reviewReadinessProbe = original })

			err := runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, tc.opts)
			require.Error(t, err)
			for _, want := range tc.want {
				assert.Contains(t, err.Error(), want)
			}
			assert.Zero(t, backend.total())
			assert.Zero(t, probes.Load())
			assert.Zero(t, fixture.catalogProbes.Load())
		})
	}
}
