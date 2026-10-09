package healthband_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// RR-7 (Phase 4 security L3): Patch Policy item 6 also denies the allowed
// source paths that a build or an IDE sync runs, derived from the Cargo and
// Gradle manifests of the base tree, never from the patched one.

const (
	libLine  = "pub fn f() {}"
	mainLine = "fn main() {}"
	denied   = healthband.PatchCodePathDenied
)

// appendLine is a diff that adds one line after the only line of a tracked
// one-line file.
func appendLine(path, first, added string) string {
	return fmt.Sprintf("diff --git a/%[1]s b/%[1]s\n--- a/%[1]s\n+++ b/%[1]s\n@@ -1 +1,2 @@\n %[2]s\n+%[3]s\n", path, first, added)
}

// editLib appends a comment to a crate's tracked one-line src/lib.rs.
func editLib(dir string) string { return appendLine(dir+"/src/lib.rs", libLine, "// x") }

// crateFiles are a crate's manifest and its one-line library source.
func crateFiles(dir, manifest string) []baseFile {
	return []baseFile{
		{path: dir + "/Cargo.toml", content: manifest},
		{path: dir + "/src/lib.rs", content: libLine + "\n"},
	}
}

// crates builds the files of one crate per manifest, by directory.
func crates(manifests map[string]string) []baseFile {
	var files []baseFile
	for dir, manifest := range manifests {
		files = append(files, crateFiles(dir, manifest)...)
	}
	return files
}

type pathCase struct{ diff, code string }

// assertPathVerdicts evaluates each diff in repo and checks its code, ""
// for an accepted diff.
func assertPathVerdicts(t *testing.T, repo *policyRepo, cases map[string]pathCase) {
	t.Helper()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			verdict := evaluate(t, repo, healthband.PatchPolicy{}, replyWith(tc.diff))
			assert.Equal(t, tc.code, verdict.Code)
		})
	}
}

func TestPatchPolicy_ProcMacroCrate_DeniesItsDirectory(t *testing.T) {
	t.Parallel()
	files := []baseFile{{path: "Cargo.toml", content: "[workspace]\nmembers = [\"macros\", \"app\"]\n"}}
	files = append(files, crates(map[string]string{
		"macros": "[package]\nname = \"macros\"\nversion = \"0.1.0\"\n\n[lib]\nproc-macro = true\n",
		"app":    "[package]\nname = \"app\"\nversion = \"0.1.0\"\n",
	})...)
	assertPathVerdicts(t, newPolicyRepo(t, files), map[string]pathCase{
		"edit of the macro source":   {editLib("macros"), denied},
		"new macro module":           {newFile("macros/src/extra.rs", "100644", "pub fn g() {}"), denied},
		"new file anywhere below it": {newFile("macros/tests/t.rs", "100644", "fn t() {}"), denied},
		"edit of an ordinary crate":  {editLib("app"), ""},
	})
}

// Every TOML spelling of the [lib] keys that make a proc-macro crate counts,
// and a [lib] path outside the crate is denied with it.
func TestPatchPolicy_ProcMacroManifestForms_DenyTheCrate(t *testing.T) {
	t.Parallel()
	forms := map[string]string{
		"underscore":   "[lib]\nproc_macro = true\n",
		"crate-type":   "[lib]\ncrate-type = [\"proc-macro\"]\n",
		"inline":       "lib = { proc-macro = true }\n",
		"dotted":       "lib.proc-macro = true\n",
		"quoted":       "[lib]\n\"proc-macro\" = true\n",
		"escaped":      "[lib]\n\"proc\\u002dmacro\" = true\n",
		"crlf-spaced":  "[ lib ]\r\nproc-macro = true # c\r\n",
		"outside-path": "[lib]\npath = \"../shared/macro.rs\"\nproc-macro = true\n",
	}
	files := append(crates(forms), baseFile{path: "shared/macro.rs", content: libLine + "\n"})
	cases := map[string]pathCase{"the [lib] path outside the crate": {appendLine("shared/macro.rs", libLine, "// x"), denied}}
	for dir := range forms {
		cases[dir] = pathCase{editLib(dir), denied}
	}
	assertPathVerdicts(t, newPolicyRepo(t, files), cases)
}

// A manifest that only mentions proc-macro is no proc-macro crate: the
// policy reads TOML, it does not search the text.
func TestPatchPolicy_ProcMacroLookalikes_Accept(t *testing.T) {
	t.Parallel()
	lookalikes := map[string]string{
		"false":         "[lib]\nproc-macro = false\n",
		"strings":       "[package]\nname = \"x\"\nkeywords = [\"proc-macro\"]\ndescription = \"proc-macro = true\"\n",
		"comments":      "# [lib]\n# proc-macro = true\n[package]\nname = \"x\"\n",
		"other-table":   "[package.metadata.lib]\nproc-macro = true\n",
		"multiline":     "[package]\ndescription = \"\"\"\n[lib]\nproc-macro = true\n\"\"\"\n",
		"case-of-table": "[Lib]\nproc-macro = true\n",
	}
	cases := map[string]pathCase{}
	for dir := range lookalikes {
		cases[dir] = pathCase{editLib(dir), ""}
	}
	assertPathVerdicts(t, newPolicyRepo(t, crates(lookalikes)), cases)
}

// A custom build script is denied with its directory when that is not the
// crate root; a build script in the crate root, build.rs included, shares
// the root's module files, whose .rs files are denied.
func TestPatchPolicy_BuildScripts_DenyTheirModules(t *testing.T) {
	t.Parallel()
	files := crates(map[string]string{
		"custom":    "[package]\nname = \"custom\"\nbuild = \"tools/build/main.rs\"\n",
		"rootbuild": "[package]\nname = \"rootbuild\"\nbuild = \"gen.rs\"\n",
		"default":   "[package]\nname = \"default\"\n",
		"nobuild":   "[package]\nname = \"nobuild\"\nbuild = false\n",
	})
	files = append(files,
		baseFile{path: "custom/tools/build/main.rs", content: mainLine + "\n"},
		baseFile{path: "rootbuild/gen.rs", content: mainLine + "\n"},
		baseFile{path: "default/build.rs", content: mainLine + "\n"},
		baseFile{path: "nobuild/build.rs", content: mainLine + "\n"},
	)
	assertPathVerdicts(t, newPolicyRepo(t, files), map[string]pathCase{
		"custom script":                 {appendLine("custom/tools/build/main.rs", mainLine, "// x"), denied},
		"module beside a custom script": {newFile("custom/tools/build/gen.rs", "100644", "pub fn g() {}"), denied},
		"library of a custom script":    {editLib("custom"), ""},
		"root-level custom script":      {appendLine("rootbuild/gen.rs", mainLine, "// x"), denied},
		"module beside it":              {newFile("rootbuild/helpers.rs", "100644", "pub fn h() {}"), denied},
		"other file beside it":          {newFile("rootbuild/notes.py", "100644", "x = 1"), ""},
		"library beside it":             {editLib("rootbuild"), ""},
		"module beside build.rs":        {newFile("default/helpers.rs", "100644", "pub fn h() {}"), denied},
		"library of a build.rs crate":   {editLib("default"), ""},
		"module of build = false":       {newFile("nobuild/helpers.rs", "100644", "pub fn h() {}"), ""},
	})
}

// A build script's and a proc-macro crate's path dependencies run at build
// time, transitively, and so do [patch] and [replace] paths; an ordinary
// crate's normal and dev dependencies do not.
func TestPatchPolicy_BuildTimeDependencies_DenyTheClosure(t *testing.T) {
	t.Parallel()
	files := []baseFile{{path: "Cargo.toml", content: "[workspace]\nmembers = [\"*\"]\n\n[workspace.dependencies]\n" +
		"wsdep = { path = \"crates/wsdep\" }\n\n[patch.crates-io]\nsyn = { path = \"third_party/syn\" }\n\n" +
		"[replace]\n\"foo:1.0.0\" = { path = \"patched/foo\" }\n"}}
	files = append(files, crates(map[string]string{
		"app": "[package]\nname = \"app\"\n\n[dependencies]\nlib1 = { path = \"../lib1\" }\n\n[dev-dependencies]\n" +
			"testutil = { path = \"../testutil\" }\n\n[build-dependencies]\ncodegen = { path = \"../codegen\" }\n" +
			"outside = { path = \"../../outside\" }\n\n[target.'cfg(unix)'.build-dependencies]\nunixgen = { path = \"../unixgen\" }\n",
		"codegen":         "[package]\nname = \"codegen\"\n\n[dependencies.codegen-core]\npath = \"../codegen-core\"\n",
		"codegen-core":    "[package]\nname = \"codegen-core\"\n",
		"macros":          "[package]\nname = \"macros\"\n\n[lib]\nproc-macro = true\n\n[dependencies]\nsupport.path = \"../support\"\nwsdep = { workspace = true }\n",
		"support":         "[package]\nname = \"support\"\n",
		"crates/wsdep":    "[package]\nname = \"wsdep\"\n",
		"unixgen":         "[package]\nname = \"unixgen\"\n",
		"lib1":            "[package]\nname = \"lib1\"\n",
		"testutil":        "[package]\nname = \"testutil\"\n",
		"third_party/syn": "[package]\nname = \"syn\"\n",
		"patched/foo":     "[package]\nname = \"foo\"\n",
	})...)
	assertPathVerdicts(t, newPolicyRepo(t, files), map[string]pathCase{
		"build dependency":              {editLib("codegen"), denied},
		"its own path dependency":       {editLib("codegen-core"), denied},
		"proc-macro path dependency":    {editLib("support"), denied},
		"inherited workspace path":      {editLib("crates/wsdep"), denied},
		"target-specific build dep":     {editLib("unixgen"), denied},
		"patch path":                    {editLib("third_party/syn"), denied},
		"replace path":                  {editLib("patched/foo"), denied},
		"ordinary normal dependency":    {editLib("lib1"), ""},
		"ordinary dev dependency":       {editLib("testutil"), ""},
		"the crate with build deps":     {editLib("app"), ""},
		"new crate below no build path": {newFile("tools/x/src/lib.rs", "100644", libLine), ""},
	})
}

// A manifest band cannot parse, or does not read because it is not a
// regular blob or is past PatchManifestMaxBytes, denies its own directory
// tree and nothing else.
func TestPatchPolicy_UnreadableManifests_DenyOnlyTheirDirectory(t *testing.T) {
	t.Parallel()
	oversized := "[package]\nname = \"big\"\n" + strings.Repeat("# pad\n", healthband.PatchManifestMaxBytes/6)
	files := crates(map[string]string{
		"broken":  "[package\nname = \"broken\"\n",
		"badtype": "[lib]\nproc-macro = \"yes\"\n",
		"big":     oversized,
		"ok":      "[package]\nname = \"ok\"\n",
	})
	files = append(files,
		baseFile{path: "linked/Cargo.toml", content: "../ok/Cargo.toml", mode: "120000"},
		baseFile{path: "linked/src/lib.rs", content: libLine + "\n"},
	)
	assertPathVerdicts(t, newPolicyRepo(t, files), map[string]pathCase{
		"unparsable manifest":   {editLib("broken"), denied},
		"value of a wrong type": {editLib("badtype"), denied},
		"oversized manifest":    {editLib("big"), denied},
		"symlinked manifest":    {editLib("linked"), denied},
		"readable crate":        {editLib("ok"), ""},
		"outside every crate":   {newFile("tools/gen.go", "100644", "package tools"), ""},
	})
}

// Only an unparsable manifest at the root denies the whole repository: its
// crate is the repository.
func TestPatchPolicy_UnparsableRootManifest_DeniesTheRepository(t *testing.T) {
	t.Parallel()
	repo := newPolicyRepo(t, append(policyBase, baseFile{path: "Cargo.toml", content: "[workspace\n"}))
	assert.Equal(t, denied, evaluate(t, repo, healthband.PatchPolicy{}, replyWith(modifyFoo("// x"))).Code)
}
