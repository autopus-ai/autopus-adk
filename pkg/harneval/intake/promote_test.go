package intake

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

func TestPromote_S10IDOutsideGrammar_RefusedBeforeAnyPathIsBuilt(t *testing.T) {
	t.Parallel()
	// Given a project root that does not exist, so any path use would fail
	// with a file system error instead.
	missing := filepath.Join(t.TempDir(), "absent")
	for _, id := range []string{"GTC-023E9302FF0B", "GTC-023e9302ff0b/../x", "../x", "", "GTC-023e9302ff0b.json"} {
		// When promote receives a candidate id outside the grammar.
		_, err := runPromote(missing, func(r *PromoteRequest) { r.CandidateID = id })

		// Then it is refused by the id check alone.
		requirePromoteRefusal(t, err, ReasonCandidateIDInvalid, "")
	}
	assert.NoDirExists(t, missing)
}

func TestPromote_CandidateMissing_RefusedWithoutWriting(t *testing.T) {
	t.Parallel()
	root := newPromoteProject(t)
	before := treeDigest(t, root)

	_, err := runPromote(root, func(r *PromoteRequest) { r.CandidateID = "GTC-8e80c7a18029" })

	requirePromoteRefusal(t, err, ReasonCandidateMissing, "")
	assert.Equal(t, before, treeDigest(t, root))
}

func TestPromote_CandidateDocumentDefect_IsCandidateInvalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, detail string
		edit         func(t *testing.T, root string)
	}{
		{"malformed JSON", DetailDecode, func(t *testing.T, root string) { writeFile(t, root, promoteCandidateAt, "{\n") }},
		{"unknown candidate field", DetailDecode, func(t *testing.T, root string) {
			body := strings.Replace(readFile(t, root, promoteCandidateAt), `"redacted": false,`, `"redacted": false, "extra": 1,`, 1)
			writeFile(t, root, promoteCandidateAt, body)
		}},
		{"unknown task field", DetailDecode, func(t *testing.T, root string) {
			body := strings.Replace(readFile(t, root, promoteCandidateAt), `"category": "hooks_settings",`, `"category": "hooks_settings", "extra": 1,`, 1)
			writeFile(t, root, promoteCandidateAt, body)
		}},
		{"trailing data", DetailDecode, func(t *testing.T, root string) {
			writeFile(t, root, promoteCandidateAt, readFile(t, root, promoteCandidateAt)+"{}\n")
		}},
		{"wrong schema", DetailDecode, func(t *testing.T, root string) {
			editCandidate(t, root, func(c *Candidate) { c.SchemaVersion = LinkSchemaV1 })
		}},
		{"short fingerprint", DetailDecode, func(t *testing.T, root string) {
			editCandidate(t, root, func(c *Candidate) { c.Fingerprint = c.Fingerprint[:12] })
		}},
		{"id differs from file name", DetailIDMismatch, func(t *testing.T, root string) {
			editCandidate(t, root, func(c *Candidate) { c.ID = "GTC-8e80c7a18029" })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			tc.edit(t, root)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, ReasonCandidateInvalid, tc.detail)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_ProvenanceMismatch_NamesTheField(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		detail string
		edit   func(*Candidate)
	}{
		"kind":                      {"kind", func(c *Candidate) { c.Task.Provenance.Kind = "manual" }},
		"fingerprint":               {"fingerprint", func(c *Candidate) { c.Task.Provenance.Fingerprint = fingerprintX }},
		"ref is not representative": {"ref", func(c *Candidate) { c.Task.Provenance.Ref = "L-1000" }},
		"ref outside learning_refs": {"ref", func(c *Candidate) { c.Representative, c.Task.Provenance.Ref = "L-007", "L-007" }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			editCandidate(t, root, tc.edit)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, ReasonCandidateProvenanceMismatch, tc.detail)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_S6DraftIncomplete_ReportsTheFirstGapInCheckOrder(t *testing.T) {
	t.Parallel()
	retire := func(c *Candidate) { c.Task.Status = harneval.TaskStatus{State: harneval.StateRetired, Reason: "dup"} }
	cases := []struct {
		name, detail string
		edit         func(*Candidate)
	}{
		{"untouched draft", "category", func(*Candidate) {}},
		{"category only", "assertions", func(c *Candidate) { c.Task.Category = "hooks_settings" }},
		{"retired before anything else", "status", retire},
		{"retired complete draft", "status", func(c *Candidate) { completeDraft(c); retire(c) }},
		{"agent kind", "kind", func(c *Candidate) { completeDraft(c); c.Task.Kind = harneval.KindAgent }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := newPromoteProject(t)
			editCandidate(t, root, tc.edit)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, ReasonDraftIncomplete, tc.detail)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_StrictTaskDefect_CarriesTheLoaderDetail(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		detail string
		edit   func(*Candidate)
	}{
		"assertion without platform": {harneval.DetailAssertionFieldInvalid, func(c *Candidate) { c.Task.Assertions[0].Platform = "" }},
		"unknown assertion kind":     {harneval.DetailUnknownAssertionKind, func(c *Candidate) { c.Task.Assertions[0].Kind = "file_present" }},
		"blank intent":               {harneval.DetailFieldInvalid, func(c *Candidate) { c.Task.Intent = " " }},
		"id outside the grammar":     {harneval.DetailFieldInvalid, func(c *Candidate) { c.Task.ID = "GT-INC-023E9302/../../x" }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			editCandidate(t, root, tc.edit)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, ReasonCandidateInvalid, tc.detail)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_S6NoSurfaceActivePath_IsRefused(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, root string){
		"agent path only": func(t *testing.T, root string) {
			writeRecord(t, root, harneval.ManifestPath, promoteManifest(promoteAgentDir))
		},
		"no manifest": func(t *testing.T, root string) {
			removeRel(t, root, harneval.ManifestPath)
		},
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			seed(t, root)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, ReasonNoActivePathForKind, harneval.KindSurface)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_S6ActiveSetAlreadyInvalid_StopsBeforePublishing(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		detail string
		seed   func(t *testing.T, root string)
	}{
		"broken surface task": {harneval.DetailUnknownField, func(t *testing.T, root string) {
			task := validTaskDoc("GT-FIX-A")
			task["extra"] = 1
			writeRecord(t, root, SurfaceTaskDir+"/GT-FIX-A.json", task)
		}},
		"corpus digest drift": {harneval.DetailCorpusDigestMismatch, func(t *testing.T, root string) {
			writeFile(t, root, "bench/corpus_a.json", "[]\n")
		}},
		"malformed manifest": {harneval.DetailMalformedJSON, func(t *testing.T, root string) {
			writeFile(t, root, harneval.ManifestPath, "{")
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			tc.seed(t, root)
			before := treeDigest(t, root)

			_, err := runPromote(root, nil)

			requirePromoteRefusal(t, err, ReasonActiveSetInvalid, tc.detail)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}
