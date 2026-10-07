package brainstorm_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// logText is a failed-step log of about size bytes whose last line is the
// failing step, the way a real CI log ends.
func logText(run, size int) string {
	var b strings.Builder
	for i := 0; b.Len() < size-32; i++ {
		fmt.Fprintf(&b, "run %d line %04d ok\n", run, i)
	}
	return b.String() + "step 9 failed: exit 2\n"
}

// fencedAfter returns the text inside the fence that follows heading.
func fencedAfter(t *testing.T, doc, heading string) string {
	t.Helper()
	at := strings.Index(doc, "\n"+heading+"\n")
	require.GreaterOrEqual(t, at, 0, heading)
	lines := strings.Split(doc[at+len(heading)+2:], "\n")
	require.Equal(t, healthband.UntrustedNotice, lines[0])
	fence := strings.TrimSuffix(lines[1], "untrusted-evidence")
	for i, line := range lines[2:] {
		if line == fence {
			return strings.Join(lines[2:2+i], "\n")
		}
	}
	t.Fatalf("fence after %q never closes", heading)
	return ""
}

// Untrusted Input Contract item 8: the whole BS stays within 32 KiB and the
// evidence is cut first. The diagnosis stays whole, the first log fits, the
// second keeps its failing tail from a line start, and the rest is dropped.
func TestRender_EvidenceIsCutFirstToKeepTheBodyWithin32KiB(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.DiagnosisStatus = "ok"
	req.Diagnosis = sanitized(strings.Repeat("diagnosis line\n", 12<<10/15))
	for run := 4242; run < 4246; run++ {
		req.Logs = append(req.Logs, healthband.RunLog{RunID: int64(run), Attempt: 1, Evidence: sanitized(logText(run, 8<<10))})
	}

	doc := render(t, "BS-BAND-013", req)

	assert.LessOrEqual(t, len(doc), healthband.MaxBSBodyBytes)
	assert.Contains(t, doc, "diagnosis_status: ok\n\n"+healthband.Fence(req.Diagnosis.Text)+"\n", "the diagnosis is not cut")
	assert.Equal(t, strings.TrimSuffix(req.Logs[0].Evidence.Text, "\n"), fencedAfter(t, doc, "### Evidence: CI run 4242 attempt 1"))
	kept := fencedAfter(t, doc, "### Evidence: CI run 4243 attempt 1")
	full := req.Logs[1].Evidence.Text
	assert.Less(t, len(kept), len(full))
	assert.True(t, strings.HasSuffix(kept, "step 9 failed: exit 2"), "the cut keeps the failing tail")
	assert.True(t, strings.HasPrefix(kept, "run 4243 line "), "the cut starts at a line")
	assert.Contains(t, full, kept+"\n")
	assert.NotContains(t, doc, "CI run 4244")
	assert.NotContains(t, doc, "CI run 4245")
	assert.Contains(t, doc, "\nevidence_status: size_cap\n\n## ICE 스코어링 — Top N\n")
	assert.Empty(t, brainstorm.Validate([]byte(doc)))
}

// A diagnosis that alone exceeds the budget drops every evidence block and
// keeps the diagnosis from its start, cut at a line end.
func TestRender_OversizedDiagnosisKeepsItsHead(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.DiagnosisStatus = "ok"
	var diagnosis strings.Builder
	for i := 0; diagnosis.Len() < healthband.ProviderExcerptBytes; i++ {
		fmt.Fprintf(&diagnosis, "finding %05d\n", i)
	}
	req.Diagnosis = sanitized(diagnosis.String())
	req.Logs = []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: sanitized(logText(4242, 1<<10))}}

	doc := render(t, "BS-BAND-013", req)

	assert.LessOrEqual(t, len(doc), healthband.MaxBSBodyBytes)
	assert.Greater(t, len(doc), healthband.MaxBSBodyBytes-64, "the cut uses the budget")
	kept := fencedAfter(t, doc, "diagnosis_status: ok\n")
	assert.True(t, strings.HasPrefix(diagnosis.String(), kept+"\n"), "the head survives, cut at a line end")
	assert.True(t, strings.HasPrefix(kept, "finding 00000\n"))
	assert.NotContains(t, doc, "CI run 4242")
	assert.Contains(t, doc, "\nevidence_status: size_cap\n")
	assert.Empty(t, brainstorm.Validate([]byte(doc)))
}

// A cut never splits a line: an oversized single-line diagnosis or log is
// dropped instead.
func TestRender_OversizedSingleLineIsDroppedNotSplit(t *testing.T) {
	t.Parallel()
	line := strings.Repeat("x", 40<<10)
	logs := o2Request()
	logs.Logs = []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: sanitized(line)}}
	diagnosis := o2Request()
	diagnosis.DiagnosisStatus, diagnosis.Diagnosis = "ok", sanitized(line)

	for name, req := range map[string]brainstorm.Request{"log": logs, "diagnosis": diagnosis} {
		doc := render(t, "BS-BAND-013", req)

		assert.NotContains(t, doc, "xxxx", name)
		assert.NotContains(t, doc, healthband.UntrustedNotice, name)
		assert.Contains(t, doc, "\nevidence_status: size_cap\n", name)
		assert.Empty(t, brainstorm.Validate([]byte(doc)), name)
	}
}

// A body within the budget is never marked.
func TestRender_FittingBodyHasNoSizeCapMark(t *testing.T) {
	t.Parallel()
	req := o2Request()
	req.Logs = []healthband.RunLog{{RunID: 4242, Attempt: 2, Evidence: sanitized(logText(4242, 8<<10))}}

	doc := render(t, "BS-BAND-013", req)

	assert.NotContains(t, doc, "evidence_status")
	assert.Equal(t, strings.TrimSuffix(req.Logs[0].Evidence.Text, "\n"), fencedAfter(t, doc, "### Evidence: CI run 4242 attempt 2"))
}
