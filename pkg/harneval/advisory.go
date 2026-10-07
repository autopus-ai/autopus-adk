package harneval

import "encoding/json"

// AdvisorySchemaV1 identifies the unsigned live-lane report.
const AdvisorySchemaV1 = "harness_live_advisory.v1"

// AdvisoryReport is the unsigned harness_live_advisory.v1 report (REQ-HE-10,
// REQ-HE-11): the session, the inputs its protocol froze, and the verdict.
// It has no signature field and no raw prompt, transcript, or payload, so the
// signed eval-regression check rejects it as unsigned before decoding it. It
// gates neither merge nor release.
type AdvisoryReport struct {
	SchemaVersion          string     `json:"schema_version"`
	Advisory               bool       `json:"advisory"`
	SessionID              string     `json:"session_id"`
	StartedAt              string     `json:"started_at"`
	WorkspaceRevision      string     `json:"workspace_revision"`
	BaselineRef            string     `json:"baseline_ref"`
	BaselineSurfaceDigest  string     `json:"baseline_surface_digest"`
	CandidateSurfaceDigest string     `json:"candidate_surface_digest"`
	AgentSetDigest         string     `json:"agent_set_digest"`
	RunnerSHA256           string     `json:"runner_sha256"`
	GraderProfileSHA256    string     `json:"grader_profile_sha256"`
	Policy                 LivePolicy `json:"policy"`
	LiveVerdict
}

// BuildAdvisory judges a decoded session and assembles its report. A session
// that does not reconcile yields the ComputeVerdict error and no report.
func BuildAdvisory(s *Session) (*AdvisoryReport, error) {
	verdict, err := ComputeVerdict(s)
	if err != nil {
		return nil, err
	}
	p := s.Protocol
	return &AdvisoryReport{
		SchemaVersion:          AdvisorySchemaV1,
		Advisory:               true,
		SessionID:              p.SessionID,
		StartedAt:              p.StartedAt,
		WorkspaceRevision:      p.WorkspaceRevision,
		BaselineRef:            p.BaselineRef,
		BaselineSurfaceDigest:  p.BaselineSurfaceDigest,
		CandidateSurfaceDigest: p.CandidateSurfaceDigest,
		AgentSetDigest:         p.AgentSetDigest,
		RunnerSHA256:           p.RunnerSHA256,
		GraderProfileSHA256:    p.GraderProfileSHA256,
		Policy:                 p.Policy,
		LiveVerdict:            verdict,
	}, nil
}

// EncodeAdvisory renders the report deterministically: fixed field order,
// arrays as [] rather than null, a null pass_rate for an arm without a valid
// trial, and a trailing newline.
func EncodeAdvisory(report *AdvisoryReport) ([]byte, error) {
	doc := *report
	doc.HardFlips = emptyIfNil(doc.HardFlips)
	doc.Calibration.Tasks = emptyIfNil(doc.Calibration.Tasks)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
