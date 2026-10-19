package spec

import (
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	goparser "go/parser"
	"go/scanner"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type Source struct {
	Name    string
	Content []byte
}

func LoadDir(root, dir string) ([]Source, error) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read specs directory: %w", err)
	}
	var sources []Source
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(dir), name))
		if err != nil {
			return nil, fmt.Errorf("read spec %s: %w", name, err)
		}
		sources = append(sources, Source{Name: path.Join(filepath.ToSlash(dir), name), Content: content})
	}
	slices.SortFunc(sources, func(a, b Source) int { return strings.Compare(a.Name, b.Name) })
	return sources, nil
}

var domainPattern = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

var reservedDomains = map[string]bool{
	"app": true, "auth": true, "bytes": true, "cmd": true, "config": true, "context": true,
	"database": true, "errors": true, "fmt": true, "http": true, "httpx": true,
	"internal": true, "io": true, "json": true, "main": true, "migrations": true,
	"os": true, "platform": true, "slog": true, "specs": true, "sql": true,
	"strconv": true, "strings": true, "testing": true, "time": true, "url": true,
	"utf8": true, "uuid": true, "validation": true,
}

var reservedEntityNames = map[string]bool{
	"APIKey": true, "APIKeyInput": true, "Accounts": true, "AuthHandler": true, "AuthService": true,
	"CreateInput": true, "CreatedAPIKey": true, "Handler": true, "ListParams": true, "LoginInput": true,
	"Page": true, "PasswordChange": true, "Repository": true, "Service": true, "UpdateInput": true,
}

var reservedFieldNames = map[string]bool{
	"Validate": true,
}

type marker struct {
	key      string
	value    string
	hasValue bool
	pos      schema.Pos
}

type markerSet struct {
	order []string
	byKey map[string]marker
}

func (ms markerSet) has(key string) bool {
	_, ok := ms.byKey[key]
	return ok
}

func (ms markerSet) get(key string) marker {
	return ms.byKey[key]
}

type entityDecl struct {
	source  string
	domain  string
	uuidImp string
	spec    *ast.TypeSpec
	st      *ast.StructType
}

type parser struct {
	fset            *token.FileSet
	diags           schema.Diagnostics
	info            *types.Info
	pkg             *types.Package
	fieldMarkers    map[*schema.Field]markerSet
	relationMarkers map[*schema.Relation]markerSet
	entityMarkers   map[*schema.Entity]markerSet
}

func Parse(sources []Source) (*schema.Model, schema.Diagnostics) {
	p := &parser{
		fset:            token.NewFileSet(),
		fieldMarkers:    map[*schema.Field]markerSet{},
		relationMarkers: map[*schema.Relation]markerSet{},
		entityMarkers:   map[*schema.Entity]markerSet{},
	}
	sorted := slices.Clone(sources)
	slices.SortFunc(sorted, func(a, b Source) int { return strings.Compare(a.Name, b.Name) })
	var files []*ast.File
	var decls []entityDecl
	for _, src := range sorted {
		f, err := goparser.ParseFile(p.fset, src.Name, src.Content, goparser.ParseComments|goparser.SkipObjectResolution)
		if err != nil {
			p.addSyntaxError(src.Name, err)
			continue
		}
		p.checkHeader(f, src.Name)
		files = append(files, f)
		decls = append(decls, p.collectDecls(f, src.Name)...)
	}
	model := &schema.Model{}
	if len(files) == 0 {
		p.diags.Sort()
		return model, p.diags
	}
	p.info = &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: newSyntheticImporter(), Error: p.addTypeError}
	p.pkg, _ = conf.Check("specs", p.fset, files, p.info)
	for _, d := range decls {
		if e := p.buildEntity(d); e != nil {
			model.Entities = append(model.Entities, e)
		}
	}
	slices.SortFunc(model.Entities, func(a, b *schema.Entity) int { return strings.Compare(a.Name, b.Name) })
	p.validateModel(model)
	p.diags.Sort()
	return model, p.diags
}

func (p *parser) pos(pos token.Pos) schema.Pos {
	position := p.fset.Position(pos)
	return schema.Pos{File: position.Filename, Line: position.Line, Column: position.Column}
}

func (p *parser) errorf(pos schema.Pos, code, suggestion, format string, args ...any) {
	p.diags = append(p.diags, schema.Diagnostic{
		Pos:        pos,
		Severity:   schema.SeverityError,
		Code:       code,
		Message:    fmt.Sprintf(format, args...),
		Suggestion: suggestion,
	})
}

func (p *parser) warnf(pos schema.Pos, code, format string, args ...any) {
	p.diags = append(p.diags, schema.Diagnostic{
		Pos:      pos,
		Severity: schema.SeverityWarning,
		Code:     code,
		Message:  fmt.Sprintf(format, args...),
	})
}

func (p *parser) addSyntaxError(file string, err error) {
	var list scanner.ErrorList
	if errors.As(err, &list) {
		for _, e := range list {
			p.errorf(schema.Pos{File: e.Pos.Filename, Line: e.Pos.Line, Column: e.Pos.Column}, CodeSyntax, "", "%s", e.Msg)
		}
		return
	}
	p.errorf(schema.Pos{File: file}, CodeSyntax, "", "%v", err)
}

func (p *parser) addTypeError(err error) {
	var terr types.Error
	if !errors.As(err, &terr) {
		p.errorf(schema.Pos{}, CodeTypeError, "", "%v", err)
		return
	}
	if strings.HasPrefix(terr.Msg, "could not import") {
		return
	}
	p.errorf(p.pos(terr.Pos), CodeTypeError, "", "%s", terr.Msg)
}

func (p *parser) checkHeader(f *ast.File, file string) {
	if f.Name.Name != "specs" {
		p.errorf(p.pos(f.Name.Pos()), CodePackageName, "specs", "spec files must declare package specs, found package %s", f.Name.Name)
	}
	switch p.buildConstraint(f) {
	case constraintMissing:
		p.errorf(schema.Pos{File: file, Line: 1, Column: 1}, CodeBuildTag, "//go:build ignore",
			"spec files must start with //go:build ignore so they stay out of the build and out of go mod tidy")
	case constraintTidy:
		p.errorf(schema.Pos{File: file, Line: 1, Column: 1}, CodeBuildTag, "//go:build ignore",
			"go mod tidy considers every build tag except ignore, so it reads this spec and adds or breaks its imports; replace the constraint with //go:build ignore")
	}
	for _, imp := range f.Imports {
		importPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil || slices.Contains(allowedImports, importPath) {
			continue
		}
		p.errorf(p.pos(imp.Path.Pos()), CodeUnsupportedImport, naming.Suggest(importPath, allowedImports),
			"import %q is not allowed in spec files; only %q, %q and %q are supported", importPath, importTime, importStdUUID, importGoogleUUID)
	}
}

type constraintState int

const (
	constraintMissing constraintState = iota
	constraintTidy
	constraintIgnored
)

func (p *parser) buildConstraint(f *ast.File) constraintState {
	for _, group := range f.Comments {
		if group.Pos() >= f.Package {
			break
		}
		for _, c := range group.List {
			if !constraint.IsGoBuild(c.Text) {
				continue
			}
			expr, err := constraint.Parse(c.Text)
			if err != nil || expr.Eval(func(string) bool { return false }) {
				return constraintMissing
			}
			if expr.Eval(func(tag string) bool { return tag != "ignore" }) {
				return constraintTidy
			}
			return constraintIgnored
		}
	}
	return constraintMissing
}

func (p *parser) collectDecls(f *ast.File, file string) []entityDecl {
	domain := strings.TrimSuffix(path.Base(file), ".go")
	uuidImport := ""
	for _, imp := range f.Imports {
		importPath, _ := strconv.Unquote(imp.Path.Value)
		if importPath == importStdUUID || importPath == importGoogleUUID {
			uuidImport = importPath
		}
	}
	var decls []entityDecl
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			p.errorf(p.pos(d.Pos()), CodeUnsupportedDecl, "", "function %s is not allowed in spec files; specs only declare entity structs", d.Name.Name)
		case *ast.GenDecl:
			switch d.Tok {
			case token.IMPORT:
			case token.TYPE:
				for _, s := range d.Specs {
					ts := s.(*ast.TypeSpec)
					if ts.TypeParams != nil {
						p.errorf(p.pos(ts.Pos()), CodeUnsupportedDecl, "", "generic type %s is not allowed in spec files", ts.Name.Name)
						continue
					}
					if ts.Assign.IsValid() {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						p.errorf(p.pos(ts.Pos()), CodeUnsupportedDecl, "",
							"type %s is not a struct; spec files only declare entity structs and type aliases such as type %s = string", ts.Name.Name, ts.Name.Name)
						continue
					}
					decls = append(decls, entityDecl{source: file, domain: domain, uuidImp: uuidImport, spec: ts, st: st})
				}
			default:
				p.errorf(p.pos(d.Pos()), CodeUnsupportedDecl, "", "%s declarations are not allowed in spec files", d.Tok)
			}
		}
	}
	switch {
	case len(decls) == 0:
		p.errorf(p.pos(f.Name.Pos()), CodeEntityCount, "", "%s declares no entity; each spec file declares exactly one entity struct", file)
	case len(decls) > 1:
		for _, d := range decls[1:] {
			p.errorf(p.pos(d.spec.Pos()), CodeEntityCount, "",
				"%s declares more than one entity; move %s to specs/%s.go", file, d.spec.Name.Name, naming.Plural(strings.ReplaceAll(naming.Snake(d.spec.Name.Name), "_", "")))
		}
		decls = decls[:1]
	}
	p.checkDomain(f, file, domain)
	return decls
}

func (p *parser) checkDomain(f *ast.File, file, domain string) {
	pos := schema.Pos{File: file, Line: 1, Column: 1}
	switch {
	case !domainPattern.MatchString(domain):
		cleaned := strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
				return r
			}
			if r >= 'A' && r <= 'Z' {
				return r + ('a' - 'A')
			}
			return -1
		}, domain)
		p.errorf(pos, CodeDomainName, cleaned, "spec file name %q is not a valid domain name; use lowercase letters and digits only (it becomes the Go package name)", path.Base(file))
	case reservedDomains[domain] || token.IsKeyword(domain) || types.Universe.Lookup(domain) != nil:
		p.errorf(pos, CodeDomainName, "", "domain name %q is reserved because it collides with a Go keyword or a package used by the generated code; rename %s", domain, path.Base(file))
	}
}

func (p *parser) buildEntity(d entityDecl) *schema.Entity {
	name := d.spec.Name.Name
	e := &schema.Entity{
		Name:       name,
		Domain:     d.domain,
		File:       d.source,
		Pos:        p.pos(d.spec.Name.Pos()),
		UUIDImport: d.uuidImp,
	}
	switch {
	case !ast.IsExported(name):
		p.errorf(e.Pos, CodeReservedName, naming.Pascal(name), "entity name %s must be exported", name)
	case reservedEntityNames[name]:
		p.errorf(e.Pos, CodeReservedName, "", "entity name %s collides with a generated type; choose another name", name)
	}
	metaSeen := false
	for _, field := range d.st.Fields.List {
		markers, ok := p.parseTag(field.Tag)
		if len(field.Names) == 0 {
			p.errorf(p.pos(field.Pos()), CodeEmbeddedField, "", "embedded fields are not supported in spec entities")
			continue
		}
		for _, ident := range field.Names {
			if ident.Name == "_" {
				if metaSeen {
					p.errorf(p.pos(ident.Pos()), CodeInvalidTag, "", "entity %s declares more than one _ struct{} field", name)
					continue
				}
				metaSeen = true
				p.buildMeta(e, field, markers, ok)
				continue
			}
			if !ident.IsExported() {
				p.errorf(p.pos(ident.Pos()), CodeUnexportedField, naming.Pascal(ident.Name), "field %s.%s must be exported", name, ident.Name)
				continue
			}
			if !ok {
				continue
			}
			ft := p.classify(p.info.TypeOf(field.Type))
			p.buildField(e, ident, field, ft, markers)
		}
	}
	if e.Table == "" {
		e.Table = naming.Plural(naming.Snake(name))
	}
	p.checkEntity(e)
	return e
}

func (p *parser) parseTag(lit *ast.BasicLit) ([]marker, bool) {
	if lit == nil {
		return nil, true
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		p.errorf(p.pos(lit.Pos()), CodeInvalidTag, "", "struct tag is not a valid string literal")
		return nil, false
	}
	litPos := p.pos(lit.Pos())
	exact := strings.HasPrefix(lit.Value, "`") && !strings.Contains(value, "\n")
	posAt := func(offset int) schema.Pos {
		if !exact {
			return litPos
		}
		return schema.Pos{File: litPos.File, Line: litPos.Line, Column: litPos.Column + 1 + offset}
	}
	entries, badOffset, err := parseStructTag(value)
	if err != nil {
		p.errorf(posAt(badOffset), CodeInvalidTag, "", "malformed struct tag; expected `gen:\"marker,marker=value\"`")
		return nil, false
	}
	var markers []marker
	for _, entry := range entries {
		if entry.key != "gen" {
			p.errorf(posAt(max(entry.offset-len(entry.key)-2, 0)), CodeUnsupportedTagKey, "gen",
				"tag key %q is not supported in spec files; only gen tags are allowed (json tags are generated)", entry.key)
			continue
		}
		for _, raw := range splitMarkers(entry.value) {
			markers = append(markers, marker{key: raw.key, value: raw.value, hasValue: raw.hasValue, pos: posAt(entry.offset + raw.offset)})
		}
	}
	return markers, true
}

func (p *parser) markerSet(markers []marker, scope markerScope) markerSet {
	set := markerSet{byKey: map[string]marker{}}
	for _, m := range markers {
		if m.key == "" {
			p.errorf(m.pos, CodeInvalidValue, "", "empty marker; remove the extra comma")
			continue
		}
		def, ok := lookupMarker(m.key)
		if !ok {
			p.errorf(m.pos, CodeUnknownMarker, naming.Suggest(m.key, markerNames(scope)), "unknown marker %q", m.key)
			continue
		}
		if def.scope != scope {
			if scope == scopeField {
				p.errorf(m.pos, CodeMarkerScope, "", "%s is an entity marker; put it on the _ struct{} field", m.key)
			} else {
				p.errorf(m.pos, CodeMarkerScope, "", "%s is a field marker and cannot be used on the _ struct{} field", m.key)
			}
			continue
		}
		if set.has(m.key) {
			p.errorf(m.pos, CodeDuplicateMarker, "", "marker %s is declared more than once", m.key)
			continue
		}
		switch def.value {
		case valueNone:
			if m.hasValue {
				p.errorf(m.pos, CodeInvalidValue, m.key, "%s does not take a value", m.key)
				continue
			}
		case valueRequired:
			if !m.hasValue || (m.value == "" && m.key != "default") {
				p.errorf(m.pos, CodeInvalidValue, "", "%s requires a value: %s", m.key, def.doc.Syntax)
				continue
			}
		}
		set.order = append(set.order, m.key)
		set.byKey[m.key] = m
	}
	return set
}

func (p *parser) buildMeta(e *schema.Entity, field *ast.Field, markers []marker, ok bool) {
	st, isStruct := field.Type.(*ast.StructType)
	if !isStruct || len(st.Fields.List) > 0 {
		p.errorf(p.pos(field.Type.Pos()), CodeUnsupportedType, "struct{}", "the _ field carries entity markers and must have type struct{}")
		return
	}
	if !ok {
		return
	}
	set := p.markerSet(markers, scopeEntity)
	p.entityMarkers[e] = set
	if set.has("table") {
		m := set.get("table")
		if !naming.IsSQLIdentifier(m.value) {
			p.errorf(m.pos, CodeInvalidValue, sqlIdentifierSuggestion(m.value), "table name %q must match ^[a-z_][a-z0-9_]*$ and have at most 63 characters", m.value)
		} else {
			e.Table = m.value
		}
	}
	e.SoftDelete = set.has("softdelete")
	e.NoRoutes = set.has("noroutes")
	e.ReadOnly = set.has("readonly")
	if set.has("auth") {
		e.Auth = p.parseMethods(set.get("auth"))
	}
	e.Register = set.has("register")
	if set.has("public") {
		e.Public = p.parsePublic(set.get("public"))
	}
	if set.has("roles") {
		e.Roles = p.parseRoles(set.get("roles"))
	}
	if set.has("writeroles") {
		e.WriteRoles = p.parseRoles(set.get("writeroles"))
	}
}

func (p *parser) parseMethods(m marker) []schema.Method {
	names := make([]string, len(schema.Methods))
	for i, method := range schema.Methods {
		names[i] = string(method)
	}
	seen := map[schema.Method]bool{}
	for _, item := range strings.Split(m.value, "|") {
		method := schema.Method(item)
		switch {
		case !slices.Contains(schema.Methods, method):
			p.errorf(m.pos, CodeInvalidValue, naming.Suggest(item, names), "unknown authentication method %q; use session, jwt or apikey", item)
			return nil
		case seen[method]:
			p.errorf(m.pos, CodeInvalidValue, "", "authentication method %q is listed more than once", item)
			return nil
		}
		seen[method] = true
	}
	switch {
	case seen[schema.MethodSession] && seen[schema.MethodJWT]:
		p.errorf(m.pos, CodeInvalidValue, "", "auth combines session and jwt; choose one login method and optionally add apikey")
		return nil
	case !seen[schema.MethodSession] && !seen[schema.MethodJWT]:
		p.errorf(m.pos, CodeInvalidValue, "session|"+m.value, "auth needs a login method, session or jwt; API keys are created by logged-in users")
		return nil
	}
	var methods []schema.Method
	for _, method := range schema.Methods {
		if seen[method] {
			methods = append(methods, method)
		}
	}
	return methods
}

func (p *parser) parsePublic(m marker) schema.Operations {
	var ops schema.Operations
	if !m.hasValue {
		for _, op := range schema.AllOperations {
			ops.Set(op)
		}
		return ops
	}
	names := make([]string, len(schema.AllOperations))
	for i, op := range schema.AllOperations {
		names[i] = string(op)
	}
	for _, item := range strings.Split(m.value, "|") {
		op := schema.Operation(item)
		switch {
		case !slices.Contains(schema.AllOperations, op):
			p.errorf(m.pos, CodeInvalidValue, naming.Suggest(item, names), "unknown operation %q; use read, create, update or delete", item)
			return schema.Operations{}
		case ops.Has(op):
			p.errorf(m.pos, CodeInvalidValue, "", "operation %q is listed more than once", item)
			return schema.Operations{}
		}
		ops.Set(op)
	}
	return ops
}

func (p *parser) entityPos(e *schema.Entity, key string) schema.Pos {
	if set, ok := p.entityMarkers[e]; ok && set.has(key) {
		return set.get(key).pos
	}
	return e.Pos
}

func (p *parser) fieldPos(f *schema.Field, key string) schema.Pos {
	if set, ok := p.fieldMarkers[f]; ok && set.has(key) {
		return set.get(key).pos
	}
	return f.Pos
}

func (p *parser) checkAccess(e *schema.Entity) {
	routed := len(e.Roles) > 0 || len(e.WriteRoles) > 0 || e.Public.Any()
	switch {
	case e.NoRoutes && routed:
		for _, key := range []string{"public", "roles", "writeroles"} {
			if set := p.entityMarkers[e]; set.has(key) {
				p.errorf(set.get(key).pos, CodeInvalidCombination, "", "%s has no effect on %s, which has noroutes", key, e.Name)
			}
		}
	case e.ReadOnly && len(e.WriteRoles) > 0:
		p.errorf(p.entityPos(e, "writeroles"), CodeInvalidCombination, "", "writeroles has no effect on readonly entity %s, which has no write operations", e.Name)
	case e.ReadOnly && (e.Public.Create || e.Public.Update || e.Public.Delete):
		p.errorf(p.entityPos(e, "public"), CodeInvalidCombination, "read", "readonly entity %s only has read operations; use public=read", e.Name)
	}
	var owners []*schema.Field
	for _, f := range e.Fields {
		if f.Owner {
			owners = append(owners, f)
		}
	}
	for _, f := range owners[min(1, len(owners)):] {
		p.errorf(p.fieldPos(f, "owner"), CodeInvalidCombination, "", "entity %s declares more than one owner field (%s and %s)", e.Name, owners[0].Name, f.Name)
	}
	if len(owners) > 0 && e.Public.Any() {
		p.errorf(p.entityPos(e, "public"), CodeInvalidCombination, "", "%s has the owner field %s, so its rows always belong to a caller and cannot be public", e.Name, owners[0].Name)
	}
}

func (p *parser) checkAuthFields(e *schema.Entity, jsonNames map[string]*schema.Field) {
	byFlag := map[string][]*schema.Field{}
	for _, f := range e.Fields {
		for key, on := range map[string]bool{"identity": f.Identity, "password": f.Password, "role": f.Role} {
			if on {
				byFlag[key] = append(byFlag[key], f)
			}
		}
	}
	if !e.IsAuth() {
		for _, key := range []string{"identity", "password", "role"} {
			for _, f := range byFlag[key] {
				p.errorf(p.fieldPos(f, key), CodeInvalidCombination, "", "%s is only valid on the entity that declares auth=; add auth=session to %s or remove the marker", key, e.Name)
			}
		}
		if e.Register {
			p.errorf(p.entityPos(e, "register"), CodeInvalidCombination, "", "register needs auth= on the same entity")
		}
		return
	}
	authPos := p.entityPos(e, "auth")
	for _, key := range []string{"identity", "password"} {
		if len(byFlag[key]) == 0 {
			p.errorf(authPos, CodeInvalidCombination, "", "account entity %s needs a field marked %s", e.Name, key)
		}
	}
	for _, key := range []string{"identity", "password", "role"} {
		for _, f := range byFlag[key][min(1, len(byFlag[key])):] {
			p.errorf(p.fieldPos(f, key), CodeInvalidCombination, "", "account entity %s declares more than one %s field (%s and %s)", e.Name, key, byFlag[key][0].Name, f.Name)
		}
	}
	switch {
	case e.ReadOnly:
		p.errorf(p.entityPos(e, "readonly"), CodeInvalidCombination, "", "account entity %s cannot be readonly: accounts are created by register or POST %s", e.Name, e.Path())
	case e.Public.Any():
		p.errorf(p.entityPos(e, "public"), CodeInvalidCombination, "register", "account entity %s cannot be public; use register for self sign-up", e.Name)
	}
	for _, f := range e.Fields {
		if f.Owner {
			p.errorf(p.fieldPos(f, "owner"), CodeInvalidCombination, "", "the account entity %s cannot have an owner field", e.Name)
		}
	}
	if pw := e.Password(); pw != nil {
		if pw.InputName() != pw.Name && e.Field(pw.InputName()) != nil {
			p.errorf(pw.Pos, CodeNameCollision, "", "password field %s is sent as %s, which collides with the field %s", pw.Name, pw.InputName(), pw.InputName())
		}
		if other, ok := jsonNames[pw.InputJSON()]; ok && other != pw {
			p.errorf(pw.Pos, CodeNameCollision, "", "password field %s is sent as the JSON member %s, which collides with the field %s", pw.Name, pw.InputJSON(), other.Name)
		}
	}
	if role := e.Role(); role != nil {
		if e.Register && !role.HasDefault {
			p.errorf(p.fieldPos(role, "role"), CodeInvalidCombination, "default="+firstOr(role.Enum, "member"), "register creates accounts without a role, so role %s needs default=", role.Name)
		}
		if !e.NoRoutes && len(e.Roles) == 0 && len(e.WriteRoles) == 0 {
			p.errorf(p.fieldPos(role, "role"), CodeInvalidCombination, "writeroles="+firstOr(role.Enum, "admin"),
				"%s has the role field %s but its routes are open to every authenticated user, who could change their own role; add roles= or writeroles= to %s", e.Name, role.Name, e.Name)
		}
	}
}

func firstOr(items []string, fallback string) string {
	if len(items) > 0 {
		return items[0]
	}
	return fallback
}

func (p *parser) parseRoles(m marker) []string {
	var roles []string
	for _, item := range strings.Split(m.value, "|") {
		switch {
		case !enumItemPattern.MatchString(item):
			p.errorf(m.pos, CodeInvalidValue, "", "role %q may only contain letters, digits, '_', '-' and '.'", item)
			return nil
		case slices.Contains(roles, item):
			p.errorf(m.pos, CodeInvalidValue, "", "role %q is listed more than once", item)
			return nil
		}
		roles = append(roles, item)
	}
	return roles
}

func sqlIdentifierSuggestion(s string) string {
	candidate := naming.Snake(s)
	if naming.IsSQLIdentifier(candidate) && candidate != s {
		return candidate
	}
	return ""
}

type fieldType struct {
	kind     schema.Kind
	nullable bool
	relation string
	entity   string
	invalid  string
}

func (p *parser) classify(t types.Type) fieldType {
	if t == nil {
		return fieldType{invalid: "unknown type"}
	}
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		elem := types.Unalias(ptr.Elem())
		if _, double := elem.(*types.Pointer); double {
			return fieldType{invalid: types.TypeString(t, qualifier)}
		}
		inner := p.classifyBase(elem)
		if inner.relation != "" {
			return fieldType{invalid: types.TypeString(t, qualifier)}
		}
		inner.nullable = true
		return inner
	}
	return p.classifyBase(t)
}

func qualifier(pkg *types.Package) string {
	if pkg.Path() == "specs" {
		return ""
	}
	return pkg.Name()
}

func (p *parser) classifyBase(t types.Type) fieldType {
	switch t := t.(type) {
	case *types.Basic:
		switch t.Kind() {
		case types.String:
			return fieldType{kind: schema.KindString}
		case types.Bool:
			return fieldType{kind: schema.KindBool}
		case types.Int16:
			return fieldType{kind: schema.KindInt16}
		case types.Int32:
			return fieldType{kind: schema.KindInt32}
		case types.Int:
			return fieldType{kind: schema.KindInt}
		case types.Int64:
			return fieldType{kind: schema.KindInt64}
		case types.Float32:
			return fieldType{kind: schema.KindFloat32}
		case types.Float64:
			return fieldType{kind: schema.KindFloat64}
		}
	case *types.Slice:
		elem := types.Unalias(t.Elem())
		if b, ok := elem.(*types.Basic); ok && b.Kind() == types.Byte {
			return fieldType{kind: schema.KindBytes}
		}
		if named, ok := elem.(*types.Named); ok && p.isEntity(named) {
			return fieldType{relation: named.Obj().Name()}
		}
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() != nil {
			switch {
			case obj.Pkg().Path() == importTime && obj.Name() == "Time":
				return fieldType{kind: schema.KindTime}
			case (obj.Pkg().Path() == importStdUUID || obj.Pkg().Path() == importGoogleUUID) && obj.Name() == "UUID":
				return fieldType{kind: schema.KindUUID}
			}
		}
		if p.isEntity(t) {
			return fieldType{entity: obj.Name()}
		}
	}
	return fieldType{invalid: types.TypeString(t, qualifier)}
}

func (p *parser) isEntity(named *types.Named) bool {
	if p.pkg == nil || named.Obj().Pkg() != p.pkg {
		return false
	}
	_, ok := named.Underlying().(*types.Struct)
	return ok
}

const supportedTypes = "string, bool, int16, int32, int, int64, float32, float64, []byte, time.Time, uuid.UUID, pointers to these, or []Entity for relations"

func (p *parser) buildField(e *schema.Entity, ident *ast.Ident, field *ast.Field, ft fieldType, markers []marker) {
	typePos := p.pos(field.Type.Pos())
	switch {
	case ft.invalid != "":
		p.errorf(typePos, CodeUnsupportedType, "", "field %s has unsupported type %s; supported types are %s", ident.Name, ft.invalid, supportedTypes)
		return
	case ft.entity != "":
		p.errorf(typePos, CodeUnsupportedType, "", "field %s cannot hold the entity %s; reference it with %sID <pk type> `gen:\"rel=belongs_to:%s\"`",
			ident.Name, ft.entity, ft.entity, ft.entity)
		return
	case ft.relation != "":
		p.buildRelation(e, ident, ft, p.markerSet(markers, scopeField))
		return
	}
	if reservedFieldNames[ident.Name] {
		p.errorf(p.pos(ident.Pos()), CodeReservedName, "", "field name %s collides with a generated method; choose another name", ident.Name)
	}
	set := p.markerSet(markers, scopeField)
	f := &schema.Field{
		Name:     ident.Name,
		Kind:     ft.kind,
		Nullable: ft.nullable,
		Column:   naming.Snake(ident.Name),
		JSON:     naming.LowerCamel(ident.Name),
		Pos:      p.pos(ident.Pos()),
	}
	p.fieldMarkers[f] = set
	fb := fieldBuilder{p: p, f: f, set: set}
	fb.apply()
	e.Fields = append(e.Fields, f)
}

func (p *parser) buildRelation(e *schema.Entity, ident *ast.Ident, ft fieldType, set markerSet) {
	pos := p.pos(ident.Pos())
	if !set.has("rel") {
		p.errorf(pos, CodeRelationTypeMismatch, "", "field %s of type []%s must declare rel=has_many:%s or rel=many_to_many:%s", ident.Name, ft.relation, ft.relation, ft.relation)
		return
	}
	relMarker := set.get("rel")
	kind, target, ok := p.parseRel(relMarker)
	if !ok {
		return
	}
	if kind == "belongs_to" {
		p.errorf(relMarker.pos, CodeRelationTypeMismatch, "", "belongs_to needs a field with the primary key type of %s, not []%s", target, ft.relation)
		return
	}
	if target != ft.relation {
		p.errorf(relMarker.pos, CodeRelationTypeMismatch, ft.relation, "rel targets %s but field %s has type []%s", target, ident.Name, ft.relation)
		return
	}
	for _, key := range set.order {
		switch key {
		case "rel", "fk", "via":
		default:
			p.errorf(set.get(key).pos, CodeInvalidCombination, "", "%s cannot be used on relation field %s", key, ident.Name)
		}
	}
	r := &schema.Relation{Name: ident.Name, Kind: schema.RelationKind(kind), Target: target, Pos: pos}
	switch r.Kind {
	case schema.RelationHasMany:
		if set.has("via") {
			p.errorf(set.get("via").pos, CodeInvalidCombination, "", "via is only valid with rel=many_to_many")
		}
		if set.has("fk") {
			m := set.get("fk")
			if !token.IsIdentifier(m.value) || !ast.IsExported(m.value) {
				p.errorf(m.pos, CodeInvalidValue, "", "fk must name an exported field of %s", target)
			} else {
				r.FK = m.value
			}
		}
	case schema.RelationManyToMany:
		if set.has("fk") {
			p.errorf(set.get("fk").pos, CodeInvalidCombination, "", "fk is only valid with rel=has_many")
		}
		if !set.has("via") {
			p.errorf(relMarker.pos, CodeInvalidValue, "", "many_to_many requires via=<join_table>")
		} else {
			m := set.get("via")
			if !naming.IsSQLIdentifier(m.value) {
				p.errorf(m.pos, CodeInvalidValue, sqlIdentifierSuggestion(m.value), "join table name %q must match ^[a-z_][a-z0-9_]*$ and have at most 63 characters", m.value)
			} else {
				r.Via = m.value
			}
		}
	}
	p.relationMarkers[r] = set
	e.Relations = append(e.Relations, r)
}

var relKinds = []string{"belongs_to", "has_many", "many_to_many"}

func (p *parser) parseRel(m marker) (string, string, bool) {
	kind, target, ok := strings.Cut(m.value, ":")
	if !ok || target == "" {
		p.errorf(m.pos, CodeInvalidValue, "", "rel must be written as rel=<kind>:<Entity>, for example rel=belongs_to:Org")
		return "", "", false
	}
	if !slices.Contains(relKinds, kind) {
		p.errorf(m.pos, CodeInvalidValue, naming.Suggest(kind, relKinds), "unknown relation kind %q; use belongs_to, has_many or many_to_many", kind)
		return "", "", false
	}
	if !token.IsIdentifier(target) {
		p.errorf(m.pos, CodeInvalidValue, "", "relation target %q is not a valid entity name", target)
		return "", "", false
	}
	return kind, target, true
}

func (p *parser) checkEntity(e *schema.Entity) {
	var pks []*schema.Field
	for _, f := range e.Fields {
		if f.PK {
			pks = append(pks, f)
		}
	}
	switch {
	case len(pks) == 0:
		suggestion := ""
		if e.Field("ID") != nil {
			suggestion = "ID"
		}
		p.errorf(e.Pos, CodeMissingPK, suggestion, "entity %s has no primary key; add gen:\"pk\" to one field", e.Name)
	case len(pks) > 1:
		for _, f := range pks[1:] {
			p.errorf(p.fieldMarkers[f].get("pk").pos, CodeMultiplePK, "", "entity %s declares more than one primary key (%s and %s); composite keys are not supported", e.Name, pks[0].Name, f.Name)
		}
	}
	columns := map[string]*schema.Field{}
	jsonNames := map[string]*schema.Field{}
	for _, f := range e.Fields {
		if other, ok := columns[f.Column]; ok {
			p.errorf(f.Pos, CodeDuplicateColumn, "", "fields %s and %s both map to column %s", other.Name, f.Name, f.Column)
		} else {
			columns[f.Column] = f
		}
		if other, ok := jsonNames[f.JSON]; ok {
			p.errorf(f.Pos, CodeDuplicateJSON, "", "fields %s and %s both map to JSON member %s", other.Name, f.Name, f.JSON)
		} else {
			jsonNames[f.JSON] = f
		}
		if f.Filterable() && (f.JSON == "limit" || f.JSON == "offset") {
			p.errorf(f.Pos, CodeNameCollision, "", "field %s is a list filter and its JSON name %s collides with the pagination parameter", f.Name, f.JSON)
		}
	}
	if e.SoftDelete {
		if f, ok := columns["deleted_at"]; ok {
			p.errorf(f.Pos, CodeDuplicateColumn, "", "softdelete adds the deleted_at column automatically; remove field %s", f.Name)
		}
	}
	p.checkAccess(e)
	p.checkAuthFields(e, jsonNames)
	generated := map[string]string{
		e.Name: "entity", "Repository": "type", "NewRepository": "function", "Service": "type",
		"NewService": "function", "Handler": "type", "NewHandler": "function", "RegisterRoutes": "function",
		"CreateInput": "type", "UpdateInput": "type", "ListParams": "type", "Page": "type",
	}
	for _, f := range e.Fields {
		for _, item := range f.Enum {
			name := f.Name + naming.Pascal(item)
			if what, ok := generated[name]; ok {
				p.errorf(p.fieldMarkers[f].get("enum").pos, CodeNameCollision, "", "enum constant %s for value %q collides with the generated %s %s", name, item, what, name)
				continue
			}
			generated[name] = "constant"
		}
	}
}
