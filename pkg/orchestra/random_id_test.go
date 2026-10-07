package orchestra

import (
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

type failingSessionRandomReader struct{}

func (failingSessionRandomReader) Read(_ []byte) (int, error) {
	return 0, errors.New("injected random source failure")
}

func TestNewSessionID_HasAtLeast128BitsAndSafeAlphabet(t *testing.T) {
	t.Parallel()

	id := NewSessionID()
	assert.Regexp(t, regexp.MustCompile(`^orch-[0-9a-f]{32,}$`), id)
}

func TestNewSessionID_Unique(t *testing.T) {
	t.Parallel()

	id1 := NewSessionID()
	id2 := NewSessionID()
	assert.NotEqual(t, id1, id2)
}

func TestNewSessionID_RandomFailureFallbackRemainsUnique(t *testing.T) {
	t.Parallel()

	id1 := newSessionID(failingSessionRandomReader{})
	id2 := newSessionID(failingSessionRandomReader{})

	assert.Regexp(t, regexp.MustCompile(`^orch-[0-9a-f]{32,}$`), id1)
	assert.Regexp(t, regexp.MustCompile(`^orch-[0-9a-f]{32,}$`), id2)
	assert.NotEqual(t, id1, id2)
}

// TestRandomHex_UniqueAndLength verifies randomHex output properties.
func TestRandomHex_UniqueAndLength(t *testing.T) {
	t.Parallel()
	a := randomHex()
	b := randomHex()
	assert.Len(t, a, 8)
	assert.Len(t, b, 8)
	assert.NotEqual(t, a, b)
}
