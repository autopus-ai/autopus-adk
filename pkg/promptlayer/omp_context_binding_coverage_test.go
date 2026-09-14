package promptlayer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The binding receipt is the identity the whole promotion chain hangs on, so
// every malformed identity, document, or history row must fail closed.

func copyDeliveryDocuments(delivery ContextDeliveryResult) ContextDeliveryResult {
	delivery.RequiredDocuments = append([]ContextDeliveryDocument(nil), delivery.RequiredDocuments...)
	return delivery
}

func TestBuildOMPContextBinding_RejectsMalformedIdentityFields(t *testing.T) {
	t.Parallel()
	_, opts, delivery := buildOMPContextFixture(t)

	tests := map[string]func(*OMPContextBindingInput){
		"blank workspace":    func(input *OMPContextBindingInput) { input.WorkspaceID = "  " },
		"blank spec":         func(input *OMPContextBindingInput) { input.SpecID = "" },
		"blank task":         func(input *OMPContextBindingInput) { input.TaskID = "" },
		"control char phase": func(input *OMPContextBindingInput) { input.Phase = "go\nrm -rf" },
		"oversized session":  func(input *OMPContextBindingInput) { input.SessionID = strings.Repeat("s", 10_000) },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			input := validOMPContextBindingInput(opts, delivery)
			mutate(&input)
			_, err := BuildOMPContextBinding(input)
			require.Error(t, err)
		})
	}
}

func TestBuildOMPContextBinding_RejectsDeliveryThatDoesNotMatchItsOptions(t *testing.T) {
	t.Parallel()
	_, opts, delivery := buildOMPContextFixture(t)

	input := validOMPContextBindingInput(opts, delivery)
	input.DeliveryOptions.Command = "review"
	_, err := BuildOMPContextBinding(input)
	require.ErrorContains(t, err, "verify OMP context delivery")

	input = validOMPContextBindingInput(opts, delivery)
	input.Delivery = copyDeliveryDocuments(delivery)
	input.Delivery.SnapshotHash = canonicalHash([]byte("forged"))
	_, err = BuildOMPContextBinding(input)
	require.ErrorContains(t, err, "verify OMP context delivery")
}

func TestOMPFullDocumentReferences_RejectsDuplicateAndUnhashedDocuments(t *testing.T) {
	t.Parallel()
	_, _, delivery := buildOMPContextFixture(t)

	duplicated := copyDeliveryDocuments(delivery)
	duplicated.RequiredDocuments = append(duplicated.RequiredDocuments, duplicated.RequiredDocuments[0])
	_, err := ompFullDocumentReferences(duplicated)
	require.ErrorContains(t, err, "duplicate")

	unhashed := copyDeliveryDocuments(delivery)
	unhashed.RequiredDocuments[0].SourceHash = "not-a-hash"
	_, err = ompFullDocumentReferences(unhashed)
	require.ErrorContains(t, err, "invalid OMP context document hash")

	promptless := copyDeliveryDocuments(delivery)
	promptless.RequiredDocuments[0].PromptHash = ""
	_, err = ompFullDocumentReferences(promptless)
	require.ErrorContains(t, err, "invalid OMP context document hash")

	escaping := copyDeliveryDocuments(delivery)
	escaping.RequiredDocuments[0].SourceRef = "../outside.md"
	_, err = ompFullDocumentReferences(escaping)
	require.ErrorContains(t, err, "invalid or duplicate")
}

func TestBuildOMPContextBinding_RejectsDocumentEphemeralCollision(t *testing.T) {
	t.Parallel()
	_, opts, delivery := buildOMPContextFixture(t)

	input := validOMPContextBindingInput(opts, delivery)
	input.Delivery = copyDeliveryDocuments(delivery)
	// An ephemeral body ID reused as a document ref would make the two sets
	// indistinguishable inside the receipt.
	input.Delivery.RequiredDocuments[0].SourceRef = "original_task"
	_, err := BuildOMPContextBinding(input)
	require.Error(t, err)
}

func TestOMPEligibleHistoryReferences_ValidatesRefsAndOrdersDeterministically(t *testing.T) {
	t.Parallel()

	refs, err := ompEligibleHistoryReferences([]OMPContextHistoryRow{
		{ID: "zeta", SourceRef: "tool/read-z", Body: "z body", Completed: true, Superseded: true},
		{ID: "alpha", SourceRef: "tool/read-a", Body: "a body", Completed: true, Superseded: true},
	})
	require.NoError(t, err)
	require.Len(t, refs, 2)
	assert.Equal(t, "alpha", refs[0].ID, "history refs are sorted by ID")
	assert.Equal(t, "zeta", refs[1].ID)
	assertCanonicalOMPHash(t, refs[0].BodyHash)

	_, err = ompEligibleHistoryReferences([]OMPContextHistoryRow{
		{ID: "abs", SourceRef: "/etc/passwd", Body: "body", Completed: true, Superseded: true},
	})
	require.ErrorContains(t, err, "invalid OMP history source ref")

	_, err = ompEligibleHistoryReferences([]OMPContextHistoryRow{
		{ID: "blank-ref", SourceRef: "  ", Body: "body", Completed: true, Superseded: true},
	})
	require.ErrorContains(t, err, "invalid OMP history source ref")

	_, err = ompEligibleHistoryReferences([]OMPContextHistoryRow{
		{ID: "dup", SourceRef: "tool/read-1", Body: "one", Completed: true, Superseded: true},
		{ID: "dup", SourceRef: "tool/read-2", Body: "two", Completed: true, Superseded: true},
	})
	require.ErrorContains(t, err, "invalid or duplicate OMP history row")
}
