package omp

// Model tokens from an installed catalog reach selectors, the generated OMP
// config, and the terminal, so no rune that can move the cursor, reorder
// the text, or hide itself may pass: C0 and C1 controls, format characters
// such as bidi overrides and zero-width spaces, and space separators.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSafeOMPModelToken_RejectsControlFormatAndSpaceRunes(t *testing.T) {
	for _, r := range []rune{0x09, 0x7f, 0x85, 0x9b, 0x200b, 0x200e, 0x202e, 0x2066, 0xfeff, 0x00a0, 0x2028, 0x3000} {
		assert.False(t, safeOMPModelToken("gpt-6"+string(r)+"astra"), "%U", r)
	}
	for _, token := range []string{"claude-fable-5-1", "gpt-6.1-sol", "model_v2+beta"} {
		assert.True(t, safeOMPModelToken(token), token)
	}
}

// A catalog entry carrying a bidi override in its id is not normalized.
func TestNormalizeOMPModelCatalog_RefusesAFormatCharacterInAnID(t *testing.T) {
	body := `{"models":[{"provider":"openai-codex","id":"gpt-6` + string(rune(0x202e)) + `astra",` +
		`"family":"openai","capabilities":["deep_reasoning"],"thinking":["high"],"auth_enabled":true}]}`

	_, reason := NormalizeOMPModelCatalog([]byte(body), 1<<20)

	assert.Equal(t, "catalog_invalid", reason)
}
