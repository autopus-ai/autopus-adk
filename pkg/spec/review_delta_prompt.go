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

// DeltaSource is one required source of a delta round and how it reaches the
// reviewers: its full text, its diff, or a link to the verified baseline.
type DeltaSource struct {
	Name     string
	SHA256   string
	Delivery string
}

// DeltaPromptInput is what the delta prompt is built from. Sources lists the
// required sources other than the SPEC documents (the checklist, the code
// context, the context-delivery references) with their delivery.
type DeltaPromptInput struct {
	Doc           *SpecDocument
	Plan          DeltaPlan
	Baseline      *ReviewBaseline
	Documents     map[string]string
	Sources       []DeltaSource
	PriorFindings []ReviewFinding
	PassCriteria  string
	Checklist     string
}

// BuildDeltaVerifyPrompt builds the spec_review_delta.v1 prompt: the changes,
// the sections they touch, the requirements and scenarios the open findings
// name, the evidence per finding, and the checklist, with every other source
// linked to the baseline by digest. It returns the manifest it declared.
func BuildDeltaVerifyPrompt(in DeltaPromptInput) (string, []DeltaSource) {
	var sb strings.Builder
	sb.WriteString("You are verifying the fixes made to a SPEC since its last review, to decide whether implementation can start from it: check the changes and their linked context for correctness, completeness, and feasibility only as far as they decide that.\n\n")
	fmt.Fprintf(&sb, "## SPEC: %s — %s\n\n", in.Doc.ID, in.Doc.Title)
	sb.WriteString("### Delta Verify Input (spec_review_delta.v1)\n\n")
	baselineRef := fmt.Sprintf("baseline %s r%d", in.Baseline.RunID, in.Baseline.Revision)
	fmt.Fprintf(&sb, "Baseline: run %s, revision %d, recorded %s. Every source marked linked below is byte-identical (SHA-256) to what that review read in full. Review the changes, the sections they touch, and the linked requirements and scenarios.\n\n",
		in.Baseline.RunID, in.Baseline.Revision, in.Baseline.RecordedAt)
	manifest := deltaManifest(in)
	sb.WriteString("#### Source Manifest\n\n")
	for _, source := range manifest {
		suffix := ""
		if source.Delivery == DeltaDeliveryLinked {
			suffix = " (" + baselineRef + ")"
		}
		fmt.Fprintf(&sb, "- %s sha256:%s %s%s\n", source.Name, source.SHA256, source.Delivery, suffix)
	}
	sb.WriteString("\n#### Changes\n\n")
	for _, change := range in.Plan.Changes {
		fmt.Fprintf(&sb, "##### %s\n\n```diff\n", change.Name)
		for _, hunk := range change.Hunks {
			sb.WriteString(hunk.Header + "\n" + strings.Join(hunk.Lines, "\n") + "\n")
		}
		sb.WriteString("```\n\n")
	}
	printed := map[string]bool{}
	sb.WriteString("#### Affected Sections\n\n")
	for _, change := range in.Plan.Changes {
		for _, start := range in.Plan.affectedUnits(change) {
			writeDeltaUnit(&sb, change.Name, in.Plan.current[change.Name], start, printed)
		}
	}
	writeLinkedUnits(&sb, in.Plan, printed)
	writeFixEvidence(&sb, in.Plan)
	InjectChecklistSection(&sb, in.Checklist, 0)
	writeReviewPolicySection(&sb, EffectiveReviewPolicy(in.Doc))
	buildVerifyInstructions(&sb, in.PriorFindings, in.PassCriteria, true)
	return sb.String(), manifest
}

func deltaManifest(in DeltaPromptInput) []DeltaSource {
	changed := map[string]bool{}
	for _, change := range in.Plan.Changes {
		changed[change.Name] = true
	}
	var manifest []DeltaSource
	for _, name := range slices.Sorted(maps.Keys(in.Documents)) {
		delivery := DeltaDeliveryLinked
		if changed[name] {
			delivery = DeltaDeliveryChanged
		}
		manifest = append(manifest, DeltaSource{Name: name, SHA256: SHA256Hex([]byte(in.Documents[name])), Delivery: delivery})
	}
	return append(manifest, in.Sources...)
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
	fmt.Fprintf(sb, "##### %s — %s\n\n%s\n", name, layout.unitLabel(start), layout.unitText(start))
}

// writeLinkedUnits writes every requirement block and scenario section the
// open findings' scope closures name, and the traceability rows naming them.
func writeLinkedUnits(sb *strings.Builder, plan DeltaPlan, printed map[string]bool) {
	ids := map[string]bool{}
	for _, scope := range plan.scopes {
		maps.Copy(ids, scope.ids)
	}
	sb.WriteString("#### Linked Requirements and Scenarios\n\n")
	for _, name := range slices.Sorted(maps.Keys(plan.current)) {
		layout := plan.current[name]
		for i := range layout.lines {
			if layout.unitStart[i] != i {
				continue
			}
			label := layout.unitLabel(i)
			if ids[label] || ids[strings.SplitN(label, ":", 2)[0]] {
				writeDeltaUnit(sb, name, layout, i, printed)
			}
		}
	}
	specLayout := plan.current["spec.md"]
	var rows []string
	for i, line := range specLayout.lines {
		if specLayout.h2[i] == "traceability matrix" && strings.HasPrefix(strings.TrimSpace(line), "|") && containsAnyID(line, idList(ids)) {
			rows = append(rows, line)
		}
	}
	if len(rows) > 0 {
		sb.WriteString("Traceability rows:\n\n" + strings.Join(rows, "\n") + "\n\n")
	}
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

// ValidateDeltaManifest proves the delta prompt accounts for every required
// source (REQ-RR-DELTA-05): each is declared with its current digest, a
// linked source is byte-identical to the baseline, a changed source carries
// its diff, and an included source (the checklist) carries its text.
func ValidateDeltaManifest(manifest []DeltaSource, current, baseline map[string]string, prompt string) error {
	declared := make(map[string]DeltaSource, len(manifest))
	for _, source := range manifest {
		declared[source.Name] = source
	}
	for _, name := range slices.Sorted(maps.Keys(current)) {
		source, ok := declared[name]
		switch {
		case !ok:
			return fmt.Errorf("delta manifest omits %s", name)
		case source.SHA256 != current[name]:
			return fmt.Errorf("delta manifest digest of %s is stale", name)
		case source.Delivery == DeltaDeliveryLinked && baseline[name] != current[name]:
			return fmt.Errorf("%s is linked but differs from the baseline", name)
		case source.Delivery == DeltaDeliveryChanged && !strings.Contains(prompt, "##### "+name+"\n\n```diff\n"):
			return fmt.Errorf("%s is changed but its diff is missing", name)
		case source.Delivery == DeltaDeliveryIncluded && (name != "checklist" || !strings.Contains(prompt, "## Quality Checklist\n\n")):
			return fmt.Errorf("%s is included but its text is missing", name)
		case source.Delivery != DeltaDeliveryLinked && source.Delivery != DeltaDeliveryChanged && source.Delivery != DeltaDeliveryIncluded:
			return fmt.Errorf("%s has unknown delivery %q", name, source.Delivery)
		}
	}
	return nil
}
