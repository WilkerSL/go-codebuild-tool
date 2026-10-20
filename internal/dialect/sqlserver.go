package dialect

import (
	"strconv"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type SQLServer struct{}

func (SQLServer) Name() string { return "sqlserver" }

func (SQLServer) Driver() Driver {
	return Driver{
		Module:     "github.com/microsoft/go-mssqldb",
		Version:    "v1.11.2",
		Import:     "github.com/microsoft/go-mssqldb",
		Name:       "sqlserver",
		ExampleDSN: "sqlserver://sa:Codebuild_Passw0rd@localhost:1433?database=app",
	}
}

func (SQLServer) Quote(ident string) string {
	return "[" + strings.ReplaceAll(ident, "]", "]]") + "]"
}

func (SQLServer) Placeholder(n int) string { return "@p" + strconv.Itoa(n) }

func (SQLServer) Positional() bool { return false }

func (SQLServer) FilterParam(placeholder string, _ schema.ColumnKind) string { return placeholder }

func (SQLServer) ColumnType(c *schema.Column) string {
	switch c.Kind {
	case schema.ColumnString:
		if c.Size > 0 && c.Size <= 4000 {
			return "NVARCHAR(" + strconv.Itoa(c.Size) + ")"
		}
		return "NVARCHAR(MAX)"
	case schema.ColumnBool:
		return "BIT"
	case schema.ColumnInt16:
		return "SMALLINT"
	case schema.ColumnInt32:
		return "INT"
	case schema.ColumnInt64:
		return "BIGINT"
	case schema.ColumnFloat32:
		return "REAL"
	case schema.ColumnFloat64:
		return "FLOAT"
	case schema.ColumnBytes:
		if c.Size > 0 && c.Size <= 8000 {
			return "VARBINARY(" + strconv.Itoa(c.Size) + ")"
		}
		return "VARBINARY(MAX)"
	case schema.ColumnTime:
		return "DATETIME2(7)"
	case schema.ColumnUUID:
		return "UNIQUEIDENTIFIER"
	}
	return "NVARCHAR(MAX)"
}

func (SQLServer) Now() string { return "SYSUTCDATETIME()" }

func (SQLServer) Strategy() InsertStrategy { return ReturnRow }

// SQL Server refuses a bare OUTPUT clause on tables that have triggers, so the new row
// goes into a table variable first and the statement reads it back from there.
func outputInto(returning []Returned) (string, string, string) {
	defs := make([]string, len(returning))
	outs := make([]string, len(returning))
	for i, r := range returning {
		defs[i] = r.Column + " " + r.Type
		outs[i] = "INSERTED." + r.Column
	}
	declare := "DECLARE @rows TABLE (\n" + indent(defs) + "\n);\n"
	into := "OUTPUT\n" + indent(outs) + "\nINTO @rows"
	read := ";\nSELECT\n" + indent(returnedColumns(returning)) + "\nFROM @rows"
	return declare, into, read
}

func (d SQLServer) InsertSQL(table string, columns, values []string, returning []Returned) string {
	declare, into, read := outputInto(returning)
	return declare + "INSERT INTO " + d.Quote(table) + " (\n" + indent(columns) + "\n)\n" + into + "\nVALUES (\n" + indent(values) + "\n)" + read
}

func (d SQLServer) UpdateSQL(table string, sets []string, where string, returning []Returned) string {
	declare, into, read := outputInto(returning)
	return declare + "UPDATE " + d.Quote(table) + "\nSET\n" + indent(sets) + "\n" + into + "\nWHERE " + where + read
}

func (SQLServer) Paginate(param func(ArgKind) string) string {
	offset := param(ArgOffset)
	return "OFFSET " + offset + " ROWS FETCH NEXT " + param(ArgLimit) + " ROWS ONLY"
}

func (SQLServer) QueryHint() string { return "OPTION (RECOMPILE)" }

func (d SQLServer) LinkSQL(table string, columns, params [2]string) string {
	return "INSERT INTO " + d.Quote(table) + " (" + d.Quote(columns[0]) + ", " + d.Quote(columns[1]) + ")\nSELECT " + params[0] + ", " + params[1] +
		"\nWHERE NOT EXISTS (\n    SELECT 1 FROM " + d.Quote(table) + " WHERE " + d.Quote(columns[0]) + " = " + params[0] + " AND " + d.Quote(columns[1]) + " = " + params[1] + "\n)"
}

func (SQLServer) ScanTimeAdapter() bool { return false }

func (SQLServer) RecreateDependents() bool { return true }

func (SQLServer) Limits() Limits {
	return Limits{IndexedString: 850, PrimaryKeyString: 450, PartialIndexes: true, SingleCascade: true}
}

func (d SQLServer) Migration(changes []schema.Change, before, after *schema.Database) string {
	return renderMigration(d, changes, before, after)
}

func (d SQLServer) literal(c *schema.Column) string {
	if c.Default.Kind == schema.DefaultNow {
		return "SYSUTCDATETIME()"
	}
	return literal(c, c.Default.Value, "N", "1", "0")
}

func (d SQLServer) defaultName(table, column string) string {
	return schema.ConstraintName("df", table, column)
}

func (d SQLServer) columnDef(table string, c *schema.Column) string {
	def := d.Quote(c.Name) + " " + d.ColumnType(c)
	if c.Identity {
		def += " IDENTITY(1,1)"
	}
	if c.Nullable {
		def += " NULL"
	} else {
		def += " NOT NULL"
	}
	if c.Default != nil {
		def += " CONSTRAINT " + d.Quote(d.defaultName(table, c.Name)) + " DEFAULT " + d.literal(c)
	}
	return def
}

func (d SQLServer) fkAction(a schema.OnDelete) string {
	if a == schema.OnDeleteRestrict {
		return "NO ACTION"
	}
	return fkActionDefault(a)
}

func (d SQLServer) fkDef(fk *schema.ForeignKey) string {
	return "CONSTRAINT " + d.Quote(fk.Name) + " FOREIGN KEY (" + strings.Join(quoteAll(d, fk.Columns), ", ") + ") REFERENCES " +
		d.Quote(fk.RefTable) + " (" + strings.Join(quoteAll(d, fk.RefColumns), ", ") + ") ON DELETE " + d.fkAction(fk.OnDelete)
}

func (d SQLServer) createTable(_ renderContext, c schema.Change) []string {
	t := c.Table
	var lines []string
	for _, col := range t.Columns {
		lines = append(lines, d.columnDef(t.Name, col))
	}
	if t.PrimaryKey != nil {
		lines = append(lines, "CONSTRAINT "+d.Quote(t.PrimaryKey.Name)+" PRIMARY KEY ("+strings.Join(quoteAll(d, t.PrimaryKey.Columns), ", ")+")")
	}
	for _, fk := range c.InlineFKs {
		lines = append(lines, d.fkDef(fk))
	}
	for _, ck := range t.Checks {
		lines = append(lines, "CONSTRAINT "+d.Quote(ck.Name)+" CHECK ("+checkExpression(d, ck, "N")+")")
	}
	return []string{"CREATE TABLE " + d.Quote(t.Name) + " (\n" + indent(lines) + "\n)"}
}

func (d SQLServer) dropTable(_ renderContext, c schema.Change) []string {
	return []string{"DROP TABLE " + d.Quote(c.TableName)}
}

func (d SQLServer) addColumn(_ renderContext, table string, col *schema.Column) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD " + d.columnDef(table, col)}
}

func (d SQLServer) dropDefault(table string, col *schema.Column) string {
	return "ALTER TABLE " + d.Quote(table) + " DROP CONSTRAINT " + d.Quote(d.defaultName(table, col.Name))
}

func (d SQLServer) addDefault(table string, col *schema.Column) string {
	return "ALTER TABLE " + d.Quote(table) + " ADD CONSTRAINT " + d.Quote(d.defaultName(table, col.Name)) + " DEFAULT " + d.literal(col) + " FOR " + d.Quote(col.Name)
}

func (d SQLServer) dropColumn(_ renderContext, table string, col *schema.Column) []string {
	var stmts []string
	if col.Default != nil {
		stmts = append(stmts, d.dropDefault(table, col))
	}
	return append(stmts, "ALTER TABLE "+d.Quote(table)+" DROP COLUMN "+d.Quote(col.Name))
}

func (d SQLServer) alterColumn(_ renderContext, table string, old, new *schema.Column) []string {
	var stmts []string
	defaultChanged := !defaultsEqual(old, new)
	if old.Default != nil && (defaultChanged || !columnsEqualType(old, new) || old.Nullable != new.Nullable) {
		stmts = append(stmts, d.dropDefault(table, old))
	}
	if !columnsEqualType(old, new) || old.Nullable != new.Nullable {
		null := " NOT NULL"
		if new.Nullable {
			null = " NULL"
		}
		stmts = append(stmts, "ALTER TABLE "+d.Quote(table)+" ALTER COLUMN "+d.Quote(new.Name)+" "+d.ColumnType(new)+null)
	}
	if new.Default != nil && (defaultChanged || !columnsEqualType(old, new) || old.Nullable != new.Nullable) {
		stmts = append(stmts, d.addDefault(table, new))
	}
	return stmts
}

func (d SQLServer) indexFilter(rc renderContext, table string, idx *schema.Index, col func(table, column string) *schema.Column) string {
	var conds []string
	if idx.NotDeleted {
		conds = append(conds, d.Quote(schema.DeletedAtColumn)+" IS NULL")
	}
	if idx.Unique {
		for _, name := range idx.Columns {
			if c := col(table, name); c != nil && c.Nullable {
				conds = append(conds, d.Quote(name)+" IS NOT NULL")
			}
		}
	}
	if len(conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conds, " AND ")
}

func (d SQLServer) createIndex(rc renderContext, table string, idx *schema.Index, _ bool) []string {
	stmt := "CREATE "
	if idx.Unique {
		stmt += "UNIQUE "
	}
	stmt += "INDEX " + d.Quote(idx.Name) + " ON " + d.Quote(table) + " (" + strings.Join(quoteAll(d, idx.Columns), ", ") + ")"
	stmt += d.indexFilter(rc, table, idx, rc.afterColumn)
	return []string{stmt}
}

func (d SQLServer) dropIndex(_ renderContext, table string, idx *schema.Index, _ bool) []string {
	return []string{"DROP INDEX " + d.Quote(idx.Name) + " ON " + d.Quote(table)}
}

func (d SQLServer) addForeignKey(_ renderContext, table string, fk *schema.ForeignKey) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD " + d.fkDef(fk)}
}

func (d SQLServer) dropForeignKey(_ renderContext, table string, fk *schema.ForeignKey) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " DROP CONSTRAINT " + d.Quote(fk.Name)}
}

func (d SQLServer) addCheck(_ renderContext, table string, ck *schema.Check) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD CONSTRAINT " + d.Quote(ck.Name) + " CHECK (" + checkExpression(d, ck, "N") + ")"}
}

func (d SQLServer) dropCheck(_ renderContext, table string, ck *schema.Check) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " DROP CONSTRAINT " + d.Quote(ck.Name)}
}
