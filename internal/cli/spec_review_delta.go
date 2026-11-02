package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
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

// chooseSpecReviewInput returns the prompt a round sends: the full prompt,
// or for a verify round under verify_input delta the delta prompt when every
// eligibility and integrity condition holds (REQ-RR-DELTA-03, REQ-RR-DELTA-05).
func chooseSpecReviewInput(
	p specReviewLoopParams, doc *spec.SpecDocument, priorFindings []spec.ReviewFinding, full string, fp *specReviewFingerprint,
) (string, specReviewRoundInput) {
	if len(priorFindings) == 0 {
		return full, specReviewRoundInput{mode: specReviewInputFull, reason: specReviewInputDiscover}
	}
	if p.verifyInput != config.ReviewVerifyInputDelta {
		return full, specReviewRoundInput{mode: specReviewInputFull, reason: specReviewInputConfiguredFull}
	}
	prompt, baseline, reason := buildSpecReviewDeltaPrompt(p, doc, priorFindings, full, fp)
	if reason != spec.DeltaReasonDelta {
		return full, specReviewRoundInput{mode: specReviewInputFull, reason: reason}
	}
	return prompt, specReviewRoundInput{mode: specReviewInputDelta, reason: reason, baseline: baseline}
}

func buildSpecReviewDeltaPrompt(
	p specReviewLoopParams, doc *spec.SpecDocument, priorFindings []spec.ReviewFinding, full string, fp *specReviewFingerprint,
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
	documents, err := spec.ReviewDocumentContents(p.specDir)
	if err != nil {
		return "", "", spec.DeltaReasonIntegrityGap
	}
	context := specReviewBaselineContext(fp, p.specDir)
	plan := spec.PlanDeltaVerify(spec.DeltaVerifyInput{Baseline: baseline, Documents: documents, Context: context, Findings: priorFindings})
	if plan.Reason != spec.DeltaReasonDelta {
		return "", "", plan.Reason
	}
	checklist, err := spec.ReviewChecklistBody()
	if err != nil {
		return "", "", spec.DeltaReasonIntegrityGap
	}
	current, linked := specReviewDeltaDigests(fp, p, documents, baseline)
	sources := []spec.DeltaSource{{Name: "checklist", SHA256: fp.Checklist, Delivery: spec.DeltaDeliveryIncluded}}
	for _, name := range slices.Sorted(maps.Keys(current)) {
		if _, document := documents[name]; !document && name != "checklist" {
			sources = append(sources, spec.DeltaSource{Name: name, SHA256: current[name], Delivery: spec.DeltaDeliveryLinked})
		}
	}
	prompt, manifest := spec.BuildDeltaVerifyPrompt(spec.DeltaPromptInput{
		Doc: doc, Plan: plan, Baseline: baseline, Documents: documents, Sources: sources,
		PriorFindings: priorFindings, PassCriteria: p.gate.PassCriteria, Checklist: checklist,
	})
	if len(prompt) >= len(full) {
		return "", "", spec.DeltaReasonNoSaving
	}
	if err := spec.ValidateDeltaManifest(manifest, current, linked, prompt); err != nil {
		return "", "", spec.DeltaReasonIntegrityGap
	}
	return prompt, fmt.Sprintf("%s r%d", baseline.RunID, baseline.Revision), spec.DeltaReasonDelta
}

// specReviewDeltaDigests returns every required source's current digest and
// its baseline digest, keyed alike: SPEC documents by name, then checklist,
// code.context when a code context is injected, and reference:<path>.
func specReviewDeltaDigests(
	fp *specReviewFingerprint, p specReviewLoopParams, documents map[string]string, baseline *spec.ReviewBaseline,
) (map[string]string, map[string]string) {
	current, linked := map[string]string{}, map[string]string{}
	for name, content := range documents {
		current[name] = spec.SHA256Hex([]byte(content))
		linked[name] = baseline.Documents[name].SHA256
	}
	context := specReviewBaselineContext(fp, p.specDir)
	keys := []string{"checklist"}
	if p.codeContext != "" {
		keys = append(keys, "code.context")
	}
	for key := range context {
		if strings.HasPrefix(key, "reference:") {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		current[key], linked[key] = context[key], baseline.Context[key]
	}
	return current, linked
}

// specReviewBaselineContext flattens the round fingerprint's non-document
// components; references inside the SPEC directory are documents and are
// compared as such.
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
	return context
}

func specReviewJSONDigest(value any) string {
	data, _ := json.Marshal(value)
	return spec.SHA256Hex(data)
}

// recordSpecReviewBaseline stores what a usable, uncut round reviewed
// (REQ-RR-DELTA-02); a failure only costs the next round its delta input.
func recordSpecReviewBaseline(p specReviewLoopParams, revision int, documents map[string]string, fp *specReviewFingerprint, findings []spec.ReviewFinding, w io.Writer) {
	if fp == nil || documents == nil {
		return
	}
	runID := ""
	if p.runtimeEvidence != nil {
		runID = p.runtimeEvidence.RunID
	}
	baseline := &spec.ReviewBaseline{
		Schema: spec.ReviewBaselineSchema, RunID: runID, Revision: revision,
		RecordedAt: time.Now().UTC().Format(time.RFC3339), Documents: spec.BaselineDocuments(documents),
		Context: specReviewBaselineContext(fp, p.specDir), OpenIDs: spec.OpenFindingIDs(findings),
	}
	if err := spec.PersistReviewBaseline(p.specDir, baseline); err != nil {
		fmt.Fprintf(w, "경고: 리뷰 baseline 저장 실패: %v\n", err)
	}
}

func printSpecReviewRoundInput(w io.Writer, input specReviewRoundInput) {
	if input.mode == specReviewInputDelta {
		fmt.Fprintf(w, "SPEC 리뷰 입력: delta (baseline %s)\n", input.baseline)
		return
	}
	fmt.Fprintf(w, "SPEC 리뷰 입력: full — %s\n", input.reason)
}
