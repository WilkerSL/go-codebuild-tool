package project

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type serviceState struct {
	types map[string]bool
	// methods maps a receiver to its methods and their number of parameters.
	methods map[string]map[string]int
	err     error
}

func readServiceState(dir string) serviceState {
	st := serviceState{types: map[string]bool{}, methods: map[string]map[string]int{}}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return st
	}
	if err != nil {
		st.err = err
		return st
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			st.err = err
			return st
		}
		if hasHeader(data) {
			continue
		}
		f, err := parser.ParseFile(fset, name, data, parser.SkipObjectResolution)
		if err != nil {
			st.err = fmt.Errorf("parse %s: %w", name, err)
			return st
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				if d.Tok != token.TYPE {
					continue
				}
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						st.types[ts.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil || len(d.Recv.List) != 1 {
					continue
				}
				recv := receiverName(d.Recv.List[0].Type)
				if st.methods[recv] == nil {
					st.methods[recv] = map[string]int{}
				}
				params := 0
				for _, field := range d.Type.Params.List {
					params += max(1, len(field.Names))
				}
				st.methods[recv][d.Name.Name] = params
			}
		}
	}
	return st
}

func hasMethod(st serviceState, receiver, method string) bool {
	_, ok := st.methods[receiver][method]
	return ok
}

func receiverName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func (p *planner) resolveServices(context.Context) error {
	states := map[string]serviceState{}
	state := func(dir string) serviceState {
		st, ok := states[dir]
		if !ok {
			st = readServiceState(filepath.Join(p.root, filepath.FromSlash(dir)))
			if st.err != nil {
				p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf("could not inspect %s for Service methods (%v); no service scaffolds were planned for it", dir, st.err))
			}
			states[dir] = st
		}
		return st
	}
	kept := p.files[:0]
	for _, f := range p.files {
		if f.domain == "" || (f.service == "" && f.method == "") {
			kept = append(kept, f)
			continue
		}
		st := state(path.Dir(f.Path))
		full := filepath.Join(p.root, filepath.FromSlash(f.Path))
		switch {
		case f.service != "":
			if st.types[f.service] && !exists(full) {
				continue
			}
		case st.err != nil || !st.types[f.receiver]:
			continue
		case hasMethod(st, f.receiver, f.method):
			if got := st.methods[f.receiver][f.method]; f.params > 0 && got != f.params {
				p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf(
					"%s.%s in %s takes %d parameters, but the generated code now calls it with %d; change it to %s", f.receiver, f.method, path.Dir(f.Path), got, f.params, f.signature))
			}
			continue
		case exists(full):
			p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf(
				"%s in %s lacks the method %s required by the generated handler, and %s already exists; add the method by hand", f.receiver, path.Dir(f.Path), f.method, path.Base(f.Path)))
		default:
			f.Reason = "adds " + f.receiver + "." + f.method + ", which the generated handler now requires"
		}
		kept = append(kept, f)
	}
	p.files = kept
	return nil
}
