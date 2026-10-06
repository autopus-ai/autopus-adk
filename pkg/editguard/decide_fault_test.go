package editguard

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testDialect stands in for the T6 platform codecs: the payload is
// {"cwd":"..","targets":[..]}, a non-string target is dropped as malformed, and
// a deny encodes as one JSON line.
type testDialect struct{ encodeErr error }

func (testDialect) Decode(payload []byte) (Call, error) {
	var doc struct {
		Cwd     string            `json:"cwd"`
		Targets []json.RawMessage `json:"targets"`
	}
	if err := json.Unmarshal(payload, &doc); err != nil {
		return Call{}, err
	}
	call := Call{Cwd: doc.Cwd}
	for _, raw := range doc.Targets {
		var target string
		if json.Unmarshal(raw, &target) != nil {
			call.Dropped++
			continue
		}
		call.Targets = append(call.Targets, target)
	}
	return call, nil
}

func (d testDialect) EncodeDeny(decision Decision) ([]byte, error) {
	data, _ := json.Marshal(map[string]string{"class": string(decision.Class), "reason": decision.Reason})
	return append(data, '\n'), d.encodeErr
}

func payloadOf(cwd string, targets ...any) string {
	data, _ := json.Marshal(map[string]any{"cwd": cwd, "targets": targets})
	return string(data)
}

type writeCounter struct {
	bytes.Buffer
	writes int
}

func (w *writeCounter) Write(p []byte) (int, error) { w.writes++; return w.Buffer.Write(p) }

type guardRun struct {
	code           int
	stdout, stderr string
	stdoutWrites   int
}

func runGuard(stdin io.Reader, dialect Dialect, opts Options) guardRun {
	var stdout writeCounter
	var stderr bytes.Buffer
	code := Run(stdin, &stdout, &stderr, dialect, opts)
	return guardRun{code: code, stdout: stdout.String(), stderr: stderr.String(), stdoutWrites: stdout.writes}
}

func (r guardRun) denied(t *testing.T) (string, string) {
	t.Helper()
	var doc map[string]string
	if r.code != 0 || r.stdoutWrites != 1 || json.Unmarshal([]byte(r.stdout), &doc) != nil {
		t.Fatalf("not one exit-0 deny write: %+v", r)
	}
	return doc["class"], doc["reason"]
}

func (r guardRun) allowed(t *testing.T, label string) {
	t.Helper()
	if r.code != 0 || r.stdout != "" || r.stdoutWrites != 0 || strings.Count(r.stderr, "\n") > 1 {
		t.Errorf("%s: want exit 0, empty stdout, at most one stderr line; got %+v", label, r)
	}
}

// S7 and REQ-EG-10: a call-level fault allows the whole call.
func TestRun_CallLevelFaults_AllowWithAtMostOneStderrLine(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	for label, stdin := range map[string]string{
		"truncated json":      `{"tool_input":`,
		"zero bytes":          "",
		"1048577 bytes":       strings.Repeat("a", MaxPayloadBytes+1),
		"no target":           `{"tool_name":"Edit","tool_input":{}}`,
		"non-string target":   `{"targets":[42]}`,
		"empty target string": payloadOf(root, ""),
		"null payload":        "null",
	} {
		runGuard(strings.NewReader(stdin), testDialect{}, Options{}).allowed(t, label)
	}
	panicked := runGuard(strings.NewReader(payloadOf(root, skillRel)), testDialect{},
		Options{panicSeam: func() { panic("seam") }})
	panicked.allowed(t, "recovered panic")
	if panicked.stderr != "autopus edit-guard: allow (internal error)\n" {
		t.Errorf("panic diagnostic = %q", panicked.stderr)
	}
	unreadable := runGuard(io.MultiReader(strings.NewReader("{"), failingReader{}), testDialect{}, Options{})
	unreadable.allowed(t, "unreadable stdin")
	if unreadable.stderr != "autopus edit-guard: allow (payload unreadable)\n" {
		t.Errorf("read-error diagnostic = %q", unreadable.stderr)
	}
	encodeFault := runGuard(strings.NewReader(payloadOf(root, skillRel)), testDialect{encodeErr: errInjected}, Options{})
	encodeFault.allowed(t, "deny not encodable")
	runGuard(strings.NewReader(payloadOf(root, skillRel)), nil, Options{}).allowed(t, "nil dialect")
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errInjected }

// REQ-EG-02: a payload of exactly 1 MiB is still decided; the bound is not off
// by one.
func TestRun_PayloadOfExactly1MiB_IsDecided(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	payload := payloadOf(root, skillRel)
	payload += strings.Repeat(" ", MaxPayloadBytes-len(payload))
	class, reason := runGuard(strings.NewReader(payload), testDialect{}, Options{}).denied(t)
	if class != string(ClassGeneratedSurface) || reason != gsConReason {
		t.Fatalf("deny = %s %q", class, reason)
	}
}

// S16 and REQ-EG-18: a stage fault drops exactly the untrustworthy part, so
// no row is stricter than the guard without that part.
func TestRun_StageFaults_DropOnlyTheUntrustworthyPart(t *testing.T) {
	t.Parallel()
	symlinkedState := func(t *testing.T) string {
		root := fixtureR(t)
		writeFile(t, root, tRel, tContent)
		mustLock(t, openTestStore(t, root, newClock(t0)), tRel, skillRel)
		moved := filepath.Join(realDir(t, t.TempDir()), "runtime")
		if err := os.Rename(filepath.Join(root, ".autopus", "runtime"), moved); err != nil {
			t.Fatal(err)
		}
		symlink(t, moved, filepath.Join(root, ".autopus", "runtime"))
		return root
	}
	lockedT := func(t *testing.T) string {
		root := fixtureR(t)
		writeFile(t, root, tRel, tContent)
		mustLock(t, openTestStore(t, root, newClock(t0)), tRel)
		return root
	}
	gsCon := [2]string{string(ClassGeneratedSurface), gsConReason}
	fl := [2]string{string(ClassFixLock), tFLReason}
	rows := []struct {
		state   string
		fixture func(t *testing.T) string
		targets []any
		want    [2]string // empty means EMPTY stdout
	}{
		{"symlinked runtime, lock on the skill", symlinkedState, []any{skillRel}, gsCon},
		{"symlinked runtime, lock on T", symlinkedState, []any{tRel}, [2]string{}},
		{"corrupt record next to T's lock", func(t *testing.T) string {
			root := lockedT(t)
			writeFile(t, root, FixLocksDir+"/planted.json", "not json")
			return root
		}, []any{tRel}, fl},
		{"T locked, claude manifest corrupt", func(t *testing.T) string {
			root := lockedT(t)
			writeFile(t, root, claudeManifest, "{")
			return root
		}, []any{tRel}, fl},
		{"opencode manifest corrupt, overridden target", corruptOpencode, []any{".agents/skills/x/SKILL.md"}, [2]string{}},
		{"opencode manifest corrupt, generated target", corruptOpencode, []any{skillRel}, [2]string{}},
		{"malformed second target", fixtureR, []any{skillRel, 42}, gsCon},
		{"malformed first target", fixtureR, []any{42, skillRel}, gsCon},
		{"only a malformed target", fixtureR, []any{42}, [2]string{}},
	}
	for _, row := range rows {
		root := row.fixture(t)
		got := runGuard(strings.NewReader(payloadOf(root, row.targets...)), testDialect{}, Options{Now: newClock(t0).Now})
		if row.want == ([2]string{}) {
			got.allowed(t, row.state)
			continue
		}
		if class, reason := got.denied(t); [2]string{class, reason} != row.want || got.stderr != "" {
			t.Errorf("%s: got %s %q, stderr %q", row.state, class, reason, got.stderr)
		}
	}
}

func corruptOpencode(t *testing.T) string {
	root := fixtureR(t)
	writeFile(t, root, ".autopus/opencode-manifest.json", "{")
	return root
}

// Each dropped part names itself in the one stderr line of an allow.
func TestRun_DroppedParts_NameThemselvesInTheDiagnostic(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, ".autopus/opencode-manifest.json", "{")
	got := runGuard(strings.NewReader(payloadOf(root, skillRel)), testDialect{}, Options{})
	if got.stderr != "autopus edit-guard: allow (manifest unreadable: .autopus/opencode-manifest.json)\n" {
		t.Errorf("manifest diagnostic = %q", got.stderr)
	}
	state := lockProject(t)
	mustLock(t, openTestStore(t, state, newClock(t0)), uRel)
	writeFile(t, state, FixLocksDir+"/planted.json", "not json")
	got = runGuard(strings.NewReader(payloadOf(state, tRel)), testDialect{}, Options{Now: newClock(t0).Now})
	if got.stderr != "autopus edit-guard: allow (lock record unreadable: .autopus/runtime/fix-locks/planted.json)\n" {
		t.Errorf("record diagnostic = %q", got.stderr)
	}
	unusable := lockProject(t)
	writeFile(t, unusable, ".autopus/runtime", "a file where the directory belongs")
	got = runGuard(strings.NewReader(payloadOf(unusable, tRel)), testDialect{}, Options{})
	if got.stderr != "autopus edit-guard: allow (lock state unusable: .autopus/runtime/fix-locks)\n" {
		t.Errorf("state diagnostic = %q", got.stderr)
	}
	got = runGuard(strings.NewReader(payloadOf(root, "pkg/a.go", 42)), testDialect{}, Options{})
	if got.stderr != "autopus edit-guard: allow (malformed target dropped)\n" {
		t.Errorf("dropped-target diagnostic = %q", got.stderr)
	}
	if got := runGuard(strings.NewReader(payloadOf(root, "pkg/a.go")), testDialect{}, Options{}); got.stderr != "" {
		t.Errorf("a normal allow wrote stderr %q", got.stderr)
	}
}
