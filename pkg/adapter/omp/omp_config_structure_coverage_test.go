package omp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const ompManagedRootSection = markerBeginYml + "\nskills:\n  customDirectories:\n    - .agents/skills\n" + markerEndYml + "\n"

// An empty config gets the managed block verbatim with no leading separator.
func TestMergeOMPConfigDocument_EmptyDocumentGetsRootSection(t *testing.T) {
	t.Parallel()

	merged, err := mergeOMPConfigDocument("")

	require.NoError(t, err)
	assert.Equal(t, ompManagedRootSection, merged)
}

// User keys outside the managed block survive byte for byte, and the section is
// separated from them by exactly one blank line regardless of how the existing
// document terminated.
func TestMergeOMPConfigDocument_AppendsRootSectionWithStableSeparator(t *testing.T) {
	t.Parallel()

	tests := map[string]struct{ existing, want string }{
		"no trailing newline":      {"model: keep-me", "model: keep-me\n\n" + ompManagedRootSection},
		"one trailing newline":     {"model: keep-me\n", "model: keep-me\n\n" + ompManagedRootSection},
		"already blank terminated": {"model: keep-me\n\n", "model: keep-me\n\n" + ompManagedRootSection},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			merged, err := mergeOMPConfigDocument(tc.existing)

			require.NoError(t, err)
			assert.Equal(t, tc.want, merged)
		})
	}
}

// A CRLF document must stay CRLF; mixing line endings would corrupt the file on
// the next round trip.
func TestMergeOMPConfigDocument_PreservesCRLFLineEndings(t *testing.T) {
	t.Parallel()

	merged, err := mergeOMPConfigDocument("model: keep-me\r\n")

	require.NoError(t, err)
	assert.Contains(t, merged, markerBeginYml+"\r\n")
	assert.Contains(t, merged, "    - .agents/skills\r\n")
	assert.NotContains(t, strings.ReplaceAll(merged, "\r\n", ""), "\n", "no bare LF may leak into a CRLF document")
}

// With a pre-existing skills mapping the managed entry is nested inside it at
// the child indentation, and the user's sibling skills keys are untouched.
func TestMergeOMPConfigDocument_InsertsNestedSectionUnderExistingSkills(t *testing.T) {
	t.Parallel()

	existing := "skills:\n  enabled: true\nmodel: keep-me\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.NoError(t, err)
	assert.Equal(t,
		"skills:\n  enabled: true\n"+
			"  "+markerBeginYml+"\n  customDirectories:\n    - .agents/skills\n  "+markerEndYml+"\n"+
			"model: keep-me\n",
		merged)
}

// The insertion point is the end of the skills subtree even when it is the last
// key in the document.
func TestMergeOMPConfigDocument_InsertsNestedSectionAtDocumentTail(t *testing.T) {
	t.Parallel()

	merged, err := mergeOMPConfigDocument("model: keep-me\nskills:\n  enabled: true")

	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(merged, "model: keep-me\nskills:\n  enabled: true\n"))
	assert.True(t, strings.HasSuffix(merged, "  "+markerEndYml+"\n"))
}

// Comments and blank lines trailing the skills subtree belong to the user, so
// the managed block must be inserted before the next top-level key rather than
// after those lines.
func TestMergeOMPConfigDocument_NestedInsertStopsAtNextTopLevelKey(t *testing.T) {
	t.Parallel()

	existing := "skills:\n  enabled: true\n\n  # user note\nmodel: keep-me\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.NoError(t, err)
	assert.Contains(t, merged, "  # user note\n  "+markerBeginYml)
	assert.True(t, strings.HasSuffix(merged, "model: keep-me\n"))
}

// A flow-style or empty skills mapping has no safe insertion point; the merge
// must fail closed instead of guessing an indentation.
func TestMergeOMPConfigDocument_RefusesUninsertableSkillsMapping(t *testing.T) {
	t.Parallel()

	for name, existing := range map[string]string{
		"flow mapping":  "skills: {enabled: true}\n",
		"empty mapping": "skills: {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			merged, err := mergeOMPConfigDocument(existing)

			require.Error(t, err)
			assert.ErrorContains(t, err, "관리 entry를 삽입할 수 없습니다")
			assert.Empty(t, merged)
		})
	}
}

// A previously generated block is regenerated in place: bytes before and after
// the span are preserved and a second merge is a fixed point.
func TestMergeOMPConfigDocument_RegeneratesManagedSpanIdempotently(t *testing.T) {
	t.Parallel()

	existing := "model: keep-me\n\n" + markerBeginYml + "\nskills:\n  customDirectories:\n    - stale/path\n" + markerEndYml + "\n"

	merged, err := mergeOMPConfigDocument(existing)
	require.NoError(t, err)
	assert.Equal(t, "model: keep-me\n\n"+ompManagedRootSection, merged)
	assert.NotContains(t, merged, "stale/path")

	again, err := mergeOMPConfigDocument(merged)
	require.NoError(t, err)
	assert.Equal(t, merged, again, "regeneration must be a fixed point")
}

// A nested managed span is regenerated at its own indentation without
// disturbing the surrounding skills keys.
func TestMergeOMPConfigDocument_RegeneratesNestedManagedSpan(t *testing.T) {
	t.Parallel()

	existing := "skills:\n  enabled: true\n  " + markerBeginYml +
		"\n  customDirectories:\n    - stale/path\n  " + markerEndYml + "\nmodel: keep-me\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.NoError(t, err)
	assert.Equal(t,
		"skills:\n  enabled: true\n  "+markerBeginYml+
			"\n  customDirectories:\n    - .agents/skills\n  "+markerEndYml+"\nmodel: keep-me\n",
		merged)
}

// A document ending inside the managed span without a trailing newline must not
// gain one; that byte difference churns the file on every generate.
func TestMergeOMPConfigDocument_KeepsMissingTrailingNewlineAtSpanEnd(t *testing.T) {
	t.Parallel()

	existing := markerBeginYml + "\nskills:\n  customDirectories:\n    - stale/path\n" + markerEndYml

	merged, err := mergeOMPConfigDocument(existing)

	require.NoError(t, err)
	assert.Equal(t, strings.TrimSuffix(ompManagedRootSection, "\n"), merged)
}

// customDirectories living outside the managed markers is user-owned; rewriting
// it would silently take over a key Autopus never generated.
func TestMergeOMPConfigDocument_RefusesUnmanagedCustomDirectories(t *testing.T) {
	t.Parallel()

	merged, err := mergeOMPConfigDocument("skills:\n  customDirectories:\n    - mine/skills\n")

	require.Error(t, err)
	assert.ErrorContains(t, err, "관리 마커 밖에 있어")
	assert.Empty(t, merged)
}

// Ambiguous or malformed marker geometry must fail closed rather than pick a
// span and rewrite the wrong bytes.
func TestMergeOMPConfigDocument_RefusesAmbiguousMarkerGeometry(t *testing.T) {
	t.Parallel()

	managedBody := "skills:\n  customDirectories:\n    - .agents/skills\n"
	for name, existing := range map[string]string{
		"duplicate begin":            markerBeginYml + "\n" + markerBeginYml + "\n" + managedBody + markerEndYml + "\n",
		"end before begin":           markerEndYml + "\n" + managedBody + markerBeginYml + "\n",
		"begin without end":          markerBeginYml + "\n" + managedBody,
		"marker inline with content": "model: " + markerBeginYml + "-x\n" + managedBody + markerEndYml + "\n",
		"indentation mismatch":       markerBeginYml + "\n" + managedBody + "  " + markerEndYml + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			merged, err := mergeOMPConfigDocument(existing)

			require.Error(t, err)
			assert.Empty(t, merged)
		})
	}
}

// Markers wrapping nothing recognisable, or a span that does not actually own
// skills.customDirectories, must be rejected.
func TestMergeOMPConfigDocument_RefusesSpanWithoutManagedKeys(t *testing.T) {
	t.Parallel()

	merged, err := mergeOMPConfigDocument(markerBeginYml + "\nmodel: keep-me\n" + markerEndYml + "\n")

	require.Error(t, err)
	assert.ErrorContains(t, err, "관리 구간에 skills.customDirectories가 없습니다")
	assert.Empty(t, merged)
}

// A nested span whose indentation disagrees with customDirectories is not a
// span this generator produced; rewriting it would reindent user YAML.
func TestMergeOMPConfigDocument_RefusesNestedSpanIndentMismatch(t *testing.T) {
	t.Parallel()

	existing := "skills:\n    " + markerBeginYml + "\n  customDirectories:\n    - a\n    " + markerEndYml + "\n"

	merged, err := mergeOMPConfigDocument(existing)

	require.Error(t, err)
	assert.Empty(t, merged)
}

// Structural YAML the generator cannot reason about must abort the rewrite
// instead of producing a merged document with ambiguous ownership.
func TestMergeOMPConfigDocument_RefusesUnreasonableDocuments(t *testing.T) {
	t.Parallel()

	for name, existing := range map[string]string{
		"invalid yaml":         "skills: [unclosed\n",
		"multiple documents":   "skills:\n  enabled: true\n---\nmodel: keep-me\n",
		"top-level sequence":   "- one\n- two\n",
		"top-level scalar":     "just-a-string\n",
		"top-level merge key":  "<<: *anchor\nskills:\n  enabled: true\n",
		"duplicate skills key": "skills:\n  enabled: true\nskills:\n  other: true\n",
		"skills scalar value":  "skills: mine\n",
	} {
		t.Run(name, func(t *testing.T) {
			merged, err := mergeOMPConfigDocument(existing)

			require.Error(t, err)
			assert.Empty(t, merged)
		})
	}
}

// A tab-indented marker is not a legal indentation the generator can reproduce.
func TestMarkerIndent_RejectsTabIndentation(t *testing.T) {
	t.Parallel()

	indent, ok := markerIndent("  " + markerBeginYml + "\n")
	require.True(t, ok)
	assert.Equal(t, 2, indent)

	_, ok = markerIndent("\t" + markerBeginYml + "\n")
	assert.False(t, ok, "tab indentation has no reproducible width")
}

// Raw line spans must cover the whole input including a final line with no
// newline, otherwise the marker span would slice bytes off the document.
func TestSplitOMPRawLines_SpansCoverEveryByte(t *testing.T) {
	t.Parallel()

	assert.Nil(t, splitOMPRawLines(""))

	value := "a\nbb\nccc"
	lines := splitOMPRawLines(value)
	require.Len(t, lines, 3)
	assert.Equal(t, 0, lines[0].start)
	assert.Equal(t, len(value), lines[len(lines)-1].end)
	for i, line := range lines {
		assert.Equal(t, value[line.start:line.end], line.text)
		if i > 0 {
			assert.Equal(t, lines[i-1].end, line.start, "line spans must be contiguous")
		}
	}
}
