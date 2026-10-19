package dialect

import (
	"strconv"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type MySQL struct{}

func (MySQL) Name() string { return "mysql" }

func (MySQL) Driver() Driver {
	return Driver{
		Module:     "github.com/go-sql-driver/mysql",
		Version:    "v1.10.1",
		Import:     "github.com/go-sql-driver/mysql",
		Name:       "mysql",
		ExampleDSN: "app:app@tcp(localhost:3306)/app",
	}
}

func (MySQL) Quote(ident string) string {
	return "`" + strings.ReplaceAll(ident, "`", "``") + "`"
}

func (MySQL) Placeholder(int) string { return "?" }

func (MySQL) Positional() bool { return true }

func (MySQL) FilterParam(placeholder string, _ schema.ColumnKind) string { return placeholder }

const mysqlInlineLimit = 4096

func (MySQL) ColumnType(c *schema.Column) string {
	switch c.Kind {
	case schema.ColumnString:
		if c.Size > 0 && c.Size <= mysqlInlineLimit {
			return "VARCHAR(" + strconv.Itoa(c.Size) + ")"
		}
		return "LONGTEXT"
	case schema.ColumnBool:
		return "BOOLEAN"
	case schema.ColumnInt16:
		return "SMALLINT"
	case schema.ColumnInt32:
		return "INT"
	case schema.ColumnInt64:
		return "BIGINT"
	case schema.ColumnFloat32:
		return "FLOAT"
	case schema.ColumnFloat64:
		return "DOUBLE"
	case schema.ColumnBytes:
		if c.Size > 0 && c.Size <= mysqlInlineLimit {
			return "VARBINARY(" + strconv.Itoa(c.Size) + ")"
		}
		return "LONGBLOB"
	case schema.ColumnTime:
		return "DATETIME(6)"
	case schema.ColumnUUID:
		return "CHAR(36)"
	}
	return "LONGTEXT"
}

func (MySQL) Now() string { return "UTC_TIMESTAMP(6)" }

func (MySQL) Strategy() InsertStrategy { return SelectAfter }

func (d MySQL) InsertSQL(table string, columns, values []string, _ []Returned) string {
	return "INSERT INTO " + d.Quote(table) + " (\n" + indent(columns) + "\n) VALUES (\n" + indent(values) + "\n)"
}

func (d MySQL) UpdateSQL(table string, sets []string, where string, _ []Returned) string {
	return "UPDATE " + d.Quote(table) + "\nSET\n" + indent(sets) + "\nWHERE " + where
}

func (MySQL) Paginate(param func(ArgKind) string) string {
	limit := param(ArgLimit)
	return "LIMIT " + limit + " OFFSET " + param(ArgOffset)
}

func (MySQL) QueryHint() string { return "" }

func (d MySQL) LinkSQL(table string, columns, params [2]string) string {
	return "INSERT INTO " + d.Quote(table) + " (" + d.Quote(columns[0]) + ", " + d.Quote(columns[1]) + ")\nVALUES (" + params[0] + ", " + params[1] + ")\nON DUPLICATE KEY UPDATE " + d.Quote(columns[0]) + " = " + d.Quote(columns[0])
}

func (MySQL) ScanTimeAdapter() bool { return false }

func (MySQL) RecreateDependents() bool { return true }

func (MySQL) Limits() Limits {
	return Limits{IndexedString: 768, PrimaryKeyString: 768}
}

func (d MySQL) Migration(changes []schema.Change, before, after *schema.Database) string {
	return renderMigration(d, changes, before, after)
}

func (d MySQL) literal(c *schema.Column) string {
	if c.Default.Kind == schema.DefaultNow {
		return "(UTC_TIMESTAMP(6))"
	}
	value := literal(c, c.Default.Value, "", "TRUE", "FALSE")
	if t := d.ColumnType(c); t == "LONGTEXT" || t == "LONGBLOB" {
		return "(" + value + ")"
	}
	return value
}

func (d MySQL) columnDef(c *schema.Column) string {
	def := d.Quote(c.Name) + " " + d.ColumnType(c)
	if !c.Nullable {
		def += " NOT NULL"
	}
	if c.Identity {
		def += " AUTO_INCREMENT"
	}
	if c.Default != nil {
		def += " DEFAULT " + d.literal(c)
	}
	return def
}

func (d MySQL) fkDef(fk *schema.ForeignKey) string {
	return "CONSTRAINT " + d.Quote(fk.Name) + " FOREIGN KEY (" + strings.Join(quoteAll(d, fk.Columns), ", ") + ") REFERENCES " +
		d.Quote(fk.RefTable) + " (" + strings.Join(quoteAll(d, fk.RefColumns), ", ") + ") ON DELETE " + fkActionDefault(fk.OnDelete)
}

func (d MySQL) indexDef(idx *schema.Index) string {
	kind := "INDEX "
	if idx.Unique {
		kind = "UNIQUE INDEX "
	}
	return kind + d.Quote(idx.Name) + " (" + strings.Join(quoteAll(d, idx.Columns), ", ") + ")"
}

func (d MySQL) createTable(_ renderContext, c schema.Change) []string {
	t := c.Table
	var lines []string
	for _, col := range t.Columns {
		lines = append(lines, d.columnDef(col))
	}
	if t.PrimaryKey != nil {
		lines = append(lines, "CONSTRAINT "+d.Quote(t.PrimaryKey.Name)+" PRIMARY KEY ("+strings.Join(quoteAll(d, t.PrimaryKey.Columns), ", ")+")")
	}
	for _, idx := range t.Indexes {
		lines = append(lines, d.indexDef(idx))
	}
	for _, fk := range c.InlineFKs {
		lines = append(lines, d.fkDef(fk))
	}
	for _, ck := range t.Checks {
		lines = append(lines, "CONSTRAINT "+d.Quote(ck.Name)+" CHECK ("+checkExpression(d, ck, "")+")")
	}
	return []string{"CREATE TABLE " + d.Quote(t.Name) + " (\n" + indent(lines) + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"}
}

func (d MySQL) dropTable(_ renderContext, c schema.Change) []string {
	return []string{"DROP TABLE " + d.Quote(c.TableName)}
}

func (d MySQL) addColumn(_ renderContext, table string, col *schema.Column) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD COLUMN " + d.columnDef(col)}
}

func (d MySQL) dropColumn(_ renderContext, table string, col *schema.Column) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " DROP COLUMN " + d.Quote(col.Name)}
}

func (d MySQL) alterColumn(_ renderContext, table string, _, new *schema.Column) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " MODIFY COLUMN " + d.columnDef(new)}
}

func (d MySQL) createIndex(_ renderContext, table string, idx *schema.Index, newTable bool) []string {
	if newTable {
		return nil
	}
	stmt := "CREATE "
	if idx.Unique {
		stmt += "UNIQUE "
	}
	return []string{stmt + "INDEX " + d.Quote(idx.Name) + " ON " + d.Quote(table) + " (" + strings.Join(quoteAll(d, idx.Columns), ", ") + ")"}
}

func (d MySQL) dropIndex(_ renderContext, table string, idx *schema.Index, newTable bool) []string {
	if newTable {
		return nil
	}
	return []string{"DROP INDEX " + d.Quote(idx.Name) + " ON " + d.Quote(table)}
}

func (d MySQL) addForeignKey(_ renderContext, table string, fk *schema.ForeignKey) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD " + d.fkDef(fk)}
}

func (d MySQL) dropForeignKey(_ renderContext, table string, fk *schema.ForeignKey) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " DROP FOREIGN KEY " + d.Quote(fk.Name)}
}

func (d MySQL) addCheck(_ renderContext, table string, ck *schema.Check) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " ADD CONSTRAINT " + d.Quote(ck.Name) + " CHECK (" + checkExpression(d, ck, "") + ")"}
}

func (d MySQL) dropCheck(_ renderContext, table string, ck *schema.Check) []string {
	return []string{"ALTER TABLE " + d.Quote(table) + " DROP CHECK " + d.Quote(ck.Name)}
}
