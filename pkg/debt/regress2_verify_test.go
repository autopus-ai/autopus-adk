package debt

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func rv2Block(name string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "\t%s%d := transformRecordWithOptions(records[%d], settings.Precision, settings.Rounding)\n", name, i, i)
	}
	return b.String()
}

// R3a: a NEW copy-paste between two files that already share a baseline
// duplicate is accepted because the place set matches.
func TestDebtRegression2_NewCopyBetweenFilesThatShareADuplicate(t *testing.T) {
	old := longBlock(8)
	root := rvRepo(t, map[string]string{
		"a.go": "package a\n\nfunc A() {\n" + old + "}\n",
		"b.go": "package a\n\nfunc B() {\n" + old + "}\n",
	})
	rvBaseline(t, root)
	fresh := rv2Block("value", 30) // a different, 30-line block copied into both files
	write(t, root, "a.go", "package a\n\nfunc A() {\n"+old+"}\n\nfunc A2() {\n"+fresh+"}\n")
	write(t, root, "b.go", "package a\n\nfunc B() {\n"+old+"}\n\nfunc B2() {\n"+fresh+"}\n")
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	for _, it := range items {
		if it.Kind == KindDuplicate {
			t.Logf("duplicate in tree: %s", it)
		}
	}
	got := rvNew(t, root)
	t.Logf("new blocking items: %v", got)
	if len(got) == 0 {
		t.Errorf("a new 30-line copy between a.go and b.go is not reported: they already shared a baseline duplicate")
	}
}

// R3b: the same inside one file.
func TestDebtRegression2_NewSameFileCopyInAFileThatHasOne(t *testing.T) {
	old := longBlock(8)
	root := rvRepo(t, map[string]string{
		"a.go": "package a\n\nfunc A() {\n" + old + "}\n\nfunc A1() {\n" + old + "}\n",
	})
	rvBaseline(t, root)
	fresh := rv2Block("value", 30)
	write(t, root, "a.go", "package a\n\nfunc A() {\n"+old+"}\n\nfunc A1() {\n"+old+"}\n\nfunc A2() {\n"+fresh+"}\n\nfunc A3() {\n"+fresh+"}\n")
	got := rvNew(t, root)
	t.Logf("new blocking items: %v", got)
	if len(got) == 0 {
		t.Errorf("a new 30-line repeat inside a.go is not reported: a.go already had a same-file duplicate")
	}
}

// R5: an apostrophe in a string before the comment hides a real TODO.
func TestDebtRegression2_ApostropheHidesMarker(t *testing.T) {
	for _, line := range []string{
		"fmt.Println(\"don't panic\") // TODO remove debug print",
		"log.Printf(\"can't open %s\", p) // FIXME wrap the error",
		"msg = \"it's done\"  # TODO localize",
	} {
		file := "a.go"
		if strings.Contains(line, "# TODO") {
			file = "a.py"
		}
		t.Logf("%-60q -> markers=%d", line, len(markers(file, []string{line})))
	}
}

// R7: multi-line import lists in TS/Python/Rust still count as duplicates.
func TestDebtRegression2_MultiLineImportsOtherLanguages(t *testing.T) {
	ts := "import {\n  CustomerAccountService,\n  CustomerBillingService,\n  CustomerProfileService,\n  CustomerSessionService,\n  CustomerSettingsService,\n  CustomerSupportService,\n  CustomerTelemetryService,\n} from '../services';\n\n"
	root := rvRepo(t, map[string]string{
		"x.ts": ts + "export const x = 1;\n",
		"y.ts": ts + "export const y = 2;\n",
	})
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	for _, it := range items {
		t.Logf("%s", it)
	}
}
