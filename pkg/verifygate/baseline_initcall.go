package verifygate

import (
	"go/ast"
	"go/token"
	"slices"
)

// pureCalls are the calls a var initializer may make without running code
// that can change how other tests behave: constructors and helpers with no
// side effect. A package maps to its pure functions; "*" is all of them.
var pureCalls = map[string][]string{
	"errors": {"New"}, "fmt": {"Errorf", "Sprintf"}, "filepath": {"Join"}, "path": {"Join"},
	"strings": {"*"}, "bytes": {"*"}, "strconv": {"*"}, "regexp": {"MustCompile"}, "time": {"Duration"},
}

// pureBuiltins are the builtins an initializer may call.
var pureBuiltins = []string{"new", "make", "len", "cap", "append", "min", "max"}

// hasCall reports whether a var spec's initializer runs code with a
// possible side effect at package init, like init does: a call to anything
// but pureCalls and pureBuiltins, or an immediately invoked function
// literal whose body makes such a call or assigns to a name it did not
// declare (a package-level var). A function literal that is not called
// runs nothing; composite literals and arithmetic run nothing either.
func hasCall(spec ast.Spec) bool {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok {
		return false
	}
	return slices.ContainsFunc(vs.Values, func(v ast.Expr) bool { return effectful(v) })
}

// effectful walks an expression or statement for a call with a possible
// side effect.
func effectful(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			// Not called here: it runs nothing.
			return false
		case *ast.CallExpr:
			if lit, ok := x.Fun.(*ast.FuncLit); ok {
				found = litEffectful(lit) || slices.ContainsFunc(x.Args, func(a ast.Expr) bool { return effectful(a) })
				return false
			}
			if !pureCall(x.Fun) {
				found = true
				return false
			}
			for _, a := range x.Args {
				if effectful(a) {
					found = true
				}
			}
			return false
		}
		return true
	})
	return found
}

// litEffectful reports whether an invoked function literal's body makes a
// call with a possible side effect, or assigns to a name it did not
// declare.
func litEffectful(lit *ast.FuncLit) bool {
	local := map[string]bool{}
	declare := func(fields *ast.FieldList) {
		if fields == nil {
			return
		}
		for _, f := range fields.List {
			for _, n := range f.Names {
				local[n.Name] = true
			}
		}
	}
	declare(lit.Type.Params)
	declare(lit.Type.Results)
	found := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				id := rootIdent(lhs)
				switch {
				case id == nil:
				case x.Tok == token.DEFINE:
					local[id.Name] = true
				case id.Name != "_" && !local[id.Name]:
					found = true
				}
			}
		case *ast.ValueSpec:
			for _, id := range x.Names {
				local[id.Name] = true
			}
		case *ast.IncDecStmt:
			if id := rootIdent(x.X); id != nil && !local[id.Name] {
				found = true
			}
		case *ast.CallExpr:
			found = effectful(x)
			return false
		}
		return !found
	})
	return found
}

// rootIdent is the variable an assignment target writes through: x for x,
// x.f, x[i] and *x.
func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}

func pureCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return slices.Contains(pureBuiltins, f.Name)
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		if !ok {
			return false
		}
		names, ok := pureCalls[pkg.Name]
		return ok && (slices.Contains(names, "*") || slices.Contains(names, f.Sel.Name))
	}
	return false
}
