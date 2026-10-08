package learn

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStoreWriter_VerbatimFields_RefuseWhatRedactionWouldChange pins review
// S2: severity, files, and packages are stored as given (files and packages
// feed the fingerprint), so the writer refuses a severity outside the enum
// and any list item redaction would change, and writes nothing.
func TestStoreWriter_VerbatimFields_RefuseWhatRedactionWouldChange(t *testing.T) {
	t.Parallel()
	ghp := "ghp_" + strings.Repeat("BCDF2468", 4) + "BCDF"
	cases := []struct {
		name   string
		opts   RecordOpts
		field  EvidenceField
		detail string
	}{
		{"unknown severity", RecordOpts{Pattern: "p", Severity: "urgent"}, FieldSeverity, DetailUnknownValue},
		{"token in files", RecordOpts{Pattern: "p", Files: []string{"pkg/a.go", ghp}}, FieldFiles, DetailNeedsRedaction},
		{"password in files", RecordOpts{Pattern: "p", Files: []string{"password=hunter2xyz"}}, FieldFiles, DetailNeedsRedaction},
		{"user path in files", RecordOpts{Pattern: "p", Files: []string{"/Users/alice/pkg/a.go"}}, FieldFiles, DetailNeedsRedaction},
		{"token in packages", RecordOpts{Pattern: "p", Packages: []string{"pkg/content", synthToken()}}, FieldPackages, DetailNeedsRedaction},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, path := newTestStore(t)

			err := store.AppendAtomic(EntryTypeFixPattern, tc.opts)

			var fieldErr *FieldError
			require.True(t, errors.As(err, &fieldErr), "want *FieldError, got %v", err)
			assert.Equal(t, tc.field, fieldErr.Field)
			assert.Equal(t, tc.detail, fieldErr.Detail)
			assert.Equal(t, "learning_field_invalid: "+string(tc.field)+": "+tc.detail, err.Error())
			assert.NoFileExists(t, path, "a refused entry writes no byte")
			assert.Equal(t, err, CheckRecordOpts(tc.opts), "the pre-check refuses the same way")
		})
	}
}

func TestStoreWriter_VerbatimFields_KeepAllowedValues(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	severities := []Severity{"", SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical}
	for _, severity := range severities {
		opts := RecordOpts{Pattern: "p", Severity: severity, Files: []string{"pkg/a.go", ""}, Packages: []string{"pkg/content"}}
		require.NoError(t, CheckRecordOpts(opts))
		require.NoError(t, store.AppendAtomic(EntryTypeGateFail, opts))
	}

	entries, err := store.Read()
	require.NoError(t, err)
	require.Len(t, entries, len(severities))
	for i, entry := range entries {
		assert.Equal(t, severities[i], entry.Severity)
		assert.Equal(t, []string{"pkg/a.go", ""}, entry.Files)
		assert.Equal(t, []string{"pkg/content"}, entry.Packages)
	}
	_, err = os.Stat(store.path)
	require.NoError(t, err)
}
