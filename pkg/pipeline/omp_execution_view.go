package pipeline

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// OMPExecutionViewInput contains the trusted authority for one OMP phase attempt.
type OMPExecutionViewInput struct {
	ProjectDir       string
	SpecID           string
	SpecDir          string
	SnapshotHash     string
	GitCommitHash    string
	PhaseID          PhaseID
	Attempt          int
	Prompt           string
	ActivePrompt     string
	CompletedHistory []string
}

// OMPExecutionViewBinding identifies the exact phase attempt allowed to open a view.
type OMPExecutionViewBinding struct {
	SpecID        string
	SnapshotHash  string
	GitCommitHash string
	PhaseID       PhaseID
	Attempt       int
}

// OMPExecutionSnapshot is a defensive copy of the authority sealed in a view.
type OMPExecutionSnapshot struct {
	ProjectDir       string
	SpecID           string
	SpecDir          string
	SnapshotHash     string
	GitCommitHash    string
	PhaseID          PhaseID
	Attempt          int
	Prompt           string
	ActivePrompt     string
	CompletedHistory []string
}

// MarshalJSON prevents opened prompt and history bodies from entering JSON receipts.
func (OMPExecutionSnapshot) MarshalJSON() ([]byte, error) {
	return nil, errors.New("pipeline: sealed OMP execution snapshot cannot be marshaled")
}

// UnmarshalJSON prevents execution authority from entering through serialized input.
func (*OMPExecutionSnapshot) UnmarshalJSON([]byte) error {
	return errors.New("pipeline: sealed OMP execution snapshot cannot be unmarshaled")
}

// OMPExecutionView keeps prompt bodies in process memory and refuses serialization.
type OMPExecutionView struct {
	snapshot OMPExecutionSnapshot
	binding  OMPExecutionViewBinding
	sealed   bool
}

// NewOMPExecutionView validates and seals authority for one OMP phase attempt.
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-OMP-004: constructor seals prompt bodies and phase authority in process memory.
// @AX:REASON [AUTO]: The engine and OMP backend depend on this view remaining non-serializable and attempt-bound.
func NewOMPExecutionView(input OMPExecutionViewInput) (*OMPExecutionView, error) {
	if err := validateOMPExecutionViewInput(input); err != nil {
		return nil, err
	}
	snapshot := OMPExecutionSnapshot{
		ProjectDir: input.ProjectDir, SpecID: input.SpecID, SpecDir: input.SpecDir,
		SnapshotHash: input.SnapshotHash, GitCommitHash: input.GitCommitHash,
		PhaseID: input.PhaseID, Attempt: input.Attempt, Prompt: input.Prompt,
		ActivePrompt:     input.ActivePrompt,
		CompletedHistory: cloneOMPHistory(input.CompletedHistory),
	}
	return &OMPExecutionView{
		snapshot: snapshot,
		binding: OMPExecutionViewBinding{
			SpecID: input.SpecID, SnapshotHash: input.SnapshotHash,
			GitCommitHash: input.GitCommitHash, PhaseID: input.PhaseID, Attempt: input.Attempt,
		},
		sealed: true,
	}, nil
}

// Binding returns the body-free identity needed to open the sealed view.
func (v *OMPExecutionView) Binding() (OMPExecutionViewBinding, error) {
	if v == nil || !v.sealed {
		return OMPExecutionViewBinding{}, errors.New("pipeline: sealed OMP execution view is unavailable")
	}
	return v.binding, nil
}

// Open returns a defensive snapshot only for the exact sealed phase binding.
func (v *OMPExecutionView) Open(binding OMPExecutionViewBinding) (OMPExecutionSnapshot, error) {
	if v == nil || !v.sealed {
		return OMPExecutionSnapshot{}, errors.New("pipeline: sealed OMP execution view is unavailable")
	}
	if err := validateOMPExecutionViewBinding(binding); err != nil {
		return OMPExecutionSnapshot{}, fmt.Errorf("pipeline: invalid OMP execution view binding: %w", err)
	}
	if binding != v.binding {
		return OMPExecutionSnapshot{}, errors.New("pipeline: OMP execution view binding mismatch")
	}
	snapshot := v.snapshot
	snapshot.CompletedHistory = cloneOMPHistory(v.snapshot.CompletedHistory)
	return snapshot, nil
}

// MarshalJSON prevents prompt and history bodies from entering JSON receipts.
func (OMPExecutionView) MarshalJSON() ([]byte, error) {
	return nil, errors.New("pipeline: sealed OMP execution view cannot be marshaled")
}

// UnmarshalJSON prevents reconstructed or caller-forged execution authority.
func (*OMPExecutionView) UnmarshalJSON([]byte) error {
	return errors.New("pipeline: sealed OMP execution view cannot be unmarshaled")
}

func (e *SubprocessEngine) newPhaseRequest(
	state *engineRunState,
	phase Phase,
	attempt int,
	prompt string,
) (PhaseRequest, error) {
	request := PhaseRequest{Prompt: prompt, PhaseID: phase.ID, Attempt: attempt}
	if e.cfg.Platform != "omp" {
		return request, nil
	}
	activePrompt, err := e.buildOMPActivePhasePrompt(phase, state.feedback[phase.ID])
	if err != nil {
		return PhaseRequest{}, fmt.Errorf("build OMP active phase prompt: %w", err)
	}
	view, err := NewOMPExecutionView(OMPExecutionViewInput{
		ProjectDir: e.cfg.ProjectDir, SpecID: e.cfg.SpecID, SpecDir: e.cfg.SpecDir,
		SnapshotHash: e.cfg.SnapshotHash, GitCommitHash: e.cfg.GitCommitHash,
		PhaseID: phase.ID, Attempt: attempt, Prompt: prompt, ActivePrompt: activePrompt,
		CompletedHistory: completedOMPHistory(state.phases, phase.ID, state.previous),
	})
	if err != nil {
		return PhaseRequest{}, fmt.Errorf("seal OMP execution view: %w", err)
	}
	request.OMPExecutionView = view
	return request, nil
}

func completedOMPHistory(phases []Phase, current PhaseID, previous map[PhaseID]string) []string {
	history := make([]string, 0, len(previous))
	for _, phase := range phases {
		if phase.ID == current {
			break
		}
		if output, ok := previous[phase.ID]; ok {
			history = append(history, output)
		}
	}
	return history
}

// @AX:WARN [AUTO]: Sealed execution input validation has cyclomatic complexity 15.
// @AX:REASON [AUTO]: Project, SPEC, snapshot, commit, phase, prompt, and completed-history authority must be admitted together.
func validateOMPExecutionViewInput(input OMPExecutionViewInput) error {
	if err := validateOMPPath("project directory", input.ProjectDir, false); err != nil {
		return err
	}
	if err := ValidateSpecID(input.SpecID); err != nil {
		return fmt.Errorf("OMP execution view: %w", err)
	}
	if err := validateOMPPath("SPEC directory", input.SpecDir, true); err != nil {
		return err
	}
	if filepath.Base(input.SpecDir) != input.SpecID {
		return errors.New("OMP execution view: SPEC directory is not bound to the SPEC ID")
	}
	if err := validateOMPSnapshotHash(input.SnapshotHash); err != nil {
		return err
	}
	if !isLowerHex(input.GitCommitHash, 40) && !isLowerHex(input.GitCommitHash, 64) {
		return errors.New("OMP execution view: git commit hash must be 40 or 64 lowercase hex characters")
	}
	if !isPipelinePhase(input.PhaseID) {
		return fmt.Errorf("OMP execution view: invalid phase %q", input.PhaseID)
	}
	if input.Attempt < 1 {
		return errors.New("OMP execution view: attempt must be positive")
	}
	if strings.TrimSpace(input.Prompt) == "" || strings.IndexByte(input.Prompt, 0) >= 0 {
		return errors.New("OMP execution view: prompt is required and must not contain NUL")
	}
	if strings.TrimSpace(input.ActivePrompt) == "" || strings.IndexByte(input.ActivePrompt, 0) >= 0 {
		return errors.New("OMP execution view: active prompt is required and must not contain NUL")
	}
	if strings.HasPrefix(strings.TrimSpace(input.ActivePrompt), "/auto") {
		return errors.New("OMP execution view: active prompt must not reissue /auto")
	}
	for i, output := range input.CompletedHistory {
		if strings.TrimSpace(output) == "" || strings.IndexByte(output, 0) >= 0 {
			return fmt.Errorf("OMP execution view: completed history row %d is invalid", i)
		}
	}
	return nil
}

func validateOMPExecutionViewBinding(binding OMPExecutionViewBinding) error {
	if err := ValidateSpecID(binding.SpecID); err != nil {
		return err
	}
	if err := validateOMPSnapshotHash(binding.SnapshotHash); err != nil {
		return err
	}
	if !isLowerHex(binding.GitCommitHash, 40) && !isLowerHex(binding.GitCommitHash, 64) {
		return errors.New("invalid git commit hash")
	}
	if !isPipelinePhase(binding.PhaseID) || binding.Attempt < 1 {
		return errors.New("invalid phase attempt")
	}
	return nil
}

func validateOMPPath(name, path string, relativeSafe bool) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(path) != path || strings.IndexByte(path, 0) >= 0 {
		return fmt.Errorf("OMP execution view: %s is invalid", name)
	}
	if filepath.Clean(path) != path {
		return fmt.Errorf("OMP execution view: %s must be clean", name)
	}
	if relativeSafe && !filepath.IsAbs(path) && (path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))) {
		return fmt.Errorf("OMP execution view: %s escapes the project", name)
	}
	return nil
}

func validateOMPSnapshotHash(hash string) error {
	const prefix = "sha256:"
	if !strings.HasPrefix(hash, prefix) || !isLowerHex(strings.TrimPrefix(hash, prefix), 64) {
		return errors.New("OMP execution view: snapshot hash must be sha256 lowercase hex")
	}
	return nil
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func isPipelinePhase(phaseID PhaseID) bool {
	switch phaseID {
	case PhasePlan, PhaseTestScaffold, PhaseImplement, PhaseValidate, PhaseReview:
		return true
	default:
		return false
	}
}

func cloneOMPHistory(history []string) []string {
	cloned := make([]string, len(history))
	copy(cloned, history)
	return cloned
}
