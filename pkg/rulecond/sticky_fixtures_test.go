package rulecond_test

// SPEC-STICKYRULE-001 sticky re-attach oracle fixtures.
//
// These tests are the API contract for the sticky runtime. The planned exported
// surface they exercise is:
//
//	consts   EventUserPromptSubmit, StickyStateDirRelPath, DefaultStickyCadence,
//	         MaxStickyBodyBytes, MaxStickyContextBytes
//	types    StickyRule
//	funcs    StickyFire, StickyStateKey, ResolveStickyCadence, IsSticky
//	fields   Manifest.Sticky, CompiledClaude.Sticky
//
// Manifest contract: the sticky rule set is recorded in the same compiled
// manifest the conditional dispatcher already reads, at rulecond.ManifestRelPath,
// under a `sticky` array of {name, body} objects. `body` is relative to the
// sticky body root rulecond.ClaudeRulesRelDir (`.claude/rules/autopus`).
//
// Output contract: an injecting invocation writes one JSON object whose
// hookSpecificOutput.hookEventName equals `UserPromptSubmit` and whose
// hookSpecificOutput.additionalContext names every injected rule and carries its
// injectable body, meaning the body with the YAML frontmatter removed. Every
// non-injecting invocation writes empty stdout. A fail-closed suppression is one
// "<rule> <reason_code>" stderr line and nothing else.
//
// State contract: the per-session counter lives at
// <root>/<StickyStateDirRelPath>/<StickyStateKey(session_id)> and holds the
// decimal prompt index as text.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

const (
	// stickyEvent is the only hook event this SPEC compiles to.
	stickyEvent = "UserPromptSubmit"
	// stickyDefaultCadence is the REQ-STICKYRULE-MAP-01 fallback.
	stickyDefaultCadence = 8
	// The two rules REQ-STICKYRULE-MAP-01 marks sticky.
	ruleLanguagePolicy     = "language-policy"
	ruleObjectiveReasoning = "objective-reasoning"
)

// stickyFixture builds a throwaway project root holding the compiled manifest
// and the sticky body root, then drives the sticky runtime against it.
type stickyFixture struct {
	t    *testing.T
	root string
}

func newStickyFixture(t *testing.T) *stickyFixture {
	t.Helper()
	root := t.TempDir()
	f := &stickyFixture{t: t, root: root}
	require.NoError(t, os.MkdirAll(f.stickyRoot(), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Dir(f.manifestPath()), 0o755))
	return f
}

// stickyRoot is the sticky body root: the baseline claude rule directory.
func (f *stickyFixture) stickyRoot() string {
	return filepath.Join(f.root, filepath.FromSlash(rulecond.ClaudeRulesRelDir))
}

func (f *stickyFixture) manifestPath() string {
	return filepath.Join(f.root, filepath.FromSlash(rulecond.ManifestRelPath))
}

func (f *stickyFixture) stateDir() string {
	return filepath.Join(f.root, filepath.FromSlash(rulecond.StickyStateDirRelPath))
}

// writeStickyBody writes one body file into the sticky body root verbatim.
func (f *stickyFixture) writeStickyBody(name, content string) {
	f.t.Helper()
	path := filepath.Join(f.stickyRoot(), name)
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o644))
}

// writeShippedStickyPair installs the two shipped sticky rules exactly as the
// claude adapter emits them, frontmatter included, so the runtime's frontmatter
// stripping is genuinely exercised.
func (f *stickyFixture) writeShippedStickyPair() {
	f.t.Helper()
	for _, name := range []string{ruleLanguagePolicy, ruleObjectiveReasoning} {
		f.writeStickyBody(name+".md", sourceRuleFile(f.t, name))
	}
	f.writeStickyManifest(
		rulecond.StickyRule{Name: ruleLanguagePolicy, Body: ruleLanguagePolicy + ".md"},
		rulecond.StickyRule{Name: ruleObjectiveReasoning, Body: ruleObjectiveReasoning + ".md"},
	)
}

func (f *stickyFixture) writeStickyManifest(rules ...rulecond.StickyRule) {
	f.t.Helper()
	blob, err := json.Marshal(rulecond.Manifest{Version: "1", Sticky: rules})
	require.NoError(f.t, err)
	f.writeRawManifestFile(string(blob))
}

func (f *stickyFixture) writeRawManifestFile(raw string) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(filepath.Dir(f.manifestPath()), 0o755))
	require.NoError(f.t, os.WriteFile(f.manifestPath(), []byte(raw), 0o644))
}

func (f *stickyFixture) removeStickyManifest() {
	f.t.Helper()
	require.NoError(f.t, os.RemoveAll(f.manifestPath()))
}

// submit runs one UserPromptSubmit invocation at the default cadence.
func (f *stickyFixture) submit(sessionID string) (string, string) {
	f.t.Helper()
	return f.submitAt(sessionID, stickyDefaultCadence)
}

// submitAt runs one UserPromptSubmit invocation at an explicit cadence.
func (f *stickyFixture) submitAt(sessionID string, cadence int) (string, string) {
	f.t.Helper()
	return f.submitRaw(stickyPayload(sessionID), cadence)
}

// submitRaw runs one invocation against a caller-supplied stdin payload. The
// sticky runtime is advisory: REQ-STICKYRULE-FIRE-03 forbids any path that
// would let the caller observe a failure, so it must never return an error.
func (f *stickyFixture) submitRaw(stdin string, cadence int) (string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	err := rulecond.StickyFire(f.root, stickyEvent, cadence, strings.NewReader(stdin), &out, &errOut)
	require.NoError(f.t, err,
		"the sticky runtime must stay advisory and never surface an error")
	return out.String(), errOut.String()
}

// counter reads the persisted prompt index for a session.
func (f *stickyFixture) counter(sessionID string) int {
	f.t.Helper()
	path := filepath.Join(f.stateDir(), rulecond.StickyStateKey(sessionID))
	raw, err := os.ReadFile(path)
	require.NoError(f.t, err, "the per-session counter file must exist at %s", path)
	value, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
	require.NoError(f.t, convErr, "the counter file must hold a decimal integer, got %q", raw)
	return value
}

// stickyPayload builds a minimal UserPromptSubmit hook payload.
func stickyPayload(sessionID string) string {
	return stickyPayloadFull(sessionID, "hello", "/tmp/transcript.jsonl", "/tmp/cwd")
}

// stickyPayloadFull builds a UserPromptSubmit hook payload carrying every field
// REQ-STICKYRULE-FIRE-04 forbids the runtime from echoing or persisting.
func stickyPayloadFull(sessionID, prompt, transcript, cwd string) string {
	blob, err := json.Marshal(map[string]any{
		"hook_event_name": stickyEvent,
		"session_id":      sessionID,
		"prompt":          prompt,
		"transcript_path": transcript,
		"cwd":             cwd,
	})
	if err != nil {
		panic(err)
	}
	return string(blob)
}

// stickyContext parses sticky stdout and returns additionalContext, asserting
// the REQ-STICKYRULE-FIRE-01 structured shape. Empty stdout yields "".
func stickyContext(t *testing.T, stdout string) string {
	t.Helper()
	if strings.TrimSpace(stdout) == "" {
		return ""
	}
	var payload struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload),
		"sticky stdout must be a JSON hook payload")
	require.Equal(t, stickyEvent, payload.HookSpecificOutput.HookEventName,
		"a bare additionalContext without hookEventName is not recognized as structured hook output")
	return payload.HookSpecificOutput.AdditionalContext
}

// sourceRuleFile returns the shipped rule file verbatim, frontmatter included.
func sourceRuleFile(t *testing.T, name string) string {
	t.Helper()
	return ruleSourceText(t, name)
}

// injectableBody is the byte-accounting unit of REQ-STICKYRULE-FIRE-05 and
// REQ-STICKYRULE-FIRE-07: the shipped body with its frontmatter removed.
func injectableBody(t *testing.T, name string) string {
	t.Helper()
	return sourceRuleBody(t, name)
}
