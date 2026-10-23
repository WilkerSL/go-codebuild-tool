package schema

import (
	"cmp"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
)

const MaxIdentifierLength = 63

const DeletedAtColumn = "deleted_at"

func ConstraintName(prefix string, parts ...string) string {
	name := prefix + "_" + strings.Join(parts, "_")
	if len(name) <= MaxIdentifierLength {
		return name
	}
	h := fnv.New32a()
	h.Write([]byte(name))
	suffix := fmt.Sprintf("_%08x", h.Sum32())
	return name[:MaxIdentifierLength-len(suffix)] + suffix
}

func Build(m *Model) *Database {
	return buildDatabase(m, func(*Entity) bool { return true })
}

func BuildDatabases(m *Model) map[string]*Database {
	out := map[string]*Database{}
	for _, s := range m.Services {
		name := s.DatabaseName()
		if out[name] == nil {
			out[name] = buildDatabase(m, func(e *Entity) bool { return e.DatabaseName() == name })
		}
	}
	return out
}

func buildDatabase(m *Model, include func(*Entity) bool) *Database {
	db := &Database{}
	for _, e := range m.Entities {
		if include(e) {
			db.Tables = append(db.Tables, buildTable(m, e))
		}
	}
	seen := map[string]bool{}
	for _, e := range m.Entities {
		if !include(e) {
			continue
		}
		for _, r := range e.Relations {
			if r.Kind != RelationManyToMany || r.Via == "" || seen[r.Via] {
				continue
			}
			target := m.Entity(r.Target)
			if target == nil || target.PK() == nil || e.PK() == nil {
				continue
			}
			seen[r.Via] = true
			db.Tables = append(db.Tables, buildJoinTable(r.Via, e, target))
		}
	}
	if auth := m.AuthEntity(); auth != nil && include(auth) {
		db.Tables = append(db.Tables, authTables(auth)...)
	}
	slices.SortFunc(db.Tables, func(a, b *Table) int { return cmp.Compare(a.Name, b.Name) })
	return db
}

func ColumnSize(f *Field) int {
	if f.Password {
		return PasswordSize
	}
	switch f.Kind {
	case KindString:
		if f.Max != "" {
			n, _ := strconv.Atoi(f.Max)
			return n
		}
		if len(f.Enum) > 0 {
			longest := 0
			for _, item := range f.Enum {
				longest = max(longest, utf8.RuneCountInString(item))
			}
			for _, bucket := range []int{32, 64, 128, 255} {
				if longest <= bucket {
					return bucket
				}
			}
			return longest
		}
	case KindBytes:
		if f.Max != "" {
			n, _ := strconv.Atoi(f.Max)
			return n
		}
	}
	return 0
}

func FieldColumn(f *Field) *Column {
	c := &Column{
		Name:     f.Column,
		Kind:     f.Kind.ColumnKind(),
		Size:     ColumnSize(f),
		Nullable: f.Nullable,
		Identity: f.AutoPK && f.Kind.IsInteger(),
	}
	switch {
	case f.AutoCreate || f.AutoUpdate:
		c.Default = &Default{Kind: DefaultNow}
	case f.HasDefault:
		c.Default = &Default{Kind: DefaultLiteral, Value: f.Default}
	}
	return c
}

func buildTable(m *Model, e *Entity) *Table {
	t := &Table{Name: e.Table}
	for _, f := range e.Fields {
		t.Columns = append(t.Columns, FieldColumn(f))
		if f.PK {
			t.PrimaryKey = &PrimaryKey{Name: ConstraintName("pk", t.Name), Columns: []string{f.Column}}
		}
		switch {
		case f.Unique:
			t.Indexes = append(t.Indexes, &Index{
				Name:       ConstraintName("ux", t.Name, f.Column),
				Columns:    []string{f.Column},
				Unique:     true,
				NotDeleted: e.SoftDelete,
			})
		case f.Index || f.Ref != nil:
			t.Indexes = append(t.Indexes, &Index{Name: ConstraintName("ix", t.Name, f.Column), Columns: []string{f.Column}})
		}
		// A reference also gets an index in list order, so the lists of one
		// parent or one owner read a page in order. The single-column index
		// stays: MySQL refuses to drop the index a foreign key uses, so
		// replacing it would break the migration of existing projects.
		if f.Ref != nil && !f.PK && listIndexable(e, f) {
			t.Indexes = append(t.Indexes, &Index{Name: ConstraintName("lx", t.Name, f.Column), Columns: append([]string{f.Column}, listOrder(e)...)})
		}
		if f.Ref != nil && !f.External(e) {
			if target := m.Entity(f.Ref.Entity); target != nil && target.PK() != nil {
				t.ForeignKeys = append(t.ForeignKeys, &ForeignKey{
					Name:       ConstraintName("fk", t.Name, f.Column),
					Columns:    []string{f.Column},
					RefTable:   target.Table,
					RefColumns: []string{target.PK().Column},
					OnDelete:   f.Ref.OnDelete,
				})
			}
		}
		if len(f.Enum) > 0 {
			t.Checks = append(t.Checks, &Check{Name: ConstraintName("ck", t.Name, f.Column), Column: f.Column, Values: slices.Clone(f.Enum)})
		}
	}
	if e.SoftDelete {
		t.Columns = append(t.Columns, &Column{Name: DeletedAtColumn, Kind: ColumnTime, Nullable: true})
	}
	if created := e.AutoCreateField(); created != nil && listIndexable(e) {
		t.Indexes = append(t.Indexes, &Index{Name: ConstraintName("lx", t.Name), Columns: listOrder(e)})
	}
	sortTable(t)
	return t
}

func JoinColumn(e *Entity) string {
	return naming.Snake(e.Name) + "_id"
}

func JoinPair(a, b *Entity) (*Entity, *Entity) {
	if a.Name > b.Name {
		return b, a
	}
	return a, b
}

func buildJoinTable(via string, a, b *Entity) *Table {
	first, second := JoinPair(a, b)
	t := &Table{Name: via}
	for _, e := range []*Entity{first, second} {
		pk := e.PK()
		column := JoinColumn(e)
		t.Columns = append(t.Columns, &Column{Name: column, Kind: pk.Kind.ColumnKind(), Size: ColumnSize(pk)})
		t.ForeignKeys = append(t.ForeignKeys, &ForeignKey{
			Name:       ConstraintName("fk", via, column),
			Columns:    []string{column},
			RefTable:   e.Table,
			RefColumns: []string{pk.Column},
			OnDelete:   OnDeleteCascade,
		})
	}
	t.PrimaryKey = &PrimaryKey{Name: ConstraintName("pk", via), Columns: []string{JoinColumn(first), JoinColumn(second)}}
	t.Indexes = append(t.Indexes, &Index{Name: ConstraintName("ix", via, JoinColumn(second)), Columns: []string{JoinColumn(second)}})
	sortTable(t)
	return t
}

func sortTable(t *Table) {
	slices.SortFunc(t.Indexes, func(a, b *Index) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(t.ForeignKeys, func(a, b *ForeignKey) int { return cmp.Compare(a.Name, b.Name) })
	slices.SortFunc(t.Checks, func(a, b *Check) int { return cmp.Compare(a.Name, b.Name) })
}

// listOrder returns the columns lists are sorted by, newest first: the
// creation time and the primary key, or the primary key alone. Indexes that
// start with them let the database read a page in order instead of sorting
// every matching row.
func listOrder(e *Entity) []string {
	pk := e.PK()
	if pk == nil {
		return nil
	}
	if created := e.AutoCreateField(); created != nil {
		return []string{created.Column, pk.Column}
	}
	return []string{pk.Column}
}

// maxListKeyChars keeps list indexes under the key size limits of SQL Server
// (1,700 bytes) and MySQL (3,072 bytes) whatever the other columns are.
const maxListKeyChars = 255

func listIndexable(e *Entity, extra ...*Field) bool {
	pk := e.PK()
	if pk == nil {
		return false
	}
	for _, f := range append(extra, pk) {
		if f.Kind == KindString && (ColumnSize(f) == 0 || ColumnSize(f) > maxListKeyChars) {
			return false
		}
	}
	return true
}
