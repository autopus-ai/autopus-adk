package cli

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// updateComposeTableGolden rewrites the table golden files. Inspect the diff
// against the SPEC cell values before committing a rewrite.
var updateComposeTableGolden = flag.Bool("update-compose-table", false,
	"rewrite testdata/quality_compose golden files")

// assertComposeTableGolden compares rendered bytes with one golden file.
func assertComposeTableGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(composeTableTestdata, name)
	if *updateComposeTableGolden {
		require.NoError(t, os.WriteFile(path, got, 0o644))
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden %s; run with -update-compose-table", path)
	assert.Equal(t, string(want), string(got), path)
}

// Golden tables (REQ-MUX-05, REQ-MUX-18, REQ-MUX-19): every catalog fixture
// against D and R, and CAT-A against each heterogeneous acceptance fixture.
func TestComposeTable_Golden(t *testing.T) {
	cases := []struct{ fixture, catalog string }{
		{"D", "CAT-A"}, {"D", "CAT-B"}, {"D", "CAT-C"}, {"D", "CAT-D"},
		{"R", "CAT-A"}, {"R", "CAT-B"}, {"R", "CAT-C"}, {"R", "CAT-D"},
		{"R2", "CAT-A"}, {"B2", "CAT-A"}, {"H", "CAT-A"}, {"P", "CAT-A"},
		{"O1", "CAT-A"}, {"O2", "CAT-A"}, {"X", "CAT-A"}, {"C", "CAT-A"}, {"K", "CAT-A"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture+"/"+tc.catalog, func(t *testing.T) {
			var out bytes.Buffer
			require.NoError(t, composeTableFor(t, tc.fixture, tc.catalog).render(&out, "ko", nil))
			assertComposeTableGolden(t, "table-"+tc.fixture+"-"+tc.catalog+".golden", out.Bytes())
		})
	}
}

// Golden preview (S6): D's after-table for the role-group custom, marked
// against D's own table under the same snapshot.
func TestComposeTable_GoldenPreview(t *testing.T) {
	before := composeTableFor(t, "D", "CAT-A")
	var out bytes.Buffer
	require.NoError(t, composeTableFor(t, "K", "CAT-A").render(&out, "ko", &before))
	assertComposeTableGolden(t, "preview-D-K-CAT-A.golden", out.Bytes())
}

// Golden English table (REQ-MUX-22).
func TestComposeTable_GoldenEnglish(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, composeTableFor(t, "R", "CAT-A").render(&out, "en", nil))
	assertComposeTableGolden(t, "table-R-CAT-A.en.golden", out.Bytes())
}
