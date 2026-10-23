package gen

import (
	"go/version"
	"slices"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

const (
	GRPCModule      = "google.golang.org/grpc"
	GRPCVersion     = "v1.84.0"
	ProtobufModule  = "google.golang.org/protobuf"
	ProtobufVersion = "v1.36.12"
	GatewayDir      = "gateway"
	SharedDir       = "databases/shared"
)

type WorkspaceOptions struct {
	Module     string
	GoVersion  string
	Caps       goversion.Capabilities
	Dialect    dialect.Dialect
	Model      *schema.Model
	Pagination Pagination
	Contracts  *contract.Set
	Migrations map[string]bool
}

type Module struct {
	Dir      string
	Path     string
	Requires [][2]string
	Locals   []string
}

type Workspace struct {
	Files   []File
	Modules []Module
}

type peerService struct {
	Field string
	Name  string
	Env   string
}

type serviceView struct {
	Service  *schema.Service
	Name     string
	TypeName string
	Dir      string
	Module   string
	Alias    string
	Import   string
	Own      bool
	Database string
	Identity bool
	Peers    []*peerService
	Imports  []goImport
	Port     int
	Field    string
	Env      string
}

type workspaceView struct {
	Module    string
	Contracts string
	Services  []*serviceView
	Shared    bool
	SharedDir string
}

func principalView(pv *projectView, d dialect.Dialect, m *schema.Model) *authView {
	if av := newAuthView(pv, d, m); av != nil {
		return av
	}
	return &authView{IDType: "string", IDParse: "return s, nil", IDFormat: "id"}
}

func GenerateWorkspace(opts WorkspaceOptions) (*Workspace, error) {
	ws := &Workspace{}
	contracts, err := opts.Contracts.Generate()
	if err != nil {
		return nil, err
	}
	for _, out := range contracts {
		ws.Files = append(ws.Files, File{Path: contract.ContractsDir + "/" + out.Path, Content: out.Content, Owner: OwnerGenerated, Module: contract.ContractsDir})
	}
	wv := &workspaceView{Module: opts.Module, Contracts: opts.Module + "/" + contract.ContractsDir, SharedDir: SharedDir}
	for i, s := range opts.Model.Services {
		file := opts.Contracts.File(s.Name)
		sv := &serviceView{
			Service:  s,
			Name:     s.Name,
			TypeName: s.TypeName,
			Dir:      "services/" + s.Name,
			Module:   opts.Module + "/services/" + s.Name,
			Alias:    file.GoName,
			Import:   file.GoImport,
			Own:      s.Database == schema.DatabaseOwn,
			Database: s.DatabaseName(),
			Port:     9001 + i,
			Field:    naming.Pascal(s.Name),
			Env:      strings.ToUpper(s.Name) + "_GRPC_ADDR",
		}
		if auth := opts.Model.AuthEntity(); auth != nil && auth.Service == s {
			sv.Identity = true
		}
		if !sv.Own {
			wv.Shared = true
		}
		wv.Services = append(wv.Services, sv)
	}
	driver := opts.Dialect.Driver()
	base := [][2]string{{driver.Module, driver.Version}, {GRPCModule, GRPCVersion}, {ProtobufModule, ProtobufVersion}}
	if !opts.Caps.StdUUID {
		base = append(base, [2]string{googleUUID, googleUUIDVersion})
	}
	ws.Modules = append(ws.Modules, Module{Dir: contract.ContractsDir, Path: wv.Contracts, Requires: [][2]string{{GRPCModule, GRPCVersion}, {ProtobufModule, ProtobufVersion}}})
	for _, sv := range wv.Services {
		files, err := generateService(opts, wv, sv)
		if err != nil {
			return nil, err
		}
		ws.Files = append(ws.Files, files...)
		mod := Module{Dir: sv.Dir, Path: sv.Module, Requires: base, Locals: []string{contract.ContractsDir}}
		if !sv.Own {
			mod.Locals = append(mod.Locals, SharedDir)
		}
		ws.Modules = append(ws.Modules, mod)
	}
	files, err := generateGateway(opts, wv)
	if err != nil {
		return nil, err
	}
	ws.Files = append(ws.Files, files...)
	gateway := Module{Dir: GatewayDir, Path: opts.Module + "/" + GatewayDir, Requires: [][2]string{{GRPCModule, GRPCVersion}, {ProtobufModule, ProtobufVersion}}, Locals: []string{contract.ContractsDir}}
	if !opts.Caps.StdUUID {
		gateway.Requires = append(gateway.Requires, [2]string{googleUUID, googleUUIDVersion})
	}
	ws.Modules = append(ws.Modules, gateway)
	infra, err := generateInfra(opts, wv)
	if err != nil {
		return nil, err
	}
	ws.Files = append(ws.Files, infra...)
	if wv.Shared {
		files, err := generateShared(opts, wv)
		if err != nil {
			return nil, err
		}
		ws.Files = append(ws.Files, files...)
		ws.Modules = append(ws.Modules, Module{Dir: SharedDir, Path: opts.Module + "/" + SharedDir, Requires: [][2]string{{driver.Module, driver.Version}}})
	}
	slices.SortFunc(ws.Files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	slices.SortFunc(ws.Modules, func(a, b Module) int { return strings.Compare(a.Dir, b.Dir) })
	return ws, nil
}

const (
	googleUUID        = "github.com/google/uuid"
	googleUUIDVersion = "v1.6.0"
)

func moduleProject(opts WorkspaceOptions, wv *workspaceView, module string) (*projectView, error) {
	pv := &projectView{
		Header:           Header,
		Module:           module,
		GoVersion:        opts.GoVersion,
		Caps:             opts.Caps,
		UUIDImport:       opts.Caps.UUIDImport(),
		JSONImport:       opts.Caps.JSONImport(),
		Dialect:          newDialectView(opts.Dialect),
		Pagination:       opts.Pagination,
		GoImage:          strings.TrimPrefix(version.Lang("go"+opts.GoVersion), "go"),
		CI:               newCIView(opts.Dialect.Name()),
		Workspace:        wv,
		MigrationsImport: module + "/migrations",
	}
	var err error
	if pv.Dialect.ConstraintCode, err = render(opts.Dialect.Name()+".constraint", pv); err != nil {
		return nil, err
	}
	if pv.Dialect.MigrateCode, err = render(opts.Dialect.Name()+".migrate", pv); err != nil {
		return nil, err
	}
	return pv, nil
}

func generateService(opts WorkspaceOptions, wv *workspaceView, sv *serviceView) ([]File, error) {
	m := opts.Model
	d := opts.Dialect
	pv, err := moduleProject(opts, wv, sv.Module)
	if err != nil {
		return nil, err
	}
	pv.Current = sv
	pv.Identity = principalView(pv, d, m)
	if sv.Own {
		pv.HasMigrations = opts.Migrations[sv.Database]
	} else {
		pv.MigrationsImport = opts.Module + "/" + SharedDir + "/migrations"
	}
	if sv.Identity {
		pv.Auth = newAuthView(pv, d, m)
	}
	file := opts.Contracts.File(sv.Name)
	enabled := m.AuthEntity() != nil
	peerClients := map[string]*peerClient{}
	peerOf := func(target *schema.Entity) *peerClient {
		tf := opts.Contracts.File(target.Service.Name)
		return &peerClient{
			Field:   peerField(target),
			Type:    target.Name + "ServiceClient",
			Service: naming.Pascal(target.Service.Name),
			Alias:   tf.GoName,
			Import:  tf.GoImport,
			Entity:  target,
		}
	}
	for _, e := range sv.Service.Entities {
		dv := newDomainView(pv, d, m, e)
		dv.GRPC = newGRPCView(dv, m, file, enabled, peerClients, peerOf)
		pv.Domains = append(pv.Domains, dv)
		if dv.IsAuth {
			pv.Auth.Wired = true
		}
	}
	imports := []goImport{{Name: sv.Alias, Path: sv.Import}}
	seenPeers := map[string]bool{}
	for _, field := range sortedKeys(peerClients) {
		pc := peerClients[field]
		if !seenPeers[pc.Service] {
			seenPeers[pc.Service] = true
			sv.Peers = append(sv.Peers, &peerService{Field: pc.Service, Name: pc.Entity.Service.Name, Env: strings.ToUpper(pc.Entity.Service.Name) + "_GRPC_ADDR"})
		}
		if !slices.ContainsFunc(imports, func(i goImport) bool { return i.Path == pc.Import }) {
			imports = append(imports, goImport{Name: pc.Alias, Path: pc.Import})
		}
	}
	slices.SortFunc(sv.Peers, func(a, b *peerService) int { return strings.Compare(a.Field, b.Field) })
	slices.SortFunc(imports, func(a, b goImport) int { return strings.Compare(a.Path, b.Path) })
	sv.Imports = imports

	g := &generator{module: sv.Module, prefix: sv.Dir}
	g.goFile("internal/platform/validation/validation_gen.go", "platform/validation.go.tmpl", pv)
	g.goFile("internal/platform/validation/validation_gen_test.go", "platform/validation_test.go.tmpl", pv)
	g.goFile("internal/platform/httpx/problem_gen.go", "platform/problem.go.tmpl", pv)
	g.goFile("internal/platform/httpx/json_gen.go", "platform/json.go.tmpl", pv)
	g.goFile("internal/platform/httpx/request_gen.go", "platform/request.go.tmpl", pv)
	g.goFile("internal/platform/httpx/middleware_gen.go", "platform/middleware.go.tmpl", pv)
	g.goFile("internal/platform/httpx/health_gen.go", "platform/health.go.tmpl", pv)
	g.goFile("internal/platform/httpx/probe_gen.go", "workspace/httpx_probe.go.tmpl", pv)
	g.goFile("internal/platform/httpx/httpx_gen_test.go", "platform/httpx_test.go.tmpl", pv)
	g.telemetry(pv)
	g.goFile("internal/platform/database/errors_gen.go", "platform/errors.go.tmpl", pv)
	g.goFile("internal/platform/database/values_gen.go", "platform/values.go.tmpl", pv)
	g.goFile("internal/platform/database/migrate_gen.go", "platform/migrate.go.tmpl", pv)
	g.goFile("internal/platform/database/migrate_gen_test.go", "platform/migrate_test.go.tmpl", pv)
	g.goFile("internal/platform/database/tx_gen.go", "platform/tx.go.tmpl", pv)
	g.goFile("internal/platform/database/trace_gen.go", "platform/trace.go.tmpl", pv)
	g.goFile("internal/platform/database/trace_gen_test.go", "platform/trace_test.go.tmpl", pv)
	g.goFile("internal/platform/database/connect_gen.go", opts.Dialect.Name()+".connect", pv)
	g.goFile("internal/platform/grpcx/tls_gen.go", "workspace/grpcx_tls.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/client_gen.go", "workspace/grpcx_client.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/client_gen_test.go", "workspace/grpcx_client_test.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/server_gen.go", "workspace/grpcx_server.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/status_gen.go", "workspace/grpcx_status.go.tmpl", pv)
	g.goFile("internal/platform/auth/internal_gen.go", "workspace/auth_internal.go.tmpl", pv)
	if pv.Auth != nil {
		g.goFile("internal/platform/auth/auth_gen.go", "platform/auth.go.tmpl", pv)
		g.goFile("internal/platform/auth/config_gen.go", "platform/auth_config.go.tmpl", pv)
		g.goFile("internal/platform/auth/password_gen.go", "platform/auth_password.go.tmpl", pv)
		g.goFile("internal/platform/auth/token_gen.go", "platform/auth_token.go.tmpl", pv)
		g.goFile("internal/platform/auth/guard_gen.go", "platform/auth_guard.go.tmpl", pv)
		g.goFile("internal/platform/auth/limiter_gen.go", "platform/auth_limiter.go.tmpl", pv)
		g.goFile("internal/platform/auth/auth_gen_test.go", "platform/auth_test.go.tmpl", pv)
		if pv.Auth.JWT {
			g.goFile("internal/platform/auth/jwt_gen.go", "platform/auth_jwt.go.tmpl", pv)
		}
	} else {
		g.goFile("internal/platform/auth/principal_gen.go", "workspace/auth_principal.go.tmpl", pv)
	}
	g.goFile("internal/app/server_gen.go", "workspace/server.go.tmpl", pv)
	g.userGoFile("internal/platform/database/database.go", "project/database.go.tmpl", pv)
	g.userGoFile("internal/platform/config/config.go", "workspace/service_config.go.tmpl", pv)
	g.userGoFile("cmd/server/main.go", "workspace/service_main.go.tmpl", pv)
	if sv.Own {
		g.goFile("migrations/embed_gen.go", "project/embed.go.tmpl", pv)
		g.userGoFile("cmd/migrate/main.go", "project/migrate.go.tmpl", pv)
	}
	for _, dv := range pv.Domains {
		dir := "internal/" + dv.Package + "/"
		g.goFile(dir+"model_gen.go", "domain/model.go.tmpl", dv)
		g.goFile(dir+"dto_gen.go", "domain/dto.go.tmpl", dv)
		g.goFile(dir+"queries_gen.go", "domain/queries.go.tmpl", dv)
		g.goFile(dir+"repository_gen.go", "domain/repository.go.tmpl", dv)
		g.goFile(dir+"repository_gen_test.go", "domain/repository_test.go.tmpl", dv)
		g.goFile(dir+"grpc_gen.go", "workspace/grpc.go.tmpl", dv)
		if !dv.ReadOnly {
			g.goFile(dir+"validation_gen.go", "domain/validation.go.tmpl", dv)
			g.goFile(dir+"validation_gen_test.go", "domain/validation_test.go.tmpl", dv)
		}
		g.userGoFile(dir+"service.go", "domain/service.go.tmpl", dv)
		g.tag(dv.Package, "Service", "", "")
		for _, method := range dv.ServiceMethods {
			g.userGoFile(dir+method.File, "domain/service_method.go.tmpl", serviceMethodView{Package: dv.Package, Module: dv.Module, UUIDImport: dv.UUIDImport, Receiver: "Service", Method: method})
			g.tag(dv.Package, "", "Service", method.Name)
		}
		if dv.IsAuth {
			g.goFile(dir+"auth_queries_gen.go", "domain/auth_queries.go.tmpl", dv)
			g.goFile(dir+"accounts_gen.go", "domain/accounts.go.tmpl", dv)
			g.goFile(dir+"auth_dto_gen.go", "domain/auth_dto.go.tmpl", dv)
			g.goFile(dir+"auth_grpc_gen.go", "workspace/auth_grpc.go.tmpl", dv)
			if dv.Creatable {
				g.goFile(dir+"accounts_gen_test.go", "domain/accounts_test.go.tmpl", dv)
				if dv.Auth.JWT {
					g.goFile(dir+"accounts_export_gen_test.go", "domain/accounts_export_test.go.tmpl", dv)
				}
			}
			g.userGoFile(dir+"auth_service.go", "domain/auth_service.go.tmpl", dv)
			g.tag(dv.Package, "AuthService", "", "")
			for _, method := range dv.AuthMethods {
				g.userGoFile(dir+method.File, "domain/service_method.go.tmpl", serviceMethodView{Package: dv.Package, Module: dv.Module, UUIDImport: dv.UUIDImport, Receiver: "AuthService", Method: method})
				g.tag(dv.Package, "", "AuthService", method.Name)
			}
			g.goFile("internal/app/accounts_gen.go", "project/accounts.go.tmpl", pv)
			g.userGoFile("cmd/accounts/main.go", "project/accounts_main.go.tmpl", pv)
		}
	}
	return g.files, g.err
}

func generateShared(opts WorkspaceOptions, wv *workspaceView) ([]File, error) {
	module := opts.Module + "/" + SharedDir
	pv, err := moduleProject(opts, wv, module)
	if err != nil {
		return nil, err
	}
	pv.HasMigrations = opts.Migrations[schema.SharedDatabase]
	g := &generator{module: module, prefix: SharedDir}
	g.goFile("migrations/embed_gen.go", "project/embed.go.tmpl", pv)
	g.goFile("internal/platform/database/migrate_gen.go", "platform/migrate.go.tmpl", pv)
	g.goFile("internal/platform/database/migrate_gen_test.go", "platform/migrate_test.go.tmpl", pv)
	g.goFile("internal/platform/database/tx_gen.go", "platform/tx.go.tmpl", pv)
	g.goFile("internal/platform/database/connect_gen.go", opts.Dialect.Name()+".connect", pv)
	g.userGoFile("internal/platform/database/database.go", "project/database.go.tmpl", pv)
	g.userGoFile("internal/platform/config/config.go", "project/config.go.tmpl", pv)
	g.userGoFile("cmd/migrate/main.go", "project/migrate.go.tmpl", pv)
	return g.files, g.err
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
