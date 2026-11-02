package spec

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Deliveries of a delta manifest entry (SPEC-REVIEWREUSE-001 REQ-RR-DELTA-04).
const (
	DeltaDeliveryIncluded = "included"
	DeltaDeliveryChanged  = "changed"
	DeltaDeliveryLinked   = "linked"
)

// deltaSelfContainedNote tells the reviewer what the request is. Every
// provider request starts a fresh process with no conversation to restore,
// so a delta request can only ever be judged from its own text.
const deltaSelfContainedNote = "This request is complete in itself. You have no access to the baseline review or to any earlier conversation: every review request starts a fresh session. " +
	"It carries the changes since the baseline, the full current text of every section they touch, of every section that names a delta scope id, and of the boundary sections of every document, and every other required source in full. " +
	"A source marked linked is not sent: it is unchanged since the baseline round, which delivered it complete, and none of its sections that are not shown here names a delta scope id. " +
	"Judge only from this request. If a finding's status depends on text that is not here, keep the finding open and name the source you need in its reason.\n"

// DeltaSource is one required source of a delta round as the request
// declares it: the digest of its source, the digest of the text a request
// delivers for it, how it reaches the reviewer, why it is linked, and for an
// included source the delivered text itself.
type DeltaSource struct {
	Name         string
	SourceSHA256 string
	PromptSHA256 string
	Delivery     string
	Reason       string
	Text         string
}

// IncludedDeltaSource is a source other than a SPEC document that the
// request carries in full: the checklist, the code context, or a context
// delivery reference.
func IncludedDeltaSource(name, sourceSHA256, text string) DeltaSource {
	return DeltaSource{Name: name, SourceSHA256: sourceSHA256, PromptSHA256: SHA256Hex([]byte(text)), Delivery: DeltaDeliveryIncluded, Text: text}
}

// RequiredDeltaSources lists every required source of a delta round with its
// current digests: the carried SPEC documents, then sources.
func RequiredDeltaSources(documents map[string]ReviewInputDocument, sources []DeltaSource) []DeltaSource {
	var required []DeltaSource
	for _, name := range slices.Sorted(maps.Keys(documents)) {
		document := documents[name]
		required = append(required, DeltaSource{Name: name, SourceSHA256: document.SourceSHA256, PromptSHA256: document.PromptSHA256})
	}
	return append(required, sources...)
}

// DeltaPromptInput is what the delta prompt is built from. Sources are the
// required sources other than the SPEC documents, each included in full.
type DeltaPromptInput struct {
	Doc           *SpecDocument
	Plan          DeltaPlan
	Baseline      *ReviewBaseline
	Documents     map[string]ReviewInputDocument
	Sources       []DeltaSource
	PriorFindings []ReviewFinding
	PassCriteria  string
}

// BuildDeltaVerifyPrompt builds the spec_review_delta.v1 prompt, complete in
// itself for a reviewer that starts with nothing: the changes as a diff of the
// delivered representations, the full text of every unit they touch, of every
// unit naming a delta scope id, and of every boundary unit, the traceability
// rows, every other required source in full, the evidence per finding, and the
// verify instructions. It returns the manifest it declared.
func BuildDeltaVerifyPrompt(in DeltaPromptInput) (string, []DeltaSource) {
	var sb strings.Builder
	sb.WriteString("You are verifying the fixes made to a SPEC since its last review, to decide whether implementation can start from it: check the changes and the context below for correctness, completeness, and feasibility only as far as they decide that.\n\n")
	fmt.Fprintf(&sb, "## SPEC: %s — %s\n\n", in.Doc.ID, in.Doc.Title)
	sb.WriteString("### Delta Verify Input (spec_review_delta.v1)\n\n")
	baselineRef := fmt.Sprintf("baseline %s r%d", in.Baseline.RunID, in.Baseline.Revision)
	fmt.Fprintf(&sb, "Baseline: run %s, revision %d, recorded %s.\n", in.Baseline.RunID, in.Baseline.Revision, in.Baseline.RecordedAt)
	sb.WriteString(deltaSelfContainedNote)
	fmt.Fprintf(&sb, "Delta scope ids: %s\n\n", strings.Join(in.Plan.sortedIDs(), ", "))
	manifest := deltaManifest(in, baselineRef)
	sb.WriteString("#### Source Manifest\n\n")
	for _, source := range manifest {
		fmt.Fprintf(&sb, "- %s source_sha256:%s prompt_sha256:%s %s", source.Name, source.SourceSHA256, source.PromptSHA256, source.Delivery)
		if source.Reason != "" {
			fmt.Fprintf(&sb, " (%s)", source.Reason)
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n#### Changes\n\n")
	for _, change := range in.Plan.Changes {
		fmt.Fprintf(&sb, "##### %s\n\n```diff\n", change.Name)
		for _, hunk := range change.Hunks {
			sb.WriteString(hunk.Header + "\n" + strings.Join(hunk.Lines, "\n") + "\n")
		}
		sb.WriteString("```\n\n")
	}
	writeDeltaUnits(&sb, in.Plan, in.Sources)
	writeFixEvidence(&sb, in.Plan)
	checklistIncluded := false
	for _, source := range in.Sources {
		if source.Name == "checklist" {
			sb.WriteString("## Quality Checklist\n\n" + source.Text + "\n\n")
			checklistIncluded = true
		}
	}
	writeReviewPolicySection(&sb, EffectiveReviewPolicy(in.Doc))
	buildVerifyInstructions(&sb, in.PriorFindings, in.PassCriteria, checklistIncluded)
	return sb.String(), manifest
}

func deltaManifest(in DeltaPromptInput, baselineRef string) []DeltaSource {
	changed := map[string]bool{}
	for _, change := range in.Plan.Changes {
		changed[change.Name] = true
	}
	var manifest []DeltaSource
	for _, name := range slices.Sorted(maps.Keys(in.Documents)) {
		document := in.Documents[name]
		source := DeltaSource{Name: name, SourceSHA256: document.SourceSHA256, PromptSHA256: document.PromptSHA256, Delivery: DeltaDeliveryChanged}
		if !changed[name] {
			source.Delivery = DeltaDeliveryLinked
			source.Reason = baselineRef + ": unchanged and delivered complete in that round; every section of it that names a delta scope id is shown in this request"
		}
		manifest = append(manifest, source)
	}
	return append(manifest, in.Sources...)
}

// writeDeltaUnits writes the full text of every unit the request must carry,
// each once: the touched units, the boundary units, and the scope units with
// the traceability rows, then every included source but the checklist.
func writeDeltaUnits(sb *strings.Builder, plan DeltaPlan, sources []DeltaSource) {
	printed := map[string]bool{}
	names := slices.Sorted(maps.Keys(plan.current))
	sb.WriteString("#### Affected Sections\n\n")
	for _, change := range plan.Changes {
		for _, start := range plan.affectedUnits(change) {
			writeDeltaUnit(sb, change.Name, plan.current[change.Name], start, printed)
		}
	}
	sb.WriteString("#### Boundary Sections\n\n")
	for _, name := range names {
		for _, start := range plan.boundaryUnits(name) {
			writeDeltaUnit(sb, name, plan.current[name], start, printed)
		}
	}
	sb.WriteString("#### Scope Sections\n\n")
	for _, name := range names {
		for _, start := range plan.scopeUnits(name) {
			writeDeltaUnit(sb, name, plan.current[name], start, printed)
		}
	}
	if rows := plan.traceabilityRows(); len(rows) > 0 {
		sb.WriteString("Traceability rows:\n\n" + strings.Join(rows, "\n") + "\n\n")
	}
	sb.WriteString("#### Required Context\n\n")
	for _, source := range sources {
		switch source.Name {
		case "checklist":
		case "code.context":
			sb.WriteString("##### Existing Code Context\n\n```\n" + source.Text + "\n```\n\n")
		default:
			fmt.Fprintf(sb, "##### %s\n\n%s\n\n", source.Name, source.Text)
		}
	}
}

// affectedUnits are the current units of a change: those holding an added
// line, and for a removed line the current unit with the same label.
func (p DeltaPlan) affectedUnits(change DocumentChange) []int {
	current, baseline := p.current[change.Name], p.baseline[change.Name]
	var starts []int
	for _, hunk := range change.Hunks {
		for _, i := range hunk.Added {
			starts = append(starts, current.unitStart[i])
		}
		for _, i := range hunk.Removed {
			starts = append(starts, current.unitByLabel(baseline.unitLabel(i)))
		}
	}
	slices.Sort(starts)
	return slices.DeleteFunc(slices.Compact(starts), func(start int) bool { return start < 0 })
}

func (l docLayout) unitByLabel(label string) int {
	if label == "" {
		return -1
	}
	for i := range l.lines {
		if l.unitStart[i] == i && l.unitLabel(i) == label {
			return i
		}
	}
	return -1
}

func writeDeltaUnit(sb *strings.Builder, name string, layout docLayout, start int, printed map[string]bool) {
	key := fmt.Sprintf("%s\x00%d", name, start)
	if printed[key] {
		return
	}
	printed[key] = true
	fmt.Fprintf(sb, "##### %s — %s\n\n%s\n", name, layout.unitName(start), layout.unitBody(start))
}

// writeFixEvidence lists, per open finding, the hunks inside its scope.
func writeFixEvidence(sb *strings.Builder, plan DeltaPlan) {
	sb.WriteString("#### Fix Evidence\n\n")
	for _, scope := range plan.scopes {
		f := scope.finding
		fmt.Fprintf(sb, "- %s [%s] %s: %s\n", f.ID, f.Severity, f.ScopeRef, f.Description)
		touched := false
		for _, change := range plan.Changes {
			for _, hunk := range change.Hunks {
				if plan.hunkTouches(change.Name, hunk, scope) {
					fmt.Fprintf(sb, "  - %s %s\n", change.Name, hunk.Header)
					touched = true
				}
			}
		}
		if !touched {
			sb.WriteString("  - no change touches this finding\n")
		}
	}
	sb.WriteString("\n")
}

func (p DeltaPlan) hunkTouches(name string, hunk DiffHunk, scope findingScope) bool {
	for _, i := range hunk.Removed {
		if scope.covers(name, p.baseline[name], i) {
			return true
		}
	}
	for _, i := range hunk.Added {
		if scope.covers(name, p.current[name], i) {
			return true
		}
	}
	return false
}
