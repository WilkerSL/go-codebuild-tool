package dialect

import (
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type AuthQueries struct {
	UserByIdentity        string
	SetPassword           string
	SetRole               string
	InsertSession         string
	SessionPrincipal      string
	DeleteSession         string
	DeleteOtherSessions   string
	DeleteUserSessions    string
	DeleteExpiredSessions string
	InsertRefresh         string
	RefreshToken          string
	UseRefresh            string
	RefreshSuccessor      string
	JWTPrincipal          string
	DeleteFamily          string
	DeleteOtherFamilies   string
	DeleteUserRefresh     string
	DeleteExpiredRefresh  string
	InsertAPIKey          string
	APIKeyPrincipal       string
	CountAPIKeys          string
	ListAPIKeys           string
	DeleteAPIKey          string
	DeleteUserAPIKeys     string
}

type sqlParams struct {
	b *builder
}

func newParams(d Dialect) sqlParams {
	return sqlParams{b: newBuilder(d)}
}

func (s sqlParams) next(key string) string {
	return s.b.param(key, Arg{Kind: ArgKind(key)})
}

func BuildAuthQueries(d Dialect, e *schema.Entity) AuthQueries {
	var q AuthQueries
	users := d.Quote(e.Table)
	pk := e.PK()
	identity := e.Identity()
	password := e.Password()
	role := e.Role()
	live := func(qualified bool) string {
		conds := Visible(d, e, qualified)
		if len(conds) == 0 {
			return ""
		}
		return " AND " + strings.Join(conds, " AND ")
	}
	touch := ""
	for _, f := range e.Fields {
		if f.AutoUpdate {
			touch += ",\n    " + d.Quote(f.Column) + " = " + d.Now()
		}
	}
	userCol := d.Quote(schema.AuthUserColumn(e))
	join := func(table string) string {
		return "\nJOIN " + users + " ON " + columnRef(d, e.Table, pk.Column, true) + " = " + columnRef(d, table, schema.AuthUserColumn(e), true)
	}
	principalCols := func() string {
		cols := []string{columnRef(d, e.Table, pk.Column, true)}
		if role != nil {
			cols = append(cols, columnRef(d, e.Table, role.Column, true))
		}
		return strings.Join(cols, ", ")
	}
	insert := func(table string, columns []string) string {
		p := newParams(d)
		values := make([]string, len(columns))
		for i, c := range columns {
			values[i] = p.next(c)
		}
		return "INSERT INTO " + d.Quote(table) + " (" + strings.Join(quoteAll(d, columns), ", ") + ")\nVALUES (" + strings.Join(values, ", ") + ")"
	}

	p := newParams(d)
	q.UserByIdentity = "SELECT\n" + indent(selectList(d, e, false)) + "\nFROM " + users + "\nWHERE " + d.Quote(identity.Column) + " = " + p.next("identity") + live(false)

	p = newParams(d)
	q.SetPassword = "UPDATE " + users + "\nSET\n    " + d.Quote(password.Column) + " = " + p.next("hash") + touch + "\nWHERE " + d.Quote(pk.Column) + " = " + p.next("id") + live(false)

	if role != nil {
		p = newParams(d)
		q.SetRole = "UPDATE " + users + "\nSET\n    " + d.Quote(role.Column) + " = " + p.next("role") + touch + "\nWHERE " + d.Quote(identity.Column) + " = " + p.next("identity") + live(false)
	}

	if e.HasMethod(schema.MethodSession) {
		sessions := d.Quote(schema.SessionsTable)
		q.InsertSession = insert(schema.SessionsTable, []string{"id", schema.AuthUserColumn(e), "created_at", "expires_at"})
		p = newParams(d)
		q.SessionPrincipal = "SELECT " + principalCols() + "\nFROM " + sessions + join(schema.SessionsTable) +
			"\nWHERE " + columnRef(d, schema.SessionsTable, "id", true) + " = " + p.next("id") +
			" AND " + columnRef(d, schema.SessionsTable, "expires_at", true) + " > " + p.next("now") + live(true)
		p = newParams(d)
		q.DeleteSession = "DELETE FROM " + sessions + "\nWHERE " + d.Quote("id") + " = " + p.next("id")
		p = newParams(d)
		q.DeleteOtherSessions = "DELETE FROM " + sessions + "\nWHERE " + userCol + " = " + p.next("user") + " AND " + d.Quote("id") + " <> " + p.next("id")
		p = newParams(d)
		q.DeleteUserSessions = "DELETE FROM " + sessions + "\nWHERE " + userCol + " = " + p.next("user")
		p = newParams(d)
		q.DeleteExpiredSessions = "DELETE FROM " + sessions + "\nWHERE " + userCol + " = " + p.next("user") + " AND " + d.Quote("expires_at") + " <= " + p.next("now")
	}

	if e.HasMethod(schema.MethodJWT) {
		tokens := d.Quote(schema.RefreshTokensTable)
		q.InsertRefresh = insert(schema.RefreshTokensTable, []string{"id", "family_id", schema.AuthUserColumn(e), "created_at", "expires_at"})
		cols := []string{
			columnRef(d, schema.RefreshTokensTable, "family_id", true),
			columnRef(d, schema.RefreshTokensTable, "used_at", true),
			principalCols(),
		}
		p = newParams(d)
		q.RefreshToken = "SELECT " + strings.Join(cols, ", ") + "\nFROM " + tokens + join(schema.RefreshTokensTable) +
			"\nWHERE " + columnRef(d, schema.RefreshTokensTable, "id", true) + " = " + p.next("id") +
			" AND " + columnRef(d, schema.RefreshTokensTable, "expires_at", true) + " > " + p.next("now") + live(true)
		p = newParams(d)
		q.UseRefresh = "UPDATE " + tokens + "\nSET " + d.Quote("used_at") + " = " + p.next("now") + "\nWHERE " + d.Quote("id") + " = " + p.next("id") + " AND " + d.Quote("used_at") + " IS NULL"
		p = newParams(d)
		q.RefreshSuccessor = "SELECT " + strings.Join(quoteAll(d, []string{"used_at", "expires_at"}), ", ") + "\nFROM " + tokens +
			"\nWHERE " + d.Quote("id") + " = " + p.next("id") + " AND " + d.Quote("family_id") + " = " + p.next("family")
		p = newParams(d)
		q.JWTPrincipal = "SELECT " + principalCols() + "\nFROM " + users +
			"\nWHERE " + columnRef(d, e.Table, pk.Column, true) + " = " + p.next("id") + live(true) + " AND EXISTS (\n    SELECT 1\n    FROM " + tokens +
			"\n    WHERE " + columnRef(d, schema.RefreshTokensTable, "family_id", true) + " = " + p.next("family") +
			" AND " + columnRef(d, schema.RefreshTokensTable, schema.AuthUserColumn(e), true) + " = " + columnRef(d, e.Table, pk.Column, true) +
			" AND " + columnRef(d, schema.RefreshTokensTable, "expires_at", true) + " > " + p.next("now") + ")"
		p = newParams(d)
		q.DeleteFamily = "DELETE FROM " + tokens + "\nWHERE " + d.Quote("family_id") + " = " + p.next("family")
		p = newParams(d)
		q.DeleteOtherFamilies = "DELETE FROM " + tokens + "\nWHERE " + userCol + " = " + p.next("user") + " AND " + d.Quote("family_id") + " <> " + p.next("family")
		p = newParams(d)
		q.DeleteUserRefresh = "DELETE FROM " + tokens + "\nWHERE " + userCol + " = " + p.next("user")
		p = newParams(d)
		q.DeleteExpiredRefresh = "DELETE FROM " + tokens + "\nWHERE " + userCol + " = " + p.next("user") + " AND " + d.Quote("expires_at") + " <= " + p.next("now")
	}

	if e.HasMethod(schema.MethodAPIKey) {
		keys := d.Quote(schema.APIKeysTable)
		q.InsertAPIKey = insert(schema.APIKeysTable, []string{"id", schema.AuthUserColumn(e), "name", "prefix", "key_hash", "created_at", "expires_at"})
		expires := columnRef(d, schema.APIKeysTable, "expires_at", true)
		p = newParams(d)
		q.APIKeyPrincipal = "SELECT " + columnRef(d, schema.APIKeysTable, "id", true) + ", " + principalCols() + "\nFROM " + keys + join(schema.APIKeysTable) +
			"\nWHERE " + columnRef(d, schema.APIKeysTable, "key_hash", true) + " = " + p.next("hash") +
			" AND (" + expires + " IS NULL OR " + expires + " > " + p.next("now") + ")" + live(true)
		p = newParams(d)
		q.CountAPIKeys = "SELECT COUNT(*)\nFROM " + keys + "\nWHERE " + userCol + " = " + p.next("user")
		p = newParams(d)
		q.ListAPIKeys = "SELECT\n" + indent(quoteAll(d, []string{"id", "name", "prefix", "created_at", "expires_at"})) + "\nFROM " + keys +
			"\nWHERE " + userCol + " = " + p.next("user") + "\nORDER BY " + d.Quote("created_at") + " DESC, " + d.Quote("id")
		p = newParams(d)
		q.DeleteAPIKey = "DELETE FROM " + keys + "\nWHERE " + d.Quote("id") + " = " + p.next("id") + " AND " + userCol + " = " + p.next("user")
		p = newParams(d)
		q.DeleteUserAPIKeys = "DELETE FROM " + keys + "\nWHERE " + userCol + " = " + p.next("user")
	}
	return q
}
