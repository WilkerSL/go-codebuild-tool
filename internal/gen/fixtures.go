package gen

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type rowFixture struct {
	Entity     *schema.Entity
	Func       string
	Type       string
	Table      string
	PKName     string
	PKType     string
	OwnerType  string
	Parents    []parentRef
	Const      string
	Literal    string
	Args       string
	Mode       string
	IDExpr     string
	ScanTarget string
}

type rowFixtures struct {
	d      dialect.Dialect
	holder *schema.Entity
	made   map[string]*rowFixture
	list   []*rowFixture
	owner  func() string
}

func randomValue(pk *schema.Field) string {
	switch {
	case pk.Kind == schema.KindUUID:
		return "uuid.New()"
	case pk.Kind.IsInteger():
		return fmt.Sprintf("fixtureInt[%s](1, 1<<30)", goBase(pk.Kind))
	}
	n := 16
	if size := schema.ColumnSize(pk); size > 0 {
		n = min(n, size)
	}
	return fmt.Sprintf("fixtureString(%d)", n)
}

func rawValue(f *schema.Field) string {
	switch {
	case f.Password:
		return strconv.Quote("fixture password hash")
	case f.Kind == schema.KindString && len(f.Enum) > 0:
		return strconv.Quote(f.Enum[0])
	}
	return validValue(f, "", f.Unique || f.PK)
}

func (r *rowFixtures) call(x *schema.Entity, owner string) string {
	fx := r.fixture(x)
	if fx.OwnerType == "" {
		return fx.Func + "(t, ctx, db)"
	}
	if owner == "" {
		owner = r.owner()
	}
	return fx.Func + "(t, ctx, db, " + owner + ")"
}

func (r *rowFixtures) fixture(x *schema.Entity) *rowFixture {
	if fx, ok := r.made[x.Name]; ok {
		return fx
	}
	pk := x.PK()
	fx := &rowFixture{
		Entity: x,
		Func:   "fixture" + x.Name + "Row",
		Type:   naming.LowerCamel(x.Name) + "Row",
		Table:  x.Table,
		PKName: pk.Name,
		PKType: goBase(pk.Kind),
		Const:  "fixture" + x.Name + "RowQuery",
	}
	r.made[x.Name] = fx
	var columns, args []string
	identity := pk.AutoPK && pk.Kind.IsInteger()
	if !identity {
		fx.Mode = "generated"
		fx.IDExpr = randomValue(pk)
		columns = append(columns, pk.Column)
		args = append(args, argExpr(nonNull(pk), "row."+pk.Name))
	}
	taken := map[string]bool{"row": true, "t": true, "ctx": true, "db": true, "owner": true, "res": true, "id": true, "err": true}
	for _, f := range x.Fields {
		if f.PK || f.Nullable || f.Auto() || (f.HasDefault && !f.Owner) {
			continue
		}
		switch {
		case f.Owner:
			fx.OwnerType = goBase(f.Kind)
			columns = append(columns, f.Column)
			args = append(args, argExpr(nonNull(f), "owner"))
		case f.Ref != nil && (f.Ref.Target == nil || !schema.SameDatabase(x, f.Ref.Target)):
			columns = append(columns, f.Column)
			target := f.Ref.Target
			value := "uuid.New()"
			if target != nil {
				value = randomValue(target.PK())
			}
			args = append(args, argExpr(nonNull(f), value))
		case f.Ref != nil:
			parent := f.Ref.Target
			v := naming.LowerCamel(parent.Name)
			for taken[v] {
				v += "Row"
			}
			taken[v] = true
			fx.Parents = append(fx.Parents, parentRef{Var: v, Call: r.call(parent, "")})
			columns = append(columns, f.Column)
			args = append(args, argExpr(nonNull(f), v+"."+parent.PK().Name))
		case f.InCreate() || f.Password:
			columns = append(columns, f.Column)
			args = append(args, argExpr(nonNull(f), rawValue(f)))
		}
	}
	var returning []dialect.Returned
	if identity {
		returning = []dialect.Returned{{Column: r.d.Quote(pk.Column), Type: r.d.ColumnType(schema.FieldColumn(pk))}}
		fx.Mode = "returning"
		if r.d.Strategy() == dialect.SelectAfter {
			fx.Mode = "lastid"
		}
		fx.ScanTarget = "&row." + pk.Name
	}
	sql := dialect.RowInsert(r.d, x.Table, columns, returning)
	fx.Literal = goStringLiteral(sql)
	fx.Args = strings.Join(args, ", ")
	r.list = append(r.list, fx)
	slices.SortFunc(r.list, func(a, b *rowFixture) int { return strings.Compare(a.Func, b.Func) })
	return fx
}
