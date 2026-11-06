package content

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readOpus5ContractFile(t *testing.T, root, relativePath string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatalf("read %s: %v", relativePath, err)
	}
	return string(data)
}

func assertOpus5ContractFragments(t *testing.T, relativePath, document string, fragments []string) {
	t.Helper()

	for _, fragment := range fragments {
		if !strings.Contains(document, fragment) {
			t.Errorf("%s missing Opus 5 contract fragment %q", relativePath, fragment)
		}
	}
}
