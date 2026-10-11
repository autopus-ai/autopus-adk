// Catalog handling for the standard native balanced role placement, which is
// the one Codex profile the adapter refuses to substitute.
package codex

import (
	"errors"
	"fmt"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ErrCodexNativePlacementUnavailable reports that the probed Codex catalog
// answered a managed native balanced profile with something weaker. The policy
// names one exact model at one exact reasoning level per role, so quietly
// installing the substitute would leave the agent claiming a rung it never got.
var ErrCodexNativePlacementUnavailable = errors.New(
	"Codex 카탈로그가 관리형 native balanced 프로필을 지원하지 않음",
)

// resolveCodexPlacement resolves a native balanced profile against a catalog
// snapshot. Three outcomes are possible:
//
//   - the catalog advertises the exact model and effort: use it;
//   - the catalog could not be read (offline, unauthenticated, malformed): keep
//     the requested profile verbatim as unverified, because an absent catalog
//     is no evidence that the model is absent;
//   - the catalog was read and offers something else: fail, so generation stops
//     before a single file is written.
func resolveCodexPlacement(requested config.CodexProfile, snapshot []byte) (config.CodexProfileResolution, error) {
	resolution := config.ResolveCodexProfile(requested, snapshot)
	if !resolution.Fallback {
		return resolution, nil
	}
	if resolution.Reason == config.CodexResolutionCatalogUnknown {
		resolution.Effective = requested
		resolution.Fallback = false
		return resolution, nil
	}
	return resolution, fmt.Errorf("%w: %s/%s (카탈로그 제공: %s, reason=%s)",
		ErrCodexNativePlacementUnavailable,
		requested.Model,
		requested.Effort,
		describeCodexProfile(resolution.Effective),
		resolution.Reason,
	)
}

// reportCodexPlacementUnverified records that a placement landed without
// catalog confirmation. It is deliberately not the fallback diagnostic: nothing
// was substituted, so an operator reading "fallback" would look for a
// downgrade that never happened.
func (a *Adapter) reportCodexPlacementUnverified(profile config.CodexProfile, catalogErr error) {
	if a.codexFallbackWriter == nil {
		return
	}
	if !a.claimCodexDiagnostic("unverified|" + profile.Model + "|" + profile.Effort) {
		return
	}
	detail := "unavailable"
	if catalogErr != nil {
		detail = catalogErr.Error()
	}
	fmt.Fprintf(a.codexFallbackWriter,
		"Codex model unverified: kept=%s/%s reason=%s detail=%s\n",
		profile.Model,
		profile.Effort,
		config.CodexResolutionCatalogUnknown,
		detail,
	)
}

func describeCodexProfile(profile config.CodexProfile) string {
	if profile.Model == "" {
		return "runtime-default"
	}
	if profile.Effort == "" {
		return profile.Model
	}
	return profile.Model + "/" + profile.Effort
}
