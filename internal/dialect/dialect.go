package dialect

import (
	"fmt"
	"slices"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type Driver struct {
	Module     string
	Version    string
	Import     string
	Name       string
	ExampleDSN string
}

type InsertStrategy string

const (
	ReturnRow   InsertStrategy = "return_row"
	SelectAfter InsertStrategy = "select_after"
)

type Dialect interface {
	Name() string
	Driver() Driver
	Quote(ident string) string
	Placeholder(n int) string
	Positional() bool
	FilterParam(placeholder string, kind schema.ColumnKind) string
	ColumnType(c *schema.Column) string
	Now() string
	Strategy() InsertStrategy
	InsertSQL(table string, columns, values, returning []string) string
	UpdateSQL(table string, sets []string, where string, returning []string) string
	Paginate(param func(ArgKind) string) string
	QueryHint() string
	LinkSQL(table string, columns, params [2]string) string
	ScanTimeAdapter() bool
	RecreateDependents() bool
	Limits() Limits
	Migration(changes []schema.Change, before, after *schema.Database) string
}

type Limits struct {
	IndexedString    int
	PrimaryKeyString int
	PartialIndexes   bool
	SingleCascade    bool
}

const Default = "postgres"

var registry = map[string]Dialect{
	"postgres":  Postgres{},
	"sqlserver": SQLServer{},
	"mysql":     MySQL{},
	"sqlite":    SQLite{},
}

func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func Get(name string) (Dialect, error) {
	d, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown dialect %q; supported dialects are %s", name, strings.Join(Names(), ", "))
	}
	return d, nil
}

func quoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func indent(lines []string) string {
	return "    " + strings.Join(lines, ",\n    ")
}

func quoteAll(d Dialect, names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = d.Quote(n)
	}
	return out
}

func joinStatements(stmts []string) string {
	if len(stmts) == 0 {
		return ""
	}
	return strings.Join(stmts, ";\n\n") + ";\n"
}

func literal(c *schema.Column, value string, stringPrefix string, trueLit, falseLit string) string {
	switch c.Kind {
	case schema.ColumnString:
		return stringPrefix + quoteString(value)
	case schema.ColumnBool:
		if value == "true" {
			return trueLit
		}
		return falseLit
	}
	return value
}

func checkExpression(d Dialect, ck *schema.Check, stringPrefix string) string {
	values := make([]string, len(ck.Values))
	for i, v := range ck.Values {
		values[i] = stringPrefix + quoteString(v)
	}
	return d.Quote(ck.Column) + " IN (" + strings.Join(values, ", ") + ")"
}
