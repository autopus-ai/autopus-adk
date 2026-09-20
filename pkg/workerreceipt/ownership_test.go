package workerreceipt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMarkedOutputRejectsOwnershipMismatch(t *testing.T) {
	for _, owned := range [][]string{{}, {"pkg/worker"}, {"pkg/elsewhere"}} {
		value := validEnvelopeFixture()
		value.Receipt.OwnedPaths = owned
		got, err := ParseMarkedOutput(markedEnvelope(t, value))
		require.Error(t, err)
		require.Nil(t, got)
	}
}

func TestValidateOwnershipSupervisorBoundary(t *testing.T) {
	tests := []struct {
		name                     string
		owned, changed, assigned []string
		valid                    bool
	}{
		{"exact", []string{"pkg/a/x.go"}, []string{"pkg/a/x.go"}, []string{"pkg/a"}, true},
		{"subtree", []string{"pkg/a"}, []string{"pkg/a/deep/x.go"}, []string{"pkg"}, true},
		{"recursive declaration", []string{"pkg/a/**"}, []string{"pkg/a/x.go"}, []string{"pkg/a"}, true},
		{"sibling prefix", []string{"pkg/a"}, []string{"pkg/ab/x.go"}, nil, false},
		{"worker broadens grant", []string{"pkg"}, []string{"pkg/a/x.go"}, []string{"pkg/a"}, false},
		{"explicit read only", []string{"pkg/a"}, nil, []string{}, false},
		{"read only receipt", []string{}, []string{}, []string{}, true},
		{"invalid grant", []string{}, []string{}, []string{"../outside"}, false},
		{"unsupported glob", []string{"pkg/*"}, []string{"pkg/a/x.go"}, nil, false},
		{"changed glob", []string{"pkg/a"}, []string{"pkg/a/*.go"}, nil, false},
		{"unsafe direct input", []string{"pkg/a"}, []string{"pkg/a/../b/x.go"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateOwnership(Receipt{OwnedPaths: tt.owned, ChangedFiles: tt.changed}, tt.assigned)
			if tt.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
