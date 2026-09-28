package lore_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/lore"
)

func TestValidate_RejectsForbiddenTrailerCaseInsensitively(t *testing.T) {
	t.Parallel()

	config := lore.LoreConfig{ForbiddenTrailers: []string{"Co-Authored-By"}}
	for _, trailer := range []string{
		"Co-Authored-By: Claude <noreply@anthropic.com>",
		"Co-authored-by: Codex <codex@example.com>",
	} {
		errs := lore.Validate("feat: x\n\nConstraint: y\n\n"+trailer+"\n", config)
		require.Len(t, errs, 1, trailer)
		assert.Equal(t, "Co-Authored-By", errs[0].Field)
	}
}

func TestValidate_ForbiddenTrailerReportedOncePerKey(t *testing.T) {
	t.Parallel()

	config := lore.LoreConfig{ForbiddenTrailers: []string{"Co-Authored-By"}}
	msg := "feat: x\n\nCo-Authored-By: A <a@x>\nCo-Authored-By: B <b@x>\n"
	assert.Len(t, lore.Validate(msg, config), 1)
}

func TestValidate_ForbiddenTrailerIgnoresSubjectAndProse(t *testing.T) {
	t.Parallel()

	config := lore.LoreConfig{ForbiddenTrailers: []string{"Co-Authored-By"}}
	msg := "Co-Authored-By: looks like a trailer but is the subject\n\n" +
		"The hook now rejects a Co-Authored-By: line in the trailer block.\n"
	assert.Empty(t, lore.Validate(msg, config))
}

func TestValidate_EmptyForbiddenListAllowsTrailer(t *testing.T) {
	t.Parallel()

	msg := "feat: x\n\nCo-Authored-By: Claude <noreply@anthropic.com>\n"
	assert.Empty(t, lore.Validate(msg, lore.LoreConfig{}))
	assert.Empty(t, lore.Validate(msg, lore.LoreConfig{ForbiddenTrailers: []string{" ", ""}}))
}
