package schema

import (
	"slices"
)

type Method string

const (
	MethodSession Method = "session"
	MethodJWT     Method = "jwt"
	MethodAPIKey  Method = "apikey"
)

var Methods = []Method{MethodSession, MethodJWT, MethodAPIKey}

type Operation string

const (
	OpRead   Operation = "read"
	OpCreate Operation = "create"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
)

var AllOperations = []Operation{OpRead, OpCreate, OpUpdate, OpDelete}

type Operations struct {
	Read   bool
	Create bool
	Update bool
	Delete bool
}

func (o Operations) Has(op Operation) bool {
	switch op {
	case OpRead:
		return o.Read
	case OpCreate:
		return o.Create
	case OpUpdate:
		return o.Update
	case OpDelete:
		return o.Delete
	}
	return false
}

func (o *Operations) Set(op Operation) {
	switch op {
	case OpRead:
		o.Read = true
	case OpCreate:
		o.Create = true
	case OpUpdate:
		o.Update = true
	case OpDelete:
		o.Delete = true
	}
}

func (o Operations) Any() bool {
	return o.Read || o.Create || o.Update || o.Delete
}

type Policy struct {
	Public bool
	Roles  []string
}

func Combine(a, b Policy) (Policy, bool) {
	switch {
	case a.Public:
		return b, true
	case b.Public:
		return a, true
	case len(a.Roles) == 0:
		return Policy{Roles: slices.Clone(b.Roles)}, true
	case len(b.Roles) == 0:
		return Policy{Roles: slices.Clone(a.Roles)}, true
	}
	var both []string
	for _, r := range a.Roles {
		if slices.Contains(b.Roles, r) {
			both = append(both, r)
		}
	}
	return Policy{Roles: both}, len(both) > 0
}

func RelationPolicies(source, target *Entity) (list Policy, link Policy, ok bool) {
	list, listOK := Combine(target.Policy(OpRead), source.Policy(OpRead))
	link, linkOK := Combine(source.Policy(OpUpdate), target.Policy(OpRead))
	return list, link, listOK && linkOK
}

const (
	SessionsTable      = "auth_sessions"
	RefreshTokensTable = "auth_refresh_tokens"
	APIKeysTable       = "auth_api_keys"
)

var AuthTables = []string{APIKeysTable, RefreshTokensTable, SessionsTable}

const (
	TokenHashSize = 64
	TokenIDSize   = 32
	KeyNameSize   = 120
	KeyPrefixSize = 16
	PasswordSize  = 255
)

func AuthUserColumn(e *Entity) string {
	return JoinColumn(e)
}

func authUserColumns(e *Entity, table string) (*Column, *ForeignKey, *Index) {
	pk := e.PK()
	column := AuthUserColumn(e)
	c := &Column{Name: column, Kind: pk.Kind.ColumnKind(), Size: ColumnSize(pk)}
	fk := &ForeignKey{
		Name:       ConstraintName("fk", table, column),
		Columns:    []string{column},
		RefTable:   e.Table,
		RefColumns: []string{pk.Column},
		OnDelete:   OnDeleteCascade,
	}
	ix := &Index{Name: ConstraintName("ix", table, column), Columns: []string{column}}
	return c, fk, ix
}

func authTables(e *Entity) []*Table {
	if e == nil || e.PK() == nil {
		return nil
	}
	var tables []*Table
	if e.HasMethod(MethodSession) {
		t := &Table{Name: SessionsTable}
		user, fk, ix := authUserColumns(e, t.Name)
		t.Columns = []*Column{
			{Name: "id", Kind: ColumnString, Size: TokenHashSize},
			user,
			{Name: "created_at", Kind: ColumnTime},
			{Name: "expires_at", Kind: ColumnTime},
		}
		t.PrimaryKey = &PrimaryKey{Name: ConstraintName("pk", t.Name), Columns: []string{"id"}}
		t.ForeignKeys = []*ForeignKey{fk}
		t.Indexes = []*Index{ix}
		tables = append(tables, t)
	}
	if e.HasMethod(MethodJWT) {
		t := &Table{Name: RefreshTokensTable}
		user, fk, ix := authUserColumns(e, t.Name)
		t.Columns = []*Column{
			{Name: "id", Kind: ColumnString, Size: TokenHashSize},
			{Name: "family_id", Kind: ColumnString, Size: TokenIDSize},
			user,
			{Name: "created_at", Kind: ColumnTime},
			{Name: "expires_at", Kind: ColumnTime},
			{Name: "used_at", Kind: ColumnTime, Nullable: true},
		}
		t.PrimaryKey = &PrimaryKey{Name: ConstraintName("pk", t.Name), Columns: []string{"id"}}
		t.ForeignKeys = []*ForeignKey{fk}
		t.Indexes = []*Index{ix, {Name: ConstraintName("ix", t.Name, "family_id"), Columns: []string{"family_id"}}}
		tables = append(tables, t)
	}
	if e.HasMethod(MethodAPIKey) {
		t := &Table{Name: APIKeysTable}
		user, fk, ix := authUserColumns(e, t.Name)
		t.Columns = []*Column{
			{Name: "id", Kind: ColumnString, Size: TokenIDSize},
			user,
			{Name: "name", Kind: ColumnString, Size: KeyNameSize},
			{Name: "prefix", Kind: ColumnString, Size: KeyPrefixSize},
			{Name: "key_hash", Kind: ColumnString, Size: TokenHashSize},
			{Name: "created_at", Kind: ColumnTime},
			{Name: "expires_at", Kind: ColumnTime, Nullable: true},
		}
		t.PrimaryKey = &PrimaryKey{Name: ConstraintName("pk", t.Name), Columns: []string{"id"}}
		t.ForeignKeys = []*ForeignKey{fk}
		t.Indexes = []*Index{ix, {Name: ConstraintName("ux", t.Name, "key_hash"), Columns: []string{"key_hash"}, Unique: true}}
		tables = append(tables, t)
	}
	for _, t := range tables {
		sortTable(t)
	}
	return tables
}
