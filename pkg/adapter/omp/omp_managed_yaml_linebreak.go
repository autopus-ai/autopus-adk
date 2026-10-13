package omp

import (
	"bytes"
	"errors"
	"fmt"
)

// errOMPYAMLOnlyLineBreak refuses a project config ADK does not edit. yaml.v3
// also breaks lines at a lone CR, NEL, LS, and PS, which the managed editor's
// byte offsets do not count, so the node lines it places entries by would
// point at other bytes (security re-check N1 of SPEC-MODELUX-003).
var errOMPYAMLOnlyLineBreak = errors.New("project_config_unclaimable: .omp/config.yml holds a lone CR, NEL, LS, or PS line break, which ADK does not edit")

// ompYAMLOnlyLineBreak reports a line break yaml.v3 counts and the managed
// editor does not: a CR that no LF follows, NEL, LS, or PS. CRLF is one break
// to both.
func ompYAMLOnlyLineBreak(data []byte) bool {
	for i, b := range data {
		if b == '\r' && (i+1 == len(data) || data[i+1] != '\n') {
			return true
		}
	}
	return bytes.Contains(data, []byte("\u0085")) || bytes.Contains(data, []byte(" ")) ||
		bytes.Contains(data, []byte(" "))
}

// planFrozenOMPClaimSetAt keeps a recorded agent-surface claim as it stands
// while the project config holds a line break ADK does not edit: the config
// and the ledger are emitted with their current bytes, so the run writes
// neither, releases nothing, and the manifest keeps recording both until the
// file reads cleanly again. A profile claim can be neither applied nor
// released that way, so it fails closed.
func planFrozenOMPClaimSetAt(
	workspace *ompRootedWorkspace,
	state ompClaimSetState,
	profile *ompProfileClaims,
) (ompClaimSetPlan, error) {
	plan := ompClaimSetPlan{state: state}
	if profile != nil || state.ownership.hasProfileClaims() {
		return plan, state.readErr
	}
	ledgerBytes, _, err := workspace.readOwnerOnlyFile(OMPModelProjectOwnershipRelativePath, 4<<20)
	if err != nil {
		return plan, fmt.Errorf("read project ownership ledger: %w", err)
	}
	config := ompIntegratedMapping(configFile, state.current)
	ledger := ompIntegratedMapping(OMPModelProjectOwnershipRelativePath, ledgerBytes)
	plan.config, plan.ledger, plan.ledgerDigest = &config, &ledger, state.ownership.LedgerDigest
	return plan, nil
}
