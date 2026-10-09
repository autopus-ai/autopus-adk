package healthband_test

import "testing"

// RR-7 reads the base manifests as TOML 1.0, so a key counts where TOML
// puts it and nowhere else. Every manifest below was checked against
// python's tomllib: the build ones are proc-macro crates, the plain ones
// are not, and the invalid ones are not TOML (except the byte order mark,
// the one superset form here, and the nesting past band's own bound). A
// manifest that band cannot read as TOML, or whose value has a type that
// Cargo does not give it, denies its crate's directory: fail closed.

var (
	syntaxBuild = map[string]string{
		"v-multibasic":   "[package]\nname = \"v\"\ndescription = \"\"\"\none \\\n   two \"\"quotes\"\" \\u00e9\\ttab\n\"\"\"\n[lib]\nproc-macro = true\n",
		"v-multiliteral": "[package]\ndescription = '''\nraw \\n [lib] ''x'''\n[lib]\nproc-macro = true\n",
		"v-arrays":       "[package]\nkeywords = [ # c\n  \"a\", [1, 2.5, -3e2], { k = \"v\", n.m = true },\n]\nauthors = []\n[lib]\ncrate-type = [\n  \"rlib\", # c\n  \"proc-macro\",\n]\n",
		"v-scalars":      "[package]\na = 1979-05-27 07:32:00Z\nb = 0xDEAD_beef\nc = +inf\nd = 07:32:00\ne = 1979-05-27\nf = -0.0\ng = 0o755\nh = 0b11\n[lib]\nproc-macro = true\n",
		"v-tablearrays":  "[[bin]]\nname = \"a\"\n[[bin]]\nname = \"b\"\n[bin.x]\ny = 1\n[lib]\nproc-macro = true\n",
		"v-dotted":       "[package]\nmetadata.a.b = 1\nmetadata.a.c = 2\n[package.metadata.d]\ne = 3\n[lib]\n\"proc-macro\" = true\n",
		"v-crlf-bom":     "\ufeff[lib]\r\nproc-macro = true\r\n",
		"v-comments":     "[lib] # c\nproc-macro = true # c\n",
		"v-emptykey":     "\"\" = 1\n\n\n[lib]\nproc-macro = true\n",
		"v-implicit":     "[lib.x]\ny = 1\n[lib]\nproc-macro = true\n",
	}
	syntaxPlain = map[string]string{
		"n-literal-keys":   "[package]\ndescription = '''\n[lib]\nproc-macro = true\n'''\n",
		"n-escaped-quotes": "[package]\ndescription = \"\"\"\n\\\"\"\"\n[lib]\nproc-macro = true\n\"\"\"\n",
		"n-other-inline":   "[package.metadata]\nlib = { proc-macro = true }\n",
	}
	syntaxInvalid = map[string]string{
		"i-dupkey":         "[package]\nname = \"a\"\nname = \"b\"\n",
		"i-duptable":       "[package]\n[package]\n",
		"i-string":         "[package]\nname = \"a\n",
		"i-multibasic":     "[package]\ndescription = \"\"\"\nabc\n",
		"i-literal":        "[package]\nname = 'a\n",
		"i-multiliteral":   "[package]\nd = '''abc\n",
		"i-escape":         "[package]\nname = \"\\q\"\n",
		"i-surrogate":      "[package]\nname = \"\\uD800\"\n",
		"i-shorthex":       "[package]\nname = \"\\u12\"\n",
		"i-lonecr":         "[package]\rname = \"a\"\n",
		"i-novalue":        "[package]\nname =\n",
		"i-twovalues":      "[package]\nname = \"a\" \"b\"\n",
		"i-bareword":       "[package]\npublish = yes\n",
		"i-arraycomma":     "[package]\nkeywords = [\"a\" \"b\"]\n",
		"i-arrayopen":      "[package]\nkeywords = [\"a\",\n",
		"i-inlineextend":   "[package]\nmeta = { a = 1 }\nmeta.b = 2\n",
		"i-inlineheader":   "[package]\nmeta = { a = 1 }\n[package.meta.b]\n",
		"i-aotvalue":       "bin = 1\n[[bin]]\n",
		"i-tableaot":       "[[bin]]\n[bin]\n",
		"i-dottedexplicit": "[a.b]\n[a]\nb.c = 1\n",
		"i-depth":          "a = [[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[[]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]]\n",
		"i-header":         "[package\n",
		"i-noequals":       "[package]\nname\n",
		"i-multikey":       "\"\"\"a\"\"\" = 1\n",
		"i-control":        "[package]\nname = \"a\u0001b\"\n",
		"i-inlineopen":     "[package]\nm = { a = 1\n",
		"i-inlinesep":      "[package]\nm = { a = 1 b = 2 }\n",
		"i-dottedvalue":    "[package]\nname = \"a\"\nname.x = 1\n",
		"i-headervalue":    "[package]\nname = \"a\"\n[package.name.x]\n",
		"i-headerarray":    "a = [1]\n[a.b]\n",
		"i-arrayitem":      "[package]\nk = [ = ]\n",
		"i-keychar":        "[package]\nna*me = 1\n",
		"i-headerdotted":   "[package]\nmetadata.x = 1\n[package.metadata]\n",
		"i-utf8":           "[package]\nname = \"" + string([]byte{0xff}) + "\"\n",
	}
	// Cargo gives these keys another type, so each manifest is unparsable.
	cargoWrongTypes = map[string]string{
		"t-procmacro":   "[lib]\nproc-macro = \"yes\"\n",
		"t-cratetype":   "[lib]\ncrate-type = \"proc-macro\"\n",
		"t-cratetypes":  "[lib]\ncrate-type = [\"proc-macro\", 1]\n",
		"t-libpath":     "[lib]\npath = 1\n",
		"t-lib":         "lib = 1\n",
		"t-package":     "package = \"x\"\n",
		"t-build":       "[package]\nbuild = 1\n",
		"t-buildempty":  "[package]\nbuild = []\n",
		"t-buildmixed":  "[package]\nbuild = [\"a.rs\", 1]\n",
		"t-dep":         "[dependencies]\nfoo = 1\n",
		"t-deppath":     "[dependencies]\nfoo = { path = 1 }\n",
		"t-depws":       "[dependencies]\nfoo = { workspace = \"yes\" }\n",
		"t-target":      "target = \"x\"\n",
		"t-targetentry": "[target]\nunix = 1\n",
		"t-workspace":   "workspace = 1\n",
		"t-wsdeps":      "[workspace]\ndependencies = 1\n",
		"t-wsdep":       "[workspace.dependencies]\nfoo = 1\n",
		"t-patch":       "patch = 1\n",
		"t-patchsource": "[patch]\ncrates-io = 1\n",
		"t-patchentry":  "[patch.crates-io]\nsyn = 1\n",
		"t-replace":     "replace = 1\n",
	}
)

func TestPatchPolicy_ManifestSyntax_ReadAsTOML(t *testing.T) {
	t.Parallel()
	manifests := map[string]string{}
	cases := map[string]pathCase{}
	for _, group := range []struct {
		manifests map[string]string
		code      string
	}{{syntaxBuild, denied}, {syntaxPlain, ""}, {syntaxInvalid, denied}, {cargoWrongTypes, denied}} {
		for dir, manifest := range group.manifests {
			manifests[dir] = manifest
			cases[dir] = pathCase{editLib(dir), group.code}
		}
	}
	// A build path is a TOML string like any other: escapes and literal
	// strings name the same directory.
	manifests["app"] = "[package]\nname = \"app\"\n\n[build-dependencies]\ngen7 = { path = \"../g\\u0065n7\" }\n" +
		"\n[build-dependencies.gen8]\npath = '../gen8'\n"
	manifests["gen7"], manifests["gen8"] = "[package]\nname = \"gen7\"\n", "[package]\nname = \"gen8\"\n"
	cases["escaped build path"] = pathCase{editLib("gen7"), denied}
	cases["literal build path"] = pathCase{editLib("gen8"), denied}
	cases["the crate that names them"] = pathCase{editLib("app"), ""}
	assertPathVerdicts(t, newPolicyRepo(t, crates(manifests)), cases)
}
