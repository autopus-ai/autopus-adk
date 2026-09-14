package promptlayer_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
)

// Promotion attestation is the fail-closed gate in front of history promotion:
// every malformed subject, policy, validity window, or clock must reject.

func attestationInput(checkedAt time.Time) promptlayer.OMPContextPromotionAttestationInputV1 {
	return promptlayer.OMPContextPromotionAttestationInputV1{
		Subject:   evidenceSubject(),
		Policy:    evidenceStorePolicy(),
		Rows:      promotionRows(),
		CheckedAt: checkedAt,
		ValidFor:  time.Hour,
	}
}

func TestBuildOMPContextPromotionAttestation_RejectsInvalidSubjectPolicyAndWindow(t *testing.T) {
	t.Parallel()
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	cases := map[string]func(*promptlayer.OMPContextPromotionAttestationInputV1){
		"blank session":       func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.Subject.SessionID = "" },
		"bad binding hash":    func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.Subject.BindingHash = "sha256:zz" },
		"shadow history mode": func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.Policy.HistoryMode = "shadow" },
		"zero target tokens":  func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.Policy.HistoryTargetTokens = 0 },
		"blank fallback":      func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.Policy.Fallback = "" },
		"zero checked at":     func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.CheckedAt = time.Time{} },
		"negative validity":   func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.ValidFor = -time.Second },
		"validity beyond ttl": func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.ValidFor = 25 * time.Hour },
		"no canary rows":      func(in *promptlayer.OMPContextPromotionAttestationInputV1) { in.Rows = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := attestationInput(checkedAt)
			mutate(&input)
			attestation, err := promptlayer.BuildOMPContextPromotionAttestationV1(input)
			require.ErrorIs(t, err, promptlayer.ErrOMPContextPromotionEvidenceRejected)
			assert.True(t, attestation.IsZero())
		})
	}
}

func TestOMPContextPromotionAttestation_DigestsBindSubjectPolicyAndRows(t *testing.T) {
	t.Parallel()
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)

	base, err := promptlayer.BuildOMPContextPromotionAttestationV1(attestationInput(checkedAt))
	require.NoError(t, err)
	assert.False(t, base.IsZero())
	assert.Equal(t, checkedAt, base.CheckedAt())
	assert.Equal(t, checkedAt.Add(time.Hour), base.ExpiresAt())
	for _, digest := range []string{base.Digest(), base.CanaryDigest(), base.PolicyDigest()} {
		assertCanonicalSHA256(t, digest)
	}

	// Row order is canonicalized, so a shuffled but identical row set attests the same.
	shuffledInput := attestationInput(checkedAt)
	rows := shuffledInput.Rows
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	shuffled, err := promptlayer.BuildOMPContextPromotionAttestationV1(shuffledInput)
	require.NoError(t, err)
	assert.Equal(t, base.CanaryDigest(), shuffled.CanaryDigest())
	assert.Equal(t, base.Digest(), shuffled.Digest())

	otherSubject := attestationInput(checkedAt)
	otherSubject.Subject.TaskID = "T6"
	changed, err := promptlayer.BuildOMPContextPromotionAttestationV1(otherSubject)
	require.NoError(t, err)
	assert.Equal(t, base.PolicyDigest(), changed.PolicyDigest())
	assert.NotEqual(t, base.Digest(), changed.Digest())
}

func TestVerifyOMPContextPromotionAttestation_DistinguishesUnavailableStaleAndMismatch(t *testing.T) {
	t.Parallel()
	checkedAt := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	subject := evidenceSubject()
	policy := evidenceStorePolicy()
	rows := promotionRows()

	attestation, err := promptlayer.BuildOMPContextPromotionAttestationV1(attestationInput(checkedAt))
	require.NoError(t, err)

	require.NoError(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, subject, policy, rows, checkedAt.Add(30*time.Minute)))

	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		promptlayer.OMPContextPromotionAttestationV1{}, subject, policy, rows, checkedAt),
		promptlayer.ErrOMPContextPromotionAttestationUnavailable)

	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, subject, policy, rows, time.Time{}),
		promptlayer.ErrOMPContextPromotionAttestationMismatch)

	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, subject, policy, rows, checkedAt.Add(-6*time.Minute)),
		promptlayer.ErrOMPContextPromotionAttestationMismatch)

	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, subject, policy, rows, checkedAt.Add(time.Hour)),
		promptlayer.ErrOMPContextPromotionAttestationStale)

	foreign := subject
	foreign.SessionID = "session-2"
	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, foreign, policy, rows, checkedAt.Add(time.Minute)),
		promptlayer.ErrOMPContextPromotionAttestationMismatch)

	tamperedRows := append([]promptlayer.OMPContextCanaryRowV1(nil), rows...)
	tamperedRows[0].Tokens += 1
	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, subject, policy, tamperedRows, checkedAt.Add(time.Minute)),
		promptlayer.ErrOMPContextPromotionAttestationMismatch)

	// A structurally invalid re-derivation surfaces the rejection, not a mismatch.
	brokenPolicy := policy
	brokenPolicy.MutationScope = ""
	require.ErrorIs(t, promptlayer.VerifyOMPContextPromotionAttestationV1(
		attestation, subject, brokenPolicy, rows, checkedAt.Add(time.Minute)),
		promptlayer.ErrOMPContextPromotionEvidenceRejected)
}

func TestOMPContextPromotionPolicyDigest_RejectsInvalidPolicy(t *testing.T) {
	t.Parallel()

	digest, err := promptlayer.OMPContextPromotionPolicyDigestV1(evidenceStorePolicy())
	require.NoError(t, err)
	assertCanonicalSHA256(t, digest)

	invalid := evidenceStorePolicy()
	invalid.Profile = strings.Repeat("p", 10_000)
	_, err = promptlayer.OMPContextPromotionPolicyDigestV1(invalid)
	require.ErrorIs(t, err, promptlayer.ErrOMPContextPromotionEvidenceRejected)
}
