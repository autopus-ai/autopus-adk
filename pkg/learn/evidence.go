package learn

// EvidenceKind says what backs a lesson (SPEC-LEARN-003). It is derived from
// the recorded exit codes, never supplied by the caller.
type EvidenceKind string

const (
	// EvidenceDeterministic: a command failed before the change and passed
	// after it, the execution contrast a lesson needs to be trusted.
	EvidenceDeterministic EvidenceKind = "deterministic"
	// EvidenceLLMOnly: evidence was offered but shows no such contrast, so the
	// lesson rests on the model's judgment.
	EvidenceLLMOnly EvidenceKind = "llm_only"
)

// Lesson scopes. An entry without a scope is a project lesson.
const (
	// ScopeProcess is an agent-process lesson that may hold in any project,
	// such as reopening produced files to verify them.
	ScopeProcess = "process"
	// ScopeProject is a lesson about this repository's own domain.
	ScopeProject = "project"
)

// Evidence is the execution record behind a lesson. Command is data only:
// nothing in the harness runs it.
type Evidence struct {
	Kind       EvidenceKind `json:"kind"`
	Command    string       `json:"command,omitempty"`
	BeforeExit *int         `json:"before_exit,omitempty"`
	AfterExit  *int         `json:"after_exit,omitempty"`
}

// evidenceFromOpts derives the evidence object. No evidence input gives nil,
// which leaves the entry unspecified like every entry written before.
func evidenceFromOpts(opts RecordOpts) *Evidence {
	if opts.EvidenceCommand == "" && opts.EvidenceBeforeExit == nil && opts.EvidenceAfterExit == nil {
		return nil
	}
	kind := EvidenceLLMOnly
	if opts.EvidenceCommand != "" &&
		opts.EvidenceBeforeExit != nil && *opts.EvidenceBeforeExit != 0 &&
		opts.EvidenceAfterExit != nil && *opts.EvidenceAfterExit == 0 {
		kind = EvidenceDeterministic
	}
	return &Evidence{
		Kind:       kind,
		Command:    opts.EvidenceCommand,
		BeforeExit: copyInt(opts.EvidenceBeforeExit),
		AfterExit:  copyInt(opts.EvidenceAfterExit),
	}
}

func copyInt(v *int) *int {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

// EffectiveScope returns the entry's scope, project when none was recorded.
func (le LearningEntry) EffectiveScope() string {
	if le.Scope == "" {
		return ScopeProject
	}
	return le.Scope
}

// EvidenceKindOf returns the entry's evidence kind, "" when unspecified.
func (le LearningEntry) EvidenceKindOf() EvidenceKind {
	if le.Evidence == nil {
		return ""
	}
	return le.Evidence.Kind
}

// validScope reports a scope the store accepts; empty means project.
func validScope(scope string) bool {
	return scope == "" || scope == ScopeProcess || scope == ScopeProject
}
