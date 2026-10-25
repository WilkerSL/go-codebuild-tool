package gen

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

const (
	sqlServerPassword = "Codebuild_Passw0rd"
	devJWTSecret      = "development-only-secret-change-me-0123456789abcdef"
	certsVolume       = "certs"
)

type binaryView struct {
	Name    string
	Package string
}

type databaseView struct {
	Name    string
	Host    string
	URL     string
	Volume  string
	Migrate string
}

type moduleView struct {
	*projectView
	Kind       string
	Name       string
	Dir        string
	Slug       string
	Locals     []string
	Binaries   []binaryView
	Entrypoint string
	Command    string
	Port       int
	Database   *databaseView
	Data       bool
	Certs      bool
	TestDB     bool
	Paths      []string
	Env        [][2]string
	DependsOn  [][2]string
	Healthy    bool
	Expose     string
	Image      string
	Health     string
}

type composeView struct {
	*projectView
	Project           string
	Databases         []*databaseView
	SQLServer         bool
	SQLServerPassword string
	YAMLHeader        string
	Modules           []*moduleView
	Jobs              []*moduleView
	Observability     *observabilityView
}

var composeName = regexp.MustCompile(`[^a-z0-9_-]+`)

func composeProject(module string) string {
	name := strings.Trim(composeName.ReplaceAllString(strings.ToLower(path.Base(module)), "-"), "-_")
	if name == "" {
		return "app"
	}
	return name
}

func databaseURL(dialectName, db string) (host, url string) {
	switch dialectName {
	case "postgres":
		host = db + "-db"
		return host, "postgres://app:app@" + host + ":5432/" + db + "?sslmode=disable"
	case "mysql":
		host = db + "-db"
		return host, "app:app@tcp(" + host + ":3306)/" + db
	case "sqlserver":
		return "sqlserver", "sqlserver://sa:" + sqlServerPassword + "@sqlserver:1433?database=" + db + "&encrypt=disable"
	}
	return "", "/data/" + db + ".db"
}

func tlsEnv(name string) [][2]string {
	return [][2]string{
		{"TLS_CA_FILE", "/certs/ca.pem"},
		{"TLS_CERT_FILE", "/certs/" + name + ".pem"},
		{"TLS_KEY_FILE", "/certs/" + name + "-key.pem"},
	}
}

func newComposeView(pv *projectView, opts WorkspaceOptions, wv *workspaceView) *composeView {
	cv := &composeView{projectView: pv, Project: composeProject(opts.Module), SQLServer: opts.Dialect.Name() == "sqlserver", SQLServerPassword: sqlServerPassword, YAMLHeader: YAMLHeader}
	sqlite := opts.Dialect.Name() == "sqlite"
	databases := map[string]*databaseView{}
	database := func(name string) *databaseView {
		if db, ok := databases[name]; ok {
			return db
		}
		host, url := databaseURL(opts.Dialect.Name(), name)
		db := &databaseView{Name: name, Host: host, URL: url, Volume: name + "-data"}
		if name == schema.SharedDatabase {
			db.Migrate = "shared-migrate"
		} else {
			db.Migrate = name + "-migrate"
		}
		databases[name] = db
		cv.Databases = append(cv.Databases, db)
		return db
	}
	waitFor := func(db *databaseView) [][2]string {
		switch {
		case sqlite:
			return nil
		case cv.SQLServer:
			return [][2]string{{"sqlserver-init", "service_completed_successfully"}}
		}
		return [][2]string{{db.Host, "service_healthy"}}
	}
	if wv.Shared {
		db := database(schema.SharedDatabase)
		job := &moduleView{projectView: pv, Kind: "migrate", Name: "shared-migrate", Dir: SharedDir, Database: db, Data: sqlite, Command: "up", Image: cv.Project + "-shared-migrate",
			Env: [][2]string{{"DATABASE_URL", db.URL}}, DependsOn: waitFor(db)}
		cv.Jobs = append(cv.Jobs, job)
	}
	gateway := &moduleView{projectView: pv, Kind: "gateway", Name: "gateway", Dir: GatewayDir, Certs: true, Healthy: true, Expose: "8080:8080", Image: cv.Project + "-gateway", Health: "/app/gateway"}
	gateway.Env = append(tlsEnv("gateway"), [2]string{"INTERNAL_TOKEN_PRIVATE_KEY_FILE", "/certs/internal-token.pem"}, [2]string{"AUTH_COOKIE_SECURE", "false"})
	gateway.DependsOn = [][2]string{{"certs", "service_completed_successfully"}}
	for _, sv := range wv.Services {
		db := database(sv.Database)
		m := &moduleView{projectView: pv, Kind: "service", Name: sv.Name, Dir: sv.Dir, Database: db, Data: sqlite, Certs: true, Healthy: true, Image: cv.Project + "-" + sv.Name, Health: "/app/server"}
		m.Env = append([][2]string{{"DATABASE_URL", db.URL}}, tlsEnv(sv.Name)...)
		m.Env = append(m.Env, [2]string{"INTERNAL_TOKEN_PUBLIC_KEY_FILE", "/certs/internal-token.pub.pem"})
		for _, peer := range sv.Peers {
			m.Env = append(m.Env, [2]string{peer.Env, peer.Name + ":9000"})
		}
		if sv.Identity {
			m.Env = append(m.Env, [2]string{"AUTH_COOKIE_SECURE", "false"})
			if pv.Auth != nil && pv.Auth.JWT {
				m.Env = append(m.Env, [2]string{"AUTH_JWT_SECRET", devJWTSecret})
			}
		}
		m.DependsOn = [][2]string{{"certs", "service_completed_successfully"}, {db.Migrate, "service_completed_successfully"}}
		if sv.Own {
			job := &moduleView{projectView: pv, Kind: "migrate", Name: db.Migrate, Dir: sv.Dir, Database: db, Data: sqlite, Command: "up", Image: cv.Project + "-" + sv.Name,
				Env: [][2]string{{"DATABASE_URL", db.URL}}, DependsOn: waitFor(db)}
			cv.Jobs = append(cv.Jobs, job)
		}
		cv.Modules = append(cv.Modules, m)
		gateway.Env = append(gateway.Env, [2]string{sv.Env, sv.Name + ":9000"})
		gateway.DependsOn = append(gateway.DependsOn, [2]string{sv.Name, "service_healthy"})
	}
	cv.Modules = append(cv.Modules, gateway)
	if pv.Parts.Observability {
		for _, m := range cv.Modules {
			m.Env = append(m.Env, [2]string{"METRICS_ADDR", ":9090"}, [2]string{"OTEL_EXPORTER_OTLP_ENDPOINT", "http://jaeger:4318"})
		}
	}
	if pv.Ops != nil {
		gateway.Env = append(gateway.Env,
			[2]string{"OPS_JAEGER_URL", "http://jaeger:16686"},
			[2]string{"OPS_GRAFANA_URL", "http://grafana:3000"},
			[2]string{"OPS_PROMETHEUS_URL", "http://prometheus:9090"})
	}
	slices.SortFunc(cv.Jobs, func(a, b *moduleView) int { return strings.Compare(a.Name, b.Name) })
	return cv
}

func moduleViews(pv *projectView, opts WorkspaceOptions, wv *workspaceView) []*moduleView {
	sqlite := opts.Dialect.Name() == "sqlite"
	shared := []string{"go.work"}
	var out []*moduleView
	out = append(out, &moduleView{projectView: pv, Kind: "contracts", Name: "contracts", Dir: contract.ContractsDir})
	if wv.Shared {
		out = append(out, &moduleView{projectView: pv, Kind: "migrate", Name: "shared", Dir: SharedDir,
			Binaries: []binaryView{{"migrate", "./cmd/migrate"}}, Entrypoint: "/app/migrate", Command: "up", Data: sqlite})
	}
	for _, sv := range wv.Services {
		m := &moduleView{projectView: pv, Kind: "service", Name: sv.Name, Dir: sv.Dir, Locals: []string{contract.ContractsDir},
			Binaries: []binaryView{{"server", "./cmd/server"}}, Entrypoint: "/app/server", Port: 9000, Data: sqlite, Certs: true, TestDB: true}
		if sv.Own {
			m.Binaries = append(m.Binaries, binaryView{"migrate", "./cmd/migrate"})
		} else {
			m.Locals = append(m.Locals, SharedDir)
		}
		if sv.Identity {
			m.Binaries = append(m.Binaries, binaryView{"accounts", "./cmd/accounts"})
		}
		out = append(out, m)
	}
	out = append(out, &moduleView{projectView: pv, Kind: "gateway", Name: "gateway", Dir: GatewayDir, Locals: []string{contract.ContractsDir},
		Binaries: []binaryView{{"gateway", "./cmd/gateway"}, {"devcerts", "./cmd/devcerts"}}, Entrypoint: "/app/gateway", Port: 8080, Certs: true})
	for _, m := range out {
		m.Slug = strings.ReplaceAll(m.Dir, "/", "-")
		m.Paths = append([]string{m.Dir + "/**"}, shared...)
		for _, l := range m.Locals {
			m.Paths = append(m.Paths, l+"/**")
		}
		m.Paths = append(m.Paths, ".github/workflows/"+m.Slug+".yml")
	}
	return out
}

func generateInfra(opts WorkspaceOptions, wv *workspaceView) ([]File, error) {
	pv, err := moduleProject(opts, wv, opts.Module)
	if err != nil {
		return nil, err
	}
	pv.Auth = newAuthView(pv, opts.Dialect, opts.Model)
	if opts.Parts.Observability {
		pv.Ops = newOpsView(pv.Auth)
	}
	g := &generator{module: opts.Module}
	g.userFile("AGENTS.md", "workspace/agents.md.tmpl", pv)
	g.userFile(".gitignore", "workspace/gitignore.tmpl", pv)
	g.userFile(".gitattributes", "project/gitattributes.tmpl", pv)
	g.userFile(".dockerignore", "workspace/dockerignore.tmpl", pv)
	cv := newComposeView(pv, opts, wv)
	var services []string
	for _, sv := range wv.Services {
		services = append(services, sv.Name)
	}
	if opts.Parts.Observability {
		cv.Observability = newObservabilityView(cv.Project, "gateway", true, pv.Ops, services)
	}
	g.generatedFile("docker-compose.yml", "workspace/compose.yml.tmpl", cv)
	if opts.Parts.Kubernetes {
		g.kubernetes(workspaceKube(cv, wv, pv))
	}
	if opts.Parts.Observability {
		g.observability(cv.Observability, true, opts.Parts.Kubernetes)
	}
	if opts.Parts.CI {
		g.userFile(".github/workflows/workspace.yml", "workspace/workspace_ci.yml.tmpl", pv)
		g.userFile(".github/dependabot.yml", "project/dependabot.yml.tmpl", pv)
	}
	for _, m := range moduleViews(pv, opts, wv) {
		if opts.Parts.CI {
			g.userFile(".github/workflows/"+m.Slug+".yml", "workspace/ci.yml.tmpl", m)
		}
		if len(m.Binaries) > 0 {
			g.userFile(m.Dir+"/Dockerfile", "workspace/dockerfile.tmpl", m)
		}
	}
	return g.files, g.err
}
