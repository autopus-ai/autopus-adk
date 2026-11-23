package verifygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stdImports is the import block the hasCall cases are parsed under.
const stdImports = "import (\n\t\"errors\"\n\t\"fmt\"\n\t\"os\"\n\t\"path/filepath\"\n\t\"regexp\"\n\t\"strings\"\n\t\"time\"\n)\n\n"

// varHasCall parses decl under imports and reports hasCall for its var.
func varHasCall(t *testing.T, imports, decl string) bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "", "package p\n\n"+imports+decl+"\n", 0)
	require.NoError(t, err)
	for _, d := range file.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
			return hasCall(g.Specs[0], fileImports(file))
		}
	}
	t.Fatal("no var")
	return false
}

func TestHasCall_OnlySideEffectsAreInitLike(t *testing.T) {
	for decl, want := range map[string]bool{
		// The c01/full initializer: an IIFE that only builds a value.
		"var ciPublic = func() *bool { b := true; return &b }()":              false,
		"var errX = errors.New(\"x\")":                                        false,
		"var re = regexp.MustCompile(\"a\")":                                  false,
		"var wait = time.Duration(5) * time.Second":                           false,
		"var p = filepath.Join(\"a\", strings.ToUpper(\"b\"))":                false,
		"var xs = append(make([]int, 0, 2), len(\"ab\"))":                     false,
		"var cases = []struct{ a int }{{1}, {2}}":                             false,
		"var lazy = func() int { return setMode() }":                          false,
		"var x = setMode()":                                                   true,
		"var y = func() bool { os.Setenv(\"K\", \"v\"); return true }()":      true,
		"var z = func() int { mode = 2; return 1 }()":                         true,
		"var w = func() int { counter++; return 1 }()":                        true,
		"var e = fmt.Errorf(\"x %d\", setMode())":                             true,
		"var v = func() []int { s := []int{}; s = append(s, 1); return s }()": false,
		// Conversions are not calls.
		"var payload = []byte(`{\"a\":1}`)":      false,
		"var n = int64(5)":                       false,
		"var s = string(rune(65))":               false,
		"var m = map[string]int(nil)":            false,
		"var u = userType(5)":                    true,
		"var b2 = []byte(fmt.Sprint(setMode()))": true,
		// A callback runs inside the call it is passed to.
		"var up = strings.Map(func(r rune) rune { setMode(); return r }, \"a\")":      true,
		"var ok = strings.FieldsFunc(\"a b\", func(r rune) bool { return r == ' ' })": false,
		// Channel operations block or wake other goroutines.
		"var got = func() int { ch <- 1; return 0 }()": true,
		"var recv = <-ch":                       true,
		"var r2 = func() int { return <-ch }()": true,
		// An IIFE that gets an outer value may write through it.
		"var reg = func(r map[string]int) int { r[\"k\"] = 1; return 0 }(registry)": true,
		"var alias = func() int { r := registry; r[\"k\"] = 1; return 0 }()":        true,
		"var pure = func(n int) int { return n * 2 }(3)":                            false,
	} {
		assert.Equal(t, want, varHasCall(t, stdImports, decl), decl)
	}
}

// The allowlist is the standard package, not any package named like it.
func TestHasCall_AllowlistResolvesImports(t *testing.T) {
	assert.True(t, varHasCall(t, "import strings \"example.com/m/strings\"\n\n", "var s = strings.ToUpper(\"a\")"))
	assert.True(t, varHasCall(t, "", "var s = strings.ToUpper(\"a\")"), "no import: a local name")
	assert.False(t, varHasCall(t, "import str \"strings\"\n\n", "var s = str.ToUpper(\"a\")"))
}

// The c01 shape end to end: a new test file whose var is a pure IIFE does
// not charge the package's failures HEAD already had.
func TestRunCharged_APureVarInitializerIsNotAnEdit(t *testing.T) {
	for name, decl := range map[string]string{
		"pure IIFE":  "var ciPublic = func() *bool { b := true; return &b }()",
		"conversion": "var payload = []byte(`{\"a\":1}`)",
	} {
		t.Run(name, func(t *testing.T) {
			file := "package basegate\n\nimport \"testing\"\n\n" + decl + "\n\n" +
				"func TestPublic(t *testing.T) {}\n"
			res := runCharged(t, redRepo(t, baselineBase(), map[string]string{"public_test.go": file}))
			assert.Nil(t, res.Failure, "edited: %v", res.Edited)
			assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
		})
	}
}
