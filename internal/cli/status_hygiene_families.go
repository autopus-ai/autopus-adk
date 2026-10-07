package cli

import (
	"fmt"
	"path"
	"strings"
)

// specEvidenceBasenames are the per-revision artifacts `auto review` and the
// self-verify gate write inside a SPEC directory. gitignorePatterns ignores
// exactly these three names under `**/.autopus/specs/**`, so any other file in
// a SPEC directory is canonical documentation that a broader rule swallowed.
var specEvidenceBasenames = map[string]bool{
	"review.md":            true,
	"review-findings.json": true,
	".self-verify.log":     true,
}

// trackedIgnoredLocalOnlyPrefixes is the "Brainstorm/runtime output ... Local
// working copy only / Do not commit" row of content/rules/doc-storage.md, plus
// the transaction log and the band metric store that hygieneAlwaysBlockPrefixes
// pairs with it. These paths are deliberately never committed, so a tracked one
// is stale index state rather than a rule that needs loosening.
var trackedIgnoredLocalOnlyPrefixes = []string{
	".autopus/brainstorms/",
	".autopus/metrics/",
	".autopus/orchestra/",
	".autopus/runtime/",
	".autopus/txns/",
}

// Dispositions say what to do with a family, because the two safe answers are
// opposites: generated output leaves the index, a canonical document keeps it
// and the rule gets narrowed instead.
const (
	trackedIgnoredUntrack  = "generated output; untrack after review"
	trackedIgnoredNarrow   = "canonical document; narrow the ignore rule, do not untrack"
	trackedIgnoredClassify = "unclassified; classify before any untrack"
)

const (
	familySpecDocument = iota
	familySpecEvidence
	familyLocalOnly
	familyGenerated
	familyUnclassified
)

// trackedIgnoredFamily groups tracked-but-ignored paths that share a single
// disposition.
//
// `auto doctor` used to print a bare count and the five alphabetically-first
// paths. On this workspace that rendered as "177 candidate(s)" followed by an
// audit log and four brainstorms, which tells an operator neither what the
// inventory is made of nor what to do about it, and hides the one case that
// must never be untracked. SPEC-ADK-GENERATED-HYGIENE-001 R13 requires SPEC
// review evidence to be classified apart from canonical SPEC documents before
// any untrack action, and its research notes require the inventory to be
// reviewed by family before any `git rm --cached`. Grouping is what makes both
// possible from the diagnostic alone.
type trackedIgnoredFamily struct {
	Label       string
	Disposition string
	Paths       []string
}

var trackedIgnoredFamilyTable = []trackedIgnoredFamily{
	{Label: "canonical SPEC document", Disposition: trackedIgnoredNarrow},
	{Label: "SPEC review evidence", Disposition: trackedIgnoredUntrack},
	{Label: "local-only brainstorm/orchestra/runtime output", Disposition: trackedIgnoredUntrack},
	{Label: "generated harness surface", Disposition: trackedIgnoredUntrack},
	{Label: "unclassified", Disposition: trackedIgnoredClassify},
}

// classifyTrackedIgnored partitions paths into families, preserving the input
// order inside each family and the table order across families. Every path
// lands in exactly one family so the printed counts sum to the total.
func classifyTrackedIgnored(paths []string) []trackedIgnoredFamily {
	buckets := make([][]string, len(trackedIgnoredFamilyTable))
	for _, rel := range paths {
		clean := normalizeGitRel(rel)
		if clean == "" {
			continue
		}
		idx := trackedIgnoredFamilyIndex(clean)
		buckets[idx] = append(buckets[idx], clean)
	}

	families := make([]trackedIgnoredFamily, 0, len(buckets))
	for idx, group := range buckets {
		if len(group) == 0 {
			continue
		}
		family := trackedIgnoredFamilyTable[idx]
		family.Paths = group
		families = append(families, family)
	}
	return families
}

// trackedIgnoredFamilyIndex is fail-closed inside a SPEC directory: only the
// three known evidence basenames are treated as disposable, so a new document
// type is reported as canonical rather than silently proposed for untracking.
func trackedIgnoredFamilyIndex(clean string) int {
	if dir, base := path.Split(clean); strings.Contains(dir, ".autopus/specs/") {
		if specEvidenceBasenames[base] {
			return familySpecEvidence
		}
		return familySpecDocument
	}
	switch {
	case hasAnyPrefix(clean, trackedIgnoredLocalOnlyPrefixes):
		return familyLocalOnly
	case isRuntimeUnignoredRisk(clean) || isGeneratedRuntime(clean):
		return familyGenerated
	}
	return familyUnclassified
}

// trackedIgnoredNeedsRuleFix reports whether any family must be repaired by
// narrowing .gitignore. Untracking is wrong for those paths, so the remediation
// line has to change rather than offer `git rm --cached` for the whole set.
func trackedIgnoredNeedsRuleFix(families []trackedIgnoredFamily) bool {
	for _, family := range families {
		if family.Disposition == trackedIgnoredNarrow {
			return true
		}
	}
	return false
}

func (f trackedIgnoredFamily) summary() string {
	return fmt.Sprintf("%s: %d file(s) — %s (e.g. %s)",
		f.Label, len(f.Paths), f.Disposition, f.Paths[0])
}

// trackedIgnoredRemediation names the command that resolves the inventory. The
// rule-fix case is deliberately first: on a repo where an ignore rule has
// swallowed a canonical document, running `git rm --cached` over the inventory
// deletes team knowledge from the next commit.
func trackedIgnoredRemediation(families []trackedIgnoredFamily) string {
	if trackedIgnoredNeedsRuleFix(families) {
		return "narrow the .gitignore rule covering the canonical document(s) first " +
			"(git check-ignore -v --no-index <path> names the rule), then untrack the rest"
	}
	return "review each family with 'git ls-files -c -i --exclude-standard', " +
		"then untrack per family with 'git rm --cached -- <paths>'"
}
