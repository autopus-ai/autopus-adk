package debt

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The cleanup flow: extracting the middle of a baseline duplicate in both
// copies leaves its head and tail repeated; the baseline rewrite refuses.
func TestDebtRegression_PartialExtractionRefusedByBaseline(t *testing.T) {
	body := longBlock(20)
	root := rvRepo(t, map[string]string{
		"b.go": "package a\n\nfunc B() {\n" + body + "}\n",
		"c.go": "package a\n\nfunc C() {\n" + body + "}\n",
	})
	rvBaseline(t, root)
	lines := strings.Split(body, "\n")
	extracted := append(append(append([]string{}, lines[:7]...), "\tshared(input7, input8, input9, input10, input11, input12)"), lines[13:]...)
	write(t, root, "b.go", "package a\n\nfunc B() {\n"+strings.Join(extracted, "\n")+"}\n")
	write(t, root, "c.go", "package a\n\nfunc C() {\n"+strings.Join(extracted, "\n")+"}\n")
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	added, _, err := WriteBaseline(root, items, false)
	t.Logf("baseline rewrite after a partial extraction: added=%d err=%v", added, err)
	base, _ := LoadBaseline(root)
	for _, it := range New(items, base) {
		t.Logf("  reported new: %s", it)
	}
}

// An identical marker added above an existing one: which line is named new?
func TestDebtRegression_IdenticalMarkerAddedAbove(t *testing.T) {
	root := rvRepo(t, map[string]string{"a.go": "package a\n\nfunc A() {}\n\n// TODO validate input\nfunc B() {}\n"})
	rvBaseline(t, root)
	write(t, root, "a.go", "package a\n\n// TODO validate input\nfunc A() {}\n\n// TODO validate input\nfunc B() {}\n")
	for _, it := range rvNew(t, root) {
		t.Logf("new marker reported at %s (the added one is at a.go:3)", it)
	}
}

// Periodic code: seven identical long lines report a duplicate of itself one line down.
func TestDebtRegression_SelfOverlap(t *testing.T) {
	line := "\tregistry.Register(newHandlerWithDefaults(defaultOptions, defaultLogger, defaultMetrics))\n"
	root := rvRepo(t, map[string]string{"r.go": "package a\n\nfunc R() {\n" + strings.Repeat(line, 7) + "}\n"})
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	for _, it := range items {
		t.Logf("%s", it)
	}
}

// Kubernetes-style license header inside /* */ without leading stars.
func TestDebtRegression_LicenseHeaderDuplicate(t *testing.T) {
	hdr := "/*\nCopyright 2024 The Example Authors.\n\nLicensed under the Apache License, Version 2.0 (the \"License\");\nyou may not use this file except in compliance with the License.\nYou may obtain a copy of the License at\n\n    http://www.apache.org/licenses/LICENSE-2.0\n\nUnless required by applicable law or agreed to in writing, software\ndistributed under the License is distributed on an \"AS IS\" BASIS,\nWITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.\nSee the License for the specific language governing permissions and\nlimitations under the License.\n*/\n\n"
	root := rvRepo(t, map[string]string{
		"a.go": hdr + "package a\n\nfunc A() {}\n",
		"b.go": hdr + "package a\n\nfunc B() {}\n",
	})
	items, err := Scan(context.Background(), root, 300)
	require.NoError(t, err)
	for _, it := range items {
		t.Logf("%s", it)
	}
}
