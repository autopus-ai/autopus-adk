package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// Round inputs and their reasons besides the delta fallback reasons
// (SPEC-REVIEWREUSE-001 REQ-RR-DELTA-06).
const (
	specReviewInputFull           = "full"
	specReviewInputDelta          = "delta"
	specReviewInputDiscover       = "discover"
	specReviewInputConfiguredFull = "configured_full"
)

// specReviewRoundInput is the input a round sends and why.
type specReviewRoundInput struct {
	mode     string
	reason   string
	baseline string
}

// specReviewRoundSources is what a round's full input carried besides the
// prompt text: its verified context delivery (nil for an inline prompt), its
// SPEC documents in their delivered representation, and for an inline prompt
// how much of each auxiliary document it injected, read when the prompt was
// built.
type specReviewRoundSources struct {
	delivery  *promptlayer.ContextDeliveryResult
	documents map[string]spec.ReviewInputDocument
	coverages []spec.DocCoverage
}

// readSpecReviewRoundSources reads the documents and the inline coverage of
// the round whose prompt was just built from delivery.
func readSpecReviewRoundSources(p specReviewLoopParams, delivery *promptlayer.ContextDeliveryResult) specReviewRoundSources {
	sources := specReviewRoundSources{delivery: delivery}
	sources.documents, _ = spec.ReviewInputDocuments(p.specDir)
	if delivery == nil {
		sources.coverages = spec.AuxDocCoverages(p.specDir, spec.ResolveAuxTotalBudget(p.gate.DocContextMaxLines))
	}
	return sources
}

// chooseSpecReviewInput returns the prompt a round sends: the full prompt,
// or for a verify round under verify_input delta the delta prompt when every
// eligibility and integrity condition holds (REQ-RR-DELTA-03, REQ-RR-DELTA-05).
func chooseSpecReviewInput(
	p specReviewLoopParams, doc *spec.SpecDocument, priorFindings []spec.ReviewFinding, full string, fp *specReviewFingerprint, round specReviewRoundSources,
) (string, specReviewRoundInput) {
	if len(priorFindings) == 0 {
		return full, specReviewRoundInput{mode: specReviewInputFull, reason: specReviewInputDiscover}
	}
	if p.verifyInput != config.ReviewVerifyInputDelta {
		return full, specReviewRoundInput{mode: specReviewInputFull, reason: specReviewInputConfiguredFull}
	}
	prompt, baseline, reason := buildSpecReviewDeltaPrompt(p, doc, priorFindings, full, fp, round)
	if reason != spec.DeltaReasonDelta {
		return full, specReviewRoundInput{mode: specReviewInputFull, reason: reason}
	}
	return prompt, specReviewRoundInput{mode: specReviewInputDelta, reason: reason, baseline: baseline}
}

func buildSpecReviewDeltaPrompt(
	p specReviewLoopParams, doc *spec.SpecDocument, priorFindings []spec.ReviewFinding, full string, fp *specReviewFingerprint, round specReviewRoundSources,
) (string, string, string) {
	// Without a complete fingerprint the delta cannot prove what did not
	// change outside the SPEC, so the round reviews the full input.
	if fp == nil || !fp.Complete {
		return "", "", spec.DeltaReasonContextChanged
	}
	baseline, err := spec.LoadReviewBaseline(p.specDir)
	if err != nil {
		return "", "", spec.DeltaReasonMissingBaseline
	}
	if round.documents == nil {
		return "", "", spec.DeltaReasonIntegrityGap
	}
	plan := spec.PlanDeltaVerify(spec.DeltaVerifyInput{
		Baseline: baseline, Documents: round.documents, Context: specReviewBaselineContext(fp, p.specDir), Findings: priorFindings,
	})
	if plan.Reason != spec.DeltaReasonDelta {
		return "", "", plan.Reason
	}
	sources, err := specReviewDeltaSources(p, round.delivery)
	if err != nil {
		return "", "", spec.DeltaReasonIntegrityGap
	}
	prompt, manifest := spec.BuildDeltaVerifyPrompt(spec.DeltaPromptInput{
		Doc: doc, Plan: plan, Baseline: baseline, Documents: round.documents, Sources: sources,
		PriorFindings: priorFindings, PassCriteria: p.gate.PassCriteria,
	})
	if len(prompt) >= len(full) {
		return "", "", spec.DeltaReasonNoSaving
	}
	if err := spec.ValidateDeltaManifest(spec.DeltaManifestCheck{
		Manifest: manifest, Required: spec.RequiredDeltaSources(round.documents, sources), Baseline: baseline, Plan: plan, Prompt: prompt,
	}); err != nil {
		return "", "", spec.DeltaReasonIntegrityGap
	}
	return prompt, fmt.Sprintf("%s r%d", baseline.RunID, baseline.Revision), spec.DeltaReasonDelta
}

// specReviewDeltaSources are the required sources of a delta round other than
// the SPEC documents, each carried in full because a reviewer cannot recall
// them: the checklist, the code context when one is injected, and every
// context document the round's verified delivery carries (REQ-RR-DELTA-04).
func specReviewDeltaSources(p specReviewLoopParams, delivery *promptlayer.ContextDeliveryResult) ([]spec.DeltaSource, error) {
	checklist, err := spec.ReviewChecklistBody()
	if err != nil {
		return nil, err
	}
	sources := []spec.DeltaSource{spec.IncludedDeltaSource("checklist", spec.SHA256Hex([]byte(checklist)), strings.TrimSpace(checklist))}
	if p.codeContext != "" {
		sources = append(sources, spec.IncludedDeltaSource("code.context", spec.SHA256Hex([]byte(p.codeContext)), p.codeContext))
	}
	if delivery == nil {
		return sources, nil
	}
	contents := make(map[string]string, len(delivery.Layers))
	for _, layer := range delivery.Layers {
		contents[layer.SourceRef] = layer.Content
	}
	for _, document := range delivery.RequiredDocuments {
		if strings.HasPrefix(document.SourceRef, delivery.SpecDir+"/") {
			continue
		}
		content, ok := contents[document.SourceRef]
		if !ok || strings.TrimSpace(content) == "" {
			return nil, fmt.Errorf("context document %s has no delivered body", document.SourceRef)
		}
		sources = append(sources, spec.IncludedDeltaSource("reference:"+document.SourceRef, strings.TrimPrefix(document.SourceHash, "sha256:"), content))
	}
	return sources, nil
}

// specReviewBaselineContext flattens the round fingerprint's non-document
// components. References inside the SPEC directory are documents and are
// compared as such; the markdown files a full input does not carry are
// compared by one digest, so adding, removing, or editing one is a context
// change.
func specReviewBaselineContext(fp *specReviewFingerprint, specDir string) map[string]string {
	context := map[string]string{
		"checklist": fp.Checklist, "code.head": fp.Code.Head, "code.worktree": fp.Code.Worktree,
		"code.context": fp.Code.Context, "contract": fp.Contract,
		"policy": specReviewJSONDigest(fp.Policy), "providers": specReviewJSONDigest(fp.Providers),
	}
	specRel := filepath.ToSlash(filepath.Join(".autopus", "specs", filepath.Base(specDir))) + "/"
	for path, digest := range fp.References {
		if !strings.Contains("/"+filepath.ToSlash(path), "/"+specRel) {
			context["reference:"+path] = digest
		}
	}
	others := maps.Clone(fp.Documents)
	for _, name := range spec.CarriedReviewDocuments() {
		delete(others, name)
	}
	context["documents.other"] = specReviewJSONDigest(others)
	return context
}

func specReviewJSONDigest(value any) string {
	data, _ := json.Marshal(value)
	return spec.SHA256Hex(data)
}

// recordSpecReviewBaseline stores what a usable, uncut round delivered
// (REQ-RR-DELTA-02); a failure only costs the next round its delta input.
func recordSpecReviewBaseline(
	p specReviewLoopParams, revision int, round specReviewRoundSources, fp *specReviewFingerprint, findings []spec.ReviewFinding, w io.Writer,
) {
	if fp == nil || round.documents == nil {
		return
	}
	runID := ""
	if p.runtimeEvidence != nil {
		runID = p.runtimeEvidence.RunID
	}
	baseline := &spec.ReviewBaseline{
		Schema: spec.ReviewBaselineSchema, RunID: runID, Revision: revision,
		RecordedAt: time.Now().UTC().Format(time.RFC3339), Documents: spec.BaselineDocuments(round.documents),
		Delivered: specReviewDeliveredScope(round),
		Context:   specReviewBaselineContext(fp, p.specDir), OpenIDs: spec.OpenFindingIDs(findings),
	}
	if err := spec.PersistReviewBaseline(p.specDir, baseline); err != nil {
		fmt.Fprintf(w, "경고: 리뷰 baseline 저장 실패: %v\n", err)
	}
}

// specReviewDeliveredScope records whether the round's full input carried
// each SPEC document whole: a verified delivery carries every document
// complete or fails; an inline prompt carries spec.md whole and each
// auxiliary document as its injection coverage says (REQ-RR-DELTA-02).
func specReviewDeliveredScope(round specReviewRoundSources) map[string]string {
	delivered := make(map[string]string, len(round.documents))
	for name := range round.documents {
		delivered[name] = spec.BaselineDeliveredComplete
	}
	for _, coverage := range round.coverages {
		if _, carried := delivered[coverage.Name]; carried && !coverage.Complete {
			delivered[coverage.Name] = spec.BaselineDeliveredPartial
		}
	}
	return delivered
}

func printSpecReviewRoundInput(w io.Writer, input specReviewRoundInput) {
	if input.mode == specReviewInputDelta {
		fmt.Fprintf(w, "SPEC 리뷰 입력: delta (baseline %s)\n", input.baseline)
		return
	}
	fmt.Fprintf(w, "SPEC 리뷰 입력: full — %s\n", input.reason)
}
