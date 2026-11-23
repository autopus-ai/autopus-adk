package verifygate

import (
	"go/ast"
	"go/token"
	"path"
	"slices"
	"strconv"
)

// pureCalls are the calls a var initializer may make without running code
// that can change how other tests behave: constructors and helpers with no
// side effect, by import path. "*" is every function of the package.
var pureCalls = map[string][]string{
	"errors": {"New"}, "fmt": {"Errorf", "Sprintf"}, "path/filepath": {"Join"}, "path": {"Join"},
	"strings": {"*"}, "bytes": {"*"}, "strconv": {"*"}, "regexp": {"MustCompile"}, "time": {"Duration"},
}

// pureBuiltins are the builtins an initializer may call.
var pureBuiltins = []string{"new", "make", "len", "cap", "append", "min", "max"}

// predeclaredTypes convert rather than call: int64(5), string(r).
var predeclaredTypes = []string{"any", "bool", "byte", "complex64", "complex128", "error", "float32", "float64",
	"int", "int8", "int16", "int32", "int64", "rune", "string", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr"}

// fileImports maps each import's name in the file to its path.
func fileImports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(p)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		out[name] = p
	}
	return out
}

// hasCall reports whether a var spec's initializer runs code with a
// possible side effect at package init, like init does: a call to anything
// but pureCalls (resolved through the file's imports, so a local package
// named strings is not the standard one) and pureBuiltins, a channel send
// or receive, or a function literal that is invoked, or passed to a call
// (which may invoke it), and whose body does any of that or writes to a
// name it did not declare. A conversion (int64(5), []byte("x")) is not a
// call; a conversion to a user-defined type cannot be told from a call and
// counts as one. Composite literals and arithmetic run nothing.
func hasCall(spec ast.Spec, imports map[string]string) bool {
	vs, ok := spec.(*ast.ValueSpec)
	if !ok {
		return false
	}
	w := effectWalker{imports: imports}
	return slices.ContainsFunc(vs.Values, func(v ast.Expr) bool { return w.effectful(v, map[string]bool{}) })
}

type effectWalker struct{ imports map[string]string }

// effectful walks node for a possible side effect; local holds the names
// the enclosing function literals declared.
func (w effectWalker) effectful(node ast.Node, local map[string]bool) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			// Not called here: it runs nothing.
			return false
		case *ast.SendStmt:
			found = true
		case *ast.UnaryExpr:
			found = x.Op == token.ARROW
		case *ast.CallExpr:
			found = w.callEffectful(x, local)
			return false
		}
		return !found
	})
	return found
}

func (w effectWalker) callEffectful(call *ast.CallExpr, local map[string]bool) bool {
	if lit, ok := call.Fun.(*ast.FuncLit); ok {
		// An argument the literal gets from outside may be written through
		// its parameter (a map, a pointer, a slice).
		for _, a := range call.Args {
			if id := rootIdent(a); id != nil && !local[id.Name] && !isConstIdent(id.Name) {
				return true
			}
		}
		if w.litEffectful(lit, local) {
			return true
		}
	} else if !isConversion(call.Fun) && !w.pureCall(call.Fun) {
		return true
	}
	for _, a := range call.Args {
		if lit, ok := a.(*ast.FuncLit); ok {
			// A callback (strings.Map, FieldsFunc) runs inside the call.
			if w.litEffectful(lit, local) {
				return true
			}
		} else if w.effectful(a, local) {
			return true
		}
	}
	return false
}

// litEffectful reports whether a function literal's body has a possible
// side effect, or writes to a name it did not declare. outer are the names
// declared around it.
func (w effectWalker) litEffectful(lit *ast.FuncLit, outer map[string]bool) bool {
	local := map[string]bool{}
	for k := range outer {
		local[k] = true
	}
	for _, fields := range []*ast.FieldList{lit.Type.Params, lit.Type.Results} {
		if fields == nil {
			continue
		}
		for _, f := range fields.List {
			for _, n := range f.Names {
				local[n.Name] = true
			}
		}
	}
	found := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if found {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				id := rootIdent(lhs)
				switch {
				case id == nil:
				case x.Tok == token.DEFINE:
					// An alias of an outer name (r := registry) is not local:
					// writing through it writes the outer value.
					if i < len(x.Rhs) && len(x.Rhs) == len(x.Lhs) {
						if src := rootIdent(x.Rhs[i]); src != nil && !local[src.Name] && !isConstIdent(src.Name) {
							continue
						}
					}
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
		case *ast.SendStmt:
			found = true
		case *ast.UnaryExpr:
			found = x.Op == token.ARROW
		case *ast.CallExpr:
			found = w.callEffectful(x, local)
			return false
		case *ast.FuncLit:
			return false
		}
		return !found
	})
	return found
}

func isConstIdent(name string) bool {
	return name == "true" || name == "false" || name == "nil" || name == "iota"
}

// isConversion reports a call whose Fun is a type: a type literal, or a
// predeclared type name.
func isConversion(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.ArrayType, *ast.MapType, *ast.ChanType, *ast.FuncType, *ast.InterfaceType, *ast.StructType:
		return true
	case *ast.ParenExpr:
		return isConversion(f.X)
	case *ast.StarExpr:
		return isConversion(f.X)
	case *ast.Ident:
		return slices.Contains(predeclaredTypes, f.Name)
	}
	return false
}

// rootIdent is the variable an expression reads or writes through: x for
// x, x.f, x[i], *x and (x).
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
		case *ast.UnaryExpr:
			if x.Op != token.AND {
				return nil
			}
			e = x.X
		default:
			return nil
		}
	}
}

func (w effectWalker) pureCall(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return slices.Contains(pureBuiltins, f.Name)
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		if !ok {
			return false
		}
		names, ok := pureCalls[w.imports[pkg.Name]]
		return ok && (slices.Contains(names, "*") || slices.Contains(names, f.Sel.Name))
	}
	return false
}
