package gen

import (
	"fmt"
	"go/token"
	"slices"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

func newDomainView(pv *projectView, d dialect.Dialect, m *schema.Model, e *schema.Entity) *domainView {
	dv := &domainView{
		projectView: pv,
		Entity:      e,
		Package:     e.Domain,
		Import:      pv.Module + "/internal/" + e.Domain,
		Name:        e.Name,
		Table:       e.Table,
		Path:        e.Path(),
		ItemPath:    e.Path() + "/{id}",
		ReadOnly:    e.ReadOnly,
		Routes:      !e.NoRoutes || pv.Workspace != nil,
		SoftDelete:  e.SoftDelete,
		allEntities: m.Entities,
		Noun:        noun(e.Name),
		Nouns:       strings.ReplaceAll(e.Table, "_", " "),
		Guarded:     pv.Auth != nil,
		IsAuth:      e.IsAuth(),
	}
	if owner := e.Owner(); owner != nil {
		dv.Owned = true
		dv.OwnerParam = "ownerID " + goBase(owner.Kind) + ", "
		dv.OwnerArg = "ownerID, "
	}
	for _, f := range e.Fields {
		fv := newFieldView(f, pv.Dialect)
		dv.Fields = append(dv.Fields, fv)
		if f.PK {
			dv.PK = fv
		}
		if f.InCreate() && !e.ReadOnly {
			dv.Create = append(dv.Create, fv)
			if f.Required {
				dv.RequiredCreate = append(dv.RequiredCreate, fv)
			}
		}
		if f.InUpdate() && !e.ReadOnly {
			dv.Update = append(dv.Update, fv)
			if !f.Nullable {
				dv.RequiredUpdate = append(dv.RequiredUpdate, fv)
			}
		}
		if f.Filterable() {
			dv.Filters = append(dv.Filters, fv)
		}
		if fv.HasRules && (f.InCreate() || f.InUpdate()) && !e.ReadOnly {
			dv.Checked = append(dv.Checked, fv)
		}
		dv.Enums = append(dv.Enums, fv.EnumConsts...)
	}
	pk := e.PK()
	dv.PKParse = pathParse(pk, "id")
	dv.PKParseItem = pathParse(pk, e.PathParam())
	dv.PKLocation = idString(pk, "item."+pk.Name)
	q := dialect.BuildEntityQueries(d, e)
	dv.IdentityPK = q.IdentityPK
	dv.AutoUUID = pk.AutoPK && pk.Kind == schema.KindUUID
	dv.HasUpdate = q.Update != nil
	add := func(name string, query dialect.Query) {
		if slices.ContainsFunc(dv.Queries, func(c queryConst) bool { return c.Name == name }) {
			return
		}
		dv.Queries = append(dv.Queries, queryConst{Name: name, SQL: query.SQL, Literal: goStringLiteral(query.SQL)})
	}
	add("getQuery", q.Get)
	add("existsQuery", q.Exists)
	add("listQuery", q.List)
	add("listAfterQuery", q.ListAfter)
	add("countQuery", q.Count)
	dv.GetArgs = queryArgs(q.Get, "")
	dv.ExistsArgs = queryArgs(q.Exists, "")
	dv.ListArgs = queryArgs(q.List, "")
	dv.ListAfterArgs = queryArgs(q.ListAfter, "")
	if f := e.AutoCreateField(); f != nil {
		for _, fv := range dv.Fields {
			if fv.F == f {
				dv.CursorTime = fv
			}
		}
	}
	dv.CountArgs = queryArgs(q.Count, "")
	if q.Create != nil {
		add("createQuery", *q.Create)
		dv.CreateArgs = queryArgs(*q.Create, "")
	}
	if q.Update != nil {
		add("updateQuery", *q.Update)
		dv.UpdateArgs = queryArgs(*q.Update, "")
	}
	if q.Delete != nil {
		add("deleteQuery", *q.Delete)
		dv.DeleteArgs = queryArgs(*q.Delete, "")
	}
	dv.SelectAfterArgs = dv.GetArgs
	if !e.ReadOnly {
		for _, fv := range dv.Fields {
			f := fv.F
			if f.Ref == nil || f.Owner || (!f.InCreate() && !f.InUpdate()) || f.External(e) {
				continue
			}
			target := m.Entity(f.Ref.Entity)
			if target == nil || target.PK() == nil {
				continue
			}
			var rq dialect.Query
			ref := refCheck{Field: fv, Const: target.Var() + "ExistsQuery"}
			switch {
			case dv.Owned && target.Owner() != nil:
				rq = dialect.OwnedReferenceQuery(d, target)
			case target.Owner() != nil && len(dialect.Visible(d, target, false)) > 0:
				rq = dialect.LiveReferenceQuery(d, target)
				ref.Const = target.Var() + "LiveQuery"
			case target.Owner() == nil && len(dialect.Visible(d, target, false)) > 0:
				rq = dialect.LiveReferenceQuery(d, target)
			default:
				continue
			}
			value := "in." + f.Name
			if f.Nullable {
				value = "*" + value
			}
			add(ref.Const, rq)
			ref.Args = queryArgs(rq, value)
			dv.RefChecks = append(dv.RefChecks, ref)
		}
		for _, rule := range dialect.SoftDeleteRules(d, m, e) {
			action := "nullify"
			if rule.Restrict {
				action = "restrict"
				dv.HasRestrict = true
			}
			name := action + naming.Pascal(rule.Child.Table) + rule.Field.Name + "Query"
			add(name, rule.Query)
			dv.DeleteRules = append(dv.DeleteRules, deleteRule{Restrict: rule.Restrict, Const: name, Args: queryArgs(rule.Query, "")})
		}
	}

	for _, f := range e.Fields {
		if f.PK || f.Unique {
			name := schema.ConstraintName("pk", e.Table)
			if !f.PK {
				name = schema.ConstraintName("ux", e.Table, f.Column)
			}
			dv.ConstraintCases = append(dv.ConstraintCases, constraintCase{Constraint: name, Column: f.Column, Field: f.JSON})
		}
	}
	for _, f := range e.Fields {
		if !f.Nullable && !f.Auto() && !f.Owner && (f.InCreate() || f.InUpdate()) {
			dv.NotNullCases = append(dv.NotNullCases, constraintCase{Column: f.Column, Field: f.InputJSON()})
		}
	}
	refs := e.BelongsTo()
	for _, f := range refs {
		dv.ConstraintCases = append(dv.ConstraintCases, constraintCase{Constraint: schema.ConstraintName("fk", e.Table, f.Column), Column: f.Column, Field: f.JSON})
	}
	if len(refs) == 1 {
		dv.FKFallback = refs[0].JSON
	}

	for _, fv := range dv.Create {
		if fv.HasRules {
			dv.CreateValidate = append(dv.CreateValidate, validateCall(fv.F))
		}
	}
	for _, fv := range dv.Update {
		if fv.HasRules {
			dv.UpdateValidate = append(dv.UpdateValidate, validateCall(fv.F))
		}
	}

	for _, source := range m.Entities {
		for _, r := range source.Relations {
			if r.Target != e.Name {
				continue
			}
			rv := newRelationView(d, source, e, r, dv)
			if rv == nil {
				continue
			}
			dv.Relations = append(dv.Relations, rv)
			if r.Kind == schema.RelationHasMany {
				dv.HasManyRelations = append(dv.HasManyRelations, rv)
			} else {
				dv.ManyToManyRels = append(dv.ManyToManyRels, rv)
			}
		}
	}
	buildRoutes(dv)
	buildTestPlan(dv, d, m)
	buildServiceMethods(dv)
	if dv.IsAuth {
		buildAuthMethods(dv)
	}
	return dv
}

func buildRoutes(dv *domainView) {
	if !dv.Routes {
		return
	}
	e := dv.Entity
	add := func(method, path, handler string, policy schema.Policy) {
		dv.RouteLines = append(dv.RouteLines, routeLine(method, path, "h."+handler, policy, dv.Guarded))
	}
	add("GET", dv.Path, "list", e.Policy(schema.OpRead))
	if !e.ReadOnly {
		add("POST", dv.Path, "idempotent(h.create)", e.Policy(schema.OpCreate))
	}
	add("GET", dv.ItemPath, "get", e.Policy(schema.OpRead))
	if dv.HasUpdate {
		add("PUT", dv.ItemPath, "update", e.Policy(schema.OpUpdate))
	}
	if !e.ReadOnly {
		add("DELETE", dv.ItemPath, "delete", e.Policy(schema.OpDelete))
	}
	for _, rv := range dv.Relations {
		list, link, _ := schema.RelationPolicies(rv.Source, e, schema.RelationKind(rv.Kind))
		add("GET", rv.Path, rv.Handler, list)
		if rv.Writable {
			add("PUT", rv.ItemPath, rv.LinkHandler, link)
			add("DELETE", rv.ItemPath, rv.UnlinkHandler, link)
		}
	}
}

func newRelationView(d dialect.Dialect, source, target *schema.Entity, r *schema.Relation, dv *domainView) *relationView {
	spk := source.PK()
	if spk == nil {
		return nil
	}
	rv := &relationView{
		Kind:           string(r.Kind),
		Source:         source,
		SourceVar:      source.Var(),
		SourceIDVar:    source.Var() + "ID",
		SourceIDType:   goBase(spk.Kind),
		SourceParse:    pathParse(spk, source.PathParam()),
		SourcePackage:  source.Domain,
		SourceHelper:   "fixture" + source.Name,
		Method:         "List" + source.Name + r.Name,
		Handler:        "list" + source.Name + r.Name,
		Path:           source.Path() + "/{" + source.PathParam() + "}/" + r.PathSegment(),
		ExistsConst:    source.Var() + "ExistsQuery",
		ListConst:      "list" + source.Name + r.Name + "Query",
		ListAfterConst: "list" + source.Name + r.Name + "AfterQuery",
		CountConst:     "count" + source.Name + r.Name + "Query",
	}
	for _, owner := range []*schema.Field{source.Owner(), target.Owner()} {
		if owner != nil {
			rv.Owned = true
			rv.OwnerParam = "ownerID " + goBase(owner.Kind) + ", "
			rv.OwnerArg = "ownerID, "
		}
	}
	add := func(name string, q dialect.Query) {
		if slices.ContainsFunc(dv.Queries, func(c queryConst) bool { return c.Name == name }) {
			return
		}
		dv.Queries = append(dv.Queries, queryConst{Name: name, SQL: q.SQL, Literal: goStringLiteral(q.SQL)})
	}
	var rq dialect.RelationQueries
	switch r.Kind {
	case schema.RelationHasMany:
		fk := target.Field(r.FK)
		if fk == nil {
			return nil
		}
		rq = dialect.BuildHasManyQueries(d, source, target, fk)
		for _, fv := range dv.Fields {
			if fv.F == fk {
				rv.FK = fv
			}
		}
		rv.FKIsOwner = fk.Owner
	case schema.RelationManyToMany:
		rv.Writable = !target.ReadOnly
		rq = dialect.BuildManyToManyQueries(d, source, target, r.Via, rv.Writable)
		rv.ItemPath = rv.Path + "/{" + target.PathParam() + "}"
		rv.LinkMethod = "Link" + source.Name + r.Name
		rv.UnlinkMethod = "Unlink" + source.Name + r.Name
		rv.LinkHandler = "link" + source.Name + r.Name
		rv.UnlinkHandler = "unlink" + source.Name + r.Name
		rv.LinkConst = "link" + source.Name + r.Name + "Query"
		rv.UnlinkConst = "unlink" + source.Name + r.Name + "Query"
	}
	rv.ExternalSource = !schema.SameDatabase(source, target)
	if !rv.ExternalSource {
		add(rv.ExistsConst, rq.SourceExists)
	}
	add(rv.ListConst, rq.List)
	add(rv.ListAfterConst, rq.ListAfter)
	add(rv.CountConst, rq.Count)
	rv.ExistsArgs = queryArgs(rq.SourceExists, rv.SourceIDVar)
	rv.ListArgs = queryArgs(rq.List, rv.SourceIDVar)
	rv.ListAfterArgs = queryArgs(rq.ListAfter, rv.SourceIDVar)
	rv.CountArgs = queryArgs(rq.Count, rv.SourceIDVar)
	if rq.Link != nil {
		add(rv.LinkConst, *rq.Link)
		add(rv.UnlinkConst, *rq.Unlink)
		rv.LinkArgs = queryArgs(*rq.Link, rv.SourceIDVar)
		rv.UnlinkArgs = queryArgs(*rq.Unlink, rv.SourceIDVar)
	}
	return rv
}

func creatable(m *schema.Model, e *schema.Entity, seen map[string]bool) bool {
	if e.ReadOnly || seen[e.Name] {
		return false
	}
	seen[e.Name] = true
	defer delete(seen, e.Name)
	for _, f := range e.BelongsTo() {
		if f.Nullable || f.CrossService(e) {
			continue
		}
		parent := m.Entity(f.Ref.Entity)
		if parent == nil || !creatable(m, parent, seen) {
			return false
		}
	}
	return true
}

func missingID(f *schema.Field) string {
	switch {
	case f.Kind == schema.KindUUID:
		return "uuid.New()"
	case f.Kind.IsInteger():
		return "-1"
	}
	return `"zz-missing"`
}

func buildTestPlan(dv *domainView, d dialect.Dialect, m *schema.Model) {
	e := dv.Entity
	dv.Creatable = creatable(m, e, map[string]bool{})
	dv.MissingID = missingValue(e.PK())
	needed := map[string]bool{}
	var order []*schema.Entity
	var need func(x *schema.Entity)
	need = func(x *schema.Entity) {
		if needed[x.Name] || x.Service != e.Service || !creatable(m, x, map[string]bool{}) {
			return
		}
		needed[x.Name] = true
		for _, f := range x.BelongsTo() {
			if parent := m.Entity(f.Ref.Entity); parent != nil && !f.CrossService(x) {
				need(parent)
			}
		}
		order = append(order, x)
	}
	if dv.Creatable {
		need(e)
	}
	for _, rv := range dv.Relations {
		need(rv.Source)
		if rv.Owned {
			if auth := m.AuthEntity(); auth != nil {
				need(auth)
			}
		}
	}
	slices.SortFunc(order, func(a, b *schema.Entity) int { return strings.Compare(a.Name, b.Name) })
	rows := &rowFixtures{d: d, holder: e, made: map[string]*rowFixture{}}
	ownerFixture := ""
	if auth := m.AuthEntity(); auth != nil && auth.PK() != nil {
		switch {
		case auth.Service == e.Service:
			ownerFixture = "fixture" + auth.Name + "(t, ctx, db)." + auth.PK().Name
		case !schema.SameDatabase(auth, e):
			ownerFixture = randomValue(auth.PK())
		default:
			ownerFixture = rows.call(auth, "") + "." + auth.PK().Name
		}
	}
	call := func(x *schema.Entity, owner string) string {
		if x.Service != e.Service {
			return rows.call(x, owner)
		}
		if x.Owner() == nil {
			return "fixture" + x.Name + "(t, ctx, db)"
		}
		if owner == "" {
			owner = ownerFixture
		}
		return "fixture" + x.Name + "(t, ctx, db, " + owner + ")"
	}
	rows.owner = func() string { return ownerFixture }
	imports := map[string]bool{}
	for _, x := range order {
		h := &helperView{
			Entity:    x,
			Package:   x.Domain,
			Qualifier: x.Domain + ".",
			Func:      "fixture" + x.Name,
			InputFunc: "fixture" + x.Name + "Input",
		}
		if owner := x.Owner(); owner != nil {
			h.OwnerType = goBase(owner.Kind)
		}
		imports[x.Domain] = true
		for _, f := range x.Fields {
			if !f.InCreate() {
				continue
			}
			switch {
			case f.Ref != nil && !f.Nullable && f.External(x):
				h.Fields = append(h.Fields, fmt.Sprintf("%s: %s,", f.Name, randomValue(f.Ref.Target.PK())))
			case f.Ref != nil && !f.Nullable:
				parent := m.Entity(f.Ref.Entity)
				v := parentVar(f, dv.TestImportNames())
				h.Parents = append(h.Parents, parentRef{Var: v, Call: call(parent, "")})
				h.Fields = append(h.Fields, fmt.Sprintf("%s: %s.%s,", f.Name, v, parent.PK().Name))
			case f.Ref != nil || f.Nullable:
			default:
				h.Fields = append(h.Fields, fmt.Sprintf("%s: %s,", f.InputName(), validValue(f, h.Qualifier, f.Unique || f.PK)))
			}
		}
		dv.TestHelpers = append(dv.TestHelpers, h)
	}
	dv.TestOwner = "owner := " + ownerFixture
	if owner := e.Owner(); owner != nil {
		dv.TestOwnerArg = "owner, "
		dv.TestOwnerSuffix = ", owner"
		dv.MissingOwner = missingValue(owner)
	}
	for _, rv := range dv.Relations {
		if rv.Owned {
			rv.TestOwnerArg = "owner, "
		}
		if rv.ExternalSource {
			continue
		}
		rv.SourceCall = call(rv.Source, "owner")
		if rv.FK != nil && !rv.FKIsOwner && (rv.FK.F.Nullable || rv.Source.Owner() != nil) {
			rv.ExplicitParent = true
		}
	}
	dv.RowFixtures = rows.list
	for name := range imports {
		dv.TestImports = append(dv.TestImports, dv.Module+"/internal/"+name)
	}
	slices.Sort(dv.TestImports)
	dv.OwnHelper = "fixture" + e.Name
	dv.OwnInputHelper = "fixture" + e.Name + "Input"
	for _, fv := range dv.Fields {
		f := fv.F
		if dv.UniqueTestField == nil && f.Unique && !f.Nullable && f.Ref == nil && f.InCreate() {
			dv.UniqueTestField = fv
		}
		if dv.MissingRefField == nil && f.Ref != nil && !f.Nullable && f.InCreate() && !f.External(e) {
			dv.MissingRefField = fv
		}
		if dv.ListFilterField == nil && f.Unique && f.Filterable() && f.Kind == schema.KindString {
			dv.ListFilterField = fv
		}
	}
	dv.MissingRefNamed = dv.Dialect.NamedFKErrors || len(e.BelongsTo()) == 1
	if dv.MissingRefField != nil {
		dv.MissingRefValue = missingValue(dv.MissingRefField.F)
	}
	for _, rv := range dv.Relations {
		rv.MissingSourceID = missingValue(rv.Source.PK())
		sourceOK := rv.Source.Service != e.Service || creatable(m, rv.Source, map[string]bool{})
		switch {
		case rv.FK != nil && rv.ExternalSource:
			rv.TestCreate = dv.Creatable && !rv.FK.F.Nullable && !rv.FKIsOwner
		case rv.FK != nil:
			rv.TestCreate = dv.Creatable && (!rv.ExplicitParent || sourceOK)
		case rv.Writable:
			rv.TestLink = dv.Creatable && sourceOK
		}
	}
	for _, fv := range dv.Update {
		f := fv.F
		switch {
		case f.Ref != nil || f.Nullable:
			dv.UpdateTestValues = append(dv.UpdateTestValues, fmt.Sprintf("%s: created.%s,", f.Name, f.Name))
		case f.Unique:
			dv.UpdateTestValues = append(dv.UpdateTestValues, fmt.Sprintf("%s: created.%s,", f.Name, f.Name))
		default:
			dv.UpdateTestValues = append(dv.UpdateTestValues, fmt.Sprintf("%s: %s,", f.Name, validValue(f, e.Domain+".", false)))
		}
	}
}

func parentVar(f *schema.Field, taken map[string]bool) string {
	v := naming.LowerCamel(strings.TrimSuffix(f.Name, "ID"))
	if v == "" || token.IsKeyword(v) || taken[v] {
		return naming.LowerCamel(f.Name) + "Row"
	}
	return v
}

func (dv *domainView) TestImportNames() map[string]bool {
	taken := map[string]bool{
		"context": true, "database": true, "errors": true, "fmt": true, "migrations": true, "os": true,
		"rand": true, "sql": true, "strings": true, "testing": true, "time": true, "uuid": true, "validation": true,
		"t": true, "ctx": true, "db": true, "owner": true, "parent": true, "repo": true, "in": true, "item": true, "err": true,
	}
	for _, e := range dv.allEntities {
		taken[e.Domain] = true
	}
	return taken
}

func missingValue(f *schema.Field) string {
	if f.Kind == schema.KindString {
		n := 10
		if size := schema.ColumnSize(f); size > 0 {
			n = min(n, size)
		}
		return fmt.Sprintf("%q", strings.Repeat("z", n))
	}
	return missingID(f)
}

func buildServiceMethods(dv *domainView) {
	e := dv.Entity
	pk := goBase(e.PK().Kind)
	add := func(name, params, results, args string) {
		dv.ServiceMethods = append(dv.ServiceMethods, serviceMethod{
			Name:    name,
			Params:  "ctx context.Context, " + params,
			Results: results,
			Args:    "ctx, " + args,
			Body:    "return s.repo." + name + "(ctx, " + args + ")",
			File:    "service_" + naming.Snake(name) + ".go",
		})
	}
	own, ownArg := dv.OwnerParam, dv.OwnerArg
	add("List", own+"p ListParams", "(Page, error)", ownArg+"p")
	add("Get", own+"id "+pk, "("+e.Name+", error)", ownArg+"id")
	if dv.Workspace != nil {
		add("Exists", own+"id "+pk, "(bool, error)", ownArg+"id")
	}
	if !e.ReadOnly {
		add("Create", own+"in CreateInput", "("+e.Name+", error)", ownArg+"in")
		if dv.HasUpdate {
			add("Update", own+"id "+pk+", in UpdateInput", "("+e.Name+", error)", ownArg+"id, in")
		}
		add("Delete", own+"id "+pk, "error", ownArg+"id")
	}
	if !dv.Routes {
		return
	}
	for _, rv := range dv.Relations {
		source := rv.OwnerParam + rv.SourceIDVar + " " + rv.SourceIDType
		args := rv.OwnerArg + rv.SourceIDVar
		add(rv.Method, source+", p ListParams", "(Page, error)", args+", p")
		if rv.Writable {
			add(rv.LinkMethod, source+", id "+pk, "error", args+", id")
			add(rv.UnlinkMethod, source+", id "+pk, "error", args+", id")
		}
	}
}

func buildAuthMethods(dv *domainView) {
	e := dv.Entity
	add := func(name, params, results, body string) {
		dv.AuthMethods = append(dv.AuthMethods, serviceMethod{
			Name:    name,
			Params:  "ctx context.Context, " + params,
			Results: results,
			Body:    body,
			File:    "auth_service_" + naming.Snake(name) + ".go",
		})
	}
	if e.Register {
		add("Register", "in CreateInput", "("+e.Name+", error)", "return s.repo.Create(ctx, in)")
	}
	add("Login", "in LoginInput", "(auth.Tokens, error)",
		"user, err := s.accounts.VerifyPassword(ctx, in.Identity, in.Password)\n\tif err != nil {\n\t\treturn auth.Tokens{}, err\n\t}\n\treturn s.accounts.Issue(ctx, user)")
	if e.HasMethod(schema.MethodJWT) {
		add("Refresh", "refreshToken string", "(auth.Tokens, error)", "return s.accounts.Refresh(ctx, refreshToken)")
	}
	add("Logout", "caller auth.Principal", "error", "return s.accounts.Revoke(ctx, caller)")
	add("Me", "caller auth.Principal", "("+e.Name+", error)", "return s.repo.Get(ctx, caller.ID)")
	add("ChangePassword", "caller auth.Principal, in PasswordChange", "error", "return s.accounts.ChangePassword(ctx, caller, in)")
	if e.HasMethod(schema.MethodAPIKey) {
		add("CreateAPIKey", "caller auth.Principal, in APIKeyInput", "(CreatedAPIKey, error)", "return s.accounts.CreateAPIKey(ctx, caller, in)")
		add("ListAPIKeys", "caller auth.Principal", "([]APIKey, error)", "return s.accounts.ListAPIKeys(ctx, caller)")
		add("DeleteAPIKey", "caller auth.Principal, id string", "error", "return s.accounts.DeleteAPIKey(ctx, caller, id)")
	}
}
