package editguard

import (
	"errors"
	"io"
	"path"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

// MaxPayloadBytes bounds the hook payload read from stdin (REQ-EG-02).
const MaxPayloadBytes = 1 << 20

// Class is the deny class of a decision.
type Class string

// The three deny classes, in precedence order (spec.md Decision Output
// Contract). Every other target is allowed.
const (
	ClassGuardState       Class = "guard_state"
	ClassFixLock          Class = "fix_lock"
	ClassGeneratedSurface Class = "generated_surface"
)

// Call is one decoded file-editing tool call.
type Call struct {
	// Cwd is the payload cwd; relative targets resolve against it.
	Cwd string
	// Targets are the call's target paths in payload order.
	Targets []string
	// Dropped counts malformed target entries the dialect skipped.
	Dropped int
}

// Decision is the guard's answer for one call. An allow carries no reason; a
// Diagnostic is the one stderr line an allow may write about a dropped part.
type Decision struct {
	Deny       bool
	Class      Class
	Reason     string
	Diagnostic string
}

// Options configures a decision.
type Options struct {
	// Now is the clock that decides lock expiry; nil means time.Now.
	Now func() time.Time
	// panicSeam is the test-only fault seam inside the decision (S7).
	panicSeam func()
}

// Dialect decodes one platform's hook payload and encodes its deny. The
// platform comes from the registered command line, never from the payload.
type Dialect interface {
	Decode(payload []byte) (Call, error)
	EncodeDeny(Decision) ([]byte, error)
}

// Run is one `auto guard edit` invocation. It always returns exit status 0:
// a deny is the dialect's bytes in one stdout write, an allow and every fault
// leave stdout empty, and stderr gets at most one diagnostic line. Nothing
// is written until the decision is complete, so a fault mid-way cannot leave
// partial deny bytes behind (REQ-EG-10, REQ-EG-11).
func Run(stdin io.Reader, stdout, stderr io.Writer, dialect Dialect, opts Options) int {
	out, diagnostic := guard(stdin, dialect, opts)
	if diagnostic != "" {
		_, _ = io.WriteString(stderr, diagnostic+"\n")
	}
	if len(out) > 0 {
		_, _ = stdout.Write(out)
	}
	return 0
}

// guard is the recover barrier: a panic anywhere in decoding, deciding, or
// encoding is a call-level fault that allows the call.
func guard(stdin io.Reader, dialect Dialect, opts Options) (out []byte, diagnostic string) {
	defer func() {
		if recover() != nil {
			out, diagnostic = nil, allowBecause("internal error")
		}
	}()
	payload, fault := readPayload(stdin)
	if fault != "" {
		return nil, allowBecause(fault)
	}
	call, err := dialect.Decode(payload)
	if err != nil {
		return nil, allowBecause("payload malformed")
	}
	decision := Decide(call, opts)
	if !decision.Deny {
		return nil, decision.Diagnostic
	}
	out, err = dialect.EncodeDeny(decision)
	if err != nil || len(out) == 0 {
		return nil, allowBecause("deny not encodable")
	}
	return out, ""
}

func readPayload(stdin io.Reader) ([]byte, string) {
	data, err := io.ReadAll(io.LimitReader(stdin, MaxPayloadBytes+1))
	switch {
	case err != nil:
		return nil, "payload unreadable"
	case len(data) == 0:
		return nil, "empty payload"
	case len(data) > MaxPayloadBytes:
		return nil, "payload over 1 MiB"
	}
	return data, ""
}

func allowBecause(fault string) string {
	return "autopus edit-guard: allow (" + fault + ")"
}

// Decide evaluates every target in payload order and returns the first deny.
// Per target the stages run guard state, lock, manifest. A fault drops exactly
// the untrustworthy part and is reported only when the call is allowed
// (REQ-EG-18).
func Decide(call Call, opts Options) Decision {
	if len(call.Targets) == 0 {
		return Decision{Diagnostic: allowBecause("no target path")}
	}
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	ev := evaluation{now: now(), roots: map[string]*rootStages{}}
	if call.Dropped > 0 {
		ev.note("malformed target dropped")
	}
	for _, raw := range call.Targets {
		if opts.panicSeam != nil {
			opts.panicSeam()
		}
		if decision, deny := ev.target(call.Cwd, raw); deny {
			return decision
		}
	}
	if ev.fault == "" {
		return Decision{}
	}
	return Decision{Diagnostic: allowBecause(ev.fault)}
}

type evaluation struct {
	now   time.Time
	roots map[string]*rootStages
	fault string // the first dropped part
}

func (ev *evaluation) note(fault string) {
	if ev.fault == "" {
		ev.fault = fault
	}
}

func (ev *evaluation) target(cwd, raw string) (Decision, bool) {
	target, err := Resolve(cwd, raw)
	if err != nil {
		// No project root is normal: nothing there is protected.
		if !errors.Is(err, ErrNoProjectRoot) {
			ev.note("target unresolvable")
		}
		return Decision{}, false
	}
	if isGuardState(target.Key) {
		return deny(ClassGuardState, gstReason(target.Rel)), true
	}
	stages := ev.stagesOf(target)
	if recorded, ok := stages.lockView(ev).match(target); ok {
		return deny(ClassFixLock, flReason(recorded)), true
	}
	if !workflow.InEditGuardNamespace(target.Key) {
		return Decision{}, false
	}
	manifests := stages.manifestStage()
	if manifests.fault != "" {
		ev.note("manifest unreadable: " + displayPath(manifests.fault))
		return Decision{}, false
	}
	if hit, ok := manifests.generated(target.Key); ok {
		return deny(ClassGeneratedSurface, gsReason(hit.display, hit.manifest, stages.sourceRepo())), true
	}
	return Decision{}, false
}

func deny(class Class, reason string) Decision {
	return Decision{Deny: true, Class: class, Reason: reason}
}

// isGuardState reports the lock store and the root manifests (REQ-EG-09).
func isGuardState(key string) bool {
	if key == FixLocksDir || strings.HasPrefix(key, FixLocksDir+"/") {
		return true
	}
	return path.Dir(key) == manifestDir && strings.HasSuffix(key, manifestSuffix)
}

// rootStages caches the stages of one project root; each loads on first use,
// so manifests are read only for namespace targets (REQ-EG-24).
type rootStages struct {
	root      string
	fold      bool
	locks     *lockView
	manifests *manifestStage
	source    *bool
}

func (ev *evaluation) stagesOf(target Target) *rootStages {
	stages, ok := ev.roots[target.Root]
	if !ok {
		stages = &rootStages{root: target.Root, fold: target.CaseInsensitive}
		ev.roots[target.Root] = stages
	}
	return stages
}

func (s *rootStages) lockView(ev *evaluation) *lockView {
	if s.locks == nil {
		view := loadLockView(s.root, s.fold, ev.now)
		s.locks = &view
		ev.note(view.fault)
	}
	return s.locks
}

func (s *rootStages) manifestStage() *manifestStage {
	if s.manifests == nil {
		stage := loadManifestStage(s.root, s.fold)
		s.manifests = &stage
	}
	return s.manifests
}

func (s *rootStages) sourceRepo() bool {
	if s.source == nil {
		source := isSourceRepo(s.root)
		s.source = &source
	}
	return *s.source
}
