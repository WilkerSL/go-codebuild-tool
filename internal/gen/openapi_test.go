package gen

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

func specOptions(t *testing.T, set, module string, edit func(*spec.Source)) Options {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	sources, err := spec.LoadDir(filepath.Join(filepath.Dir(file), "..", "..", "testdata"), "specs/"+set)
	if err != nil {
		t.Fatal(err)
	}
	if edit != nil {
		for i := range sources {
			edit(&sources[i])
		}
	}
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	target, _ := goversion.Resolve("1.27.0")
	d, _ := dialect.Get("postgres")
	return Options{Module: module, GoVersion: "1.27.0", Caps: target.Caps, Dialect: d, Model: model,
		Pagination: Pagination{DefaultLimit: 20, MaxLimit: 100}, HasMigrations: true}
}

func TestOpenAPIMatchesRoutes(t *testing.T) {
	jwt := func(s *spec.Source) {
		s.Content = []byte(strings.Replace(string(s.Content), "auth=session|apikey", "auth=jwt|apikey", 1))
	}
	for name, opts := range map[string]Options{
		"saas":       specOptions(t, "saas", "example.com/saas", nil),
		"secure":     specOptions(t, "secure", "example.com/secure", nil),
		"secure-jwt": specOptions(t, "secure", "example.com/secure", jwt),
	} {
		t.Run(name, func(t *testing.T) {
			files, err := Generate(opts)
			if err != nil {
				t.Fatal(err)
			}
			checkOpenAPI(t, files, "")
		})
	}
	t.Run("shop", func(t *testing.T) {
		opts := specOptions(t, "shop", "example.com/shop", nil)
		databases := schema.BuildDatabases(opts.Model)
		migrations := map[string]bool{}
		for name := range databases {
			migrations[name] = true
		}
		ws, err := GenerateWorkspace(WorkspaceOptions{
			Module: opts.Module, GoVersion: opts.GoVersion, Caps: opts.Caps, Dialect: opts.Dialect, Model: opts.Model,
			Pagination: opts.Pagination, Contracts: contract.Build(opts.Module, opts.Model, nil), Migrations: migrations, Parts: opts.Parts,
		})
		if err != nil {
			t.Fatal(err)
		}
		checkOpenAPI(t, ws.Files, GatewayDir+"/")
	})
}

var muxRoute = regexp.MustCompile(`mux\.Handle(?:Func)?\("([A-Z]+) (/[^"]*)"`)

// checkOpenAPI compares the routes registered on the generated ServeMux under
// prefix with the operations of openapi.json, and checks that every reference,
// path parameter and operation id of the document is consistent.
func checkOpenAPI(t *testing.T, files []File, prefix string) {
	t.Helper()
	var doc map[string]any
	routes := map[string]bool{"GET /livez": true, "GET /readyz": true}
	for _, f := range files {
		switch {
		case f.Path == OpenAPIPath:
			if err := json.Unmarshal(f.Content, &doc); err != nil {
				t.Fatalf("openapi.json is not JSON: %v", err)
			}
		case strings.HasPrefix(f.Path, prefix) && strings.HasSuffix(f.Path, ".go") && !strings.HasSuffix(f.Path, "_test.go") && !strings.Contains(f.Path, "platform/telemetry/"):
			for _, m := range muxRoute.FindAllStringSubmatch(string(f.Content), -1) {
				if m[2] != "/"+OpenAPIPath {
					routes[m[1]+" "+m[2]] = true
				}
			}
		}
	}
	if doc == nil {
		t.Fatal("no openapi.json was generated")
	}
	if doc["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v", doc["openapi"])
	}
	documented := map[string]bool{}
	ids := map[string]string{}
	for route, item := range doc["paths"].(map[string]any) {
		want := pathParamPattern.FindAllStringSubmatch(route, -1)
		for method, raw := range item.(map[string]any) {
			op := raw.(map[string]any)
			key := strings.ToUpper(method) + " " + route
			documented[key] = true
			id, _ := op["operationId"].(string)
			if prev, dup := ids[id]; dup || id == "" {
				t.Errorf("%s: operationId %q is empty or also used by %s", key, id, prev)
			}
			ids[id] = key
			var declared []string
			params, _ := op["parameters"].([]any)
			for _, p := range params {
				if m, _ := p.(map[string]any); m["in"] == "path" {
					declared = append(declared, m["name"].(string))
				}
			}
			var names []string
			for _, m := range want {
				names = append(names, m[1])
			}
			if !slices.Equal(declared, names) {
				t.Errorf("%s declares path parameters %v, want %v", key, declared, names)
			}
			if _, ok := op["responses"].(map[string]any)["default"]; !ok && !strings.HasSuffix(route, "z") {
				t.Errorf("%s has no default response", key)
			}
		}
	}
	for route := range routes {
		if !documented[route] {
			t.Errorf("%s is served but not in openapi.json", route)
		}
	}
	for route := range documented {
		if !routes[route] {
			t.Errorf("%s is in openapi.json but not served", route)
		}
	}
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok && !resolves(doc, ref) {
				t.Errorf("$ref %s does not resolve", ref)
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(doc)
}

func resolves(doc map[string]any, ref string) bool {
	parts, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return false
	}
	var cur any = doc
	for _, part := range strings.Split(parts, "/") {
		m, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		if cur, ok = m[part]; !ok {
			return false
		}
	}
	return true
}
