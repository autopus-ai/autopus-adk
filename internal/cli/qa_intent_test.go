package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/intent"
)

func TestReadQAIntentRequest_TakesExactlyOneSource(t *testing.T) {
	file := filepath.Join(t.TempDir(), "request.md")
	require.NoError(t, os.WriteFile(file, []byte("from the file\n"), 0o644))

	got, err := readQAIntentRequest(strings.NewReader(""), qaIntentOptions{Request: "inline"})
	require.NoError(t, err)
	assert.Equal(t, "inline", got)

	got, err = readQAIntentRequest(strings.NewReader(""), qaIntentOptions{RequestFile: file})
	require.NoError(t, err)
	assert.Equal(t, "from the file\n", got)

	got, err = readQAIntentRequest(strings.NewReader("from stdin"), qaIntentOptions{RequestFile: "-"})
	require.NoError(t, err)
	assert.Equal(t, "from stdin", got)

	for _, opts := range []qaIntentOptions{{}, {Request: "  "}, {Request: "a", RequestFile: file}} {
		_, err = readQAIntentRequest(strings.NewReader(""), opts)
		assert.Equal(t, intent.CodeRequestMissing, intent.ErrorCode(err), "%+v", opts)
	}
}

// A missing request fails before any agent CLI is looked up or started.
func TestQAIntentCmd_MissingRequestFailsAsJSON(t *testing.T) {
	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetArgs([]string{"qa", "intent", "--project-dir", t.TempDir(), "--format", "json"})
	_ = root.Execute()
	assert.Contains(t, out.String(), intent.CodeRequestMissing)
}
