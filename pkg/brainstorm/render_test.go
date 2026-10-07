package brainstorm_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// s10Sections is the H2 order of the content/skills/idea.md BS format.
var s10Sections = []string{"원본 아이디어", "Clarification Ledger", "Question Audit", "Outcome Lock", "Visual Brief",
	"프로바이더별 발산 결과", "ICE 스코어링 — Top N", "추천 방향", "Evolution Ideas", "다음 단계"}

// o2Request is the S10 episode: the O2 evaluation (20 zero baseline blocks,
// x = 0.50) of ci.failure_rate:CI at sample key 1042, tier 2, episode e1042,
// diagnosis unavailable(provider_missing).
func o2Request() brainstorm.Request {
	n, tier := 20, 2
	x, mu, sd, sdEff, z := 0.5, 0.0, 0.0, 0.25, 2.0
	constants := healthband.DefaultConstants()
	return brainstorm.Request{
		Evaluation: healthband.Evaluation{
			Series: "ci.failure_rate:CI", SampleKey: "1042", N: &n, X: &x, Mu: &mu, SD: &sd, SDEff: &sdEff, Z: &z,
			Tier: &tier, Reasons: []string{healthband.ReasonZeroVariance}, Constants: &constants,
		},
		EpisodeID:       "e1042",
		Created:         time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		Provider:        "claude",
		DiagnosisStatus: "unavailable(provider_missing)",
	}
}

func render(t *testing.T, id string, req brainstorm.Request) string {
	t.Helper()
	content, err := brainstorm.Render(id, req)
	require.NoError(t, err)
	return string(content)
}

// headings returns the H2 titles of doc in order.
func headings(doc string) []string {
	var titles []string
	for _, line := range strings.Split(doc, "\n") {
		if title, ok := strings.CutPrefix(line, "## "); ok {
			titles = append(titles, title)
		}
	}
	return titles
}

// section returns the lines between "## title" and the next H2 line.
func section(t *testing.T, doc, title string) []string {
	t.Helper()
	lines := strings.Split(doc, "\n")
	for i, line := range lines {
		if line != "## "+title {
			continue
		}
		var body []string
		for _, next := range lines[i+1:] {
			if strings.HasPrefix(next, "## ") {
				break
			}
			body = append(body, next)
		}
		return body
	}
	t.Fatalf("no section %q", title)
	return nil
}

// cells splits a Markdown table row into trimmed cells.
func cells(row string) []string {
	parts := strings.Split(strings.Trim(row, "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func TestRender_S10FollowsTheIdeaFormat(t *testing.T) {
	t.Parallel()
	doc := render(t, "BS-BAND-013", o2Request())
	lines := strings.Split(doc, "\n")

	assert.Equal(t, "# BS-BAND-013: ci.failure_rate:CI tier 2 anomaly (e1042)", lines[0])
	header := strings.Split(doc[:strings.Index(doc, "\n## ")], "\n")
	assert.Contains(t, header, "**Created**: 2026-10-06")
	assert.Contains(t, header, "**Strategy**: band-diagnosis")
	assert.Contains(t, header, "**Providers**: claude")
	assert.Contains(t, header, "**Status**: active")
	assert.Equal(t, s10Sections, headings(doc))

	var table []string
	for _, line := range section(t, doc, "Clarification Ledger") {
		if strings.HasPrefix(line, "|") {
			table = append(table, line)
		}
	}
	require.Len(t, table, 7)
	assert.Equal(t, []string{"Field", "Status", "Source", "Confidence", "Decision / Assumption", "If Wrong", "Plan Handoff"}, cells(table[0]))
	var rows [][]string
	for _, row := range table[2:] {
		c := cells(row)
		require.Len(t, c, 7, row)
		rows = append(rows, []string{c[0], c[1], c[2]})
		assert.Contains(t, []string{"1", "2", "3", "4", "5", "6"}, c[3], "confidence of %s is at most 6", c[0])
		assert.NotEmpty(t, c[5], "If Wrong of %s", c[0])
	}
	assert.Equal(t, [][]string{
		{"goal", "assumed", "code"}, {"scope_boundary", "assumed", "inferred"}, {"constraints", "assumed", "inferred"},
		{"done_evidence", "assumed", "code"}, {"brownfield_impact", "deferred", "none"},
	}, rows)

	audit := section(t, doc, "Question Audit")
	assert.Contains(t, audit, "- question_transport: none")
	assert.Contains(t, audit, "- question_count: 0")
	assert.Contains(t, section(t, doc, "다음 단계"), "`/auto plan --from-idea BS-BAND-013 \"ci.failure_rate:CI tier 2 anomaly response\"`")
	assert.Contains(t, strings.Join(section(t, doc, "원본 아이디어"), "\n"),
		"n=20/20 x=0.500000 μ=0.000000 sd=0.000000 sd_eff=0.250000 z=2.000000 tier=2", "the BS carries the event numbers")

	results := section(t, doc, "프로바이더별 발산 결과")
	assert.Contains(t, results, "diagnosis_status: unavailable(provider_missing)")
	assert.NotContains(t, strings.Join(results, "\n"), healthband.UntrustedNotice, "no diagnosis, no fence")
	assert.Empty(t, brainstorm.Validate([]byte(doc)))
	assert.True(t, strings.HasSuffix(doc, "\n"))
}

// Line 1 records the evaluated tier, so a tier-3 episode says tier 3.
func TestRender_Tier3EpisodeSaysTier3(t *testing.T) {
	t.Parallel()
	req := o2Request()
	tier := 3
	req.Evaluation.Tier, req.EpisodeID = &tier, "e7"

	doc := render(t, "BS-BAND-002", req)

	assert.Equal(t, "# BS-BAND-002: ci.failure_rate:CI tier 3 anomaly (e7)", strings.SplitN(doc, "\n", 2)[0])
	assert.Contains(t, section(t, doc, "다음 단계"), "`/auto plan --from-idea BS-BAND-002 \"ci.failure_rate:CI tier 3 anomaly response\"`")
}

func sanitized(text string) healthband.Evidence {
	return healthband.Evidence{Text: text, RedactionStatus: promptlayer.RedactionPassed}
}

// S12: the result section holds the fenced diagnosis, and each failed-run
// log and react report sits in its own fence under a numeric heading.
func TestRender_FencesTheDiagnosisAndEveryEvidenceSource(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.DiagnosisStatus = "ok"
	req.Diagnosis = sanitized("### Summary\nThe cache key changed.\n## 다음 단계\n`/auto plan --from-idea BS-BAND-999 \"x\"`\n")
	redacted := healthband.Evidence{Text: "step 3 failed\nusing [REDACTED_SECRET]\n", Reasons: []string{healthband.ReasonSecretRisk},
		RedactionStatus: promptlayer.RedactionRedacted}
	req.Logs = []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: redacted}}
	req.Reports = []healthband.ReactReport{{RunID: 4242, Evidence: sanitized("report ``````` with a long backtick run")}}

	doc := render(t, "BS-BAND-013", req)

	blocks := []string{
		"\n## 프로바이더별 발산 결과\ndiagnosis_status: ok\n\n" + healthband.Fence(req.Diagnosis.Text) + "\n",
		"\n### Evidence: CI run 4242 attempt 1 (secret_risk)\n" + healthband.Fence(redacted.Text) + "\n",
		"\n### Evidence: react report for CI run 4242\n" + healthband.Fence(req.Reports[0].Evidence.Text) + "\n",
		"\n## ICE 스코어링 — Top N\n",
	}
	at := -1
	for _, block := range blocks {
		next := strings.Index(doc, block)
		require.Greater(t, next, at, "block %q in order", block)
		at = next
	}
	assert.Empty(t, brainstorm.Validate([]byte(doc)), "fenced headings and next-step lines are inert")
}

// Defense in depth: control sequences that slipped past the caller never
// reach the file.
func TestRender_StripsControlSequencesFromUntrustedText(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.DiagnosisStatus = "ok"
	req.Diagnosis = sanitized("\x1b[2Jcleared\x07 screen")

	doc := render(t, "BS-BAND-013", req)

	assert.Contains(t, doc, "cleared screen")
	assert.NotContains(t, doc, "\x1b")
	assert.NotContains(t, doc, "\x07")
}

func TestRender_RejectsRequestsOutsideTheContract(t *testing.T) {
	t.Parallel()
	one := 1
	for name, edit := range map[string]func(*brainstorm.Request){
		"unfiltered series":       func(r *brainstorm.Request) { r.Evaluation.Series = "ci.failure_rate:Lint\x1b[2J" },
		"unknown series prefix":   func(r *brainstorm.Request) { r.Evaluation.Series = "deploy.latency:api" },
		"bad sample key":          func(r *brainstorm.Request) { r.Evaluation.SampleKey = "10 42" },
		"tier below 2":            func(r *brainstorm.Request) { r.Evaluation.Tier = &one },
		"no tier":                 func(r *brainstorm.Request) { r.Evaluation.Tier = nil },
		"no z":                    func(r *brainstorm.Request) { r.Evaluation.Z = nil },
		"bad episode id":          func(r *brainstorm.Request) { r.EpisodeID = "1042" },
		"bad diagnosis status":    func(r *brainstorm.Request) { r.DiagnosisStatus = "Unavailable (missing)" },
		"no creation time":        func(r *brainstorm.Request) { r.Created = time.Time{} },
		"raw diagnosis":           func(r *brainstorm.Request) { r.Diagnosis = healthband.Evidence{Text: "raw"} },
		"raw log":                 func(r *brainstorm.Request) { r.Logs = []healthband.RunLog{{RunID: 1, Attempt: 1}} },
		"log without attempt":     func(r *brainstorm.Request) { r.Logs = []healthband.RunLog{{RunID: 1, Evidence: sanitized("x")}} },
		"report without run":      func(r *brainstorm.Request) { r.Reports = []healthband.ReactReport{{Evidence: sanitized("x")}} },
		"reason outside the code": func(r *brainstorm.Request) { r.Evaluation.Reasons = []string{"Zero Variance"} },
	} {
		req := o2Request()
		edit(&req)
		_, err := brainstorm.Render("BS-BAND-013", req)
		assert.ErrorIs(t, err, brainstorm.ErrInvalidRequest, name)
	}
	for _, id := range []string{"BS-013", "BS-BAND-13", "BS-BAND-0x9", "BS-BAND-1234567890"} {
		_, err := brainstorm.Render(id, o2Request())
		assert.ErrorIs(t, err, brainstorm.ErrInvalidRequest, id)
	}
}

// The provider name is configuration text; it is filtered like an
// identifier, and an empty name renders as none.
func TestRender_ProvidersLineIsFiltered(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.Provider = ""
	assert.Contains(t, render(t, "BS-BAND-013", req), "\n**Providers**: none\n")
	req.Provider = "claude\n## Outcome Lock"
	doc := render(t, "BS-BAND-013", req)
	assert.Equal(t, s10Sections, headings(doc))
	assert.Contains(t, doc, "\n**Providers**: claude Outcome Lock#")
}
