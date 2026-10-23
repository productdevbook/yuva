package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"slices"
	"strings"
	"testing"
)

var riverRequiredStates = []string{"JobStateAvailable", "JobStatePending", "JobStateRunning", "JobStateScheduled"}

func TestEveryUniqueByStateHasTheStatesRiverRequires(t *testing.T) {
	t.Parallel()
	for _, dir := range []string{".", "../jobs"} {
		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, pkg := range pkgs {
			decls := map[string]ast.Node{}
			for _, f := range pkg.Files {
				for _, d := range f.Decls {
					switch d := d.(type) {
					case *ast.FuncDecl:
						if d.Recv == nil {
							decls[d.Name.Name] = d
						}
					case *ast.GenDecl:
						for _, sp := range d.Specs {
							if v, ok := sp.(*ast.ValueSpec); ok {
								for _, n := range v.Names {
									decls[n.Name] = v
								}
							}
						}
					}
				}
			}
			var states func(n ast.Node, seen map[string]bool) []string
			states = func(n ast.Node, seen map[string]bool) []string {
				var out []string
				ast.Inspect(n, func(x ast.Node) bool {
					switch x := x.(type) {
					case *ast.SelectorExpr:
						if id, ok := x.X.(*ast.Ident); ok && id.Name == "rivertype" && strings.HasPrefix(x.Sel.Name, "JobState") {
							out = append(out, x.Sel.Name)
						}
					case *ast.Ident:
						if d, ok := decls[x.Name]; ok && !seen[x.Name] {
							seen[x.Name] = true
							out = append(out, states(d, seen)...)
						}
					}
					return true
				})
				return out
			}
			for _, f := range pkg.Files {
				ast.Inspect(f, func(n ast.Node) bool {
					kv, ok := n.(*ast.KeyValueExpr)
					if !ok {
						return true
					}
					if k, ok := kv.Key.(*ast.Ident); !ok || k.Name != "ByState" {
						return true
					}
					got := states(kv.Value, map[string]bool{})
					for _, want := range riverRequiredStates {
						if !slices.Contains(got, want) {
							t.Errorf("%s: UniqueOpts.ByState lacks %s, so River refuses the insert", fset.Position(kv.Pos()), want)
						}
					}
					return true
				})
			}
		}
	}
}
