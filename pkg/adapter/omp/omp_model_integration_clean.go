package omp

import "os"

type ompProjectCleanState struct {
	preimage []byte
	missing  bool
	mode     os.FileMode
}

// prepareOMPProjectCleanStateAt returns what releasing the project claim set
// restores, or nil when no ledger manages a key. A ledger with profile claims
// keeps the receipt proof it always needed; one with only the agent surface
// claim has no receipt, and a ledger holding only the user-ownership mark
// restores nothing.
func (a *Adapter) prepareOMPProjectCleanStateAt(workspace *ompRootedWorkspace) (*ompProjectCleanState, error) {
	ownership, exists, err := readOMPModelProjectOwnershipAt(workspace)
	if err != nil || !exists || ownership.markerOnly() {
		return nil, err
	}
	if ownership.hasProfileClaims() {
		if _, err := validateCurrentOMPModelProjectConfigAt(workspace, ownership); err != nil {
			return nil, err
		}
		if err := requireOMPProjectReleaseReceiptAt(workspace, ownership); err != nil {
			return nil, err
		}
	}
	state, err := evaluateOMPClaimSetAt(workspace, false)
	if err != nil {
		return nil, err
	}
	if state.frozen {
		// What the claim would restore cannot be placed in the file as it is.
		return nil, state.readErr
	}
	mode := state.preimageMode
	if mode == 0 {
		mode = ompConfigFileMode
	}
	return &ompProjectCleanState{preimage: state.preimage, missing: state.preimageMissing, mode: mode}, nil
}
