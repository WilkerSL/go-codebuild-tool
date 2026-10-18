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
		Routes:      !e.NoRoutes,
		SoftDelete:  e.SoftDelete,
		allEntities: m.Entities,
		Noun:        noun(e.Name),
		Nouns:       strings.ReplaceAll(e.Table, "_", " "),
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
	add("countQuery", q.Count)
	dv.GetArgs = queryArgs(q.Get, "")
	dv.ExistsArgs = queryArgs(q.Exists, "")
	dv.ListArgs = queryArgs(q.List, "")
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
	dv.SelectAfterIDArg = argExpr(nonNull(pk), "id")

	for _, f := range e.Fields {
		if f.PK || f.Unique {
			name := schema.ConstraintName("pk", e.Table)
			if !f.PK {
				name = schema.ConstraintName("ux", e.Table, f.Column)
			}
			dv.ConstraintCases = append(dv.ConstraintCases, constraintCase{Constraint: name, Column: f.Column, Field: f.JSON})
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
	buildTestPlan(dv, m)
	return dv
}

func newRelationView(d dialect.Dialect, source, target *schema.Entity, r *schema.Relation, dv *domainView) *relationView {
	spk := source.PK()
	if spk == nil {
		return nil
	}
	rv := &relationView{
		Kind:          string(r.Kind),
		Source:        source,
		SourceVar:     source.Var(),
		SourceIDVar:   source.Var() + "ID",
		SourceIDType:  goBase(spk.Kind),
		SourceParse:   pathParse(spk, source.PathParam()),
		SourcePackage: source.Domain,
		SourceHelper:  "new" + source.Name,
		Method:        "List" + source.Name + r.Name,
		Handler:       "list" + source.Name + r.Name,
		Path:          source.Path() + "/{" + source.PathParam() + "}/" + r.PathSegment(),
		ExistsConst:   source.Var() + "ExistsQuery",
		ListConst:     "list" + source.Name + r.Name + "Query",
		CountConst:    "count" + source.Name + r.Name + "Query",
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
	add(rv.ExistsConst, rq.SourceExists)
	add(rv.ListConst, rq.List)
	add(rv.CountConst, rq.Count)
	rv.ExistsArgs = queryArgs(rq.SourceExists, rv.SourceIDVar)
	rv.ListArgs = queryArgs(rq.List, rv.SourceIDVar)
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
		if f.Nullable {
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

func buildTestPlan(dv *domainView, m *schema.Model) {
	e := dv.Entity
	dv.Creatable = creatable(m, e, map[string]bool{})
	dv.MissingID = missingValue(e.PK())
	needed := map[string]bool{}
	var order []*schema.Entity
	var need func(x *schema.Entity)
	need = func(x *schema.Entity) {
		if needed[x.Name] || !creatable(m, x, map[string]bool{}) {
			return
		}
		needed[x.Name] = true
		for _, f := range x.BelongsTo() {
			if parent := m.Entity(f.Ref.Entity); parent != nil {
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
	}
	slices.SortFunc(order, func(a, b *schema.Entity) int { return strings.Compare(a.Name, b.Name) })
	imports := map[string]bool{}
	for _, x := range order {
		h := &helperView{
			Entity:    x,
			Package:   x.Domain,
			Qualifier: x.Domain + ".",
			Func:      "new" + x.Name,
			InputFunc: "new" + x.Name + "Input",
		}
		imports[x.Domain] = true
		for _, f := range x.Fields {
			if !f.InCreate() {
				continue
			}
			switch {
			case f.Ref != nil && !f.Nullable:
				parent := m.Entity(f.Ref.Entity)
				v := parentVar(f, dv.TestImportNames())
				h.Parents = append(h.Parents, parentRef{Var: v, Helper: "new" + parent.Name})
				h.Fields = append(h.Fields, fmt.Sprintf("%s: %s.%s,", f.Name, v, parent.PK().Name))
			case f.Ref != nil || f.Nullable:
			default:
				h.Fields = append(h.Fields, fmt.Sprintf("%s: %s,", f.Name, validValue(f, h.Qualifier, f.Unique || f.PK)))
			}
		}
		dv.TestHelpers = append(dv.TestHelpers, h)
	}
	for name := range imports {
		dv.TestImports = append(dv.TestImports, dv.Module+"/internal/"+name)
	}
	slices.Sort(dv.TestImports)
	dv.OwnHelper = "new" + e.Name
	dv.OwnInputHelper = "new" + e.Name + "Input"
	for _, fv := range dv.Fields {
		f := fv.F
		if dv.UniqueTestField == nil && f.Unique && !f.Nullable && f.Ref == nil && f.InCreate() {
			dv.UniqueTestField = fv
		}
		if dv.MissingRefField == nil && f.Ref != nil && !f.Nullable && f.InCreate() {
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
		sourceOK := creatable(m, rv.Source, map[string]bool{})
		switch {
		case rv.FK != nil:
			rv.TestCreate = dv.Creatable && (!rv.FK.F.Nullable || sourceOK)
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
		"t": true, "ctx": true, "db": true, "repo": true, "in": true, "item": true, "err": true,
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
