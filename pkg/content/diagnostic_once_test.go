package content

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrintDiagnosticOnce_DropsRepeatsKeepsDistinctLines(t *testing.T) {
	var buf bytes.Buffer
	first := "  [test-once] extended skills: 1 compatible, 0 platform-skipped"
	second := "  [test-once] extended skills: 2 compatible, 0 platform-skipped"

	printDiagnosticOnce(&buf, first)
	printDiagnosticOnce(&buf, first)
	printDiagnosticOnce(&buf, second)
	printDiagnosticOnce(&buf, "")

	assert.Equal(t, first+"\n"+second+"\n", buf.String())
}
