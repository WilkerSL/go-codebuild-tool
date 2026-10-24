package gen

import (
	"strconv"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type authView struct {
	Entity           *schema.Entity
	Package          string
	Import           string
	Name             string
	Noun             string
	PK               *fieldView
	IDType           string
	IDScanOpen       string
	IDScanClose      string
	IDArgOpen        string
	IDArgClose       string
	IDParse          string
	IDFormat         string
	Session          bool
	JWT              bool
	APIKey           bool
	LoginMethod      string
	Register         bool
	Routes           bool
	Path             string
	PKLocation       string
	Identity         *fieldView
	IdentityMax      string
	Password         *fieldView
	Role             *fieldView
	RoleConsts       string
	RoleItems        string
	EmailIdentity    bool
	MinPassword      string
	CredentialCookie string
	ScanTime         bool
	Queries          []queryConst
	RoleDefault      string
	Wired            bool
	Methods          string
}

func newAuthView(pv *projectView, d dialect.Dialect, m *schema.Model) *authView {
	e := m.AuthEntity()
	if e == nil {
		return nil
	}
	pk := e.PK()
	av := &authView{
		Entity:      e,
		Package:     e.Domain,
		Import:      pv.Module + "/internal/" + e.Domain,
		Name:        e.Name,
		Noun:        noun(e.Name),
		PK:          newFieldView(pk, pv.Dialect),
		IDType:      goBase(pk.Kind),
		Session:     e.HasMethod(schema.MethodSession),
		JWT:         e.HasMethod(schema.MethodJWT),
		APIKey:      e.HasMethod(schema.MethodAPIKey),
		LoginMethod: string(e.LoginMethod()),
		Register:    e.Register,
		Routes:      !e.NoRoutes,
		Path:        e.Path(),
		PKLocation:  idString(pk, "item."+pk.Name),
		Identity:    newFieldView(e.Identity(), pv.Dialect),
		Password:    newFieldView(e.Password(), pv.Dialect),
		ScanTime:    pv.Dialect.ScanTime,
	}
	av.EmailIdentity = av.Identity.F.Email
	av.IdentityMax = "1024"
	if size := schema.ColumnSize(e.Identity()); size > 0 {
		av.IdentityMax = strconv.Itoa(size)
	}
	for i, m := range e.Auth {
		if i > 0 {
			av.Methods += ", "
		}
		av.Methods += string(m)
	}
	av.MinPassword = av.Password.F.Min
	av.CredentialCookie = "session"
	if av.JWT {
		av.CredentialCookie = "access_token"
	}
	if role := e.Role(); role != nil {
		av.Role = newFieldView(role, pv.Dialect)
		consts, items := enumList(role)
		av.RoleConsts, av.RoleItems = consts, items
		if role.HasDefault {
			av.RoleDefault = defaultLiteral(role)
		}
	}
	switch {
	case pk.Kind == schema.KindUUID:
		av.IDScanOpen, av.IDScanClose = "database.ScanUUID(", ")"
		av.IDArgOpen, av.IDArgClose = "database.UUIDArg(", ")"
		av.IDParse = "return uuid.Parse(s)"
		av.IDFormat = "id.String()"
	case pk.Kind == schema.KindInt:
		av.IDParse = "return strconv.Atoi(s)"
		av.IDFormat = "strconv.Itoa(id)"
	case pk.Kind.IsInteger():
		av.IDParse = "n, err := strconv.ParseInt(s, 10, " + strconv.Itoa(pk.Kind.BitSize()) + ")\n\treturn " + goBase(pk.Kind) + "(n), err"
		av.IDFormat = "strconv.FormatInt(int64(id), 10)"
	default:
		av.IDParse = "return s, nil"
		av.IDFormat = "id"
	}
	q := dialect.BuildAuthQueries(d, e)
	add := func(name, sql string) {
		if sql != "" {
			av.Queries = append(av.Queries, queryConst{Name: name, SQL: sql, Literal: goStringLiteral(sql)})
		}
	}
	add("userByIdentityQuery", q.UserByIdentity)
	add("setPasswordQuery", q.SetPassword)
	add("replacePasswordQuery", q.ReplacePassword)
	add("lockAccountQuery", q.LockAccount)
	add("lockPasswordQuery", q.LockPassword)
	add("setRoleQuery", q.SetRole)
	add("insertSessionQuery", q.InsertSession)
	add("sessionPrincipalQuery", q.SessionPrincipal)
	add("deleteSessionQuery", q.DeleteSession)
	add("deleteOtherSessionsQuery", q.DeleteOtherSessions)
	add("deleteUserSessionsQuery", q.DeleteUserSessions)
	add("deleteExpiredSessionsQuery", q.DeleteExpiredSessions)
	add("insertRefreshQuery", q.InsertRefresh)
	add("refreshTokenQuery", q.RefreshToken)
	add("useRefreshQuery", q.UseRefresh)
	add("refreshSuccessorQuery", q.RefreshSuccessor)
	add("jwtPrincipalQuery", q.JWTPrincipal)
	add("deleteFamilyQuery", q.DeleteFamily)
	add("deleteOtherFamiliesQuery", q.DeleteOtherFamilies)
	add("deleteUserRefreshQuery", q.DeleteUserRefresh)
	add("deleteExpiredRefreshQuery", q.DeleteExpiredRefresh)
	add("insertAPIKeyQuery", q.InsertAPIKey)
	add("apiKeyPrincipalQuery", q.APIKeyPrincipal)
	add("countAPIKeysQuery", q.CountAPIKeys)
	add("listAPIKeysQuery", q.ListAPIKeys)
	add("deleteAPIKeyQuery", q.DeleteAPIKey)
	add("deleteUserAPIKeysQuery", q.DeleteUserAPIKeys)
	add("expireRateLimitQuery", q.ExpireRateLimit)
	add("hitRateLimitQuery", q.HitRateLimit)
	add("rateLimitResetQuery", q.RateLimitReset)
	add("insertRateLimitQuery", q.InsertRateLimit)
	add("deleteRateLimitQuery", q.DeleteRateLimit)
	add("deleteExpiredLimitsQuery", q.DeleteExpiredLimits)
	add("deleteSingleLimitsQuery", q.DeleteSingleLimits)
	add("countRateLimitsQuery", q.CountRateLimits)
	return av
}

func routeLine(method, path, handler string, policy schema.Policy, guarded bool) string {
	pattern := strconv.Quote(method + " " + path)
	if !guarded || policy.Public {
		return "mux.HandleFunc(" + pattern + ", " + handler + ")"
	}
	args := []string{handler}
	for _, r := range policy.Roles {
		args = append(args, strconv.Quote(r))
	}
	return "mux.Handle(" + pattern + ", guard.Protect(" + strings.Join(args, ", ") + "))"
}
