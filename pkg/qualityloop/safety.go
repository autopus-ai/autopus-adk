package qualityloop

import (
	"strings"

	"github.com/insajin/autopus-adk/pkg/workflow"
)

func ValidateCandidateSafety(candidate CandidateDraft) SafetyDecision {
	reasons := make([]string, 0)
	status := StatusQuarantined

	if candidate.RawPayloadPresent {
		reasons = appendUnique(reasons, "raw_payload_present")
	}
	if candidate.ProviderWriteCallCount != 0 {
		reasons = appendUnique(reasons, "provider_write_not_allowed")
		status = StatusRejected
	}
	if isGeneratedSurfacePath(candidate.TargetArtifact) {
		reasons = appendUnique(reasons, "generated_surface_mutation_forbidden")
		status = StatusRejected
	}
	for _, ref := range append(append([]string{}, candidate.EvidenceRefs...), candidate.DisplayRefs...) {
		if crossWorkspaceRef(candidate.WorkspaceID, ref) {
			reasons = appendUnique(reasons, "cross_workspace_ref")
		}
		if unsafeText(ref) {
			reasons = appendUnique(reasons, "unsafe_evidence_ref")
		}
	}

	if len(reasons) == 0 {
		return SafetyDecision{
			Accepted:               true,
			Status:                 StatusNormalized,
			ProviderWriteCallCount: 0,
			Active:                 false,
		}
	}
	return SafetyDecision{
		Accepted:               false,
		Status:                 status,
		ReasonCodes:            reasons,
		ProviderWriteCallCount: 0,
		Active:                 false,
		RawRetainedPayload:     "",
	}
}

// generatedSurfacePrefixes are the prefix members of the categorized surface
// table (pkg/workflow surface_class.go) this check reads.
var generatedSurfacePrefixes = workflow.SurfacePrefixes(workflow.ConsumerQualityLoop)

// isGeneratedSurfacePath keeps its own matching rules: a prefix matches at the
// root or below any directory, and each named member keeps its member-specific
// rule. Only the member strings come from the table.
func isGeneratedSurfacePath(path string) bool {
	rel := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	if rel == "" {
		return false
	}
	for _, prefix := range generatedSurfacePrefixes {
		if strings.HasPrefix(rel, prefix) || strings.Contains(rel, "/"+prefix) {
			return true
		}
	}
	if strings.HasSuffix(rel, workflow.SurfaceAgentsMarketplace) ||
		strings.Contains(rel, "/"+workflow.SurfaceAgentsMarketplace) ||
		strings.Contains(rel, workflow.SurfaceContextSignatures) ||
		rel == workflow.SurfaceRootConfig ||
		strings.HasSuffix(rel, "/"+workflow.SurfaceRootConfig) ||
		strings.Contains(rel, workflow.SurfaceManifestDir) && strings.HasSuffix(rel, workflow.SurfaceManifestSuffix) {
		return true
	}
	return strings.Contains(rel, "/"+workflow.SurfacePluginCache)
}

func crossWorkspaceRef(workspaceID, ref string) bool {
	if strings.TrimSpace(workspaceID) == "" {
		return false
	}
	for _, id := range workspaceRefs(ref) {
		if id != workspaceID {
			return true
		}
	}
	return false
}

func workspaceRefs(ref string) []string {
	const marker = "workspace:"
	var ids []string
	for start := strings.Index(ref, marker); start >= 0; start = strings.Index(ref, marker) {
		ref = ref[start+len(marker):]
		end := len(ref)
		for index, r := range ref {
			if r == ':' || r == '/' || r == '?' || r == '&' || r == '#' || r == '|' || r == ' ' || r == ',' {
				end = index
				break
			}
		}
		if end > 0 {
			ids = append(ids, ref[:end])
		}
		if end >= len(ref) {
			break
		}
		ref = ref[end:]
	}
	return ids
}
