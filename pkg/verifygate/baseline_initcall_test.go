package verifygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func firstVarSpec(t *testing.T, decl string) ast.Spec {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "", "package p\n\n"+decl+"\n", 0)
	require.NoError(t, err)
	for _, d := range file.Decls {
		if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.VAR {
			return g.Specs[0]
		}
	}
	t.Fatal("no var")
	return nil
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
	} {
		assert.Equal(t, want, hasCall(firstVarSpec(t, decl)), decl)
	}
}

// The c01 shape end to end: a new test file whose var is a pure IIFE does
// not charge the package's failures HEAD already had.
func TestRunCharged_APureVarInitializerIsNotAnEdit(t *testing.T) {
	file := "package basegate\n\nimport \"testing\"\n\nvar ciPublic = func() *bool { b := true; return &b }()\n\n" +
		"func TestPublic(t *testing.T) {\n\tif !*ciPublic {\n\t\tt.Fatal(\"not public\")\n\t}\n}\n"
	res := runCharged(t, redRepo(t, baselineBase(), map[string]string{"public_test.go": file}))
	assert.Nil(t, res.Failure, "edited: %v", res.Edited)
	assert.Equal(t, []string{"basegate.TestNeedsTool"}, res.Preexisting)
}
