package cli

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/qualityloop"
	"github.com/insajin/autopus-adk/pkg/workflow"
	"github.com/stretchr/testify/assert"
)

// surfaceParityProbePaths are the 13 SPEC-EDITGUARD-001 S10 probe paths in
// acceptance order.
var surfaceParityProbePaths = []string{
	".claude/x",
	".agents/skills/x",
	".agents/plugins/marketplace.json",
	"a/.codex/x",
	".autopus/runtime/x",
	".omp/x",
	".autopus/specs/x",
	"sub/config.toml",
	"x/plugins/cache/y",
	".autopus/claude-code-manifest.json",
	".autopus/backup/x",
	".mcp.json",
	".agents/hooks.json",
}

func driftGateVerdict(rel string) bool {
	return len(workflow.DetectGeneratedDrift([]string{rel}, false)) > 0
}

func qualityLoopVerdict(rel string) bool {
	decision := qualityloop.ValidateCandidateSafety(qualityloop.CandidateDraft{TargetArtifact: rel})
	for _, code := range decision.ReasonCodes {
		if code == "generated_surface_mutation_forbidden" {
			return true
		}
	}
	return false
}

// CD-6 / S10: the four consumers of the unified surface table keep their
// current verdict sets on the 13 probe paths.
func TestSurfaceTable_FourConsumerParity_S10(t *testing.T) {
	t.Parallel()
	consumers := []struct {
		name string
		eval func(string) bool
		want []bool
	}{
		{"drift gate", driftGateVerdict,
			[]bool{true, false, true, false, false, false, false, false, false, true, false, false, false}},
		{"qualityloop", qualityLoopVerdict,
			[]bool{true, true, true, true, true, false, false, true, true, true, false, false, true}},
		{"status hygiene", isRuntimeUnignoredRisk,
			[]bool{true, true, true, false, true, false, false, false, false, true, true, true, true}},
		{"guard namespace", workflow.InEditGuardNamespace,
			[]bool{true, true, true, false, false, true, false, false, false, false, false, false, true}},
	}
	for _, consumer := range consumers {
		got := make([]bool, len(surfaceParityProbePaths))
		for i, probe := range surfaceParityProbePaths {
			got[i] = consumer.eval(probe)
		}
		assert.Equal(t, consumer.want, got, consumer.name)
	}
}

// S10: the hygiene extra member lists keep their current members and order.
func TestStatusHygiene_ExtraMembersKeepCurrentMembersAndOrder(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{
		".agents/commands/",
		".agents/skills/",
		".autopus/backup/",
		".autopus/cache/",
		".autopus/canary/",
		".autopus/design/imports/",
		".autopus/design/verify/",
		".autopus/docs/",
		".autopus/qa/cache/",
		".autopus/qa/evidence/",
		".autopus/qa/feedback/",
		".autopus/qa/gui/",
		".autopus/qa/releases/",
		".autopus/qa/runs/",
		".autopus/runtime/",
		".autopus/telemetry/",
	}, runtimeUnignoredExtraPrefixes)
	assert.Equal(t, map[string]bool{
		".agents/hooks.json":   true,
		".autopus/audit.jsonl": true,
		".autopus/state.json":  true,
		".claude.json":         true,
		".mcp.json":            true,
	}, runtimeUnignoredExtraExactPaths)
}
