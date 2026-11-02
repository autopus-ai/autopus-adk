// Package pipeline provides pipeline state management types and persistence.
package pipeline

import "gopkg.in/yaml.v3"

// @AX:NOTE [AUTO] @AX:REASON: checkpoint state constants must match CLI output parsing
// CheckpointStatus represents the execution status of a pipeline task.
type CheckpointStatus string

const (
	// CheckpointStatusPending indicates the task has not started.
	CheckpointStatusPending CheckpointStatus = "pending"
	// CheckpointStatusInProgress indicates the task is currently running.
	CheckpointStatusInProgress CheckpointStatus = "in_progress"
	// CheckpointStatusDone indicates the task completed successfully.
	CheckpointStatusDone CheckpointStatus = "done"
	// CheckpointStatusFailed indicates the task failed.
	CheckpointStatusFailed CheckpointStatus = "failed"
	// CheckpointStatusSkipped indicates the task was intentionally not dispatched.
	CheckpointStatusSkipped CheckpointStatus = "skipped"
	// CheckpointStatusCancelled indicates the task was cancelled before completion.
	CheckpointStatusCancelled CheckpointStatus = "cancelled"
)

// String returns the canonical string representation of a CheckpointStatus.
func (s CheckpointStatus) String() string {
	return string(s)
}

// Checkpoint holds the persisted state of a pipeline execution.
type Checkpoint struct {
	Version       string                      `yaml:"version,omitempty"`
	RouteVersion  string                      `yaml:"route_version,omitempty"`
	SpecID        string                      `yaml:"spec_id,omitempty"`
	SnapshotHash  string                      `yaml:"snapshot_hash,omitempty"`
	Phase         string                      `yaml:"phase"`
	GitCommitHash string                      `yaml:"git_commit_hash"`
	TaskStatus    map[string]CheckpointStatus `yaml:"task_status"`
	Receipt       *OrchestrationRunReceipt    `yaml:"receipt,omitempty"`
	// PhaseOutputs are the completed phases' output records in route order.
	PhaseOutputs []PhaseOutputRecord `yaml:"phase_outputs,omitempty"`
	// Stale is set to true when the saved hash differs from the current HEAD.
	// It is not persisted to disk.
	Stale bool `yaml:"-"`
}

// checkpointYAML is the on-disk representation used for marshalling.
type checkpointYAML struct {
	Version       string                   `yaml:"version,omitempty"`
	RouteVersion  string                   `yaml:"route_version,omitempty"`
	SpecID        string                   `yaml:"spec_id,omitempty"`
	SnapshotHash  string                   `yaml:"snapshot_hash,omitempty"`
	Phase         string                   `yaml:"phase"`
	GitCommitHash string                   `yaml:"git_commit_hash"`
	TaskStatus    map[string]string        `yaml:"task_status"`
	Receipt       *OrchestrationRunReceipt `yaml:"receipt,omitempty"`
	PhaseOutputs  []PhaseOutputRecord      `yaml:"phase_outputs,omitempty"`
}

// MarshalYAML serialises the Checkpoint to YAML bytes.
func (c *Checkpoint) MarshalYAML() ([]byte, error) {
	raw := checkpointYAML{
		Version:       c.Version,
		RouteVersion:  c.RouteVersion,
		SpecID:        c.SpecID,
		SnapshotHash:  c.SnapshotHash,
		Phase:         c.Phase,
		GitCommitHash: c.GitCommitHash,
		TaskStatus:    make(map[string]string, len(c.TaskStatus)),
		Receipt:       c.Receipt,
		PhaseOutputs:  c.PhaseOutputs,
	}
	for k, v := range c.TaskStatus {
		raw.TaskStatus[k] = string(v)
	}
	return yaml.Marshal(raw)
}

// UnmarshalYAML deserialises YAML bytes into the Checkpoint.
func (c *Checkpoint) UnmarshalYAML(data []byte) error {
	var raw checkpointYAML
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Phase = raw.Phase
	c.Version = raw.Version
	c.RouteVersion = raw.RouteVersion
	c.SpecID = raw.SpecID
	c.SnapshotHash = raw.SnapshotHash
	c.GitCommitHash = raw.GitCommitHash
	c.Receipt = raw.Receipt
	c.PhaseOutputs = raw.PhaseOutputs
	if raw.TaskStatus != nil {
		c.TaskStatus = make(map[string]CheckpointStatus, len(raw.TaskStatus))
		for k, v := range raw.TaskStatus {
			c.TaskStatus[k] = CheckpointStatus(v)
		}
	}
	return nil
}
