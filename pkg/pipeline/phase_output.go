package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const (
	// PhaseOutputSchemaVersion identifies the persisted phase output record.
	PhaseOutputSchemaVersion = "pipeline_phase_output.v1"
	// PhaseOutputPolicyVersion identifies how a record's content is derived
	// from a phase's output: sanitized, compacted, and injected into later
	// prompts as is. A record made under another policy is never restored.
	PhaseOutputPolicyVersion = "pipeline_phase_output_policy.v1"
)

// PhaseOutputRecord is a completed phase's dynamic output exactly as later
// prompts read it, bound to the inputs that produced it
// (SPEC-PIPELINE-RESUME-001 REQ-PRR-OUT-01).
type PhaseOutputRecord struct {
	Schema           string  `yaml:"schema" json:"schema"`
	PhaseID          PhaseID `yaml:"phase_id" json:"phase_id"`
	Policy           string  `yaml:"policy" json:"policy"`
	Attempt          int     `yaml:"attempt" json:"attempt"`
	RunID            string  `yaml:"run_id" json:"run_id"`
	InputFingerprint string  `yaml:"input_fingerprint" json:"input_fingerprint"`
	ContentHash      string  `yaml:"content_hash" json:"content_hash"`
	Content          string  `yaml:"content" json:"content"`
}

// outputBinding is the run identity every input fingerprint covers.
type outputBinding struct {
	specID, snapshotHash, gitCommitHash string
	route                               PhaseRoute
}

func (e *SubprocessEngine) outputBinding() outputBinding {
	return outputBinding{
		specID: e.cfg.SpecID, snapshotHash: e.cfg.SnapshotHash,
		gitCommitHash: e.cfg.GitCommitHash, route: NormalizeRoute(e.cfg.Route),
	}
}

// recordPhaseOutput builds the record of the phase at index, which just
// completed with content as its dynamic output.
func (e *SubprocessEngine) recordPhaseOutput(state *engineRunState, index, attempt int, content string) PhaseOutputRecord {
	phase := state.phases[index]
	return PhaseOutputRecord{
		Schema: PhaseOutputSchemaVersion, PhaseID: phase.ID, Policy: PhaseOutputPolicyVersion,
		Attempt: attempt, RunID: state.result.Receipt.RunID,
		InputFingerprint: phaseInputFingerprint(e.outputBinding(), phase, state.upstreamRecords(index)),
		ContentHash:      phaseContentHash(content), Content: content,
	}
}

// upstreamRecords are the records of the completed phases before index on
// the route, in route order.
func (s *engineRunState) upstreamRecords(index int) []PhaseOutputRecord {
	var upstream []PhaseOutputRecord
	for _, phase := range s.phases[:index] {
		if record, ok := s.outputs[phase.ID]; ok && s.statuses[phase.ID] == CheckpointStatusDone {
			upstream = append(upstream, record)
		}
	}
	return upstream
}

// verifyPhaseOutput admits the single record of a completed phase when its
// schema, policy, content hash, and input fingerprint all hold. The reason
// names the first check that failed.
func verifyPhaseOutput(binding outputBinding, phase Phase, records, upstream []PhaseOutputRecord) (PhaseOutputRecord, string) {
	switch {
	case len(records) == 0:
		return PhaseOutputRecord{}, "no persisted output record"
	case len(records) > 1:
		return PhaseOutputRecord{}, fmt.Sprintf("%d output records for one phase", len(records))
	}
	record := records[0]
	switch {
	case record.Schema != PhaseOutputSchemaVersion:
		return record, fmt.Sprintf("output schema %q is not %s", record.Schema, PhaseOutputSchemaVersion)
	case record.Policy != PhaseOutputPolicyVersion:
		return record, fmt.Sprintf("output policy %q is not %s", record.Policy, PhaseOutputPolicyVersion)
	case phaseContentHash(record.Content) != record.ContentHash:
		return record, "content hash mismatch"
	case phaseInputFingerprint(binding, phase, upstream) != record.InputFingerprint:
		return record, "input fingerprint mismatch: the SPEC, snapshot, source revision, route, " +
			"policy, or an earlier output differs from the run that produced it"
	}
	return record, ""
}

// phaseInputFingerprint hashes what a phase's output depends on: the record
// contract, the SPEC identity and snapshot, the source revision, the route,
// the phase definition, and every completed output before it on the route.
func phaseInputFingerprint(binding outputBinding, phase Phase, upstream []PhaseOutputRecord) string {
	dependencies := make([]string, len(phase.DependsOn))
	for i, dependency := range phase.DependsOn {
		dependencies[i] = string(dependency)
	}
	sort.Strings(dependencies)
	parts := []string{
		PhaseOutputSchemaVersion, PhaseOutputPolicyVersion, binding.specID, binding.snapshotHash,
		binding.gitCommitHash, PipelineRouteVersion, string(binding.route), string(phase.ID),
		string(phase.Gate), strings.Join(dependencies, ","),
	}
	for _, record := range upstream {
		parts = append(parts, string(record.PhaseID)+"="+record.ContentHash)
	}
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func phaseContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
