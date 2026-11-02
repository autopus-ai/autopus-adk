package pipeline

// ResumeAdmission is the engine's decision on a checkpoint before any phase
// is dispatched: what `auto pipeline run --continue` would admit, restore,
// and dispatch again (SPEC-STATUSEVIDENCE-001 REQ-SEV-EXEC-02).
type ResumeAdmission struct {
	// Err is why the engine refuses the resume; nil when it admits it.
	Err error
	// Restored are the completed phases whose verified outputs a resume
	// takes over; Rerun are completed phases it dispatches again.
	Restored []PhaseID
	Rerun    []PhaseID
}

// AdmitResume runs the resume admission the engine runs at the start of a
// resumed run — identity, version, snapshot, dependency closure, route, and
// output-record verification — without dispatching, persisting, or reading
// anything beyond cp. gitCommitHash is the revision the resumed run binds its
// outputs to; the CLI refuses a checkpoint from another revision before the
// engine is reached, which the caller checks.
func AdmitResume(cp *Checkpoint, specID, snapshotHash, gitCommitHash string, route PhaseRoute) ResumeAdmission {
	engine := &SubprocessEngine{cfg: EngineConfig{
		SpecID: specID, SnapshotHash: snapshotHash, GitCommitHash: gitCommitHash,
		Route: route, Checkpoint: cp,
	}}
	plan, err := engine.planResume()
	admission := ResumeAdmission{Err: err}
	if plan != nil {
		admission.Restored = append([]PhaseID(nil), plan.resume.RestoredPhases...)
		admission.Rerun = append([]PhaseID(nil), plan.resume.RerunPhases...)
	}
	return admission
}
