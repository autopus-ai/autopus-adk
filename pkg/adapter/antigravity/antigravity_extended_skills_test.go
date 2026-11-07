package antigravity

import (
	"testing"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Extended Skills ---

func TestRenderExtendedSkills(t *testing.T) {
	t.Parallel()
	files, err := NewWithRoot(t.TempDir()).renderExtendedSkills(config.DefaultFullConfig("test"))
	require.NoError(t, err)
	assert.NotEmpty(t, files)
	for _, f := range files {
		assert.Contains(t, f.TargetPath, ".gemini/skills/autopus/")
		assert.Equal(t, adapter.OverwriteAlways, f.OverwritePolicy)
	}
}

func TestLogTransformReport_Nil(t *testing.T) {
	t.Parallel()
	logTransformReport(nil)
}

func TestLogTransformReport_WithData(t *testing.T) {
	t.Parallel()
	report := &pkgcontent.TransformReport{
		Platform:     "gemini",
		Compatible:   []string{"a", "b"},
		Incompatible: []string{"c"},
	}
	logTransformReport(report)
}
