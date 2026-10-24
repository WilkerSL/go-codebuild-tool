package gen

import (
	"strconv"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/naming"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type proxyField struct {
	Name string
	JSON string
	Type string
	From string
}

type proxyRelation struct {
	*relationView
	ParamsFunc  string
	SourceProto string
	Filters     []proxyField
}

type proxyView struct {
	Alias     string
	Import    string
	Client    string
	Backend   string
	IDType    string
	IDProto   string
	Location  string
	Fields    []proxyField
	Filters   []proxyField
	Create    []proxyField
	Update    []proxyField
	Register  []proxyField
	Relations []*proxyRelation
}

type edgeView struct {
	AuthAlias   string
	AuthBackend string
}

func responseType(f *schema.Field) string {
	base := goBase(f.Kind)
	switch f.Kind {
	case schema.KindUUID:
		base = "string"
	case schema.KindTime:
		base = "time.Time"
	}
	if f.Nullable {
		return "*" + base
	}
	return base
}

func fromProto(f *schema.Field, m, goName string) string {
	get := m + ".Get" + goName + "()"
	field := m + "." + goName
	switch {
	case f.Kind == schema.KindTime && f.Nullable:
		return "grpcx.NullTime(" + get + ")"
	case f.Kind == schema.KindTime:
		return get + ".AsTime()"
	case f.Kind == schema.KindInt16 && f.Nullable:
		return "grpcx.Convert[int32, int16](" + field + ")"
	case f.Kind == schema.KindInt16:
		return "int16(" + get + ")"
	case f.Kind == schema.KindInt && f.Nullable:
		return "grpcx.Convert[int64, int](" + field + ")"
	case f.Kind == schema.KindInt:
		return "int(" + get + ")"
	case f.Kind == schema.KindBytes && f.Nullable:
		return "grpcx.BytesPtr(" + field + ")"
	case f.Kind == schema.KindBytes:
		return "grpcx.NonNil(" + get + ")"
	case f.Nullable:
		return field
	}
	return get
}

func toProto(f *schema.Field, expr, json string) string {
	switch f.Kind {
	case schema.KindTime:
		return "grpcx.ParseTime(" + expr + ", " + strconv.Quote(json) + ", &errs)"
	case schema.KindInt16:
		return "grpcx.Convert[int16, int32](" + expr + ")"
	case schema.KindInt:
		return "grpcx.Convert[int, int64](" + expr + ")"
	case schema.KindBytes:
		return "grpcx.Bytes(" + expr + ")"
	}
	return expr
}

func filterProto(f *schema.Field, expr string) string {
	switch f.Kind {
	case schema.KindUUID:
		return "grpcx.UUIDString(" + expr + ")"
	case schema.KindInt16:
		return "grpcx.Convert[int16, int32](" + expr + ")"
	case schema.KindInt:
		return "grpcx.Convert[int, int64](" + expr + ")"
	}
	return expr
}

func protoID(pk *schema.Field, expr string) string {
	switch pk.Kind {
	case schema.KindUUID:
		return expr + ".String()"
	case schema.KindInt:
		return "int64(" + expr + ")"
	case schema.KindInt16:
		return "int32(" + expr + ")"
	}
	return expr
}

func responseLocation(pk *schema.Field, expr string) string {
	switch {
	case pk.Kind == schema.KindUUID:
		return expr
	case pk.Kind.IsInteger():
		return "strconv.FormatInt(int64(" + expr + "), 10)"
	}
	return "url.PathEscape(" + expr + ")"
}

func newProxyView(dv *domainView, file *contract.File, backend string) *proxyView {
	e := dv.Entity
	pk := e.PK()
	pv := &proxyView{
		Alias:    file.GoName,
		Import:   file.GoImport,
		Client:   e.Name + "ServiceClient",
		Backend:  backend,
		IDType:   goBase(pk.Kind),
		IDProto:  protoID(pk, "id"),
		Location: responseLocation(pk, "item."+pk.Name),
	}
	for _, fd := range file.Message(e.Name).Fields {
		f := fd.Source
		pv.Fields = append(pv.Fields, proxyField{Name: f.Name, JSON: f.JSON, Type: responseType(f), From: fromProto(f, "m", fd.GoName)})
	}
	filters := func(message string, skip *schema.Field) []proxyField {
		msg := file.Message(message)
		if msg == nil {
			return nil
		}
		byName := messageFields(msg)
		var out []proxyField
		for _, fv := range dv.Filters {
			if fv.F == skip {
				continue
			}
			if fd := byName[contract.ProtoName(fv.F.Name)]; fd != nil {
				out = append(out, proxyField{Name: fd.GoName, From: filterProto(fv.F, "p."+fv.Name)})
			}
		}
		return out
	}
	pv.Filters = filters(e.Name+"ListRequest", nil)
	conversion := func(message string, fields []*fieldView, update bool) []proxyField {
		msg := file.Message(message)
		if msg == nil {
			return nil
		}
		byName := messageFields(msg)
		var out []proxyField
		for _, fv := range fields {
			name := contract.ProtoName(fv.F.InputName())
			if update {
				name = contract.ProtoName(fv.F.Name)
			}
			if fd := byName[name]; fd != nil {
				out = append(out, proxyField{Name: fd.GoName, From: toProto(fv.F, "req."+fv.Input, fv.InputJSON)})
			}
		}
		return out
	}
	if !e.ReadOnly {
		pv.Create = conversion(e.Name+"CreateRequest", dv.Create, false)
		if dv.HasUpdate {
			pv.Update = conversion(e.Name+"UpdateRequest", dv.Update, true)
		}
		if e.Register {
			var register []*fieldView
			for _, fv := range dv.Create {
				if !fv.F.Role {
					register = append(register, fv)
				}
			}
			pv.Register = conversion(e.Name+"CreateRequest", register, false)
		}
	}
	if dv.Routes {
		for _, rv := range dv.Relations {
			pv.Relations = append(pv.Relations, &proxyRelation{
				relationView: rv,
				ParamsFunc:   "list" + rv.Method[len("List"):] + "Request",
				SourceProto:  protoID(rv.Source.PK(), rv.SourceIDVar),
				Filters:      filters(rv.Method+"Request", relationFK(rv)),
			})
		}
	}
	return pv
}

func generateGateway(opts WorkspaceOptions, wv *workspaceView) ([]File, error) {
	m := opts.Model
	d := opts.Dialect
	module := opts.Module + "/" + GatewayDir
	pv, err := moduleProject(opts, wv, module)
	if err != nil {
		return nil, err
	}
	pv.Gateway = true
	pv.Auth = newAuthView(pv, d, m)
	pv.Ops = newOpsView(pv.Auth)
	identity := *pv.Auth
	identity.IDType, identity.IDParse, identity.IDFormat = "string", "return s, nil", "id"
	pv.Identity = &identity
	authService := pv.Auth.Entity.Service
	pv.Edge = &edgeView{AuthAlias: opts.Contracts.File(authService.Name).GoName, AuthBackend: naming.Pascal(authService.Name)}
	for _, s := range m.Services {
		file := opts.Contracts.File(s.Name)
		for _, e := range s.Entities {
			if e.NoRoutes && !e.IsAuth() {
				continue
			}
			dv := newDomainView(pv, d, m, e)
			dv.Routes = !e.NoRoutes
			dv.RouteLines = nil
			buildRoutes(dv)
			dv.Proxy = newProxyView(dv, file, naming.Pascal(s.Name))
			pv.Domains = append(pv.Domains, dv)
			if dv.Routes {
				pv.Wired = append(pv.Wired, dv)
			}
			if dv.IsAuth {
				pv.Auth.Wired = true
			}
		}
	}

	g := &generator{module: module, prefix: GatewayDir}
	g.goFile("internal/platform/validation/validation_gen.go", "platform/validation.go.tmpl", pv)
	g.goFile("internal/platform/validation/validation_gen_test.go", "platform/validation_test.go.tmpl", pv)
	g.goFile("internal/platform/httpx/problem_gen.go", "platform/problem.go.tmpl", pv)
	g.goFile("internal/platform/httpx/json_gen.go", "platform/json.go.tmpl", pv)
	g.goFile("internal/platform/httpx/request_gen.go", "platform/request.go.tmpl", pv)
	g.goFile("internal/platform/httpx/middleware_gen.go", "platform/middleware.go.tmpl", pv)
	g.goFile("internal/platform/httpx/idempotency_gen.go", "platform/idempotency_http.go.tmpl", pv)
	g.goFile("internal/platform/httpx/health_gen.go", "platform/health.go.tmpl", pv)
	g.goFile("internal/platform/httpx/probe_gen.go", "workspace/httpx_probe.go.tmpl", pv)
	g.goFile("internal/platform/httpx/httpx_gen_test.go", "platform/httpx_test.go.tmpl", pv)
	g.goFile("internal/platform/httpx/cors_gen.go", "platform/cors.go.tmpl", pv)
	g.telemetry(pv)
	g.goFile("internal/platform/grpcx/tls_gen.go", "workspace/grpcx_tls.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/client_gen.go", "workspace/grpcx_client.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/client_gen_test.go", "workspace/grpcx_client_test.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/status_gen.go", "gateway/grpcx_status.go.tmpl", pv)
	g.goFile("internal/platform/grpcx/grpcx_gen_test.go", "gateway/grpcx_test.go.tmpl", pv)
	g.goFile("internal/platform/auth/auth_gen.go", "gateway/auth.go.tmpl", pv)
	g.goFile("internal/platform/auth/config_gen.go", "gateway/auth_config.go.tmpl", pv)
	g.goFile("internal/platform/auth/guard_gen.go", "gateway/guard.go.tmpl", pv)
	g.goFile("internal/platform/auth/limiter_gen.go", "platform/auth_limiter.go.tmpl", pv)
	g.goFile("internal/platform/auth/internal_gen.go", "workspace/auth_internal.go.tmpl", pv)
	g.goFile("internal/platform/auth/auth_gen_test.go", "gateway/auth_test.go.tmpl", pv)
	g.ops(pv)
	g.goFile("internal/app/routes_gen.go", "gateway/app.go.tmpl", pv)
	g.openAPI(pv)
	g.goFile("cmd/devcerts/main_gen.go", "gateway/devcerts.go.tmpl", pv)
	g.userGoFile("internal/platform/config/config.go", "gateway/config.go.tmpl", pv)
	g.userGoFile("cmd/gateway/main.go", "gateway/main.go.tmpl", pv)
	for _, dv := range pv.Domains {
		dir := "internal/" + dv.Package + "/"
		g.goFile(dir+"dto_gen.go", "gateway/dto.go.tmpl", dv)
		if dv.Routes {
			g.goFile(dir+"handler_gen.go", "gateway/handler.go.tmpl", dv)
			g.goFile(dir+"routes_gen.go", "domain/routes.go.tmpl", dv)
		}
		if dv.IsAuth {
			g.goFile(dir+"auth_handler_gen.go", "gateway/auth_handler.go.tmpl", dv)
			g.goFile(dir+"auth_routes_gen.go", "domain/auth_routes.go.tmpl", dv)
		}
	}
	return g.files, g.err
}
