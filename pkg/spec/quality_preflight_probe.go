package spec

import (
	"fmt"
	"strings"
)

// probeSectionHeading names the plan.md section that records the risk-first
// integration probe verdicts. The pre-fan-out probe gate reads this table
// before implementation workers are dispatched, so an author who leaves a
// probe row structurally invalid silently disables that gate.
const probeSectionHeading = "## Risk-First Integration Probe"

// Probe table column positions. The header is
// assumption_id | class | risk | boundary | input | oracle | isolation | status | reason | evidence.
const (
	probeColAssumptionID = 0
	probeColStatus       = 7
	probeColReason       = 8
	probeColEvidence     = 9
	probeColumnCount     = 10
)

// The table is bounded on purpose: it records the riskiest assumptions worth
// probing before fan-out, not a full risk register (that stays in
// "## Risks & Mitigations").
const (
	probeMinRows = 1
	probeMaxRows = 3
)

const (
	probeStatusPass   = "PASS"
	probeStatusFail   = "FAIL"
	probeStatusNotRun = "not-run"
)

var probeStatuses = []string{probeStatusPass, probeStatusFail, probeStatusNotRun}

// validateRiskFirstProbe checks the structure of the plan.md probe table. A
// missing section is reported by the review policy section check, so this validator
// stays silent in that case instead of emitting a duplicate finding.
func validateRiskFirstProbe(planMD string) []ValidationError {
	if !strings.Contains(planMD, probeSectionHeading) {
		return nil
	}

	rows := tableRows(sectionBody(planMD, probeSectionHeading))
	var errs []ValidationError
	if len(rows) < probeMinRows || len(rows) > probeMaxRows {
		errs = append(errs, probeError(fmt.Sprintf(
			"표에는 %d-%d개의 행이 필요합니다 (현재 %d개). 가장 위험한 가정만 남기고, 실행하지 않은 행은 %s status와 reason으로 기록해야 합니다",
			probeMinRows, probeMaxRows, len(rows), probeStatusNotRun,
		)))
	}
	for i, row := range rows {
		errs = append(errs, validateProbeRow(i, row)...)
	}
	return errs
}

func validateProbeRow(index int, row []string) []ValidationError {
	label := probeRowLabel(index, row)
	if len(row) < probeColumnCount {
		return []ValidationError{probeError(fmt.Sprintf(
			"%s 행에 %d개 컬럼(assumption_id, class, risk, boundary, input, oracle, isolation, status, reason, evidence)이 필요하지만 %d개입니다",
			label, probeColumnCount, len(row),
		))}
	}

	var errs []ValidationError
	status, known := canonicalProbeStatus(row[probeColStatus])
	if !known {
		errs = append(errs, probeError(fmt.Sprintf(
			"%s 행 status가 %s 중 하나가 아닙니다: %q",
			label, strings.Join(probeStatuses, " / "), row[probeColStatus],
		)))
	}
	if probeCellEmpty(row[probeColReason]) {
		errs = append(errs, probeError(fmt.Sprintf(
			"%s 행에 reason이 없습니다. PASS, FAIL, %s 모두 판정 근거를 남겨야 합니다",
			label, probeStatusNotRun,
		)))
	}
	if status == probeStatusPass && probeCellEmpty(row[probeColEvidence]) {
		errs = append(errs, probeError(fmt.Sprintf(
			"%s 행이 PASS인데 evidence가 없습니다. 실제 실행 증거가 없으면 %s으로 기록해야 하며 PASS로 승격할 수 없습니다",
			label, probeStatusNotRun,
		)))
	}
	return errs
}

// canonicalProbeStatus maps an authored status cell onto the closed enum.
// Matching is case-insensitive because markdown authors vary, but the enum
// itself stays closed so "not-run" can never be read as a pass.
func canonicalProbeStatus(cell string) (string, bool) {
	cell = strings.TrimSpace(cell)
	for _, status := range probeStatuses {
		if strings.EqualFold(cell, status) {
			return status, true
		}
	}
	return "", false
}

// probeCellEmpty treats "-" as empty, matching the placeholder convention the
// other SPEC tables already use (see rowIsEmpty).
func probeCellEmpty(cell string) bool {
	cell = strings.TrimSpace(cell)
	return cell == "" || cell == "-"
}

func probeRowLabel(index int, row []string) string {
	if len(row) > probeColAssumptionID {
		if id := strings.TrimSpace(row[probeColAssumptionID]); id != "" && id != "-" {
			return id
		}
	}
	return fmt.Sprintf("#%d", index+1)
}

func probeError(message string) ValidationError {
	return ValidationError{
		Field:   "plan.md",
		Message: fmt.Sprintf("Risk-First Integration Probe: %s", message),
		Level:   "error",
	}
}
