package healthband_test

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

const (
	promptBase  = "0123456789abcdef0123456789abcdef01234567"
	promptClaim = "fedcba9876543210fedcba9876543210"
)

var noncePattern = regexp.MustCompile("(?m)^(`{4,})untrusted-([0-9a-f]{32})$")

// s8Input is the S8 setup: the key-1042 evaluation event, a diagnosis with a
// line of seven backticks, and the run 4242 attempt 1 evidence.
func s8Input(t *testing.T) healthband.PatchPromptInput {
	t.Helper()
	dir := t.TempDir()
	claim := plannedClaim(t, dir)
	diagnosis := "### Summary\nThe CI job fails in pkg/foo/foo.go:12:3.\n```````\nstill data\n```````\n### Likely cause\nA nil map in ./pkg/foo/foo.go."
	return healthband.PatchPromptInput{
		Event: claim.Event, DiagnoseClaimID: promptClaim, BaseSHA: promptBase,
		Diagnosis: healthband.SanitizeProviderOutput(diagnosis, false, dir),
		Logs: []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: healthband.SanitizeCILog(
			"--- FAIL: TestFoo (0.00s)\n    foo_test.go:9: panic in "+dir+"/pkg/foo/foo.go\nusing "+syntheticToken+"\n", false, dir)}},
		TrackedPaths: []string{"pkg/foo/foo.go", "pkg/foo/foo_test.go", "README.md", "foo.go"},
	}
}

// S8: the manifest lists exactly the two stable layers, the evaluation
// snapshot, and the diagnosis, run log, and base ephemeral layers; every
// ephemeral layer sits in an untrusted-<nonce> fence longer than any
// backtick run inside; the nonce occurs nowhere else but where the
// instructions name it; and no layer holds a BS ID.
func TestPatchPrompt_S8LayeredFencedManifest(t *testing.T) {
	t.Parallel()
	in := s8Input(t)
	hash, err := healthband.EventHash(in.Event)
	require.NoError(t, err)
	snapshot, err := healthband.EvaluationLayer(in.Event)
	require.NoError(t, err)

	rendered, err := healthband.PatchPrompt(in)

	require.NoError(t, err)
	assert.Equal(t, []manifestRow{
		{"band.patch_instructions.v1", "stable", "healthband/patchprompt.go#band.patch_instructions.v1", true, "passed", "none"},
		{"band.patch_rules.v1", "stable", "healthband/patchprompt.go#band.patch_rules.v1", true, "passed", "none"},
		{"band.evaluation." + hash, "snapshot", "band-events.jsonl#seq=1", false, "passed", "none"},
		{"band.base." + promptBase, "ephemeral", "git-ls-tree/" + promptBase, false, "passed", "none"},
		{"band.diagnosis." + promptClaim, "ephemeral", "band-diagnosis/" + promptClaim, false, "passed", "none"},
		{"band.evidence.run.4242.a1", "ephemeral", "gh-run-log/4242/attempt/1", false, "redacted", "secret_risk"},
	}, manifestRows(rendered.Manifest))
	assert.Contains(t, rendered.Prompt, snapshot.Content)
	assert.NotContains(t, rendered.Prompt, "BS-BAND")
	assert.NotContains(t, rendered.Prompt, "ghp_")

	fences := noncePattern.FindAllStringSubmatch(rendered.Prompt, -1)
	require.Len(t, fences, 3, "one fence per ephemeral layer")
	fence, nonce := fences[0][1], fences[0][2]
	closing := 0
	for _, line := range strings.Split(rendered.Prompt, "\n") {
		if line == fence {
			closing++
		}
	}
	assert.Equal(t, 3, closing, "every fence closes with the same run")
	assert.GreaterOrEqual(t, len(fence), 8, "longer than the seven-backtick line")
	for _, other := range fences {
		assert.Equal(t, fence+"untrusted-"+nonce, other[0], "one fence and one nonce per prompt")
	}
	rules := strings.Index(rendered.Prompt, "Patch policy:")
	first := strings.Index(rendered.Prompt, fences[0][0])
	require.Positive(t, rules)
	assert.Contains(t, rendered.Prompt[:rules], nonce, "the instructions name the nonce")
	assert.NotContains(t, rendered.Prompt[rules:first], nonce)
	assert.NotContains(t, noncePattern.ReplaceAllString(rendered.Prompt[first:], ""), nonce, "no layer text holds the nonce")
	assert.Contains(t, rendered.Prompt, in.Diagnosis.Text, "the diagnosis layer holds the sanitized text from memory")
	assert.Contains(t, rendered.Prompt, "pkg/foo/foo.go", "the base layer names tracked paths of the evidence")
}

// S8: the instructions frame the diff as a proposal that band checks and
// applies outside the session, which edits no file, and ask for exactly one
// diff fence and no command; the rules layer states the caps.
func TestPatchPrompt_InstructionsFrameAProposal(t *testing.T) {
	t.Parallel()
	rendered, err := healthband.PatchPrompt(s8Input(t))
	require.NoError(t, err)
	instructions := rendered.Prompt[:strings.Index(rendered.Prompt, "Patch policy:")]

	for _, phrase := range []string{"proposal", "band checks", "applies it outside this session", "edits no file", "exactly one diff fence", "run no command", "untrusted-"} {
		assert.Contains(t, instructions, phrase)
	}
	for _, phrase := range []string{"10 files", "400 changed lines", "64 KiB", "mode 100644", "segment that starts with a dot", "40 or more base64 or hex"} {
		assert.Contains(t, rendered.Prompt, phrase)
	}
}

// The base layer lists the tracked paths that the evidence names, longest
// tracked suffix per token, sorted, and never file content.
func TestPatchPrompt_BaseLayerListsTrackedPathsNamedInTheEvidence(t *testing.T) {
	t.Parallel()
	rendered, err := healthband.PatchPrompt(s8Input(t))
	require.NoError(t, err)

	base := layerText(t, rendered.Prompt, "Base commit")
	assert.Equal(t, "base: "+promptBase+"\npaths:\n- pkg/foo/foo.go", base)

	in := s8Input(t)
	in.TrackedPaths = nil
	none, err := healthband.PatchPrompt(in)
	require.NoError(t, err)
	assert.Equal(t, "base: "+promptBase+"\npaths: none", layerText(t, none.Prompt, "Base commit"))

	var named []string
	in.TrackedPaths = nil
	for i := range 70 {
		path := fmt.Sprintf("pkg/gen/f%02d.go", i)
		in.TrackedPaths, named = append(in.TrackedPaths, path), append(named, path)
	}
	in.Diagnosis = healthband.SanitizeProviderOutput(strings.Join(named, "\n"), false, "")
	capped, err := healthband.PatchPrompt(in)
	require.NoError(t, err)
	listed := strings.Split(layerText(t, capped.Prompt, "Base commit"), "\n- ")[1:]
	assert.Equal(t, named[:64], listed, "at most 64 paths, sorted")
}

// The nonce is drawn again while it occurs inside any layer, and a nonce
// source that fails is an error.
func TestPatchPrompt_NonceInsideALayer_IsDrawnAgain(t *testing.T) {
	t.Parallel()
	in := s8Input(t)
	first := strings.Repeat("ab", 16)
	in.Diagnosis = healthband.SanitizeProviderOutput("hash "+first+" seen in CI", false, "")
	in.Rand = bytes.NewReader(append(bytes.Repeat([]byte{0xab}, 16), bytes.Repeat([]byte{0x01}, 16)...))

	rendered, err := healthband.PatchPrompt(in)

	require.NoError(t, err)
	fences := noncePattern.FindAllStringSubmatch(rendered.Prompt, -1)
	require.NotEmpty(t, fences)
	assert.Equal(t, strings.Repeat("01", 16), fences[0][2])
	in.Rand = bytes.NewReader(nil)
	_, err = healthband.PatchPrompt(in)
	assert.Error(t, err)
	in.Rand = bytes.NewReader(bytes.Repeat([]byte{0xab}, 16*8))
	_, err = healthband.PatchPrompt(in)
	assert.Error(t, err, "a source that keeps drawing a nonce inside a layer")
}

// The stable rules layer is the same for every request, so it stays cache
// eligible, while the instructions name each request's nonce.
func TestPatchPrompt_RulesLayerIsStableAcrossRequests(t *testing.T) {
	t.Parallel()
	first, err := healthband.PatchPrompt(s8Input(t))
	require.NoError(t, err)
	second, err := healthband.PatchPrompt(s8Input(t))
	require.NoError(t, err)

	changed := changedIDs(promptlayer.CompareManifests(first.Manifest, second.Manifest))

	assert.NotContains(t, changed, "band.patch_rules.v1")
	assert.Contains(t, changed, "band.patch_instructions.v1")
}

// Input outside the contract is refused: raw evidence, a malformed claim
// id or base SHA, an invalid run, or a non-evaluation event.
func TestPatchPrompt_InputOutsideTheContract_IsAnError(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*healthband.PatchPromptInput){
		"raw diagnosis":  func(in *healthband.PatchPromptInput) { in.Diagnosis = healthband.Evidence{Text: "raw"} },
		"raw log":        func(in *healthband.PatchPromptInput) { in.Logs[0].Evidence = healthband.Evidence{Text: "raw"} },
		"bad claim id":   func(in *healthband.PatchPromptInput) { in.DiagnoseClaimID = "BS-BAND-001" },
		"bad base":       func(in *healthband.PatchPromptInput) { in.BaseSHA = "HEAD" },
		"bad run":        func(in *healthband.PatchPromptInput) { in.Logs[0].RunID = 0 },
		"bad attempt":    func(in *healthband.PatchPromptInput) { in.Logs[0].Attempt = 0 },
		"duplicate run":  func(in *healthband.PatchPromptInput) { in.Logs = append(in.Logs, in.Logs[0]) },
		"no evaluation":  func(in *healthband.PatchPromptInput) { in.Event.Kind = "action_result" },
		"failing source": func(in *healthband.PatchPromptInput) { in.Rand = failingReader{} },
	} {
		in := s8Input(t)
		mutate(&in)
		_, err := healthband.PatchPrompt(in)
		assert.Error(t, err, name)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("no entropy") }

// layerText returns the fenced text below the ephemeral layer heading that
// starts with heading.
func layerText(t *testing.T, prompt, heading string) string {
	t.Helper()
	_, rest, ok := strings.Cut(prompt, "\n"+heading)
	require.True(t, ok, "no layer %q", heading)
	open := noncePattern.FindStringSubmatchIndex(rest)
	require.NotNil(t, open)
	fence := rest[open[2]:open[3]]
	body := rest[open[1]+1:]
	end := strings.Index(body, "\n"+fence)
	require.GreaterOrEqual(t, end, 0)
	return body[:end]
}
