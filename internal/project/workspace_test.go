package project

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/contract"
	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/gen"
)

func workspaceStarter(t *testing.T, goVersion, dialectName string) string {
	t.Helper()
	dir := t.TempDir()
	copySpecSet(t, dir, "shop")
	build(t, Options{Dir: dir, Starter: true, Module: "example.com/shop", Dialect: dialectName, Toolchain: fakeToolchain("go" + goVersion)})
	return dir
}

func TestGoldenWorkspaces(t *testing.T) {
	for _, name := range dialect.Names() {
		t.Run(name, func(t *testing.T) {
			dir := workspaceStarter(t, "1.27.0", name)
			golden := filepath.Join(repoRoot(t), "testdata", "golden", "shop", "go1.27", name)
			got := treeFiles(t, dir)
			if *update {
				updateGolden(t, golden, got)
				return
			}
			want := treeFiles(t, golden)
			for rel, data := range got {
				if w, ok := want[rel]; !ok {
					t.Errorf("unexpected file %s (run go test ./internal/project -update)", rel)
				} else if !bytes.Equal(w, data) {
					t.Errorf("%s differs from the golden file (run go test ./internal/project -update)", rel)
				}
			}
			for rel := range want {
				if _, ok := got[rel]; !ok {
					t.Errorf("missing file %s", rel)
				}
			}
		})
	}
}

func TestWorkspaceStarter(t *testing.T) {
	dir := workspaceStarter(t, "1.27.0", "postgres")
	for _, f := range []string{
		"go.mod", WorkFile, contract.ContractRegistry, "contracts/go.mod", "gateway/go.mod", "databases/shared/go.mod",
		"services/billing/go.mod", "services/billing/migrations/0001_init.up.sql", "services/billing/migrations/" + SnapshotName,
		"databases/shared/migrations/0001_init.up.sql", "services/identity/cmd/accounts/main.go", "gateway/cmd/devcerts/main_gen.go",
		"docker-compose.yml", ".github/workflows/services-billing.yml", ".github/workflows/workspace.yml", "services/billing/Dockerfile",
	} {
		if !exists(filepath.Join(dir, filepath.FromSlash(f))) {
			t.Errorf("missing %s", f)
		}
	}
	for _, f := range []string{"cmd/api/main.go", SnapshotFile, "internal", "services/catalog/migrations"} {
		if exists(filepath.Join(dir, filepath.FromSlash(f))) {
			t.Errorf("unexpected %s", f)
		}
	}
	work := readTestFile(t, filepath.Join(dir, WorkFile))
	for _, use := range []string{"\t.\n", "./contracts", "./gateway", "./databases/shared", "./services/billing", "./services/inventory"} {
		if !strings.Contains(work, use) {
			t.Errorf("go.work misses %q:\n%s", use, work)
		}
	}
	catalog := readTestFile(t, filepath.Join(dir, "services", "catalog", "go.mod"))
	for _, want := range []string{"module example.com/shop/services/catalog", "example.com/shop/databases/shared v0.0.0", "replace example.com/shop/contracts => ../../contracts", "replace example.com/shop/databases/shared => ../../databases/shared", gen.GRPCModule} {
		if !strings.Contains(catalog, want) {
			t.Errorf("services/catalog/go.mod misses %q:\n%s", want, catalog)
		}
	}
	if compose := readTestFile(t, filepath.Join(dir, "docker-compose.yml")); !strings.HasPrefix(compose, gen.YAMLHeader+"\n") || !strings.Contains(compose, "billing-db:") || !strings.Contains(compose, "shared-db:") {
		t.Errorf("docker-compose.yml:\n%s", compose)
	}
	plan, err := BuildPlan(context.Background(), Options{Dir: filepath.Join(dir, "services", "billing", "internal"), Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Root != dir {
		t.Errorf("root = %s, want %s", plan.Root, dir)
	}
	if plan.HasChanges() || len(plan.Migrations) > 0 {
		for _, f := range plan.Files {
			if f.Action != ActionUnchanged && f.Action != ActionSkip {
				t.Errorf("%s: %s", f.Path, f.Action)
			}
		}
	}
}

func TestWorkspaceMigrationsPerDatabase(t *testing.T) {
	dir := t.TempDir()
	copySpecSet(t, dir, "shop")
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Starter: true, Module: "example.com/shop", Dialect: "sqlite", Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	var databases []string
	for _, m := range plan.Migrations {
		databases = append(databases, m.Database+":"+m.Up)
	}
	want := []string{"billing:services/billing/migrations/0001_init.up.sql", "identity:services/identity/migrations/0001_init.up.sql", "shared:databases/shared/migrations/0001_init.up.sql"}
	if !slices.Equal(databases, want) || plan.Migration != nil {
		t.Fatalf("migrations = %v, want %v", databases, want)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(dir, "specs", "billing.go")
	writeTestFile(t, spec, strings.Replace(readTestFile(t, spec), "Notes     *string       `gen:\"max=500\"`", "DueAt     *time.Time    `gen:\"index\"`", 1))
	plan = build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if len(plan.Migrations) != 1 || plan.Migrations[0].Database != "billing" || plan.Migrations[0].Version != 2 {
		t.Fatalf("migrations = %+v", plan.Migrations)
	}
	registry, err := contract.UnmarshalRegistry([]byte(readTestFile(t, filepath.Join(dir, filepath.FromSlash(contract.ContractRegistry)))))
	if err != nil {
		t.Fatal(err)
	}
	invoice := registry.Messages["billing.v1.Invoice"]
	if invoice == nil || invoice.Fields["due_at"] != 7 || !slices.Equal(invoice.Reserved, []int32{5}) || invoice.Fields["created_at"] != 6 {
		t.Fatalf("billing.v1.Invoice numbers = %+v", invoice)
	}
	if proto := readTestFile(t, filepath.Join(dir, "contracts", "billing", "v1", "billing.proto")); !strings.Contains(proto, "reserved 5;") {
		t.Fatalf("billing.proto does not reserve the removed field:\n%s", proto)
	}
}

func TestRemovedServiceLeavesStaleFiles(t *testing.T) {
	dir := workspaceStarter(t, "1.27.0", "sqlite")
	if err := os.Remove(filepath.Join(dir, "specs", "inventory.go")); err != nil {
		t.Fatal(err)
	}
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	stale := map[string]bool{}
	for _, f := range plan.Files {
		if f.Action == ActionStale {
			stale[f.Path] = true
		}
	}
	for _, f := range []string{"contracts/inventory/v1/inventory.proto", "contracts/inventory/v1/inventory.pb.go", "gateway/internal/stocks/handler_gen.go", "services/inventory/internal/stocks/grpc_gen.go", "deploy/kubernetes/base/inventory.yaml"} {
		if !stale[f] {
			t.Errorf("%s is not stale", f)
		}
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "services/inventory no longer matches a service") }) {
		t.Errorf("warnings = %v", plan.Warnings)
	}
	if len(plan.Migrations) != 1 || plan.Migrations[0].Database != "shared" {
		t.Errorf("migrations = %+v", plan.Migrations)
	}
}

func TestLayoutChangeIsRejected(t *testing.T) {
	monolith := starter(t, "1.27.0", "sqlite")
	if err := os.RemoveAll(filepath.Join(monolith, "specs")); err != nil {
		t.Fatal(err)
	}
	copySpecSet(t, monolith, "shop")
	if e := planError(t, monolith); e.Code != "layout_change" || !strings.Contains(e.Message, "does not convert a monolith") {
		t.Errorf("monolith to workspace = %s: %s", e.Code, e.Message)
	}
	workspace := workspaceStarter(t, "1.27.0", "sqlite")
	if err := os.RemoveAll(filepath.Join(workspace, "specs")); err != nil {
		t.Fatal(err)
	}
	copySpecs(t, workspace)
	if e := planError(t, workspace); e.Code != "layout_change" || !strings.Contains(e.Message, "back into a monolith") {
		t.Errorf("workspace to monolith = %s: %s", e.Code, e.Message)
	}
}

func TestEditedComposeFileIsAConflict(t *testing.T) {
	dir := workspaceStarter(t, "1.27.0", "sqlite")
	compose := filepath.Join(dir, "docker-compose.yml")
	writeTestFile(t, compose, strings.TrimPrefix(readTestFile(t, compose), gen.YAMLHeader+"\n"))
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Path != "docker-compose.yml" || !strings.Contains(conflicts[0].Reason, "docker-compose.override.yml") {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestEditedKubernetesManifestIsAConflict(t *testing.T) {
	dir := workspaceStarter(t, "1.27.0", "sqlite")
	manifest := filepath.Join(dir, "deploy", "kubernetes", "base", "gateway.yaml")
	writeTestFile(t, manifest, strings.TrimPrefix(readTestFile(t, manifest), gen.YAMLHeader+"\n"))
	overlay := filepath.Join(dir, "deploy", "kubernetes", "overlays", "production", "kustomization.yaml")
	writeTestFile(t, overlay, readTestFile(t, overlay)+"commonAnnotations:\n  team: shop\n")
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Path != "deploy/kubernetes/base/gateway.yaml" || !strings.Contains(conflicts[0].Reason, "deploy/kubernetes/overlays/production") {
		t.Fatalf("conflicts = %+v", conflicts)
	}
	for _, f := range plan.Files {
		if f.Path == "deploy/kubernetes/overlays/production/kustomization.yaml" && f.Action != ActionSkip {
			t.Fatalf("the user overlay was planned as %s", f.Action)
		}
	}
}

// TestGoldenCodeHasNoComments keeps the generated Go code free of comments in
// every golden project, monoliths with authentication and workspaces included.
func TestGoldenCodeHasNoComments(t *testing.T) {
	golden := filepath.Join(repoRoot(t), "testdata", "golden")
	for rel, data := range treeFiles(t, golden) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".pb.go") {
			continue
		}
		for line := range strings.Lines(string(data)) {
			if text := strings.TrimSpace(line); strings.HasPrefix(text, "//") && !strings.HasPrefix(text, "//go:") && !strings.HasPrefix(text, "// Code generated") {
				t.Errorf("%s contains a comment: %s", rel, text)
				break
			}
		}
	}
}
