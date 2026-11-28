package debt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// A TODO in a legacy-encoded (CP949 / Latin-1) source file: the JSON
// baseline rewrites the invalid UTF-8 in its key, so it never matches again.
func TestDebtRegression_NonUTF8MarkerNeverBaselined(t *testing.T) {
	cp949 := string([]byte{0xC7, 0xD1, 0xB1, 0xDB}) // "한글" in CP949
	latin1 := string([]byte{'c', 'a', 'f', 0xE9})   // "café" in Latin-1
	root := rvRepo(t, map[string]string{
		"legacy.c":  "/* TODO " + cp949 + " encoding */\nint f(void) { return 0; }\n",
		"legacy2.c": "// FIXME " + latin1 + " menu\nint g(void) { return 0; }\n",
	})
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	_, _, err = WriteBaseline(root, items, true)
	require.NoError(t, err)
	base, err := LoadBaseline(root)
	require.NoError(t, err)
	fresh := New(items, base)
	for _, it := range fresh {
		t.Logf("still new right after `baseline --force`: %s", it)
	}
	_, _, err = WriteBaseline(root, items, false)
	t.Logf("plain `auto debt baseline` afterwards: err=%v", err)
	if len(fresh) > 0 {
		t.Errorf("%d marker(s) in non-UTF-8 files can never be recorded in the baseline", len(fresh))
	}
}

// A binary file with a product extension does not break the scan.
func TestDebtRegression_BinaryProductFile(t *testing.T) {
	bin := make([]byte, 4096)
	for i := range bin {
		bin[i] = byte(i * 7)
	}
	root := rvRepo(t, map[string]string{"blob.js": string(bin), "a.go": "package a\n"})
	_, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
}
