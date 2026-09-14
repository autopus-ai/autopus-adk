// Package pipeline provides pipeline state management types and persistence.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const checkpointFilename = ".autopus-checkpoint.yaml"

// CheckpointVersion identifies the canonical pipeline checkpoint schema.
const CheckpointVersion = "pipeline_checkpoint.v1"

// Save writes the checkpoint as YAML to {dir}/.autopus-checkpoint.yaml.
func (c *Checkpoint) Save(dir string) error {
	return c.SaveFile(filepath.Join(dir, checkpointFilename))
}

// SaveFile atomically writes the checkpoint to an explicit path.
func (c *Checkpoint) SaveFile(path string) error {
	data, err := c.MarshalYAML()
	if err != nil {
		return fmt.Errorf("checkpoint: marshal: %w", err)
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".autopus-checkpoint-*")
	if err != nil {
		return fmt.Errorf("checkpoint: create temp in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("checkpoint: chmod %s: %w", tmpPath, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("checkpoint: write %s: %w", tmpPath, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("checkpoint: sync %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("checkpoint: close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("checkpoint: rename %s: %w", path, err)
	}
	return nil
}

// Load reads the checkpoint file from {dir}/.autopus-checkpoint.yaml.
// Returns an error if the file does not exist.
func Load(dir string) (*Checkpoint, error) {
	return LoadFile(filepath.Join(dir, checkpointFilename))
}

// LoadFile reads a checkpoint from an explicit path.
func LoadFile(path string) (*Checkpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("checkpoint: file not found: %s: %w", path, err)
		}
		return nil, fmt.Errorf("checkpoint: read %s: %w", path, err)
	}

	var cp Checkpoint
	if err := yaml.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("checkpoint: unmarshal: %w", err)
	}
	return &cp, nil
}

// LoadWithHash loads the checkpoint and sets Stale=true when the saved
// GitCommitHash differs from currentHash.
func LoadWithHash(dir string, currentHash string) (*Checkpoint, error) {
	cp, err := Load(dir)
	if err != nil {
		return nil, err
	}
	if cp.GitCommitHash != currentHash {
		cp.Stale = true
	}
	return cp, nil
}

// ValidateResume rejects stale, incomplete, mismatched, or dependency-invalid checkpoints.
func (c *Checkpoint) ValidateResume(specID, routeVersion, snapshotHash string) error {
	if c == nil {
		return nil
	}
	if c.Stale {
		return fmt.Errorf("checkpoint is stale: saved git hash %s", c.GitCommitHash)
	}
	if c.Version == "" || c.SpecID == "" || c.RouteVersion == "" || c.SnapshotHash == "" ||
		specID == "" || routeVersion == "" || snapshotHash == "" {
		return fmt.Errorf("checkpoint identity is incomplete")
	}
	if c.Version != CheckpointVersion {
		return fmt.Errorf("checkpoint identity version mismatch: saved %s, required %s", c.Version, CheckpointVersion)
	}
	if c.SpecID != specID {
		return fmt.Errorf("checkpoint SPEC mismatch: saved %s, requested %s", c.SpecID, specID)
	}
	if c.RouteVersion != routeVersion {
		return fmt.Errorf("checkpoint route mismatch: saved %s, requested %s", c.RouteVersion, routeVersion)
	}
	if c.SnapshotHash != snapshotHash {
		return fmt.Errorf("checkpoint snapshot mismatch: saved %s, current %s", c.SnapshotHash, snapshotHash)
	}
	return c.validateDependencyClosedStatuses()
}

func (c *Checkpoint) validateDependencyClosedStatuses() error {
	known := make(map[PhaseID]struct{}, len(DefaultPhases()))
	for _, phase := range DefaultPhases() {
		known[phase.ID] = struct{}{}
	}
	for rawID, status := range c.TaskStatus {
		if _, ok := known[PhaseID(rawID)]; !ok {
			return fmt.Errorf("checkpoint contains unknown phase %s", rawID)
		}
		if !validCheckpointStatus(status) {
			return fmt.Errorf("checkpoint phase %s has unknown status %s", rawID, status)
		}
	}
	for _, phase := range c.routePhases() {
		if c.TaskStatus[string(phase.ID)] != CheckpointStatusDone {
			continue
		}
		for _, dependency := range phase.DependsOn {
			if c.TaskStatus[string(dependency)] != CheckpointStatusDone {
				return fmt.Errorf("checkpoint phase %s is done but dependency %s is not done", phase.ID, dependency)
			}
		}
	}
	return nil
}

// SavedRoute is the route the checkpoint recorded. A checkpoint whose receipt
// names no route is read as the full route: an absent record is not evidence
// that a shorter route was ever authorized.
func (c *Checkpoint) SavedRoute() PhaseRoute {
	if c == nil || c.Receipt == nil {
		return RouteFull
	}
	return NormalizeRoute(c.Receipt.RoutePhaseSet)
}

// ValidateResumeRoute rejects resuming a checkpoint under a route other than
// the one it ran. Resuming a full run on the compact route would silently
// drop phases that were still owed; resuming a compact run on the full route
// would dispatch phases the run never planned. Either way the operator has
// to decide, not the resume path.
func (c *Checkpoint) ValidateResumeRoute(authorized PhaseRoute) error {
	if c == nil {
		return nil
	}
	if saved := c.SavedRoute(); saved != NormalizeRoute(authorized) {
		return fmt.Errorf("checkpoint phase route mismatch: saved %s, authorized %s",
			saved, NormalizeRoute(authorized))
	}
	return nil
}

// routePhases returns the phases whose dependencies this checkpoint must
// close. Only a receipt that names the compact route shortens that set.
func (c *Checkpoint) routePhases() []Phase {
	return PhasesForRoute(c.SavedRoute())
}

func validCheckpointStatus(status CheckpointStatus) bool {
	switch status {
	case CheckpointStatusPending, CheckpointStatusInProgress, CheckpointStatusDone,
		CheckpointStatusFailed, CheckpointStatusSkipped, CheckpointStatusCancelled:
		return true
	default:
		return false
	}
}
