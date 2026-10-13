package omp

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

// mergeOMPIntegratedProjectConfig merges the project-managed profile's claims
// and, in autopus mode, the agent surface claim over the one ledger preimage.
// The claim count check stays with the profile claims: the agent surface
// claim is the adapter's own and is never declared in managed_keys.
func (i *ompModelIntegration) mergeOMPIntegratedProjectConfig(
	workspace *ompRootedWorkspace,
	mapping adapter.FileMapping,
	values map[string]any,
) (adapter.FileMapping, adapter.FileMapping, string, error) {
	if len(i.profile.ManagedKeys) != len(values) {
		return mapping, adapter.FileMapping{}, "", fmt.Errorf(
			"managed_key_claim_required: expected=%d actual=%d", len(values), len(i.profile.ManagedKeys),
		)
	}
	plan, err := planOMPClaimSetAt(workspace, &ompProfileClaims{
		values: values, claims: i.profile.ManagedKeys,
	}, i.claimSurface)
	if err != nil {
		return mapping, adapter.FileMapping{}, "", err
	}
	if plan.config == nil || plan.ledger == nil {
		return mapping, adapter.FileMapping{}, "", fmt.Errorf("managed_key_claim_required: project claim set is empty")
	}
	merged := *plan.config
	merged.TargetPath, merged.OverwritePolicy = mapping.TargetPath, mapping.OverwritePolicy
	return merged, *plan.ledger, plan.ledgerDigest, nil
}

func fingerprintOMPManagedPath(data []byte, path string) (string, error) {
	document, err := parseOMPManagedDocument(data)
	if err != nil {
		return "", err
	}
	if err := rejectDuplicateOMPManagedKeys(document); err != nil {
		return "", err
	}
	location, err := locateOMPManagedPath(document, strings.Split(path, "."))
	if err != nil {
		return "", err
	}
	if !location.exists {
		return OMPMissingManagedValueFingerprint(), nil
	}
	value, err := decodeOMPManagedNode(location.value)
	if err != nil {
		return "", err
	}
	return FingerprintOMPManagedValue(value)
}

func ompModelProjectOwnedPath(keys []ompModelProjectOwnedKey, path string) bool {
	for _, key := range keys {
		if key.Path == path {
			return true
		}
	}
	return false
}
