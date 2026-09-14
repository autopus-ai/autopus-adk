package antigravity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func readGeneratedAntigravitySurface(t *testing.T, root, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	require.NoError(t, err, rel)
	return string(body)
}
