package verifygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"slices"
	"strings"
)

// testFuncName is a function go test runs by name. TestMain is not one: it
// runs around all of them.
var testFuncName = regexp.MustCompile(`^(Test|Example|Fuzz|Benchmark)([A-Z0-9_].*)?$`)

// predeclared are Go's predeclared identifiers. A package-level
// declaration of one of them compiles and changes what every use in the
// package means.
var predeclared = strings.Fields("any bool byte comparable complex64 complex128 error float32 float64 int int8 int16 " +
	"int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr true false iota nil append cap clear close " +
	"complex copy delete imag len make max min new panic print println real recover")

// declInfo is what supportEdited needs to know about a new declaration.
type declInfo struct {
	names []string
	recv  string // the receiver type of a method
	calls bool   // a var initializer that calls a function: it runs at init
}

// testDecls is a package's top-level test functions by name, and the
// source of every other declaration except imports, keyed by what it
// declares ("func name", "method Recv.name", "var a,b", "type T", and
// "const block a,b,c" for a grouped const block, whose iota values shift
// when a constant is inserted); init may repeat, so each key holds a
// sorted list.
type testDecls struct {
	tests   map[string]string
	support map[string][]string
	info    map[string]declInfo
}

func newTestDecls() *testDecls {
	return &testDecls{tests: map[string]string{}, support: map[string][]string{}, info: map[string]declInfo{}}
}

// supportEdited reports an edit to the package's existing test support:
//   - a declaration HEAD had that is gone or differs;
//   - an init, TestMain or blank var (its initializer runs at package init)
//     that is new or differs;
//   - a new method on a type that is not new: it changes the type's method
//     set, so a promoted method or an interface match can change;
//   - a new declaration named like a predeclared identifier (len, min,
//     error ...), which changes what every use of it means;
//   - a new named var whose initializer calls a function, which runs at
//     package init like init does.
//
// Any other new helper, type, var or const cannot change how a test HEAD
// already had behaves: redefining a name does not compile.
func supportEdited(before, after *testDecls) bool {
	for key, src := range before.support {
		if !slices.Equal(src, after.support[key]) {
			return true
		}
	}
	for _, key := range []string{"func init", "func TestMain", "var _"} {
		if !slices.Equal(before.support[key], after.support[key]) {
			return true
		}
	}
	for key, info := range after.info {
		if _, existed := before.support[key]; existed {
			continue
		}
		if info.recv != "" {
			if _, newType := after.support["type "+info.recv]; !newType || before.support["type "+info.recv] != nil {
				return true
			}
		}
		if info.calls || slices.ContainsFunc(info.names, func(n string) bool { return slices.Contains(predeclared, n) }) {
			return true
		}
	}
	return false
}

// add parses one test file into d. A file that does not parse adds
// nothing; the build reports the parse error, and a package that does not
// build is never pre-existing unless HEAD fails it the same.
func (d *testDecls) add(src []byte) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return
	}
	text := func(n ast.Node) string {
		return string(src[fset.Position(n.Pos()).Offset:fset.Position(n.End()).Offset])
	}
	put := func(key, s string, info declInfo) {
		d.support[key] = append(d.support[key], s)
		slices.Sort(d.support[key])
		d.info[key] = info
	}
	for _, decl := range file.Decls {
		switch dl := decl.(type) {
		case *ast.FuncDecl:
			name := dl.Name.Name
			switch {
			case dl.Recv != nil:
				recv := recvName(dl.Recv)
				put("method "+recv+"."+name, text(dl), declInfo{recv: recv})
			case name != "TestMain" && testFuncName.MatchString(name):
				d.tests[name] = text(dl)
			default:
				put("func "+name, text(dl), declInfo{names: []string{name}})
			}
		case *ast.GenDecl:
			if dl.Tok == token.IMPORT {
				continue
			}
			var all []string
			for _, spec := range dl.Specs {
				key, names := specKey(dl.Tok, spec)
				all = append(all, names...)
				put(key, text(spec), declInfo{names: names, calls: dl.Tok == token.VAR && hasCall(spec)})
			}
			if dl.Tok == token.CONST && dl.Lparen.IsValid() {
				put("const block "+strings.Join(all, ","), text(dl), declInfo{})
			}
		}
	}
}

func specKey(tok token.Token, spec ast.Spec) (string, []string) {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return "type " + s.Name.Name, []string{s.Name.Name}
	case *ast.ValueSpec:
		names := make([]string, 0, len(s.Names))
		for _, n := range s.Names {
			names = append(names, n.Name)
		}
		return tok.String() + " " + strings.Join(names, ","), names
	}
	return tok.String(), nil
}

// hasCall reports a function call in a var spec's initializer, outside a
// function literal (which only runs when called).
func hasCall(spec ast.Spec) bool {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok {
		return false
	}
	found := false
	for _, v := range vs.Values {
		ast.Inspect(v, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.FuncLit:
				return false
			case *ast.CallExpr:
				found = true
			}
			return !found
		})
	}
	return found
}

func recvName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	t := recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch x := t.(type) {
	case *ast.IndexExpr:
		t = x.X
	case *ast.IndexListExpr:
		t = x.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
