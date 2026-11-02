package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/worker/adapter"
)

// Postcondition states and the report version (SPEC-WORKERPOST-001
// REQ-WPOST-STATE-01). Version 1 reports used `passed` and `failed`.
const (
	statusSatisfied   = "satisfied"
	statusUnsatisfied = "unsatisfied"
	statusNotRequired = "not_required"
	statusUnknown     = "unknown"

	postconditionReportVersion = 2
)

var (
	// errPostconditionFailed keeps the pre-contract error prefix for checks
	// that observed the opposite of what the task required.
	errPostconditionFailed = errors.New("postcondition failed")
	// errPostconditionUnverified marks checks that could not be decided; the
	// task fails rather than completing on an unverified result.
	errPostconditionUnverified = errors.New("postcondition unverified")

	// postconditionLocalTimeout bounds a read of the local repository, which
	// takes milliseconds unless the machine is overloaded or git hangs.
	postconditionLocalTimeout = 30 * time.Second
	// postconditionRemoteTimeout bounds one ls-remote round trip.
	postconditionRemoteTimeout = 15 * time.Second
)

type postconditionCheck struct {
	Name        string `json:"name"`
	Requirement string `json:"requirement"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	Remote      string `json:"remote,omitempty"`
	Ref         string `json:"ref,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Observed    string `json:"observed,omitempty"`
	Match       string `json:"match,omitempty"`
}

type postconditionReport struct {
	Version  int                  `json:"version"`
	Active   bool                 `json:"active"`
	Source   string               `json:"source"`
	Outcome  string               `json:"outcome"`
	Checks   []postconditionCheck `json:"checks,omitempty"`
	Branches []string             `json:"branches,omitempty"`
}

// verify judges every action of the plan and returns the postconditions.json
// artifact. Only the outcomes satisfied and not_required return a nil error
// (REQ-WPOST-STATE-02). Fallback mode stays silent, as it was before the
// contract, when the prompt mentions no action or the work directory had no
// commit before execution (REQ-WPOST-LEGACY-03); a baseline that could not be
// read is not a missing one, so it still yields a report.
func (g postconditionGit) verify(workDir string, plan taskPostconditions, base executionBaseline) (adapter.Artifact, error) {
	if plan.Source != sourceContract && (base.Head == presenceAbsent || !plan.Mentioned) {
		return adapter.Artifact{}, nil
	}
	report := postconditionReport{
		Version:  postconditionReportVersion,
		Active:   true,
		Source:   plan.Source,
		Branches: append([]string(nil), plan.Branches...),
	}
	if len(plan.Problems) > 0 {
		// handleTask rejects such a plan before execution; this keeps a
		// direct caller from reading an invalid contract as satisfied.
		report.Checks = []postconditionCheck{{
			Name: "contract", Requirement: string(levelRequired), Status: statusUnknown,
			Detail: "invalid completion_contract: " + strings.Join(plan.Problems, "; "),
		}}
	} else {
		report.Checks = append(report.Checks, g.commitCheck(workDir, plan.Commit, base), g.pushCheck(workDir, plan, base))
		report.Checks = append(report.Checks, g.branchChecks(workDir, plan, base)...)
		softenAmbiguousChecks(report.Checks)
	}
	report.Outcome = postconditionOutcome(report.Checks)

	artifact := adapter.Artifact{
		Name:     "postconditions.json",
		MimeType: "application/json",
		Data:     mustMarshalPostconditionReport(report),
	}
	return artifact, postconditionError(report)
}

// softenAmbiguousChecks turns an unsatisfied fallback check into unknown when
// the prompt both asked for and declined the action: missing evidence then is
// not a demand to act (REQ-WPOST-LEGACY-03).
func softenAmbiguousChecks(checks []postconditionCheck) {
	for i := range checks {
		if checks[i].Requirement == string(levelAmbiguous) && checks[i].Status == statusUnsatisfied {
			checks[i].Status = statusUnknown
			checks[i].Detail = fmt.Sprintf("the prompt both asks for and declines %s; %s", checks[i].Name, checks[i].Detail)
		}
	}
}

// postconditionOutcome is the first present of unsatisfied, unknown,
// satisfied, and not_required.
func postconditionOutcome(checks []postconditionCheck) string {
	for _, status := range []string{statusUnsatisfied, statusUnknown, statusSatisfied} {
		for _, check := range checks {
			if check.Status == status {
				return status
			}
		}
	}
	return statusNotRequired
}

func postconditionError(report postconditionReport) error {
	var parts []string
	for _, check := range report.Checks {
		switch check.Status {
		case statusUnsatisfied:
			parts = append(parts, fmt.Sprintf("%s: %s", check.Name, check.Detail))
		case statusUnknown:
			if report.Outcome == statusUnsatisfied {
				parts = append(parts, fmt.Sprintf("%s (unverified): %s", check.Name, check.Detail))
			} else {
				parts = append(parts, fmt.Sprintf("%s: %s", check.Name, check.Detail))
			}
		}
	}
	switch report.Outcome {
	case statusUnsatisfied:
		return fmt.Errorf("%w: %s", errPostconditionFailed, strings.Join(parts, "; "))
	case statusUnknown:
		return fmt.Errorf("%w: %s", errPostconditionUnverified, strings.Join(parts, "; "))
	default:
		return nil
	}
}

func mustMarshalPostconditionReport(report postconditionReport) string {
	data, err := json.Marshal(report)
	if err != nil {
		return `{"version":2,"active":true,"outcome":"unknown","checks":[{"name":"marshal","requirement":"required","status":"unknown","detail":"report serialization failed"}]}`
	}
	return string(data)
}
