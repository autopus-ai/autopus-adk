// Package pipeline provides pipeline state management types and persistence.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/insajin/autopus-adk/pkg/memindex"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// PhaseContext holds runtime context passed to each phase's prompt builder.
type PhaseContext struct {
	// PreviousResults maps PhaseID to the normalized output of that phase.
	PreviousResults map[PhaseID]string
	// ContextResult is the bounded delegated context receipt for this phase.
	ContextResult *memindex.ContextResult
	// FrozenRequiredDocuments requires one immutable snapshot of all required SPEC documents.
	FrozenRequiredDocuments bool
	// Route is the phase route this run dispatches. The compact route drops
	// the separate scaffold dispatch, so the implementation phase has to
	// carry the test-first contract itself.
	Route PhaseRoute
}

// compactImplementDirective keeps test-first work inside the implementation
// phase when the compact route drops the dedicated scaffold dispatch. Without
// it, removing a phase would quietly remove the behaviour it carried.
const compactImplementDirective = "## Compact Route\n" +
	"This run dispatches implement, validate, and review only; no separate " +
	"test-scaffold phase runs. Write the failing test for each referenced " +
	"acceptance criterion first, observe it fail, then implement until it " +
	"passes. Report the test files you added or changed alongside the " +
	"implementation."

// PhasePromptBuilder builds prompts for each pipeline phase by reading files
// from a spec directory and injecting previous phase results.
type PhasePromptBuilder struct {
	specDir                       string
	requiredSnapshotOnce          sync.Once
	requiredSnapshotLayers        []promptlayer.Layer
	requiredSnapshotLoadErr       error
	requiredSnapshotFirstPassHook func()
	expectedSnapshotHash          string
}

// NewPhasePromptBuilder creates a PhasePromptBuilder that reads files from specDir.
func NewPhasePromptBuilder(specDir string) *PhasePromptBuilder {
	return &PhasePromptBuilder{specDir: specDir}
}

// @AX:NOTE [AUTO]: hardcoded section headers — "## SPEC", "## Plan" etc. are implicit prompt contract with the AI backend
// BuildPrompt constructs the prompt for the given phase using the spec directory
// contents and any prior phase results available in ctx.
func (b *PhasePromptBuilder) BuildPrompt(phaseID PhaseID, ctx PhaseContext) (string, error) {
	prompt, _, err := b.BuildPromptWithManifest(phaseID, ctx)
	return prompt, err
}

// BuildPromptWithManifest constructs a phase prompt and diagnostic layer manifest.
// @AX:ANCHOR [AUTO]: Public prompt-manifest contract consumed by the engine and pipeline context receipts.
// @AX:REASON [AUTO]: Layer ordering, frozen SPEC authority, and rendered manifest identities cross the AI backend boundary.
// @AX:WARN [AUTO]: Prompt construction has cyclomatic complexity 21 across frozen-document and phase-specific injection paths.
// @AX:REASON [AUTO]: Each phase must receive the exact required snapshot and only its admitted prior outputs.
// @AX:NOTE [AUTO] @AX:SPEC: SPEC-AGENT-PROMPT-001: phase:* layer IDs mirror prompt sections and prior-phase injections.
func (b *PhasePromptBuilder) BuildPromptWithManifest(phaseID PhaseID, ctx PhaseContext) (string, PromptManifest, error) {
	receiptMode := ctx.ContextResult != nil && strings.TrimSpace(ctx.ContextResult.Prompt) != ""
	frozenMode := receiptMode || ctx.FrozenRequiredDocuments
	var layers []promptlayer.Layer

	if frozenMode {
		requiredLayers, err := b.requiredPhaseDocumentLayers()
		if err != nil {
			return "", PromptManifest{}, err
		}
		layers = append(layers, requiredLayers...)
	} else {
		// @AX:NOTE [AUTO]: magic constant — "spec.md" filename is a hardcoded filesystem contract
		// Always include spec.md when it exists.
		specContent, err := b.readFile("spec.md")
		if err != nil && !os.IsNotExist(err) {
			return "", PromptManifest{}, fmt.Errorf("read spec.md: %w", err)
		}
		if specContent != "" {
			sanitized := sanitizePromptContent(specContent)
			layers = append(layers, phaseFileLayer("phase:spec", "spec.md", "SPEC", sanitized))
		}
	}

	// Phase-specific additional files and context injection.
	switch phaseID {
	case PhasePlan:
		if !frozenMode {
			planContent, _ := b.readFile("plan.md")
			if planContent != "" {
				sanitized := sanitizePromptContent(planContent)
				layers = append(layers, phaseFileLayer("phase:plan", "plan.md", "Plan", sanitized))
			}
		}

	case PhaseTestScaffold:
		if !frozenMode {
			b.appendFileSectionLayerIfPresent(&layers, "acceptance.md", "Acceptance")
		}
		b.injectPriorLayer(&layers, ctx, PhasePlan, "Plan Output")

	case PhaseImplement:
		if !frozenMode {
			b.appendFileSectionLayerIfPresent(&layers, "acceptance.md", "Acceptance")
		}
		b.injectPriorLayer(&layers, ctx, PhasePlan, "Plan Output")
		b.injectPriorLayer(&layers, ctx, PhaseTestScaffold, "Test Scaffold Output")
		if ctx.Route == RouteCompact {
			layers = append(layers, promptlayer.Layer{
				ID: "phase:route:compact-implement", Kind: promptlayer.KindEphemeral,
				Group: promptlayer.GroupTaskContext, SourceRef: "route:compact",
				Content: compactImplementDirective, CacheEligible: false,
				RedactionStatus: promptlayer.RedactionPassed, InvalidationReason: promptlayer.InvalidationNone,
			})
		}

	case PhaseValidate:
		if !frozenMode {
			b.appendFileSectionLayerIfPresent(&layers, "acceptance.md", "Acceptance")
		}
		b.injectPriorLayer(&layers, ctx, PhaseImplement, "Implementation Output")

	case PhaseReview:
		if !frozenMode {
			b.appendFileSectionLayerIfPresent(&layers, "acceptance.md", "Acceptance")
		}
		b.injectPriorLayer(&layers, ctx, PhaseValidate, "Validation Output")
	}
	if receiptMode {
		sanitized := sanitizePromptContent(ctx.ContextResult.Prompt)
		layers = append(layers, promptlayer.Layer{
			ID: "phase:context-receipt", Kind: promptlayer.KindSnapshot,
			Group: promptlayer.GroupTaskContext, SourceRef: "context-receipt",
			Content: sanitized.Content, CacheEligible: false,
			RedactionStatus:    sanitized.RedactionStatus,
			InvalidationReason: sanitized.InvalidationReason,
		})
	}

	rendered, err := promptlayer.Render(layers)
	if err != nil {
		return "", PromptManifest{}, err
	}
	return rendered.Prompt, rendered.Manifest, nil
}

func (b *PhasePromptBuilder) appendFileSectionLayerIfPresent(layers *[]promptlayer.Layer, name, label string) {
	content, err := b.readFile(name)
	if err != nil || content == "" {
		return
	}
	sanitized := sanitizePromptContent(content)
	*layers = append(*layers, phaseFileLayer("phase:"+strings.TrimSuffix(name, ".md"), name, label, sanitized))
}

func (b *PhasePromptBuilder) injectPriorLayer(layers *[]promptlayer.Layer, ctx PhaseContext, id PhaseID, label string) {
	if ctx.PreviousResults == nil {
		return
	}
	if result, ok := ctx.PreviousResults[id]; ok && result != "" {
		sanitized := sanitizePromptContent(result)
		*layers = append(*layers, promptlayer.Layer{
			ID:                 "phase:previous:" + string(id),
			Kind:               promptlayer.KindEphemeral,
			Group:              promptlayer.GroupTaskContext,
			SourceRef:          "previous:" + string(id),
			Content:            formatSection(label, sanitized.Content),
			RedactionStatus:    sanitized.RedactionStatus,
			InvalidationReason: sanitized.InvalidationReason,
		})
	}
}

func phaseFileLayer(id, sourceRef, label string, sanitized promptlayer.SanitizedContent) promptlayer.Layer {
	return promptlayer.Layer{
		ID:                 id,
		Kind:               promptlayer.KindSnapshot,
		Group:              promptlayer.GroupFrozenSnapshot,
		SourceRef:          sourceRef,
		Content:            formatSection(label, sanitized.Content),
		CacheEligible:      sanitized.RedactionStatus == promptlayer.RedactionPassed,
		RedactionStatus:    sanitized.RedactionStatus,
		InvalidationReason: sanitized.InvalidationReason,
	}
}

func formatSection(label, content string) string {
	return fmt.Sprintf("## %s\n%s", label, content)
}

func sanitizePromptContent(raw string) promptlayer.SanitizedContent {
	maxBytes := len(raw)*2 + 1024
	if maxBytes < 1 {
		maxBytes = 1
	}
	return promptlayer.SanitizeContent(raw, promptlayer.ContextOptions{MaxBytes: maxBytes})
}

// readFile reads a file relative to the spec directory and returns its contents.
func (b *PhasePromptBuilder) readFile(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(b.specDir, name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
