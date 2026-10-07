package harneval

import (
	"regexp"
	"slices"
	"sort"
	"time"
)

// sessionIDPattern is a 128-bit session id in lowercase hex.
var sessionIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// signalOutcomes is the closed REQ-HE-08 signal table: a record's signal fixes
// its outcome. Only a failure before the arm surface enters the trial
// workspace is an error, so a candidate surface cannot remove its own failures
// from the denominator, and only an accepted oracle result is a pass.
var signalOutcomes = map[string]string{
	"workspace_setup_failed": OutcomeError,
	"mutation_failed":        OutcomeError,
	"warmup_failed":          OutcomeError,
	"agent_launch_failed":    OutcomeFail,
	"agent_exit_nonzero":     OutcomeFail,
	"agent_timeout":          OutcomeFail,
	"forbidden_construct":    OutcomeFail,
	"oracle_failed":          OutcomeFail,
	"oracle_timeout":         OutcomeFail,
	"oracle_output_invalid":  OutcomeFail,
	"scope_violation":        OutcomeFail,
	"observation_failed":     OutcomeFail,
	"accepted":               OutcomePass,
}

func validateProtocol(p *Protocol) error {
	if p.SchemaVersion != "" && p.SchemaVersion != ProtocolSchemaV1 {
		return invalidf(DetailFieldInvalid, "protocol schema_version %q is not %s", p.SchemaVersion, ProtocolSchemaV1)
	}
	if !sessionIDPattern.MatchString(p.SessionID) {
		return invalidf(DetailFieldInvalid, "session_id %q is not 128-bit lowercase hex", p.SessionID)
	}
	if _, err := time.Parse(time.RFC3339, p.StartedAt); err != nil {
		return invalidf(DetailFieldInvalid, "started_at %q is not an RFC 3339 time", p.StartedAt)
	}
	for _, digest := range []struct{ name, value string }{
		{"baseline_surface_digest", p.BaselineSurfaceDigest},
		{"candidate_surface_digest", p.CandidateSurfaceDigest},
		{"agent_set_digest", p.AgentSetDigest},
		{"runner_sha256", p.RunnerSHA256},
		{"grader_profile_sha256", p.GraderProfileSHA256},
	} {
		if !sha256Pattern.MatchString(digest.value) {
			return invalidf(DetailFieldInvalid, "%s %q is not a 64-hex digest", digest.name, digest.value)
		}
	}
	if len(p.CorpusDigests) == 0 || len(p.PromptLayers) == 0 {
		return invalidf(DetailFieldInvalid, "protocol needs corpus_digests and prompt_layers")
	}
	if blank(p.CLIVersion) {
		return invalidf(DetailFieldInvalid, "protocol needs a cli_version")
	}
	if err := completePolicy(p); err != nil {
		return err
	}
	if err := validatePins(p.Pins); err != nil {
		return err
	}
	if err := validateOrder(p.Order, p.Policy.K); err != nil {
		return err
	}
	if err := validatePhase("protocol calibration", p.Calibration); err != nil {
		return err
	}
	if p.Calibration.Status == CalibrationPassed && !phaseCovers(p.Calibration, p.Order) {
		return invalidf(DetailFieldInvalid, "protocol calibration passed without exactly the scheduled tasks")
	}
	return nil
}

// completePolicy fills the policy's input identifiers from the protocol's top
// level, where the contract places them; a policy naming other values is
// invalid. The completed policy must meet the manifest live-policy rules.
func completePolicy(p *Protocol) error {
	for _, field := range []struct {
		name  string
		inner *string
		outer string
	}{
		{"workspace_revision", &p.Policy.WorkspaceRevision, p.WorkspaceRevision},
		{"baseline_ref", &p.Policy.BaselineRef, p.BaselineRef},
		{"model", &p.Policy.Model, p.Model},
	} {
		switch *field.inner {
		case "":
			*field.inner = field.outer
		case field.outer:
		default:
			return invalidf(DetailFieldInvalid, "policy %s %q differs from the protocol's %q", field.name, *field.inner, field.outer)
		}
	}
	return validateLivePolicy(p.Policy)
}

// validateOrder requires the order to schedule every (task, arm, trial) of its
// tasks × both arms × k trials exactly once.
func validateOrder(order []Attempt, k int) error {
	seen := make(map[Attempt]bool, len(order))
	tasks := map[string]bool{}
	for _, attempt := range order {
		if !taskIDPattern.MatchString(attempt.TaskID) || (attempt.Arm != ArmBaseline && attempt.Arm != ArmCandidate) ||
			attempt.Trial < 0 || attempt.Trial >= k || seen[attempt] {
			return invalidf(DetailFieldInvalid, "order attempt %s is malformed, outside k=%d, or repeated", attempt, k)
		}
		seen[attempt] = true
		tasks[attempt.TaskID] = true
	}
	if len(order) == 0 || len(order) != 2*len(tasks)*k {
		return invalidf(DetailFieldInvalid, "order holds %d attempts, not %d tasks x 2 arms x %d trials", len(order), len(tasks), k)
	}
	return nil
}

// scheduledTasks returns the order's task ids, unique and ascending.
func scheduledTasks(order []Attempt) []string {
	seen := map[string]bool{}
	tasks := []string{}
	for _, attempt := range order {
		if !seen[attempt.TaskID] {
			seen[attempt.TaskID] = true
			tasks = append(tasks, attempt.TaskID)
		}
	}
	sort.Strings(tasks)
	return tasks
}

// validatePhase checks one calibration phase. A passed phase must prove every
// task in both directions: the oracle accepted the clean workspace and did not
// accept the mutated one. A failed phase may list no task, as when the trusted
// preparation could not build the module cache.
func validatePhase(name string, phase CalibrationPhase) error {
	if (phase.Status != CalibrationPassed && phase.Status != CalibrationFailed) || phase.Tasks == nil ||
		(phase.Status == CalibrationPassed && len(phase.Tasks) == 0) {
		return invalidf(DetailFieldInvalid, "%s needs a passed or failed status and its tasks", name)
	}
	seen := make(map[string]bool, len(phase.Tasks))
	for _, task := range phase.Tasks {
		if !taskIDPattern.MatchString(task.TaskID) || seen[task.TaskID] {
			return invalidf(DetailFieldInvalid, "%s task %q is malformed or repeated", name, task.TaskID)
		}
		seen[task.TaskID] = true
		if phase.Status == CalibrationPassed && (!task.CleanAccepted || task.MutatedAccepted) {
			return invalidf(DetailFieldInvalid, "%s passed, yet task %s did not prove both directions", name, task.TaskID)
		}
	}
	return nil
}

// phaseCovers reports whether a validated phase calibrated exactly the tasks
// the order schedules.
func phaseCovers(phase CalibrationPhase, order []Attempt) bool {
	scheduled := scheduledTasks(order)
	if len(phase.Tasks) != len(scheduled) {
		return false
	}
	for _, task := range phase.Tasks {
		if _, found := slices.BinarySearch(scheduled, task.TaskID); !found {
			return false
		}
	}
	return true
}

// validateCalibration checks calibration.json alone. After exists only once a
// trial ran, and no trial runs after a failed before calibration.
func validateCalibration(c Calibration) error {
	if c.SchemaVersion != "" && c.SchemaVersion != CalibrationSchemaV1 {
		return invalidf(DetailFieldInvalid, "calibration schema_version %q is not %s", c.SchemaVersion, CalibrationSchemaV1)
	}
	if !sessionIDPattern.MatchString(c.SessionID) {
		return invalidf(DetailFieldInvalid, "calibration session_id %q is not 128-bit lowercase hex", c.SessionID)
	}
	if err := validatePhase("before", c.Before); err != nil {
		return err
	}
	if c.After == nil {
		return nil
	}
	if c.Before.Status != CalibrationPassed {
		return invalidf(DetailFieldInvalid, "after is recorded, yet a failed before calibration starts no trial")
	}
	return validatePhase("after", *c.After)
}

// validateRecord checks one record alone; whether it belongs to the protocol
// order is decided when the session is reconciled.
func validateRecord(r Record) error {
	if r.SchemaVersion != "" && r.SchemaVersion != RecordSchemaV1 {
		return invalidf(DetailFieldInvalid, "record schema_version %q is not %s", r.SchemaVersion, RecordSchemaV1)
	}
	outcome, known := signalOutcomes[r.Signal]
	if !known {
		return invalidf(DetailFieldInvalid, "signal %q is not a REQ-HE-08 signal", r.Signal)
	}
	if r.Outcome != outcome {
		return invalidf(DetailFieldInvalid, "outcome %q contradicts signal %s, which is %s", r.Outcome, r.Signal, outcome)
	}
	if r.Oracle == nil || r.Oracle.ExpectedPassed < 0 || r.Oracle.ExpectedFailed < 0 || r.DurationS < 0 {
		return invalidf(DetailFieldInvalid, "record needs an oracle observation with non-negative counts and duration")
	}
	return nil
}
