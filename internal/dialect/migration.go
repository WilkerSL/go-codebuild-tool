package dialect

import (
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type renderContext struct {
	before *schema.Database
	after  *schema.Database
}

func (rc renderContext) afterColumn(table, column string) *schema.Column {
	if t := rc.after.Table(table); t != nil {
		return t.Column(column)
	}
	return nil
}

func (rc renderContext) beforeColumn(table, column string) *schema.Column {
	if t := rc.before.Table(table); t != nil {
		return t.Column(column)
	}
	return nil
}

type ddl interface {
	createTable(rc renderContext, c schema.Change) []string
	dropTable(rc renderContext, c schema.Change) []string
	addColumn(rc renderContext, table string, col *schema.Column) []string
	dropColumn(rc renderContext, table string, col *schema.Column) []string
	alterColumn(rc renderContext, table string, old, new *schema.Column) []string
	createIndex(rc renderContext, table string, idx *schema.Index, newTable bool) []string
	dropIndex(rc renderContext, table string, idx *schema.Index, newTable bool) []string
	addForeignKey(rc renderContext, table string, fk *schema.ForeignKey) []string
	dropForeignKey(rc renderContext, table string, fk *schema.ForeignKey) []string
	addCheck(rc renderContext, table string, ck *schema.Check) []string
	dropCheck(rc renderContext, table string, ck *schema.Check) []string
}

func renderStatements(d ddl, rc renderContext, changes []schema.Change) []string {
	var stmts []string
	for _, c := range changes {
		stmts = append(stmts, renderChange(d, rc, c)...)
	}
	return stmts
}

func renderChange(d ddl, rc renderContext, c schema.Change) []string {
	switch c.Kind {
	case schema.ChangeCreateTable:
		return d.createTable(rc, c)
	case schema.ChangeDropTable:
		return d.dropTable(rc, c)
	case schema.ChangeAddColumn:
		return d.addColumn(rc, c.TableName, c.Column)
	case schema.ChangeDropColumn:
		return d.dropColumn(rc, c.TableName, c.Column)
	case schema.ChangeAlterColumn:
		return d.alterColumn(rc, c.TableName, c.OldColumn, c.Column)
	case schema.ChangeAddIndex:
		return d.createIndex(rc, c.TableName, c.Index, c.NewTable)
	case schema.ChangeDropIndex:
		if c.NewTable {
			return nil
		}
		return d.dropIndex(rc, c.TableName, c.Index, c.NewTable)
	case schema.ChangeAddForeignKey:
		return d.addForeignKey(rc, c.TableName, c.ForeignKey)
	case schema.ChangeDropForeignKey:
		return d.dropForeignKey(rc, c.TableName, c.ForeignKey)
	case schema.ChangeAddCheck:
		return d.addCheck(rc, c.TableName, c.Check)
	case schema.ChangeDropCheck:
		return d.dropCheck(rc, c.TableName, c.Check)
	}
	return nil
}

func renderMigration(d ddl, changes []schema.Change, before, after *schema.Database) string {
	return joinStatements(renderStatements(d, renderContext{before: orEmpty(before), after: orEmpty(after)}, changes))
}

func orEmpty(db *schema.Database) *schema.Database {
	if db == nil {
		return &schema.Database{}
	}
	return db
}

func fkActionDefault(a schema.OnDelete) string {
	switch a {
	case schema.OnDeleteCascade:
		return "CASCADE"
	case schema.OnDeleteSetNull:
		return "SET NULL"
	}
	return "RESTRICT"
}

func columnsEqualType(a, b *schema.Column) bool {
	return a.Kind == b.Kind && a.Size == b.Size
}

func defaultsEqual(a, b *schema.Column) bool {
	if a.Default == nil || b.Default == nil {
		return a.Default == b.Default
	}
	return *a.Default == *b.Default
}
