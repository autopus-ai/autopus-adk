package verifygate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// One line longer than any buffer must not hide the files after it.
func TestWalkAdded_LongLineDoesNotHideLaterFiles(t *testing.T) {
	long := strings.Repeat("x", 2<<20)
	diff := "diff --git a/big.js b/big.js\n--- a/big.js\n+++ b/big.js\n@@ -1 +1 @@\n+" + long + "\n" +
		"diff --git a/src/config.go b/src/config.go\n--- a/src/config.go\n+++ b/src/config.go\n@@ -2,0 +3 @@\n+token := \"" + fakeToken() + "\"\n"
	hits := diffSecrets([]byte(diff))
	assert.Len(t, hits, 1)
	if len(hits) == 1 {
		assert.Equal(t, "src/config.go", hits[0].File)
		assert.Equal(t, 3, hits[0].Line)
	}
}

func TestDiffPath_SpacesAndQuotes(t *testing.T) {
	assert.Equal(t, "src/my pkg/c.py", diffPath("b/src/my pkg/c.py\t"))
	assert.Equal(t, "src/a\tb.go", diffPath(`"b/src/a\tb.go"`))
	assert.Equal(t, "", diffPath("/dev/null"))
}
