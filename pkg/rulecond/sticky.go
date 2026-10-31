package rulecond

import (
	"bytes"
	"io"
	"strings"
)

// The SPEC-STICKYRULE-001 platform and budget contract.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: the sticky event name, state location, cadence default, and byte caps — the compiler, the runtime, `auto rules list`, and the generated hook all read these.
// @AX:REASON: A bare additionalContext without EventUserPromptSubmit is not recognized as structured hook output, and the caps are the context budget the aggregate assembly enforces.
const (
	// EventUserPromptSubmit is the only hook event this SPEC compiles to. It
	// fires once before each user turn, which is what makes a prompt-index
	// cadence expressible at all.
	EventUserPromptSubmit = "UserPromptSubmit"
	// StickyStateDirRelPath holds one counter file per session. It sits under
	// the gitignored `.autopus/runtime/` tree (REQ-STICKYRULE-STATE-01).
	StickyStateDirRelPath = ".autopus/runtime/sticky-rules"
	// DefaultStickyCadence is the prompt interval used when the configured
	// cadence is absent, zero, or negative (REQ-STICKYRULE-MAP-01).
	DefaultStickyCadence = 8
	// MaxStickyBodyBytes caps a single injectable body. It equals MaxBodyBytes,
	// which is also what ReadBody enforces on the file it reads.
	MaxStickyBodyBytes = MaxBodyBytes
	// MaxStickyContextBytes caps the aggregate injected context. It admits the
	// shipped pair, whose injectable bodies measure 4492 bytes or less, that a
	// 4000-byte cap would have truncated.
	MaxStickyContextBytes = 6000
)

// stickyPanicHook is a test seam. The sticky runtime invokes it during context
// assembly when non-nil, which is what lets the exit barrier be exercised as an
// enforced property rather than an asserted one. It stays unexported so the
// production surface carries no test-only API.
//
// @AX:WARN [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: mutable package-level state read on every user turn and written only by tests.
// @AX:REASON: It is nil in production, so a test that sets it must restore it, and parallel tests in this package must not set it at all; a leaked non-nil value panics the runtime on every prompt of every session.
var stickyPanicHook func()

// ResolveStickyCadence resolves the effective prompt cadence: the configured
// value when it is positive, and DefaultStickyCadence when it is absent, zero,
// or negative (REQ-STICKYRULE-MAP-01).
//
// A non-integer scalar never reaches this function. The typed yaml.Unmarshal in
// pkg/config/loader.go rejects it at config load, so it is a load error rather
// than a fallback case, and this function has no string form to be lenient with.
func ResolveStickyCadence(configured int) int {
	if configured > 0 {
		return configured
	}
	return DefaultStickyCadence
}

// StickyFire runs one UserPromptSubmit invocation against the sticky set
// recorded in the manifest under root.
//
// Every runtime condition lands in exactly one of four cases
// (REQ-STICKYRULE-FIRE-08). Whole-run benign absence — a missing or malformed
// manifest, an empty sticky set, an unparseable stdin payload, an unusable state
// directory, or an off-cadence index — writes no stdout and no stderr. Per-rule
// benign absence — a body file that is simply gone — skips only that rule while
// every other sticky rule still injects. Per-rule fail-closed — a containment or
// size violation — suppresses only the offending rule and writes its name and
// reason code to stderr. An unresolvable project root is the caller's case: it
// is resolved before StickyFire is reached.
//
// The returned error is always nil. That is the contract, not an omission:
// exit code 2 on UserPromptSubmit erases the user's prompt, so no failure inside
// this runtime may reach the caller in a form it could turn into a non-zero exit.
//
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-STICKYRULE-001: advisory sticky entry point — the generated UserPromptSubmit hook and internal/cli/rules_sticky.go depend on the exit-zero contract and on stdout being written only once, whole.
// @AX:REASON: The deferred recover is an enforced barrier: an unrecovered Go panic terminates the process with exit status 2, which on this event discards the prompt the user just typed.
func StickyFire(root, event string, cadence int, stdin io.Reader, stdout, stderr io.Writer) (err error) {
	return StickyFireWithLanguage(root, event, cadence, StickyLanguage{}, stdin, stdout, stderr)
}

// StickyFireWithLanguage is StickyFire that also attaches the configured
// language values to the language-policy rule. It keeps the same exit-zero
// contract.
func StickyFireWithLanguage(
	root, event string, cadence int, lang StickyLanguage, stdin io.Reader, stdout, stderr io.Writer,
) (err error) {
	defer func() {
		// REQ-STICKYRULE-FIRE-11: suppress the whole defect class, including a
		// panic, and leave no error behind. Partial stdout cannot leak because
		// the payload is buffered and written once, at the end of stickyRun.
		_ = recover()
		err = nil
	}()

	stickyRun(root, event, cadence, lang, stdin, stdout, stderr)
	return nil
}

// stickyRun is the ordered pipeline StickyFire wraps in its exit barrier. Each
// early return is a whole-run benign case: no stdout, no stderr, no error.
func stickyRun(root, event string, cadence int, lang StickyLanguage, stdin io.Reader, stdout, stderr io.Writer) {
	if strings.TrimSpace(event) == "" {
		event = EventUserPromptSubmit
	}

	// The manifest is read before the counter is touched, so a project with no
	// sticky rules never creates state for a session that can never inject.
	sticky, rejected, ok := loadStickySet(root)
	if !ok {
		return
	}

	payload, decodeErr := decodeStickyPayload(stdin)
	if decodeErr != nil {
		return
	}
	index, ok := nextPromptIndex(root, payload.SessionID)
	if !ok {
		return
	}
	if !stickyDue(index, ResolveStickyCadence(cadence)) {
		return
	}

	// Diagnostics start only once this invocation is actually due. Reporting a
	// forged manifest name on every prompt would turn one bad entry into a
	// per-turn stderr stream.
	reportRejectedNames(rejected, stderr)

	entries := readStickyBodies(root, sticky, stderr)
	if len(entries) == 0 {
		return
	}
	entries = attachLanguageValues(entries, lang)

	var payloadBuf bytes.Buffer
	if writeOutput(&payloadBuf, event, assembleStickyContext(entries)) != nil {
		return
	}
	_, _ = stdout.Write(payloadBuf.Bytes())
}

// loadStickySet reads the sticky array of the manifest at its fixed
// project-relative location. A refused root, an unreadable or malformed
// manifest, and an empty sticky set are all whole-run benign.
//
// REQ-STICKYRULE-FIRE-09: the location comes from ResolveManifestPath and from
// nowhere else. No environment variable, command flag, or stdin field may
// redirect it.
func loadStickySet(root string) (rules []StickyRule, rejected []string, ok bool) {
	manifestPath, err := ResolveManifestPath(root)
	if err != nil {
		return nil, nil, false
	}
	manifest, _, err := loadManifest(manifestPath)
	if err != nil {
		return nil, nil, false
	}
	rules, rejected = filterStickyRules(manifest.Sticky)
	if len(rules) == 0 && len(rejected) == 0 {
		return nil, nil, false
	}
	return rules, rejected, true
}

// stickyDue reports whether a prompt index is an injection point: index 1, or
// any index whose distance from 1 is an exact multiple of the cadence
// (REQ-STICKYRULE-FIRE-01).
func stickyDue(index, cadence int) bool {
	if index <= 0 || cadence <= 0 {
		return false
	}
	return (index-1)%cadence == 0
}
