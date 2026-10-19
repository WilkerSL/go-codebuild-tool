package gen

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

var dumpWorkspace = flag.String("dump-workspace", "", "write the generated shop workspace to this directory")

func TestGenerateShopWorkspace(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	sources, err := spec.LoadDir(filepath.Join(filepath.Dir(file), "..", "..", "testdata"), "specs/shop")
	if err != nil {
		t.Fatal(err)
	}
	model, diags := spec.Parse(sources)
	if diags.HasErrors() {
		t.Fatal(diags.Error())
	}
	target, _ := goversion.Resolve("1.27.0")
	d, _ := dialect.Get(os.Getenv("CODEBUILD_DUMP_DIALECT"))
	if d == nil {
		d, _ = dialect.Get("sqlite")
	}
	databases := schema.BuildDatabases(model)
	migrations := map[string]bool{}
	for name := range databases {
		migrations[name] = true
	}
	ws, err := GenerateWorkspace(WorkspaceOptions{
		Module:     "example.com/shop",
		GoVersion:  "1.27.0",
		Caps:       target.Caps,
		Dialect:    d,
		Model:      model,
		Pagination: Pagination{DefaultLimit: 50, MaxLimit: 200},
		Contracts:  contract.Build("example.com/shop", model, nil),
		Migrations: migrations,
	})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, f := range ws.Files {
		paths[f.Path] = true
	}
	for _, p := range []string{"services/billing/internal/invoices/grpc_gen.go", "services/identity/internal/users/auth_grpc_gen.go", "contracts/billing/v1/billing_grpc.pb.go", "databases/shared/migrations/embed_gen.go", "gateway/internal/invoices/handler_gen.go", "gateway/internal/users/auth_handler_gen.go", "gateway/cmd/devcerts/main_gen.go"} {
		if !paths[p] {
			t.Errorf("missing %s", p)
		}
	}
	for _, f := range ws.Files {
		if f.Path != "services/inventory/internal/stocks/grpc_gen.go" && f.Path != "services/billing/internal/invoices/grpc_gen.go" {
			continue
		}
		_, exists, _ := strings.Cut(string(f.Content), "func (s *GRPCServer) Exists(")
		exists, _, _ = strings.Cut(exists, "\n}\n")
		owned := strings.Contains(f.Path, "invoices")
		if strings.Contains(exists, "grpcx.Authorize(ctx)") != owned || strings.Contains(exists, `"admin"`) {
			t.Errorf("%s: Exists must only require a caller for owned entities and never a role:\n%s", f.Path, exists)
		}
	}
	if *dumpWorkspace == "" {
		return
	}
	for _, f := range ws.Files {
		if f.ServiceMethod != "" {
			continue
		}
		full := filepath.Join(*dumpWorkspace, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, f.Content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, db := range databases {
		dir := "services/" + name + "/migrations"
		if name == schema.SharedDatabase {
			dir = SharedDir + "/migrations"
		}
		diff := schema.Diff(nil, db, schema.DiffOptions{})
		up := d.Migration(diff.Changes, nil, db)
		down := d.Migration(diff.Down(), db, nil)
		for suffix, sql := range map[string]string{".up.sql": up, ".down.sql": down} {
			if err := os.WriteFile(filepath.Join(*dumpWorkspace, filepath.FromSlash(dir), "0001_init"+suffix), []byte(sql), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	var use []string
	for _, m := range ws.Modules {
		use = append(use, "./"+m.Dir)
		var b strings.Builder
		b.WriteString("module " + m.Path + "\n\ngo 1.27.0\n\nrequire (\n")
		for _, r := range m.Requires {
			b.WriteString("\t" + r[0] + " " + r[1] + "\n")
		}
		for _, l := range m.Locals {
			b.WriteString("\texample.com/shop/" + l + " v0.0.0\n")
		}
		b.WriteString(")\n")
		for _, l := range m.Locals {
			rel, _ := filepath.Rel(m.Dir, l)
			b.WriteString("\nreplace example.com/shop/" + l + " => " + filepath.ToSlash(rel) + "\n")
		}
		if err := os.WriteFile(filepath.Join(*dumpWorkspace, filepath.FromSlash(m.Dir), "go.mod"), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	work := "go 1.27.0\n\nuse (\n\t" + strings.Join(use, "\n\t") + "\n)\n"
	if err := os.WriteFile(filepath.Join(*dumpWorkspace, "go.work"), []byte(work), 0o644); err != nil {
		t.Fatal(err)
	}
}
