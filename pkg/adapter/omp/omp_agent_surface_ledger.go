package omp

import (
	"crypto/sha256"
	"fmt"
	"sort"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompClaimEmission is one merge of the claim set. skipped keeps the user's
// bytes when only a user-owned task.disabledAgents differs from them; surface
// is the user-owned value the emission rendered.
type ompClaimEmission struct {
	bytes        []byte
	fingerprints map[string]string
	surface      *ompAgentSurfaceValue
	skipped      bool
}

// ompSurfaceClaim is the task.disabledAgents entry of one merge: ADK's claim,
// or a user-owned value merged at the same position so the emission's layout
// matches every earlier one and a later edit of that key alone stays
// recognizable.
type ompSurfaceClaim struct {
	value any
	prior string
}

// ompClaimSetMergeBase picks the file a merge starts from. ADK's claim starts
// from the preimage, whose task.disabledAgents is in its prior state; a
// user-owned value starts from the base it is applied to.
func ompClaimSetMergeBase(state ompClaimSetState, claimDA bool) ([]byte, *ompSurfaceClaim, error) {
	if !state.userOwned {
		base := state.preimage
		if state.preimageMissing {
			base = nil
		}
		if !claimDA {
			return base, nil, nil
		}
		return base, &ompSurfaceClaim{value: config.OMPAutopusDisabledAgents(), prior: state.prior}, nil
	}
	base := state.base
	if state.baseMissing {
		base = nil
	}
	if !state.present {
		removed, err := setOMPDisabledAgents(base, nil, false)
		return removed, nil, err
	}
	value, present, err := ompDisabledAgentsIn(base)
	if err != nil {
		return nil, nil, err
	}
	return base, &ompSurfaceClaim{value: state.value, prior: ompDisabledAgentsFingerprint(value, present)}, nil
}

// ompClaimSetClaims builds C over base. A profile key the ledger already owns
// starts from its preimage value.
func ompClaimSetClaims(
	state ompClaimSetState,
	profile *ompProfileClaims,
	surface *ompSurfaceClaim,
	base []byte,
) ([]OMPManagedKeyClaim, error) {
	var claims []OMPManagedKeyClaim
	if profile != nil {
		if len(profile.claims) != len(profile.values) {
			return nil, fmt.Errorf("managed_key_claim_required: expected=%d actual=%d",
				len(profile.values), len(profile.claims))
		}
		keys := make([]string, 0, len(profile.values))
		for key := range profile.values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			claim, ok := profile.claims[key]
			if !ok {
				return nil, fmt.Errorf("managed_key_claim_required: %s", key)
			}
			prior := claim.PriorFingerprint
			if state.ledger && ompModelProjectOwnedPath(state.ownership.ManagedKeys, key) {
				var err error
				if prior, err = fingerprintOMPManagedPath(base, key); err != nil {
					return nil, err
				}
			}
			claims = append(claims, OMPManagedKeyClaim{
				Path: key, Value: profile.values[key], Complete: claim.Complete,
				FullArrayOwnership: claim.FullArrayOwnership, PriorFingerprint: prior,
			})
		}
	}
	if surface != nil {
		claims = append(claims, OMPManagedKeyClaim{
			Path: ompDisabledAgentsKey, Value: surface.value, Complete: true,
			FullArrayOwnership: true, PriorFingerprint: surface.prior,
		})
	}
	return claims, nil
}

// nextOMPClaimLedger keeps the ledger's origin while profile claims stay,
// because the transaction chain still leads to it. Any other change writes a
// new origin at this transaction: its config write holds the before-bytes a
// later walk stops at, and the agent surface claim's prior restores that key.
func nextOMPClaimLedger(
	state ompClaimSetState,
	hasProfile, claimDA bool,
	emission ompClaimEmission,
) (ompModelProjectOwnership, []byte, error) {
	fingerprints := emission.fingerprints
	if !claimDA {
		// A user-owned value merged for its position is not a claim.
		fingerprints = make(map[string]string, len(emission.fingerprints))
		for path, fingerprint := range emission.fingerprints {
			if path != ompDisabledAgentsKey {
				fingerprints[path] = fingerprint
			}
		}
	}
	keys := annotateOMPAgentSurfaceKeys(ompModelProjectOwnedKeys(fingerprints), state.prior)
	old := state.ownership
	if state.ledger && !old.markerOnly() && !state.rebased && old.hasProfileClaims() == hasProfile {
		salt, err := decodeOMPModelProjectSalt(old.Salt)
		if err != nil {
			return ompModelProjectOwnership{}, nil, err
		}
		if !emission.skipped {
			old.LastEmittedCommitment = ompModelProjectCommitment(salt, emission.bytes, false)
		}
		old.ManagedKeys = keys
		old.AgentSurfaceUserOwned = state.userOwned
		switch {
		case !state.userOwned:
			old.AgentSurfaceEmitted = nil
		case emission.surface != nil:
			old.AgentSurfaceEmitted = emission.surface
		}
		return canonicalOMPModelProjectOwnership(old)
	}
	original, originalMissing := state.current, state.currentMissing
	if claimDA || !hasProfile {
		original, originalMissing = state.preimage, state.preimageMissing
	}
	start := newOMPModelProjectOwnership
	if !hasProfile {
		start = newOMPAgentSurfaceOwnership
	}
	ownership, _, err := start(original, originalMissing, emission.bytes, fingerprints)
	if err != nil {
		return ompModelProjectOwnership{}, nil, err
	}
	ownership.InitialManagedKeys = append([]ompModelProjectOwnedKey(nil), keys...)
	ownership.ManagedKeys = keys
	ownership.AgentSurfaceUserOwned = state.userOwned
	if state.userOwned {
		ownership.AgentSurfaceEmitted = emission.surface
	}
	return canonicalOMPModelProjectOwnership(ownership)
}

func annotateOMPAgentSurfaceKeys(keys []ompModelProjectOwnedKey, prior string) []ompModelProjectOwnedKey {
	for index := range keys {
		if keys[index].Path == ompDisabledAgentsKey {
			keys[index].Owner = ompAgentSurfaceOwner
			keys[index].PriorFingerprint = prior
		}
	}
	return keys
}

// requireOMPProjectReleaseReceiptAt keeps the guard every profile release has
// had: the ledger must be the one the last project-managed activation proved.
func requireOMPProjectReleaseReceiptAt(workspace *ompRootedWorkspace, ownership ompModelProjectOwnership) error {
	receipt, reason := readOMPModelDoctorReceiptAt(workspace)
	if reason != "" || receipt.ConfigSource != config.RoleModelConfigModeProjectManaged ||
		receipt.ProjectOwnershipDigest != ownership.LedgerDigest {
		return fmt.Errorf("managed_key_conflict: project ownership receipt mismatch")
	}
	return nil
}

// newOMPAgentSurfaceOwnership starts a ledger that claims only the agent
// surface. Its salt is derived from the preimage, so the same project state
// always yields the same ledger bytes and a generated tree stays a pure
// function of its inputs. The salt hides nothing here that the owner-only
// transaction backups of the same preimage do not already hold, and no
// transaction chain is walked back to this ledger's origin.
func newOMPAgentSurfaceOwnership(
	original []byte,
	originalMissing bool,
	emitted []byte,
	fingerprints map[string]string,
) (ompModelProjectOwnership, []byte, error) {
	salt := sha256.Sum256(append(append([]byte("autopus.omp.agent-surface-salt.v1\x00"),
		ompModelProjectCommitmentMarker(originalMissing)...), original...))
	return newOMPModelProjectOwnershipWithSalt(salt[:], original, originalMissing, emitted, fingerprints)
}

func validOMPModelProjectKeyOwner(key ompModelProjectOwnedKey) bool {
	switch key.Owner {
	case "":
		return key.PriorFingerprint == ""
	case ompAgentSurfaceOwner:
		return key.Path == ompDisabledAgentsKey && validOMPModelHash(key.PriorFingerprint)
	}
	return false
}

// markerOnly reports a ledger that only remembers a user-owned
// task.disabledAgents: it manages no key and so restores nothing.
func (o ompModelProjectOwnership) markerOnly() bool {
	return o.AgentSurfaceUserOwned && o.AgentSurfaceEmitted == nil && o.Salt == "" &&
		o.OriginalCommitment == "" && o.InitialEmittedCommitment == "" && o.LastEmittedCommitment == "" &&
		!o.OriginalMissing && len(o.InitialManagedKeys) == 0 && len(o.ManagedKeys) == 0
}

func newOMPMarkerOnlyOwnership() (ompModelProjectOwnership, []byte, error) {
	return canonicalOMPModelProjectOwnership(ompModelProjectOwnership{AgentSurfaceUserOwned: true})
}

func (o ompModelProjectOwnership) agentSurfaceKey() (ompModelProjectOwnedKey, bool) {
	return ompAgentSurfaceKeyIn(o.ManagedKeys)
}

func ompAgentSurfaceKeyIn(keys []ompModelProjectOwnedKey) (ompModelProjectOwnedKey, bool) {
	for _, key := range keys {
		if key.Owner == ompAgentSurfaceOwner {
			return key, true
		}
	}
	return ompModelProjectOwnedKey{}, false
}

// hasProfileClaims reports a ledger whose preimage is only recoverable from
// the transaction chain, because a profile claim may own any key's value.
func (o ompModelProjectOwnership) hasProfileClaims() bool {
	for _, key := range o.ManagedKeys {
		if key.Owner == "" {
			return true
		}
	}
	return false
}
