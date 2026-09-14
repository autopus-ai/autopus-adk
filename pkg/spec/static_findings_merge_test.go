package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func staticFinding(scope, id string, first, last int) ReviewFinding {
	return ReviewFinding{
		Provider: staticContractProvider, ScopeRef: scope, ID: id,
		Severity: "major", Description: "static: " + scope,
		Status: FindingStatusOpen, FirstSeenRev: first, LastSeenRev: last,
	}
}

func findingByID(t *testing.T, findings []ReviewFinding, id string) ReviewFinding {
	t.Helper()
	for _, finding := range findings {
		if finding.ID == id {
			return finding
		}
	}
	t.Fatalf("finding %s absent from %+v", id, findings)
	return ReviewFinding{}
}

// A static finding that survives a revision keeps the ID and first-seen
// revision reviewers already cited; only its last-seen revision advances.
// Re-issuing a new ID for the same scope would read as a new defect.
func TestMergeDeterministicFindings_ReusesPriorIDForSameScope(t *testing.T) {
	t.Parallel()

	prior := []ReviewFinding{staticFinding("REQ-001", "F-007", 2, 3)}
	deterministic := []ReviewFinding{{ScopeRef: "REQ-001", Severity: "major", Description: "still broken"}}

	merged := MergeDeterministicFindings(nil, deterministic, prior, 4)

	require.Len(t, merged, 1)
	assert.Equal(t, "F-007", merged[0].ID)
	assert.Equal(t, 2, merged[0].FirstSeenRev)
	assert.Equal(t, 4, merged[0].LastSeenRev)
	assert.Equal(t, FindingStatusOpen, merged[0].Status)
	assert.Equal(t, staticContractProvider, merged[0].Provider)
}

// A scope the static pass no longer reports is resolved, not dropped: a
// disappearing row would erase the evidence that it was ever raised.
func TestMergeDeterministicFindings_ResolvesScopesNoLongerReported(t *testing.T) {
	t.Parallel()

	prior := []ReviewFinding{
		staticFinding("REQ-001", "F-001", 1, 2),
		staticFinding("REQ-002", "F-002", 1, 2),
	}
	deterministic := []ReviewFinding{{ScopeRef: "REQ-002", Description: "unchanged"}}

	merged := MergeDeterministicFindings(nil, deterministic, prior, 3)

	require.Len(t, merged, 2)
	gone := findingByID(t, merged, "F-001")
	assert.Equal(t, FindingStatusResolved, gone.Status)
	assert.Equal(t, 3, gone.LastSeenRev)
	assert.Equal(t, FindingStatusOpen, findingByID(t, merged, "F-002").Status)
}

// The static pass owns its provider slot: the previous revision's static rows
// are rebuilt from `deterministic`, so carrying the current ones through would
// duplicate every finding. Provider findings in the same list must survive.
func TestMergeDeterministicFindings_DropsCurrentStaticRowsAndKeepsProviderRows(t *testing.T) {
	t.Parallel()

	current := []ReviewFinding{
		staticFinding("REQ-001", "F-001", 1, 1),
		{Provider: "claude", ID: "F-002", ScopeRef: "REQ-003", Status: FindingStatusOpen},
	}

	merged := MergeDeterministicFindings(current, nil, nil, 2)

	require.Len(t, merged, 1)
	assert.Equal(t, "claude", merged[0].Provider)
	assert.Equal(t, "F-002", merged[0].ID)
}

// New IDs continue past the highest ID in either list, so a static finding
// never reuses an ID a provider finding already published.
func TestMergeDeterministicFindings_AllocatesIDsPastEveryKnownID(t *testing.T) {
	t.Parallel()

	current := []ReviewFinding{{Provider: "codex", ID: "F-009", ScopeRef: "REQ-009"}}
	prior := []ReviewFinding{staticFinding("REQ-004", "F-012", 1, 1)}
	deterministic := []ReviewFinding{
		{ScopeRef: "REQ-020", Description: "first new"},
		{ScopeRef: "REQ-021", Description: "second new"},
	}

	merged := MergeDeterministicFindings(current, deterministic, prior, 5)

	ids := map[string]string{}
	for _, finding := range merged {
		ids[finding.ScopeRef] = finding.ID
	}
	assert.Equal(t, "F-013", ids["REQ-020"])
	assert.Equal(t, "F-014", ids["REQ-021"])
	// REQ-004 vanished from the static pass, so it is carried as resolved.
	assert.Equal(t, FindingStatusResolved, findingByID(t, merged, "F-012").Status)
}

// A malformed or absent ID must not stall allocation at F-001 forever, and it
// must not be parsed as a number either.
func TestMergeDeterministicFindings_IgnoresMalformedIDsWhenAllocating(t *testing.T) {
	t.Parallel()

	prior := []ReviewFinding{
		staticFinding("REQ-001", "", 1, 1),
		staticFinding("REQ-002", "F-abc", 1, 1),
		staticFinding("REQ-003", "F-0005", 1, 1),
	}
	deterministic := []ReviewFinding{{ScopeRef: "REQ-010", Description: "new"}}

	merged := MergeDeterministicFindings(nil, deterministic, prior, 2)

	for _, finding := range merged {
		if finding.ScopeRef == "REQ-010" {
			assert.Equal(t, "F-006", finding.ID)
			return
		}
	}
	t.Fatal("new deterministic finding missing from merge")
}
