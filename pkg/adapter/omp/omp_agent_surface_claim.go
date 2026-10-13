package omp

import (
	"io/fs"
	"os"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The Claim Set Contract keeps one ledger and one preimage for .omp/config.yml.
// The claim set C is the project-managed profile's managed keys plus, in
// autopus mode, task.disabledAgents unless the user owns it. Plain `omp`
// reads the project file, not the overlay, so the agent surface claim goes to
// .omp/config.yml whatever the profile's config mode is.

// ompClaimSetState is one reading of the project config and its ledger.
type ompClaimSetState struct {
	ownership      ompModelProjectOwnership
	ledger         bool
	current        []byte
	currentMissing bool
	currentMode    fs.FileMode
	value          any
	present        bool
	// userOwned reports task.disabledAgents as the user's after this reading;
	// newlyUserOwned reports that this reading decided it.
	userOwned      bool
	newlyUserOwned bool
	// prior is the preimage state of task.disabledAgents a claim starts from.
	prior string
	// preimage is what a release restores; preimageMissing restores no file.
	preimage        []byte
	preimageMissing bool
	preimageMode    fs.FileMode
	// base is the preimage before a user-owned value is applied to it, the
	// file a merge starts from while the user owns task.disabledAgents.
	base        []byte
	baseMissing bool
	// rebased reports an agent-surface-only ledger re-derived from a user
	// edit outside the claim set.
	rebased bool
	// unclaimable reports a config ADK has never claimed that the managed
	// parser refuses: it stays the user's, untouched and unmarked. A profile
	// claim on such a file still fails with readErr, as it always has.
	unclaimable bool
	readErr     error
}

// evaluateOMPClaimSetAt decides whose task.disabledAgents is and what a
// release restores. claimSurface is autopus mode: only it decides about a
// value it finds the first time, or claims a restored default set again.
func evaluateOMPClaimSetAt(workspace *ompRootedWorkspace, claimSurface bool) (ompClaimSetState, error) {
	var state ompClaimSetState
	var err error
	if state.ownership, state.ledger, err = readOMPModelProjectOwnershipAt(workspace); err != nil {
		return state, err
	}
	unclaimed := !state.ledger || state.ownership.markerOnly()
	state.current, state.currentMissing, state.currentMode, err = readOMPModelProjectConfigAt(workspace)
	if err == nil {
		state.value, state.present, err = ompDisabledAgentsIn(state.current)
	}
	if err != nil {
		if !unclaimed {
			return state, err
		}
		// A file ADK never claimed and cannot parse safely (a symlink,
		// invalid YAML, several documents) is not read further or written.
		state.unclaimable, state.userOwned, state.readErr = true, true, err
		return state, nil
	}
	state.preimageMode = state.currentMode
	isDefault := ompDisabledAgentsAreDefault(state.value, state.present)
	if !state.ledger || state.ownership.markerOnly() {
		// The file as it stands is the preimage, user value included.
		state.preimage, state.preimageMissing = state.current, state.currentMissing
		state.base, state.baseMissing = state.current, state.currentMissing
		state.prior = ompDisabledAgentsFingerprint(state.value, state.present)
		switch {
		case state.ledger:
			state.userOwned = !claimSurface || !isDefault
		case claimSurface:
			state.userOwned = state.present && !isDefault
			state.newlyUserOwned = state.userOwned
		}
		return state, nil
	}
	return evaluateOMPClaimLedgerAt(workspace, state, claimSurface, isDefault)
}

// @AX:WARN [AUTO]: claim ledger evaluation contains more than eight conditional branches.
// @AX:REASON [AUTO]: anchor validation, user-ownership transfer, re-claim, re-base, and preimage recovery are decided together.
func evaluateOMPClaimLedgerAt(
	workspace *ompRootedWorkspace,
	state ompClaimSetState,
	claimSurface, isDefault bool,
) (ompClaimSetState, error) {
	ownership := state.ownership
	surface, claimed := ownership.agentSurfaceKey()
	anchor, edited, err := ompProjectConfigAnchor(state.current, state.currentMissing, ownership)
	if err != nil {
		return state, err
	}
	if anchor == nil {
		if ownership.hasProfileClaims() || !claimed {
			_, err := validateCurrentOMPModelProjectConfigAt(workspace, ownership)
			return state, err
		}
		// Only the agent surface is claimed: the rest of the file is the
		// user's and becomes the new preimage.
		state.rebased = true
		state.userOwned, state.newlyUserOwned = !isDefault, !isDefault
		state.prior, state.preimage = surface.PriorFingerprint, state.current
		if !state.userOwned {
			if state.preimage, err = applyOMPAgentSurfacePrior(state.current, surface.PriorFingerprint); err != nil {
				return state, err
			}
		}
		state.preimageMissing = ompBlankConfig(state.preimage) && ownership.OriginalMissing
		state.base, state.baseMissing = state.current, state.currentMissing
		return state, nil
	}
	state.userOwned = ownership.AgentSurfaceUserOwned
	switch {
	case claimed && edited && !isDefault:
		state.userOwned, state.newlyUserOwned = true, true
	case !claimed && !ownership.AgentSurfaceUserOwned && claimSurface:
		state.userOwned = state.present && !isDefault
		state.newlyUserOwned = state.userOwned
	case ownership.AgentSurfaceUserOwned && claimSurface && isDefault:
		state.userOwned = false
	}
	base, baseMissing := state.current, state.currentMissing
	if ownership.hasProfileClaims() {
		if base, baseMissing, state.preimageMode, err = loadOMPModelProjectOriginalPreimageAt(workspace, ownership); err != nil {
			return state, err
		}
	}
	state.base, state.baseMissing = base, baseMissing
	switch {
	case state.userOwned && !ownership.hasProfileClaims():
		state.prior, state.preimage = ompDisabledAgentsFingerprint(state.value, state.present), state.current
	case state.userOwned:
		state.prior = ompDisabledAgentsFingerprint(state.value, state.present)
		state.preimage, err = setOMPDisabledAgents(base, state.value, state.present)
	case claimed:
		state.prior = surface.PriorFingerprint
		state.preimage, err = applyOMPAgentSurfacePrior(base, surface.PriorFingerprint)
	case ownership.AgentSurfaceUserOwned:
		// The user restored the default set: it is the preimage value now.
		state.prior = ompDefaultDisabledAgentsFingerprint()
		state.preimage, err = setOMPDisabledAgents(base, state.value, state.present)
	default:
		value, present, readErr := ompDisabledAgentsIn(base)
		state.prior, state.preimage, err = ompDisabledAgentsFingerprint(value, present), base, readErr
	}
	if err != nil {
		return state, err
	}
	state.preimageMissing = ompBlankConfig(state.preimage) && (baseMissing || ownership.OriginalMissing)
	return state, nil
}

// ompProfileClaims are the project-managed profile's values and claims.
type ompProfileClaims struct {
	values map[string]any
	claims map[string]config.RoleManagedKeyClaimConf
}

type ompClaimSetPlan struct {
	config       *adapter.FileMapping
	ledger       *adapter.FileMapping
	ledgerDigest string
	state        ompClaimSetState
}

func (p ompClaimSetPlan) mappings() []adapter.FileMapping {
	var mappings []adapter.FileMapping
	for _, mapping := range []*adapter.FileMapping{p.config, p.ledger} {
		if mapping != nil {
			mappings = append(mappings, *mapping)
		}
	}
	return mappings
}

// planOMPClaimSetAt merges the whole claim set over the preimage once and
// produces the config and ledger mappings of this generation. An empty C
// leaves the release of a claims ledger to the transaction plan and keeps
// only a user-ownership mark.
func planOMPClaimSetAt(
	workspace *ompRootedWorkspace,
	profile *ompProfileClaims,
	claimSurface bool,
) (ompClaimSetPlan, error) {
	state, err := evaluateOMPClaimSetAt(workspace, claimSurface)
	if err != nil {
		return ompClaimSetPlan{}, err
	}
	plan := ompClaimSetPlan{state: state}
	if state.unclaimable {
		if state.ledger {
			_, data, markErr := newOMPMarkerOnlyOwnership()
			ledger := ompIntegratedMapping(OMPModelProjectOwnershipRelativePath, data)
			plan.ledger = &ledger
			return plan, markErr
		}
		if profile != nil {
			return plan, state.readErr
		}
		return plan, nil
	}
	claimDA := claimSurface && !state.userOwned
	if profile == nil && state.ledger && state.ownership.hasProfileClaims() {
		if err := requireOMPProjectReleaseReceiptAt(workspace, state.ownership); err != nil {
			return plan, err
		}
	}
	if profile == nil && !claimDA {
		if state.userOwned {
			_, data, markErr := newOMPMarkerOnlyOwnership()
			if markErr != nil {
				return plan, markErr
			}
			ledger := ompIntegratedMapping(OMPModelProjectOwnershipRelativePath, data)
			plan.ledger = &ledger
		}
		return plan, nil
	}
	base, surface, err := ompClaimSetMergeBase(state, claimDA)
	if err != nil {
		return plan, err
	}
	claims, err := ompClaimSetClaims(state, profile, surface, base)
	if err != nil {
		return plan, err
	}
	mode := os.FileMode(0o600)
	if state.currentMode != 0 {
		mode = state.currentMode
	}
	result, err := MergeOMPProjectManagedConfig(OMPProjectManagedInput{Existing: base, Mode: mode, Claims: claims})
	if err != nil {
		return plan, err
	}
	emission := ompClaimEmission{bytes: result.Bytes, fingerprints: result.ManagedFingerprints}
	if state.userOwned {
		// A user-owned key edited alone needs no rewrite of the user's bytes.
		if !state.currentMissing && ompOnlyDisabledAgentsDiffer(result.Bytes, state.current) {
			emission.bytes, emission.skipped = state.current, true
		} else {
			emission.surface = &ompAgentSurfaceValue{Present: state.present, Value: state.value}
		}
	}
	ownership, data, err := nextOMPClaimLedger(state, profile != nil, claimDA, emission)
	if err != nil {
		return plan, err
	}
	configMapping := ompIntegratedMapping(configFile, emission.bytes)
	ledger := ompIntegratedMapping(OMPModelProjectOwnershipRelativePath, data)
	plan.config, plan.ledger, plan.ledgerDigest = &configMapping, &ledger, ownership.LedgerDigest
	return plan, nil
}

// ompOnlyDisabledAgentsDiffer reports two files equal once task.disabledAgents
// is rendered the same in both.
func ompOnlyDisabledAgentsDiffer(left, right []byte) bool {
	leftCanonical, leftErr := setOMPDisabledAgents(left, config.OMPAutopusDisabledAgents(), true)
	rightCanonical, rightErr := setOMPDisabledAgents(right, config.OMPAutopusDisabledAgents(), true)
	return leftErr == nil && rightErr == nil && string(leftCanonical) == string(rightCanonical)
}
