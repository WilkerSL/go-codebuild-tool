package dialect

import (
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type ArgKind string

const (
	ArgID       ArgKind = "id"
	ArgInput    ArgKind = "input"
	ArgFilter   ArgKind = "filter"
	ArgLimit    ArgKind = "limit"
	ArgOffset   ArgKind = "offset"
	ArgSourceID ArgKind = "source"
	ArgOwner    ArgKind = "owner"
	ArgPassword ArgKind = "password"
)

type Arg struct {
	Kind  ArgKind
	Field *schema.Field
}

type Query struct {
	SQL  string
	Args []Arg
}

type builder struct {
	d     Dialect
	args  []Arg
	index map[string]int
}

func newBuilder(d Dialect) *builder {
	return &builder{d: d, index: map[string]int{}}
}

func (b *builder) param(key string, arg Arg) string {
	if !b.d.Positional() {
		if n, ok := b.index[key]; ok {
			return b.d.Placeholder(n)
		}
	}
	b.args = append(b.args, arg)
	b.index[key] = len(b.args)
	return b.d.Placeholder(len(b.args))
}

func (b *builder) query(sql string) Query {
	return Query{SQL: sql, Args: b.args}
}

type EntityQueries struct {
	Strategy   InsertStrategy
	Columns    []string
	Get        Query
	Exists     Query
	List       Query
	Count      Query
	Create     *Query
	Update     *Query
	Delete     *Query
	IdentityPK bool
}

func columnRef(d Dialect, table, column string, qualified bool) string {
	if qualified {
		return d.Quote(table) + "." + d.Quote(column)
	}
	return d.Quote(column)
}

func returnedFields(d Dialect, e *schema.Entity) []Returned {
	out := make([]Returned, len(e.Fields))
	for i, f := range e.Fields {
		out[i] = Returned{Column: d.Quote(f.Column), Type: d.ColumnType(schema.FieldColumn(f))}
	}
	return out
}

func selectList(d Dialect, e *schema.Entity, qualified bool) []string {
	cols := make([]string, len(e.Fields))
	for i, f := range e.Fields {
		cols[i] = columnRef(d, e.Table, f.Column, qualified)
	}
	return cols
}

func orderBy(d Dialect, e *schema.Entity, qualified bool) string {
	pk := columnRef(d, e.Table, e.PK().Column, qualified)
	if f := e.AutoCreateField(); f != nil {
		return "ORDER BY " + columnRef(d, e.Table, f.Column, qualified) + " DESC, " + pk + " DESC"
	}
	return "ORDER BY " + pk
}

func Visible(d Dialect, e *schema.Entity, qualified bool) []string {
	return visible(d, e, qualified, map[*schema.Entity]bool{})
}

func visible(d Dialect, e *schema.Entity, qualified bool, chain map[*schema.Entity]bool) []string {
	var conds []string
	if e.SoftDelete {
		conds = append(conds, columnRef(d, e.Table, schema.DeletedAtColumn, qualified)+" IS NULL")
	}
	chain[e] = true
	defer delete(chain, e)
	for _, f := range e.Fields {
		if f.Ref == nil || f.Ref.OnDelete != schema.OnDeleteCascade || f.Ref.Target == nil || chain[f.Ref.Target] || f.Ref.Target.PK() == nil || f.External(e) {
			continue
		}
		parent := f.Ref.Target
		inner := visible(d, parent, true, chain)
		if len(inner) == 0 {
			continue
		}
		ref := columnRef(d, e.Table, f.Column, true)
		cond := "EXISTS (SELECT 1 FROM " + d.Quote(parent.Table) + " WHERE " + columnRef(d, parent.Table, parent.PK().Column, true) + " = " + ref + " AND " + strings.Join(inner, " AND ") + ")"
		if f.Nullable {
			cond = "(" + ref + " IS NULL OR " + cond + ")"
		}
		conds = append(conds, cond)
	}
	return conds
}

func ownerCondition(b *builder, e *schema.Entity, qualified bool) []string {
	owner := e.Owner()
	if owner == nil {
		return nil
	}
	return []string{columnRef(b.d, e.Table, owner.Column, qualified) + " = " + b.param("owner", Arg{Kind: ArgOwner, Field: owner})}
}

func filterConditions(b *builder, e *schema.Entity, qualified bool, skip *schema.Field) []string {
	conds := ownerCondition(b, e, qualified)
	conds = append(conds, Visible(b.d, e, qualified)...)
	for _, f := range e.FilterFields() {
		if f == skip {
			continue
		}
		key := "filter." + f.Name
		first := b.param(key, Arg{Kind: ArgFilter, Field: f})
		second := b.param(key, Arg{Kind: ArgFilter, Field: f})
		conds = append(conds, "("+b.d.FilterParam(first, f.Kind.ColumnKind())+" IS NULL OR "+columnRef(b.d, e.Table, f.Column, qualified)+" = "+second+")")
	}
	return conds
}

func where(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return "\nWHERE " + strings.Join(conds, "\n    AND ")
}

func withHint(d Dialect, sql string, hasFilters bool) string {
	if hint := d.QueryHint(); hint != "" && hasFilters {
		return sql + "\n" + hint
	}
	return sql
}

func BuildEntityQueries(d Dialect, e *schema.Entity) EntityQueries {
	pk := e.PK()
	q := EntityQueries{Strategy: d.Strategy(), IdentityPK: pk.AutoPK && pk.Kind.IsInteger()}
	cols := selectList(d, e, false)
	q.Columns = cols
	table := d.Quote(e.Table)
	idCond := func(b *builder) string {
		cond := d.Quote(pk.Column) + " = " + b.param("id", Arg{Kind: ArgID, Field: pk})
		for _, c := range append(ownerCondition(b, e, false), Visible(d, e, false)...) {
			cond += " AND " + c
		}
		return cond
	}

	b := newBuilder(d)
	q.Get = b.query("SELECT\n" + indent(cols) + "\nFROM " + table + "\nWHERE " + idCond(b))

	b = newBuilder(d)
	q.Exists = b.query("SELECT 1\nFROM " + table + "\nWHERE " + idCond(b))

	b = newBuilder(d)
	conds := filterConditions(b, e, false, nil)
	hasFilters := len(e.FilterFields()) > 0
	list := "SELECT\n" + indent(cols) + "\nFROM " + table + where(conds) + "\n" + orderBy(d, e, false) + "\n" + d.Paginate(func(k ArgKind) string { return b.param(string(k), Arg{Kind: k}) })
	q.List = b.query(withHint(d, list, hasFilters))

	b = newBuilder(d)
	conds = filterConditions(b, e, false, nil)
	q.Count = b.query(withHint(d, "SELECT COUNT(*)\nFROM "+table+where(conds), hasFilters))

	if e.ReadOnly {
		return q
	}

	b = newBuilder(d)
	var insertCols, insertVals []string
	for _, f := range e.Fields {
		switch {
		case f.PK && q.IdentityPK:
		case f.PK && f.AutoPK:
			insertCols = append(insertCols, d.Quote(f.Column))
			insertVals = append(insertVals, b.param("id", Arg{Kind: ArgID, Field: f}))
		case f.AutoCreate || f.AutoUpdate:
			insertCols = append(insertCols, d.Quote(f.Column))
			insertVals = append(insertVals, d.Now())
		case f.Owner:
			insertCols = append(insertCols, d.Quote(f.Column))
			insertVals = append(insertVals, b.param("owner", Arg{Kind: ArgOwner, Field: f}))
		case f.Password:
			insertCols = append(insertCols, d.Quote(f.Column))
			insertVals = append(insertVals, b.param("password", Arg{Kind: ArgPassword, Field: f}))
		case f.InCreate():
			insertCols = append(insertCols, d.Quote(f.Column))
			insertVals = append(insertVals, b.param("in."+f.Name, Arg{Kind: ArgInput, Field: f}))
		}
	}
	create := b.query(d.InsertSQL(e.Table, insertCols, insertVals, returnedFields(d, e)))
	q.Create = &create

	updateFields := e.UpdateFields()
	hasAutoUpdate := false
	for _, f := range e.Fields {
		if f.AutoUpdate {
			hasAutoUpdate = true
		}
	}
	if len(updateFields) > 0 {
		b = newBuilder(d)
		var sets []string
		for _, f := range e.Fields {
			switch {
			case f.AutoUpdate:
				sets = append(sets, d.Quote(f.Column)+" = "+d.Now())
			case f.InUpdate():
				sets = append(sets, d.Quote(f.Column)+" = "+b.param("in."+f.Name, Arg{Kind: ArgInput, Field: f}))
			}
		}
		update := b.query(d.UpdateSQL(e.Table, sets, idCond(b), returnedFields(d, e)))
		q.Update = &update
	}

	b = newBuilder(d)
	var del Query
	if e.SoftDelete {
		sets := []string{d.Quote(schema.DeletedAtColumn) + " = " + d.Now()}
		if hasAutoUpdate {
			for _, f := range e.Fields {
				if f.AutoUpdate {
					sets = append(sets, d.Quote(f.Column)+" = "+d.Now())
				}
			}
		}
		del = b.query("UPDATE " + table + "\nSET " + strings.Join(sets, ", ") + "\nWHERE " + idCond(b))
	} else {
		del = b.query("DELETE FROM " + table + "\nWHERE " + idCond(b))
	}
	q.Delete = &del
	return q
}

type RelationQueries struct {
	SourceExists Query
	List         Query
	Count        Query
	Link         *Query
	Unlink       *Query
}

func existsQuery(d Dialect, e *schema.Entity) Query {
	b := newBuilder(d)
	pk := e.PK()
	cond := d.Quote(pk.Column) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: pk})
	for _, c := range append(ownerCondition(b, e, false), Visible(d, e, false)...) {
		cond += " AND " + c
	}
	return b.query("SELECT 1\nFROM " + d.Quote(e.Table) + "\nWHERE " + cond)
}

func LiveReferenceQuery(d Dialect, target *schema.Entity) Query {
	b := newBuilder(d)
	pk := target.PK()
	cond := d.Quote(pk.Column) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: pk})
	for _, c := range Visible(d, target, false) {
		cond += " AND " + c
	}
	return b.query("SELECT 1\nFROM " + d.Quote(target.Table) + "\nWHERE " + cond)
}

type DeleteRule struct {
	Restrict bool
	Child    *schema.Entity
	Field    *schema.Field
	Query    Query
}

func SoftDeleteRules(d Dialect, m *schema.Model, e *schema.Entity) []DeleteRule {
	if !e.SoftDelete {
		return nil
	}
	var rules []DeleteRule
	for _, child := range m.Entities {
		for _, f := range child.Fields {
			if f.Ref == nil || f.Ref.Entity != e.Name || f.External(child) {
				continue
			}
			b := newBuilder(d)
			switch f.Ref.OnDelete {
			case schema.OnDeleteRestrict:
				cond := d.Quote(f.Column) + " = " + b.param("id", Arg{Kind: ArgID, Field: e.PK()})
				for _, c := range Visible(d, child, false) {
					cond += " AND " + c
				}
				rules = append(rules, DeleteRule{Restrict: true, Child: child, Field: f, Query: b.query("SELECT COUNT(*)\nFROM " + d.Quote(child.Table) + "\nWHERE " + cond)})
			case schema.OnDeleteSetNull:
				sql := "UPDATE " + d.Quote(child.Table) + "\nSET " + d.Quote(f.Column) + " = NULL\nWHERE " + d.Quote(f.Column) + " = " + b.param("id", Arg{Kind: ArgID, Field: e.PK()})
				rules = append(rules, DeleteRule{Child: child, Field: f, Query: b.query(sql)})
			}
		}
	}
	return rules
}

func BuildHasManyQueries(d Dialect, parent, child *schema.Entity, fk *schema.Field) RelationQueries {
	rq := RelationQueries{SourceExists: existsQuery(d, parent)}
	cols := selectList(d, child, false)
	table := d.Quote(child.Table)
	hasFilters := len(child.FilterFields()) > 0

	b := newBuilder(d)
	conds := append([]string{d.Quote(fk.Column) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: parent.PK()})}, filterConditions(b, child, false, fk)...)
	list := "SELECT\n" + indent(cols) + "\nFROM " + table + where(conds) + "\n" + orderBy(d, child, false) + "\n" + d.Paginate(func(k ArgKind) string { return b.param(string(k), Arg{Kind: k}) })
	rq.List = b.query(withHint(d, list, hasFilters))

	b = newBuilder(d)
	conds = append([]string{d.Quote(fk.Column) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: parent.PK()})}, filterConditions(b, child, false, fk)...)
	rq.Count = b.query(withHint(d, "SELECT COUNT(*)\nFROM "+table+where(conds), hasFilters))
	return rq
}

func BuildManyToManyQueries(d Dialect, source, target *schema.Entity, via string, writable bool) RelationQueries {
	rq := RelationQueries{SourceExists: existsQuery(d, source)}
	sourceCol := schema.JoinColumn(source)
	targetCol := schema.JoinColumn(target)
	cols := selectList(d, target, true)
	join := "\nFROM " + d.Quote(target.Table) + "\nJOIN " + d.Quote(via) + " ON " + columnRef(d, via, targetCol, true) + " = " + columnRef(d, target.Table, target.PK().Column, true)
	hasFilters := len(target.FilterFields()) > 0

	b := newBuilder(d)
	conds := append([]string{columnRef(d, via, sourceCol, true) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: source.PK()})}, filterConditions(b, target, true, nil)...)
	list := "SELECT\n" + indent(cols) + join + where(conds) + "\n" + orderBy(d, target, true) + "\n" + d.Paginate(func(k ArgKind) string { return b.param(string(k), Arg{Kind: k}) })
	rq.List = b.query(withHint(d, list, hasFilters))

	b = newBuilder(d)
	conds = append([]string{columnRef(d, via, sourceCol, true) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: source.PK()})}, filterConditions(b, target, true, nil)...)
	rq.Count = b.query(withHint(d, "SELECT COUNT(*)"+join+where(conds), hasFilters))

	if !writable {
		return rq
	}
	b = newBuilder(d)
	params := [2]string{
		b.param("source", Arg{Kind: ArgSourceID, Field: source.PK()}),
		b.param("id", Arg{Kind: ArgID, Field: target.PK()}),
	}
	link := b.query(d.LinkSQL(via, [2]string{sourceCol, targetCol}, params))
	rq.Link = &link

	b = newBuilder(d)
	unlink := b.query("DELETE FROM " + d.Quote(via) + "\nWHERE " + d.Quote(sourceCol) + " = " + b.param("source", Arg{Kind: ArgSourceID, Field: source.PK()}) +
		" AND " + d.Quote(targetCol) + " = " + b.param("id", Arg{Kind: ArgID, Field: target.PK()}))
	rq.Unlink = &unlink
	return rq
}

func OwnedReferenceQuery(d Dialect, target *schema.Entity) Query {
	return existsQuery(d, target)
}

func RowInsert(d Dialect, table string, columns []string, returning []Returned) string {
	params := make([]string, len(columns))
	for i := range columns {
		params[i] = d.Placeholder(i + 1)
	}
	if len(returning) > 0 {
		return d.InsertSQL(table, quoteAll(d, columns), params, returning)
	}
	return "INSERT INTO " + d.Quote(table) + " (" + strings.Join(quoteAll(d, columns), ", ") + ")\nVALUES (" + strings.Join(params, ", ") + ")"
}
