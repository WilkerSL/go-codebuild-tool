package gen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

type importSpec struct {
	name string
	path string
}

func defaultImportName(importPath string) string {
	parts := strings.Split(importPath, "/")
	name := parts[len(parts)-1]
	if majorVersion.MatchString(name) && len(parts) > 1 {
		name = parts[len(parts)-2]
	}
	return strings.TrimPrefix(path.Base(name), "go-")
}

func FormatGo(src []byte) ([]byte, error) {
	return FormatGoModule(src, "")
}

// Templates import everything they might need. FormatGoModule drops the imports a file
// ends up not using, groups the rest the way goimports does and runs gofmt.
func FormatGoModule(src []byte, module string) ([]byte, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse generated code: %w\n%s", err, numbered(src))
	}
	used := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				used[id.Name] = true
			}
		}
		return true
	})
	var keep []importSpec
	var start, end token.Pos
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		if !start.IsValid() {
			start = gd.Pos()
		}
		end = gd.End()
		for _, s := range gd.Specs {
			spec := s.(*ast.ImportSpec)
			importPath, _ := strconv.Unquote(spec.Path.Value)
			name := ""
			if spec.Name != nil {
				name = spec.Name.Name
			}
			local := name
			if local == "" {
				local = defaultImportName(importPath)
			}
			if local == "_" || used[local] {
				keep = append(keep, importSpec{name: name, path: importPath})
			}
		}
	}
	if start.IsValid() {
		file := fset.File(start)
		var b bytes.Buffer
		b.Write(src[:file.Offset(start)])
		b.WriteString(renderImports(keep, module))
		b.Write(src[file.Offset(end):])
		src = b.Bytes()
	}
	out, err := format.Source(src)
	if err != nil {
		return nil, fmt.Errorf("format generated code: %w\n%s", err, numbered(src))
	}
	return out, nil
}

func renderImports(specs []importSpec, module string) string {
	if len(specs) == 0 {
		return ""
	}
	var std, other, local []string
	seen := map[string]bool{}
	for _, s := range specs {
		line := strconv.Quote(s.path)
		if s.name != "" {
			line = s.name + " " + line
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		switch {
		case module != "" && (s.path == module || strings.HasPrefix(s.path, module+"/")):
			local = append(local, line)
		case strings.Contains(strings.Split(s.path, "/")[0], "."):
			other = append(other, line)
		default:
			std = append(std, line)
		}
	}
	var b strings.Builder
	b.WriteString("import (\n")
	first := true
	for _, group := range [][]string{std, other, local} {
		if len(group) == 0 {
			continue
		}
		slices.Sort(group)
		if !first {
			b.WriteString("\n")
		}
		first = false
		for _, line := range group {
			b.WriteString("\t" + line + "\n")
		}
	}
	b.WriteString(")")
	return b.String()
}

func numbered(src []byte) string {
	lines := strings.Split(string(src), "\n")
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%4d  %s\n", i+1, line)
	}
	return b.String()
}
