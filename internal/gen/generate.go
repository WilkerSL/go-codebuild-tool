// Package gen renders the code of a project from embedded templates. Most of the logic
// lives in the view types, which precompute code fragments so the templates stay
// readable.
package gen

import (
	"bytes"
	"embed"
	"fmt"
	"go/version"
	"io/fs"
	"path"
	"slices"
	"strings"
	"text/template"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

//go:embed templates
var templateFS embed.FS

var templates = loadTemplates()

func loadTemplates() *template.Template {
	root := template.New("root")
	err := fs.WalkDir(templateFS, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = root.New(strings.TrimPrefix(p, "templates/")).Parse(string(data))
		return err
	})
	if err != nil {
		panic(fmt.Sprintf("load templates: %v", err))
	}
	return root
}

type Owner string

const (
	OwnerGenerated Owner = "generated"
	OwnerUser      Owner = "user"
)

type File struct {
	Path          string
	Content       []byte
	Owner         Owner
	Domain        string
	ServiceType   string
	Receiver      string
	ServiceMethod string
	Module        string
}

type Options struct {
	Module        string
	GoVersion     string
	Caps          goversion.Capabilities
	Dialect       dialect.Dialect
	Model         *schema.Model
	Pagination    Pagination
	ManualWiring  []string
	HasMigrations bool
}

var errorImports = map[string][]string{
	"postgres":  {`"github.com/jackc/pgx/v5/pgconn"`},
	"sqlserver": {`mssql "github.com/microsoft/go-mssqldb"`},
	"mysql":     {`"github.com/go-sql-driver/mysql"`},
	"sqlite":    {`"modernc.org/sqlite"`, `sqlite3 "modernc.org/sqlite/lib"`},
}

func newDialectView(d dialect.Dialect) dialectView {
	driver := d.Driver()
	return dialectView{
		Name:          d.Name(),
		DriverModule:  driver.Module,
		DriverImport:  driver.Import,
		DriverName:    driver.Name,
		ExampleDSN:    driver.ExampleDSN,
		UUIDSwap:      d.Name() == "sqlserver",
		ScanTime:      d.ScanTimeAdapter(),
		NamedFKErrors: d.Name() != "sqlite",
		SelectAfter:   d.Strategy() == dialect.SelectAfter,
		Embedded:      d.Name() == "sqlite",
		Transactional: d.Name() != "mysql",
		ErrorImports:  errorImports[d.Name()],
	}
}

func Generate(opts Options) ([]File, error) {
	pv := &projectView{
		Header:           Header,
		Module:           opts.Module,
		GoVersion:        opts.GoVersion,
		Caps:             opts.Caps,
		UUIDImport:       opts.Caps.UUIDImport(),
		JSONImport:       opts.Caps.JSONImport(),
		Dialect:          newDialectView(opts.Dialect),
		Pagination:       opts.Pagination,
		HasMigrations:    opts.HasMigrations,
		GoImage:          strings.TrimPrefix(version.Lang("go"+opts.GoVersion), "go"),
		CI:               newCIView(opts.Dialect.Name()),
		MigrationsImport: opts.Module + "/migrations",
	}
	var err error
	if pv.Dialect.ConstraintCode, err = render(opts.Dialect.Name()+".constraint", pv); err != nil {
		return nil, err
	}
	if pv.Dialect.MigrateCode, err = render(opts.Dialect.Name()+".migrate", pv); err != nil {
		return nil, err
	}
	pv.Auth = newAuthView(pv, opts.Dialect, opts.Model)
	var authDomain *domainView
	for _, e := range opts.Model.Entities {
		dv := newDomainView(pv, opts.Dialect, opts.Model, e)
		pv.Domains = append(pv.Domains, dv)
		if dv.Routes && !slices.Contains(opts.ManualWiring, e.Domain) {
			pv.Wired = append(pv.Wired, dv)
			if dv.IsAuth {
				pv.Auth.Wired = true
			}
		}
		if dv.IsAuth {
			authDomain = dv
		}
	}
	g := &generator{module: opts.Module}
	g.goFile("internal/platform/validation/validation_gen.go", "platform/validation.go.tmpl", pv)
	g.goFile("internal/platform/validation/validation_gen_test.go", "platform/validation_test.go.tmpl", pv)
	g.goFile("internal/platform/httpx/problem_gen.go", "platform/problem.go.tmpl", pv)
	g.goFile("internal/platform/httpx/json_gen.go", "platform/json.go.tmpl", pv)
	g.goFile("internal/platform/httpx/request_gen.go", "platform/request.go.tmpl", pv)
	g.goFile("internal/platform/httpx/middleware_gen.go", "platform/middleware.go.tmpl", pv)
	g.goFile("internal/platform/httpx/health_gen.go", "platform/health.go.tmpl", pv)
	g.goFile("internal/platform/httpx/httpx_gen_test.go", "platform/httpx_test.go.tmpl", pv)
	g.goFile("internal/platform/httpx/cors_gen.go", "platform/cors.go.tmpl", pv)
	g.telemetry(pv)
	g.goFile("internal/platform/database/errors_gen.go", "platform/errors.go.tmpl", pv)
	g.goFile("internal/platform/database/values_gen.go", "platform/values.go.tmpl", pv)
	g.goFile("internal/platform/database/migrate_gen.go", "platform/migrate.go.tmpl", pv)
	g.goFile("internal/platform/database/migrate_gen_test.go", "platform/migrate_test.go.tmpl", pv)
	g.goFile("internal/platform/database/tx_gen.go", "platform/tx.go.tmpl", pv)
	g.goFile("internal/platform/database/trace_gen.go", "platform/trace.go.tmpl", pv)
	g.goFile("internal/platform/database/trace_gen_test.go", "platform/trace_test.go.tmpl", pv)
	g.goFile("internal/platform/database/connect_gen.go", opts.Dialect.Name()+".connect", pv)
	g.goFile("internal/app/routes_gen.go", "project/app.go.tmpl", pv)
	g.openAPI(pv)
	g.goFile("migrations/embed_gen.go", "project/embed.go.tmpl", pv)
	g.userGoFile("internal/platform/database/database.go", "project/database.go.tmpl", pv)
	g.userGoFile("internal/platform/config/config.go", "project/config.go.tmpl", pv)
	g.userGoFile("cmd/api/main.go", "project/main.go.tmpl", pv)
	g.userGoFile("cmd/migrate/main.go", "project/migrate.go.tmpl", pv)
	g.userFile("AGENTS.md", "project/agents.md.tmpl", pv)
	g.userFile(".gitignore", "project/gitignore.tmpl", pv)
	g.userFile(".gitattributes", "project/gitattributes.tmpl", pv)
	g.userFile(".dockerignore", "project/dockerignore.tmpl", pv)
	g.userFile("Dockerfile", "project/dockerfile.tmpl", pv)
	g.userFile(".github/workflows/ci.yml", "project/ci.yml.tmpl", pv)
	for _, dv := range pv.Domains {
		dir := "internal/" + dv.Package + "/"
		g.goFile(dir+"model_gen.go", "domain/model.go.tmpl", dv)
		g.goFile(dir+"dto_gen.go", "domain/dto.go.tmpl", dv)
		g.goFile(dir+"queries_gen.go", "domain/queries.go.tmpl", dv)
		g.goFile(dir+"repository_gen.go", "domain/repository.go.tmpl", dv)
		g.goFile(dir+"repository_gen_test.go", "domain/repository_test.go.tmpl", dv)
		if !dv.ReadOnly {
			g.goFile(dir+"validation_gen.go", "domain/validation.go.tmpl", dv)
			g.goFile(dir+"validation_gen_test.go", "domain/validation_test.go.tmpl", dv)
		}
		if dv.Routes {
			g.goFile(dir+"handler_gen.go", "domain/handler.go.tmpl", dv)
			g.goFile(dir+"routes_gen.go", "domain/routes.go.tmpl", dv)
		}
		g.userGoFile(dir+"service.go", "domain/service.go.tmpl", dv)
		g.tag(dv.Package, "Service", "", "")
		for _, m := range dv.ServiceMethods {
			g.userGoFile(dir+m.File, "domain/service_method.go.tmpl", serviceMethodView{Package: dv.Package, Module: dv.Module, UUIDImport: dv.UUIDImport, Receiver: "Service", Method: m})
			g.tag(dv.Package, "", "Service", m.Name)
		}
	}
	if authDomain != nil {
		g.authFiles(pv, authDomain)
	}
	if g.err != nil {
		return nil, g.err
	}
	slices.SortFunc(g.files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return g.files, nil
}

type generator struct {
	module string
	prefix string
	files  []File
	err    error
}

func render(name string, data any) (string, error) {
	var b bytes.Buffer
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return b.String(), nil
}

func (g *generator) add(filePath, tmpl string, data any, owner Owner, goSource bool) {
	if g.err != nil {
		return
	}
	text, err := render(tmpl, data)
	if err != nil {
		g.err = err
		return
	}
	content := []byte(text)
	if goSource {
		content, err = FormatGoModule(content, g.module)
		if err != nil {
			g.err = fmt.Errorf("%s: %w", filePath, err)
			return
		}
	}
	g.files = append(g.files, File{Path: path.Join(g.prefix, filePath), Content: content, Owner: owner, Module: g.prefix})
}

func (g *generator) goFile(filePath, tmpl string, data any) {
	g.add(filePath, tmpl, data, OwnerGenerated, true)
}

func (g *generator) userGoFile(filePath, tmpl string, data any) {
	g.add(filePath, tmpl, data, OwnerUser, true)
}

func (g *generator) userFile(filePath, tmpl string, data any) {
	g.add(filePath, tmpl, data, OwnerUser, false)
}

func (g *generator) generatedFile(filePath, tmpl string, data any) {
	g.add(filePath, tmpl, data, OwnerGenerated, false)
}

func noun(name string) string {
	return strings.ReplaceAll(naming.Snake(name), "_", " ")
}

type serviceMethodView struct {
	Package    string
	Module     string
	UUIDImport string
	Receiver   string
	Method     serviceMethod
}

func (g *generator) tag(domain, serviceType, receiver, method string) {
	if g.err != nil || len(g.files) == 0 {
		return
	}
	last := &g.files[len(g.files)-1]
	last.Domain, last.ServiceType, last.Receiver, last.ServiceMethod = domain, serviceType, receiver, method
}

func (g *generator) authFiles(pv *projectView, dv *domainView) {
	av := pv.Auth
	g.goFile("internal/platform/auth/auth_gen.go", "platform/auth.go.tmpl", pv)
	g.goFile("internal/platform/auth/config_gen.go", "platform/auth_config.go.tmpl", pv)
	g.goFile("internal/platform/auth/password_gen.go", "platform/auth_password.go.tmpl", pv)
	g.goFile("internal/platform/auth/token_gen.go", "platform/auth_token.go.tmpl", pv)
	g.goFile("internal/platform/auth/guard_gen.go", "platform/auth_guard.go.tmpl", pv)
	g.goFile("internal/platform/auth/limiter_gen.go", "platform/auth_limiter.go.tmpl", pv)
	g.goFile("internal/platform/auth/auth_gen_test.go", "platform/auth_test.go.tmpl", pv)
	if av.JWT {
		g.goFile("internal/platform/auth/jwt_gen.go", "platform/auth_jwt.go.tmpl", pv)
	}
	g.goFile("internal/app/accounts_gen.go", "project/accounts.go.tmpl", pv)
	g.userGoFile("cmd/accounts/main.go", "project/accounts_main.go.tmpl", pv)
	dir := "internal/" + dv.Package + "/"
	g.goFile(dir+"auth_queries_gen.go", "domain/auth_queries.go.tmpl", dv)
	g.goFile(dir+"accounts_gen.go", "domain/accounts.go.tmpl", dv)
	g.goFile(dir+"auth_dto_gen.go", "domain/auth_dto.go.tmpl", dv)
	g.goFile(dir+"auth_handler_gen.go", "domain/auth_handler.go.tmpl", dv)
	g.goFile(dir+"auth_routes_gen.go", "domain/auth_routes.go.tmpl", dv)
	if dv.Creatable {
		g.goFile(dir+"accounts_gen_test.go", "domain/accounts_test.go.tmpl", dv)
		if dv.Auth.JWT {
			g.goFile(dir+"accounts_export_gen_test.go", "domain/accounts_export_test.go.tmpl", dv)
		}
	}
	g.userGoFile(dir+"auth_service.go", "domain/auth_service.go.tmpl", dv)
	g.tag(dv.Package, "AuthService", "", "")
	for _, m := range dv.AuthMethods {
		g.userGoFile(dir+m.File, "domain/service_method.go.tmpl", serviceMethodView{Package: dv.Package, Module: dv.Module, UUIDImport: dv.UUIDImport, Receiver: "AuthService", Method: m})
		g.tag(dv.Package, "", "AuthService", m.Name)
	}
}

func (g *generator) telemetry(data any) {
	g.goFile("internal/platform/telemetry/telemetry_gen.go", "platform/telemetry.go.tmpl", data)
	g.goFile("internal/platform/telemetry/metrics_gen.go", "platform/telemetry_metrics.go.tmpl", data)
	g.goFile("internal/platform/telemetry/trace_gen.go", "platform/telemetry_trace.go.tmpl", data)
	g.goFile("internal/platform/telemetry/http_gen.go", "platform/telemetry_http.go.tmpl", data)
	g.goFile("internal/platform/telemetry/telemetry_gen_test.go", "platform/telemetry_test.go.tmpl", data)
}
