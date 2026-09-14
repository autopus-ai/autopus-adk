package omp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// YAML merge keys make skills ownership indeterminate: the effective mapping
// differs from the literal bytes the rewrite would edit.
func TestMergeOMPConfigDocument_RefusesMergeKeys(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ existing, want string }{
		"top level": {
			existing: "defaults: &d\n  enabled: true\n<<: *d\nskills:\n  enabled: true\n",
			want:     "top-level YAML merge key",
		},
		"inside skills": {
			existing: "defaults: &d\n  enabled: true\nskills:\n  <<: *d\n  other: true\n",
			want:     "skills YAML merge key",
		},
	} {
		t.Run(name, func(t *testing.T) {
			merged, err := mergeOMPConfigDocument(tc.existing)

			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
			assert.Empty(t, merged)
		})
	}
}

// Two customDirectories keys mean two candidate owners; picking one would drop
// the other silently.
func TestMergeOMPConfigDocument_RefusesDuplicateCustomDirectories(t *testing.T) {
	t.Parallel()

	existing := "skills:\n  customDirectories:\n    - a\n  customDirectories:\n    - b\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.Error(t, err)
	assert.ErrorContains(t, err, "customDirectories key가 중복")
	assert.Empty(t, merged)
}

// An indented managed span that also encloses a sibling skills key would delete
// that user key on regeneration.
func TestMergeOMPConfigDocument_RefusesIndentedSpanSwallowingSiblingSkillsKey(t *testing.T) {
	t.Parallel()

	existing := "skills:\n  " + markerBeginYml +
		"\n  customDirectories:\n    - a\n  enabled: true\n  " + markerEndYml + "\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.Error(t, err)
	assert.ErrorContains(t, err, "customDirectories 외 skills key를 포함합니다")
	assert.Empty(t, merged)
}

// An indented span beginning above the skills key is not a skills child span.
func TestMergeOMPConfigDocument_RefusesIndentedSpanAboveSkillsKey(t *testing.T) {
	t.Parallel()

	existing := "  " + markerBeginYml + "\nskills:\n  customDirectories:\n    - a\n  " + markerEndYml + "\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.Error(t, err)
	assert.ErrorContains(t, err, "skills direct child에 있지 않습니다")
	assert.Empty(t, merged)
}

// A root-level span whose skills subtree continues past the END marker would be
// truncated by regeneration.
func TestMergeOMPConfigDocument_RefusesRootSpanWithSubtreeSpillingOut(t *testing.T) {
	t.Parallel()

	existing := markerBeginYml + "\nskills:\n  customDirectories:\n    - a\n  enabled: true\n" +
		markerEndYml + "\nmodel: keep-me\n"

	merged, err := mergeOMPConfigDocument(existing)
	require.NoError(t, err, "a fully enclosed skills subtree is regenerable")
	assert.Equal(t, ompManagedRootSection+"model: keep-me\n", merged)

	spilling := markerBeginYml + "\nskills:\n  customDirectories:\n" + markerEndYml + "\n    - a\n"
	merged, err = mergeOMPConfigDocument(spilling)
	require.Error(t, err)
	assert.Empty(t, merged)
}
