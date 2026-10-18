package dialect

import (
	"slices"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type SQLite struct{}

func (SQLite) Name() string { return "sqlite" }

func (SQLite) Driver() Driver {
	return Driver{
		Module:     "modernc.org/sqlite",
		Version:    "v1.60.1",
		Import:     "modernc.org/sqlite",
		Name:       "sqlite",
		ExampleDSN: "file:app.db",
	}
}

func (SQLite) Quote(ident string) string {
	return `"` + strings.ReplaceAll(ident, `"`, `""`) + `"`
}

func (SQLite) Placeholder(int) string { return "?" }

func (SQLite) Positional() bool { return true }

func (SQLite) FilterParam(placeholder string, _ schema.ColumnKind) string { return placeholder }

func (SQLite) ColumnType(c *schema.Column) string {
	switch c.Kind {
	case schema.ColumnBool:
		return "BOOLEAN"
	case schema.ColumnInt16, schema.ColumnInt32, schema.ColumnInt64:
		return "INTEGER"
	case schema.ColumnFloat32, schema.ColumnFloat64:
		return "REAL"
	case schema.ColumnBytes:
		return "BLOB"
	case schema.ColumnTime:
		return "DATETIME"
	}
	return "TEXT"
}

const sqliteNow = "strftime('%Y-%m-%d %H:%M:%f', 'now')"

func (SQLite) Now() string { return sqliteNow }

func (SQLite) Strategy() InsertStrategy { return ReturnRow }

func (d SQLite) InsertSQL(table string, columns, values, returning []string) string {
	return "INSERT INTO " + d.Quote(table) + " (\n" + indent(columns) + "\n) VALUES (\n" + indent(values) + "\n)\nRETURNING\n" + indent(returning)
}

func (d SQLite) UpdateSQL(table string, sets []string, where string, returning []string) string {
	return "UPDATE " + d.Quote(table) + "\nSET\n" + indent(sets) + "\nWHERE " + where + "\nRETURNING\n" + indent(returning)
}

func (SQLite) Paginate(param func(ArgKind) string) string {
	limit := param(ArgLimit)
	return "LIMIT " + limit + " OFFSET " + param(ArgOffset)
}

func (SQLite) QueryHint() string { return "" }

func (d SQLite) LinkSQL(table string, columns, params [2]string) string {
	return "INSERT INTO " + d.Quote(table) + " (" + d.Quote(columns[0]) + ", " + d.Quote(columns[1]) + ")\nVALUES (" + params[0] + ", " + params[1] + ")\nON CONFLICT DO NOTHING"
}

func (SQLite) ScanTimeAdapter() bool { return true }

func (SQLite) RecreateDependents() bool { return false }

func (SQLite) Limits() Limits {
	return Limits{PartialIndexes: true}
}

func (d SQLite) literal(c *schema.Column) string {
	if c.Default.Kind == schema.DefaultNow {
		return "(" + sqliteNow + ")"
	}
	return literal(c, c.Default.Value, "", "1", "0")
}

func (d SQLite) columnDef(c *schema.Column, inlinePK bool) string {
	if inlinePK {
		return d.Quote(c.Name) + " INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT"
	}
	def := d.Quote(c.Name) + " " + d.ColumnType(c)
	if !c.Nullable {
		def += " NOT NULL"
	}
	if c.Default != nil {
		def += " DEFAULT " + d.literal(c)
	}
	return def
}

func (d SQLite) fkDef(fk *schema.ForeignKey) string {
	return "CONSTRAINT " + d.Quote(fk.Name) + " FOREIGN KEY (" + strings.Join(quoteAll(d, fk.Columns), ", ") + ") REFERENCES " +
		d.Quote(fk.RefTable) + " (" + strings.Join(quoteAll(d, fk.RefColumns), ", ") + ") ON DELETE " + fkActionDefault(fk.OnDelete)
}

func (d SQLite) tableSQL(name string, t *schema.Table) string {
	var lines []string
	identityPK := false
	for _, col := range t.Columns {
		inline := col.Identity && t.PrimaryKey != nil && slices.Equal(t.PrimaryKey.Columns, []string{col.Name})
		identityPK = identityPK || inline
		lines = append(lines, d.columnDef(col, inline))
	}
	if t.PrimaryKey != nil && !identityPK {
		lines = append(lines, "CONSTRAINT "+d.Quote(t.PrimaryKey.Name)+" PRIMARY KEY ("+strings.Join(quoteAll(d, t.PrimaryKey.Columns), ", ")+")")
	}
	for _, fk := range t.ForeignKeys {
		lines = append(lines, d.fkDef(fk))
	}
	for _, ck := range t.Checks {
		lines = append(lines, "CONSTRAINT "+d.Quote(ck.Name)+" CHECK ("+checkExpression(d, ck, "")+")")
	}
	return "CREATE TABLE " + d.Quote(name) + " (\n" + indent(lines) + "\n)"
}

func (d SQLite) Migration(changes []schema.Change, before, after *schema.Database) string {
	rc := renderContext{before: orEmpty(before), after: orEmpty(after)}
	created := map[string]bool{}
	dropped := map[string]bool{}
	for _, c := range changes {
		switch c.Kind {
		case schema.ChangeCreateTable:
			created[c.TableName] = true
		case schema.ChangeDropTable:
			dropped[c.TableName] = true
		}
	}
	rebuild := map[string]bool{}
	for _, c := range changes {
		if !created[c.TableName] && !dropped[c.TableName] && d.needsRebuild(rc, c) {
			rebuild[c.TableName] = true
		}
	}
	var stmts []string
	rebuilt := map[string]bool{}
	for _, c := range changes {
		switch {
		case rebuild[c.TableName]:
			if !rebuilt[c.TableName] {
				rebuilt[c.TableName] = true
				stmts = append(stmts, d.rebuild(rc, c.TableName)...)
			}
		case (c.Kind == schema.ChangeAddForeignKey || c.Kind == schema.ChangeDropForeignKey) && (created[c.TableName] || dropped[c.TableName]):
		default:
			stmts = append(stmts, renderChange(d, rc, c)...)
		}
	}
	return joinStatements(stmts)
}

func (d SQLite) needsRebuild(rc renderContext, c schema.Change) bool {
	switch c.Kind {
	case schema.ChangeAlterColumn, schema.ChangeAddForeignKey, schema.ChangeDropForeignKey, schema.ChangeAddCheck, schema.ChangeDropCheck:
		return true
	case schema.ChangeAddColumn:
		col := c.Column
		if col.Default != nil && col.Default.Kind == schema.DefaultNow {
			return true
		}
		return !col.Nullable && col.Default == nil
	case schema.ChangeDropColumn:
		t := rc.before.Table(c.TableName)
		if t == nil {
			return true
		}
		if t.PrimaryKey != nil && slices.Contains(t.PrimaryKey.Columns, c.Column.Name) {
			return true
		}
		for _, fk := range t.ForeignKeys {
			if slices.Contains(fk.Columns, c.Column.Name) {
				return true
			}
		}
		for _, ck := range t.Checks {
			if ck.Column == c.Column.Name {
				return true
			}
		}
	}
	return false
}

func (d SQLite) rebuild(rc renderContext, table string) []string {
	after := rc.after.Table(table)
	before := rc.before.Table(table)
	if after == nil || before == nil {
		return nil
	}
	tmp := table + "__new"
	var common []string
	for _, col := range after.Columns {
		if before.Column(col.Name) != nil {
			common = append(common, d.Quote(col.Name))
		}
	}
	list := strings.Join(common, ", ")
	stmts := []string{
		d.tableSQL(tmp, after),
		"INSERT INTO " + d.Quote(tmp) + " (" + list + ")\nSELECT " + list + "\nFROM " + d.Quote(table),
		"DROP TABLE " + d.Quote(table),
		"ALTER TABLE " + d.Quote(tmp) + " RENAME TO " + d.Quote(table),
	}
	for _, idx := range after.Indexes {
		stmts = append(stmts, d.createIndex(rc, table, idx, false)...)
	}
	return stmts
}

func (d SQLite) createTable(_ renderContext, c schema.Change) []string {
	return []string{d.tableSQL(c.Table.Name, c.Table)}
}

func (d SQLite) dropTable(_ renderContext, c schema.Change) []string {
	return []string{"DROP TABLE " + d.Quote(c.TableName)}
}

func (d SQLite) addColumn(_ renderContext, table string, col *schema.Column) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD COLUMN " + d.columnDef(col, false)}
}

func (d SQLite) dropColumn(_ renderContext, table string, col *schema.Column) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " DROP COLUMN " + d.Quote(col.Name)}
}

func (d SQLite) alterColumn(rc renderContext, table string, _, _ *schema.Column) []string {
	return d.rebuild(rc, table)
}

func (d SQLite) createIndex(_ renderContext, table string, idx *schema.Index, _ bool) []string {
	stmt := "CREATE "
	if idx.Unique {
		stmt += "UNIQUE "
	}
	stmt += "INDEX " + d.Quote(idx.Name) + " ON " + d.Quote(table) + " (" + strings.Join(quoteAll(d, idx.Columns), ", ") + ")"
	if idx.NotDeleted {
		stmt += " WHERE " + d.Quote(schema.DeletedAtColumn) + " IS NULL"
	}
	return []string{stmt}
}

func (d SQLite) dropIndex(_ renderContext, _ string, idx *schema.Index, _ bool) []string {
	return []string{"DROP INDEX " + d.Quote(idx.Name)}
}

func (d SQLite) addForeignKey(rc renderContext, table string, _ *schema.ForeignKey) []string {
	return d.rebuild(rc, table)
}

func (d SQLite) dropForeignKey(rc renderContext, table string, _ *schema.ForeignKey) []string {
	return d.rebuild(rc, table)
}

func (d SQLite) addCheck(rc renderContext, table string, _ *schema.Check) []string {
	return d.rebuild(rc, table)
}

func (d SQLite) dropCheck(rc renderContext, table string, _ *schema.Check) []string {
	return d.rebuild(rc, table)
}
