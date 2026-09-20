// Package agentprobe evaluates normalized native lifecycle observations.
// A structurally valid supplied trace is not independently authenticated proof.
package agentprobe

const MaxEvidenceBytes = 1 << 20
const MaxEvents = 64

// Evidence contains body-free normalized observations, not model assertions.
type Evidence struct {
	Version        int     `json:"version"`
	Platform       string  `json:"platform"`
	RuntimeVersion string  `json:"runtime_version"`
	RunID          string  `json:"run_id"`
	SupervisorID   string  `json:"supervisor_id"`
	Challenge      string  `json:"challenge"`
	Events         []Event `json:"events"`
}

type Event struct {
	Sequence   int    `json:"sequence"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	RunID      string `json:"run_id"`
	ParentID   string `json:"parent_id"`
	ChildID    string `json:"child_id,omitempty"`
	Case       string `json:"case,omitempty"`
	ResultCode string `json:"result_code,omitempty"`
	Status     string `json:"status,omitempty"`
	// OwnedIDs is present only on inventory events and must be explicit, even empty.
	OwnedIDs []string `json:"owned_ids"`
}

type Gate struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type Report struct {
	Version        int    `json:"version"`
	Platform       string `json:"platform"`
	RuntimeVersion string `json:"runtime_version"`
	RunID          string `json:"run_id"`
	Overall        string `json:"overall"`
	Gates          []Gate `json:"gates"`
}
