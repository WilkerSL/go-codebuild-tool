package project

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/gen"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
)

var update = flag.Bool("update", false, "rewrite golden projects")

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func fakeToolchain(local string) func(context.Context) (goversion.Toolchain, error) {
	return func(context.Context) (goversion.Toolchain, error) {
		return goversion.Toolchain{Local: local, Setting: "local"}, nil
	}
}

func copySpecs(t *testing.T, dir string) {
	t.Helper()
	copySpecSet(t, dir, "saas")
}

func copySpecSet(t *testing.T, dir, set string) {
	t.Helper()
	src := filepath.Join(repoRoot(t), "testdata", "specs", set)
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(dir, "specs", e.Name()), string(data))
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func build(t *testing.T, opts Options) *Plan {
	t.Helper()
	plan, err := BuildPlan(context.Background(), opts)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return plan
}

func starter(t *testing.T, goVersion, dialectName string) string {
	t.Helper()
	dir := t.TempDir()
	copySpecs(t, dir)
	build(t, Options{Dir: dir, Starter: true, Module: "example.com/saas", Dialect: dialectName, Toolchain: fakeToolchain("go" + goVersion)})
	return dir
}

func treeFiles(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		files[filepath.ToSlash(rel)] = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

var goldenProjects = []struct {
	name     string
	specs    string
	jwt      bool
	versions []string
}{
	{name: "saas", specs: "saas", versions: []string{"1.26.0", "1.27.0"}},
	{name: "secure", specs: "secure", versions: []string{"1.26.0", "1.27.0"}},
	{name: "secure-jwt", specs: "secure", jwt: true, versions: []string{"1.27.0"}},
}

func secureStarter(t *testing.T, goVersion, dialectName string, jwt bool) string {
	t.Helper()
	dir := t.TempDir()
	copySpecSet(t, dir, "secure")
	if jwt {
		spec := filepath.Join(dir, "specs", "users.go")
		writeTestFile(t, spec, strings.Replace(readTestFile(t, spec), "auth=session|apikey", "auth=jwt|apikey", 1))
	}
	build(t, Options{Dir: dir, Starter: true, Module: "example.com/secure", Dialect: dialectName, Toolchain: fakeToolchain("go" + goVersion)})
	return dir
}

func TestGoldenProjects(t *testing.T) {
	for _, project := range goldenProjects {
		for _, goVersion := range project.versions {
			for _, name := range dialect.Names() {
				t.Run(project.name+"/go"+goVersion+"/"+name, func(t *testing.T) {
					dir := ""
					if project.specs == "saas" {
						dir = starter(t, goVersion, name)
					} else {
						dir = secureStarter(t, goVersion, name, project.jwt)
					}
					release := goVersion[:strings.LastIndex(goVersion, ".")]
					golden := filepath.Join(repoRoot(t), "testdata", "golden", project.name, "go"+release, name)
					got := treeFiles(t, dir)
					if *update {
						if err := os.RemoveAll(golden); err != nil {
							t.Fatal(err)
						}
						for rel, data := range got {
							writeTestFile(t, filepath.Join(golden, filepath.FromSlash(rel)), string(data))
						}
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
	}
}

func TestBuildIsIdempotent(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	plan, err := BuildPlan(context.Background(), Options{Dir: filepath.Join(dir, "internal"), Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if plan.HasChanges() || plan.Migration != nil {
		for _, f := range plan.Files {
			if f.Action != ActionUnchanged && f.Action != ActionSkip {
				t.Errorf("%s: %s", f.Path, f.Action)
			}
		}
	}
	if plan.Root != dir {
		t.Errorf("root = %s, want %s", plan.Root, dir)
	}
}

func TestUserFilesAreNeverOverwritten(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	service := filepath.Join(dir, "internal", "users", "service.go")
	custom := "package users\n\ntype Service struct {\n\t*Repository\n\tcustom bool\n}\n\nfunc NewService(repo *Repository) *Service {\n\treturn &Service{Repository: repo}\n}\n"
	writeTestFile(t, service, custom)
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if readTestFile(t, service) != custom {
		t.Fatal("service.go was overwritten")
	}
	idx := slices.IndexFunc(plan.Files, func(f FileChange) bool { return f.Path == "internal/users/service.go" })
	if plan.Files[idx].Action != ActionSkip {
		t.Fatalf("service.go action = %s", plan.Files[idx].Action)
	}
}

func TestEditedGeneratedFileIsAConflict(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	path := filepath.Join(dir, "internal", "users", "handler_gen.go")
	content := readTestFile(t, path)
	edited := strings.Replace(content, "// Code generated by codebuild. DO NOT EDIT.\n", "", 1)
	writeTestFile(t, path, edited)
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts()) != 1 || plan.Conflicts()[0].Path != "internal/users/handler_gen.go" {
		t.Fatalf("conflicts = %+v", plan.Conflicts())
	}
	_, err = Apply(plan)
	var perr *Error
	if !errors.As(err, &perr) || perr.Kind != KindOwnership {
		t.Fatalf("apply error = %v", err)
	}
	if readTestFile(t, path) != edited {
		t.Fatal("edited generated file was overwritten")
	}
}

func TestEditedOpenAPIDocumentIsAConflict(t *testing.T) {
	dir := starter(t, "1.27.0", "sqlite")
	path := filepath.Join(dir, gen.OpenAPIPath)
	content := readTestFile(t, path)

	// Changing the body but keeping the marker is an ordinary update.
	writeTestFile(t, path, strings.Replace(content, `"openapi": "3.1.0"`, `"openapi": "3.0.0"`, 1))
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts()) != 0 {
		t.Fatalf("conflicts with the marker kept = %+v", plan.Conflicts())
	}

	marker := `  "x-codebuild": "` + gen.OpenAPIMarker + `",` + "\n"
	writeTestFile(t, path, strings.Replace(content, marker, "", 1))
	plan, err = BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	conflicts := plan.Conflicts()
	if len(conflicts) != 1 || conflicts[0].Path != gen.OpenAPIPath || !strings.Contains(conflicts[0].Reason, "x-codebuild") {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}

func TestSpecChangeCreatesIncrementalMigration(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	initial := readTestFile(t, filepath.Join(dir, "migrations", "0001_init.up.sql"))
	usersSpec := filepath.Join(dir, "specs", "users.go")
	writeTestFile(t, usersSpec, strings.Replace(readTestFile(t, usersSpec), "\tBio       *string   `gen:\"max=500\"`\n", "\tBio       *string   `gen:\"max=500\"`\n\tPhone     *string   `gen:\"max=20\"`\n", 1))
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if plan.Migration == nil || plan.Migration.Name != "0002_alter_users" {
		t.Fatalf("migration = %+v", plan.Migration)
	}
	if readTestFile(t, filepath.Join(dir, "migrations", "0001_init.up.sql")) != initial {
		t.Fatal("an existing migration was modified")
	}
	up := readTestFile(t, filepath.Join(dir, "migrations", "0002_alter_users.up.sql"))
	if !strings.Contains(up, `ALTER TABLE "users" ADD COLUMN "phone" VARCHAR(20);`) {
		t.Fatalf("unexpected migration:\n%s", up)
	}
	again, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if again.Migration != nil {
		t.Fatal("a second build created another migration")
	}
}

func TestRemovedSpecLeavesStaleFiles(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	if err := os.Remove(filepath.Join(dir, "specs", "audits.go")); err != nil {
		t.Fatal(err)
	}
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	stale := 0
	for _, f := range plan.Files {
		if f.Action == ActionStale {
			stale++
		}
	}
	if stale == 0 || !exists(filepath.Join(dir, "internal", "audits", "model_gen.go")) {
		t.Fatalf("expected stale files to be kept, got %d", stale)
	}
	dropped := plan.Migration
	plan = build(t, Options{Dir: dir, Prune: true, Toolchain: fakeToolchain("go1.27.0")})
	if exists(filepath.Join(dir, "internal", "audits", "model_gen.go")) {
		t.Fatal("stale generated file was not pruned")
	}
	if !exists(filepath.Join(dir, "internal", "audits", "service.go")) {
		t.Fatal("prune removed a user-owned file")
	}
	if dropped == nil || plan.Migration != nil || !strings.Contains(readTestFile(t, filepath.Join(dir, filepath.FromSlash(dropped.Up))), `DROP TABLE "audit_log"`) {
		t.Fatalf("expected one drop migration, got %+v then %+v", dropped, plan.Migration)
	}
}

func TestDialectChangeRequiresFlag(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Dialect: "sqlserver", Toolchain: fakeToolchain("go1.27.0")})
	var perr *Error
	if !errors.As(err, &perr) || perr.Kind != KindConfig || perr.Code != "dialect_change" {
		t.Fatalf("error = %v", err)
	}
	plan := build(t, Options{Dir: dir, Dialect: "sqlserver", ChangeDialect: true, Toolchain: fakeToolchain("go1.27.0")})
	if plan.PreviousDialect != "postgres" || plan.Dialect != "sqlserver" || len(plan.Warnings) == 0 {
		t.Fatalf("plan = %+v", plan)
	}
	cfg, err := ParseConfig([]byte(readTestFile(t, filepath.Join(dir, ConfigFile))))
	if err != nil || cfg.Dialect != "sqlserver" {
		t.Fatalf("config = %+v %v", cfg, err)
	}
	if !strings.Contains(readTestFile(t, filepath.Join(dir, "internal", "platform", "database", "connect_gen.go")), "go-mssqldb") {
		t.Fatal("platform code was not regenerated for sqlserver")
	}
}

func existingModule(t *testing.T, gomod string) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), gomod)
	copySpecs(t, dir)
	return dir
}

func TestGoVersionBelowMinimum(t *testing.T) {
	gomod := "module example.com/legacy\n\ngo 1.24\n"
	var perr *Error

	dir := existingModule(t, gomod)
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if !errors.As(err, &perr) || perr.Kind != KindUnsupportedGo || perr.Details["detected"] != "1.24" {
		t.Fatalf("no TTY: error = %v", err)
	}

	asked := ""
	_, err = BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0"), Confirm: func(q string) (bool, error) {
		asked = q
		return false, nil
	}})
	if !errors.As(err, &perr) || perr.Kind != KindUnsupportedGo || !strings.Contains(asked, "1.24") || !strings.Contains(asked, "1.27.0") {
		t.Fatalf("declined: error = %v, question = %q", err, asked)
	}

	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0"), Confirm: func(string) (bool, error) { return true, nil }})
	if plan.GoUpgrade == nil || plan.GoUpgrade.To != "1.27.0" || !strings.Contains(readTestFile(t, filepath.Join(dir, "go.mod")), "go 1.27.0") {
		t.Fatalf("accepted: plan = %+v", plan.GoUpgrade)
	}

	dir = existingModule(t, gomod)
	plan = build(t, Options{Dir: dir, UpgradeGo: true, Toolchain: fakeToolchain("go1.27.0")})
	if plan.GoUpgrade == nil || !plan.Target.Caps.StdUUID {
		t.Fatalf("--upgrade-go: plan = %+v", plan.Target)
	}
}

func TestToolchainTooOld(t *testing.T) {
	dir := existingModule(t, "module example.com/app\n\ngo 1.27.0\n")
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.26.3")})
	var perr *Error
	if !errors.As(err, &perr) || perr.Kind != KindToolchain {
		t.Fatalf("error = %v", err)
	}
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: func(context.Context) (goversion.Toolchain, error) {
		return goversion.Toolchain{Local: "go1.26.3", Setting: "auto"}, nil
	}})
	if err != nil || !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "download") }) {
		t.Fatalf("auto: %v %v", err, plan)
	}
}

func TestExistingModuleAdoption(t *testing.T) {
	dir := existingModule(t, "module example.com/app\n\ngo 1.26.0\n\ntoolchain go1.27.0\n\nrequire github.com/jackc/pgx/v5 v5.9.0\n")
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if plan.Target.Caps.StdUUID || plan.Target.Toolchain != "go1.27.0" {
		t.Fatalf("target = %+v", plan.Target)
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, ConfigFile) }) {
		t.Fatalf("missing config warning: %v", plan.Warnings)
	}
	gomod := readTestFile(t, filepath.Join(dir, "go.mod"))
	if !strings.Contains(gomod, "github.com/jackc/pgx/v5 v5.9.0") || !strings.Contains(gomod, "github.com/google/uuid v1.6.0") || !strings.Contains(gomod, "toolchain go1.27.0") {
		t.Fatalf("go.mod = %s", gomod)
	}
	if !strings.Contains(readTestFile(t, filepath.Join(dir, "internal", "users", "model_gen.go")), `"github.com/google/uuid"`) {
		t.Fatal("go 1.26 project must use github.com/google/uuid")
	}
}

func TestAboveLatestKnownVersion(t *testing.T) {
	dir := existingModule(t, "module example.com/app\n\ngo 1.29.0\n")
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.29.0")})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Status != goversion.StatusAboveKnown || plan.Target.Caps.Release != "1.27" || len(plan.Warnings) == 0 {
		t.Fatalf("target = %+v", plan.Target)
	}
}

func TestStarterErrors(t *testing.T) {
	var perr *Error
	dir := existingModule(t, "module example.com/app\n\ngo 1.27.0\n")
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Starter: true, Module: "example.com/x", Toolchain: fakeToolchain("go1.27.0")})
	if !errors.As(err, &perr) || perr.Code != "module_exists" {
		t.Fatalf("error = %v", err)
	}
	_, err = BuildPlan(context.Background(), Options{Dir: t.TempDir(), Starter: true, Toolchain: fakeToolchain("go1.27.0")})
	if !errors.As(err, &perr) || perr.Kind != KindUsage {
		t.Fatalf("error = %v", err)
	}
	_, err = BuildPlan(context.Background(), Options{Dir: t.TempDir(), Toolchain: fakeToolchain("go1.27.0")})
	if !errors.As(err, &perr) || perr.Code != "no_module" {
		t.Fatalf("error = %v", err)
	}
}

func TestEmptyStarter(t *testing.T) {
	dir := t.TempDir()
	plan := build(t, Options{Dir: dir, Starter: true, Module: "example.com/empty", Toolchain: fakeToolchain("go1.27.0")})
	if plan.Migration != nil || len(plan.Domains) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	if !strings.Contains(readTestFile(t, filepath.Join(dir, "migrations", "embed_gen.go")), "var FS embed.FS") {
		t.Fatal("missing migrations package")
	}
	if strings.Contains(readTestFile(t, filepath.Join(dir, "migrations", "embed_gen.go")), "//go:embed") {
		t.Fatal("embed directive without migrations does not compile")
	}
	if readTestFile(t, filepath.Join(dir, ".gitattributes")) != "* text=auto eol=lf\n" {
		t.Fatal("missing .gitattributes")
	}
}

func TestInvalidSpecIsReported(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	writeTestFile(t, filepath.Join(dir, "specs", "broken.go"), "//go:build ignore\n\npackage specs\n\ntype Broken struct {\n\tID int64 `gen:\"pk,auot\"`\n}\n")
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	var perr *Error
	if !errors.As(err, &perr) || perr.Kind != KindInvalidSpec || len(perr.Diagnostics) == 0 || perr.Diagnostics[0].Suggestion != "auto" {
		t.Fatalf("error = %v", err)
	}
	diags, err := ValidateSpec(dir, "broken.go", []byte("//go:build ignore\n\npackage specs\n\ntype Broken struct {\n\tID int64 `gen:\"pk,auto\"`\n}\n"))
	if err != nil || diags.HasErrors() {
		t.Fatalf("ValidateSpec = %v %v", diags, err)
	}
}

func TestSnapshotMissing(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	if err := os.Remove(filepath.Join(dir, filepath.FromSlash(SnapshotFile))); err != nil {
		t.Fatal(err)
	}
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "snapshot_missing" {
		t.Fatalf("error = %v", err)
	}
}

func TestInspect(t *testing.T) {
	dir := starter(t, "1.26.0", "sqlserver")
	info, err := Inspect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Dialect != "sqlserver" || info.GoVersion != "1.26.0" || len(info.Domains) != 6 || info.Capabilities.StdUUID {
		t.Fatalf("info = %+v", info)
	}
}

func TestServiceMethodScaffolds(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	servicePath := filepath.Join(dir, "internal", "users", "service.go")
	service := readTestFile(t, servicePath)
	for _, method := range []string{"List", "Get", "Create", "Update", "Delete", "ListOrgUsers", "ListTagUsers", "LinkTagUsers", "UnlinkTagUsers"} {
		if !strings.Contains(service, "func (s *Service) "+method+"(") {
			t.Fatalf("service.go lacks %s:\n%s", method, service)
		}
	}
	deleteStart := strings.Index(service, "func (s *Service) Delete(")
	deleteEnd := deleteStart + strings.Index(service[deleteStart:], "\n}\n") + 3
	writeTestFile(t, servicePath, service[:deleteStart]+service[deleteEnd:])
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	scaffold := filepath.Join(dir, "internal", "users", "service_delete.go")
	if !strings.Contains(readTestFile(t, scaffold), "func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {\n\treturn s.repo.Delete(ctx, id)\n}") {
		t.Fatalf("unexpected scaffold:\n%s", readTestFile(t, scaffold))
	}
	idx := slices.IndexFunc(plan.Files, func(f FileChange) bool { return f.Path == "internal/users/service_delete.go" })
	if idx < 0 || plan.Files[idx].Action != ActionCreate || !strings.Contains(plan.Files[idx].Reason, "Service.Delete") {
		t.Fatalf("plan entry = %+v", plan.Files[idx])
	}
	again, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil || again.HasChanges() {
		t.Fatalf("second plan: %v %v", err, again.HasChanges())
	}

	if err := os.Rename(servicePath, filepath.Join(dir, "internal", "users", "users_service.go")); err != nil {
		t.Fatal(err)
	}
	again, err = BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil || again.HasChanges() {
		t.Fatalf("plan after moving Service: %v %+v", err, again.Files)
	}

	writeTestFile(t, filepath.Join(dir, "specs", "badges.go"), "//go:build ignore\n\npackage specs\n\ntype Badge struct {\n\tID    int64  `gen:\"pk,auto\"`\n\tUsers []User `gen:\"rel=many_to_many:User,via=user_badges\"`\n}\n")
	plan = build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	for _, name := range []string{"service_list_badge_users.go", "service_link_badge_users.go", "service_unlink_badge_users.go"} {
		if !exists(filepath.Join(dir, "internal", "users", name)) {
			t.Errorf("missing scaffold %s", name)
		}
	}
	if exists(servicePath) {
		t.Error("service.go was recreated although Service is declared in another file")
	}
	if plan.Migration == nil {
		t.Error("expected a migration for the new tables")
	}
}

func TestAuthServiceScaffolds(t *testing.T) {
	dir := t.TempDir()
	copySpecSet(t, dir, "secure")
	spec := filepath.Join(dir, "specs", "users.go")
	writeTestFile(t, spec, strings.Replace(readTestFile(t, spec), ",register", "", 1))
	build(t, Options{Dir: dir, Starter: true, Module: "example.com/secure", Toolchain: fakeToolchain("go1.27.0")})
	service := filepath.Join(dir, "internal", "users", "auth_service.go")
	if content := readTestFile(t, service); strings.Contains(content, "Register") || !strings.Contains(content, "func (s *AuthService) Login(") {
		t.Fatalf("auth_service.go:\n%s", content)
	}
	writeTestFile(t, spec, strings.Replace(readTestFile(t, spec), "auth=session|apikey", "auth=session|apikey,register", 1))
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	scaffold := "internal/users/auth_service_register.go"
	i := slices.IndexFunc(plan.Files, func(f FileChange) bool { return f.Path == scaffold })
	if i < 0 || plan.Files[i].Action != ActionCreate || plan.Files[i].Reason != "adds AuthService.Register, which the generated handler now requires" {
		t.Fatalf("scaffold plan = %+v", plan.Files)
	}
	if content := readTestFile(t, filepath.Join(dir, filepath.FromSlash(scaffold))); !strings.Contains(content, "func (s *AuthService) Register(ctx context.Context, in CreateInput) (User, error) {\n\treturn s.repo.Create(ctx, in)\n}") {
		t.Fatalf("scaffold:\n%s", content)
	}
	if strings.Contains(readTestFile(t, service), "Register") {
		t.Fatal("auth_service.go was edited")
	}
	mainGo := filepath.Join(dir, "cmd", "api", "main.go")
	writeTestFile(t, mainGo, strings.Replace(readTestFile(t, mainGo), "if err := app.RegisterRoutes(mux, db, logger); err != nil {\n\t\treturn err\n\t}", "app.RegisterRoutes(mux, db, logger)", 1))
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "ignores the error returned by app.RegisterRoutes") }) {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
	if slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "written before metrics") }) {
		t.Fatalf("a generated main.go was reported as unwired: %v", plan.Warnings)
	}
	writeTestFile(t, mainGo, strings.Replace(readTestFile(t, mainGo), "telemetry.Start(", "telemetry.Begin(", 1))
	plan, err = BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "cmd/api/main.go: written before metrics") }) {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}

func addPhone(t *testing.T, dir, column string) {
	t.Helper()
	usersSpec := filepath.Join(dir, "specs", "users.go")
	writeTestFile(t, usersSpec, strings.Replace(readTestFile(t, usersSpec), "\tBio       *string   `gen:\"max=500\"`\n",
		"\tBio       *string   `gen:\"max=500\"`\n\t"+column+" *string `gen:\"max=20\"`\n", 1))
}

func planError(t *testing.T, dir string) *Error {
	t.Helper()
	_, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("error = %v", err)
	}
	return perr
}

func TestDuplicateMigrationVersions(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	addPhone(t, dir, "Phone")
	build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	migrations := filepath.Join(dir, "migrations")
	writeTestFile(t, filepath.Join(migrations, "0002_alter_orgs.up.sql"), "SELECT 1;\n")
	writeTestFile(t, filepath.Join(migrations, "0002_alter_orgs.down.sql"), "SELECT 1;\n")
	if perr := planError(t, dir); perr.Code != "migration_conflict" || !strings.Contains(perr.Message, "0002_alter_orgs and 0002_alter_users share version 2") {
		t.Fatalf("error = %+v", perr)
	}
}

func TestMergedOlderMigration(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	addPhone(t, dir, "Phone")
	build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	addPhone(t, dir, "Fax")
	build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	migrations := filepath.Join(dir, "migrations")
	for _, ext := range []string{".up.sql", ".down.sql"} {
		if err := os.Rename(filepath.Join(migrations, "0002_alter_users"+ext), filepath.Join(migrations, "0002_alter_users_phone"+ext)); err != nil {
			t.Fatal(err)
		}
	}
	if perr := planError(t, dir); perr.Code != "migration_conflict" || !strings.Contains(perr.Message, "0002_alter_users_phone is older than the newest migration") {
		t.Fatalf("error = %+v", perr)
	}
}

func TestInterruptedBuildReusesMigration(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	snapshot := filepath.Join(dir, filepath.FromSlash(SnapshotFile))
	before := readTestFile(t, snapshot)
	addPhone(t, dir, "Phone")
	build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	writeTestFile(t, snapshot, before)
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if plan.Migration != nil || !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, "earlier build was interrupted") }) {
		t.Fatalf("migration = %+v, warnings = %v", plan.Migration, plan.Warnings)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrations", "0003_alter_users.up.sql")); err == nil {
		t.Fatal("the interrupted migration was generated twice")
	}
	if snap := readTestFile(t, snapshot); !strings.Contains(snap, `"0002_alter_users"`) || !strings.Contains(snap, `"phone"`) {
		t.Fatalf("snapshot was not repaired:\n%s", snap)
	}
}

func TestHandWrittenMigrationIsRecorded(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	migrations := filepath.Join(dir, "migrations")
	writeTestFile(t, filepath.Join(migrations, "0002_seed.up.sql"), "INSERT INTO countries (code, name) VALUES ('BR', 'Brazil');\n")
	writeTestFile(t, filepath.Join(migrations, "0002_seed.down.sql"), "DELETE FROM countries WHERE code = 'BR';\n")
	build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	addPhone(t, dir, "Phone")
	plan := build(t, Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if plan.Migration == nil || plan.Migration.Name != "0003_alter_users" {
		t.Fatalf("migration = %+v", plan.Migration)
	}
	if snap := readTestFile(t, filepath.Join(dir, filepath.FromSlash(SnapshotFile))); !strings.Contains(snap, `"0002_seed"`) {
		t.Fatalf("the hand-written migration is not recorded:\n%s", snap)
	}
}

func TestApplyHoldsAProjectLock(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	addPhone(t, dir, "Phone")
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, LockFile)
	writeTestFile(t, lock, "4242\n")
	_, err = Apply(plan)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "locked" || !strings.Contains(perr.Message, "4242") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "migrations", "0002_alter_users.up.sql")); err == nil {
		t.Fatal("files were written while another build held the lock")
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(plan); err != nil {
		t.Fatalf("a stale lock blocked the build: %v", err)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Fatal("the lock was not released")
	}
}

func TestApplyRejectsAnOutdatedPlan(t *testing.T) {
	dir := starter(t, "1.27.0", "postgres")
	addPhone(t, dir, "Phone")
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(dir, "migrations", "0002_alter_users.up.sql"), "SELECT 1;\n")
	_, err = Apply(plan)
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "plan_outdated" {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(readTestFile(t, filepath.Join(dir, "internal", "users", "model_gen.go")), "Phone") {
		t.Fatal("generated files were written although the plan was outdated")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "internal", "users"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".codebuild-") {
			t.Fatalf("temporary file %s was left behind", e.Name())
		}
	}
}

func TestNearestModuleWinsOverParentConfig(t *testing.T) {
	parent := t.TempDir()
	writeTestFile(t, filepath.Join(parent, ConfigFile), string(DefaultConfig("mysql").Marshal()))
	writeTestFile(t, filepath.Join(parent, "go.mod"), "module example.com/parent\n\ngo 1.27.0\n")
	child := filepath.Join(parent, "services", "api")
	copySpecs(t, child)
	writeTestFile(t, filepath.Join(child, "go.mod"), "module example.com/api\n\ngo 1.27.0\n")
	plan, err := BuildPlan(context.Background(), Options{Dir: filepath.Join(child, "specs"), Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Root != child || plan.Module != "example.com/api" || plan.Dialect != dialect.Default {
		t.Fatalf("root %s, module %s, dialect %s", plan.Root, plan.Module, plan.Dialect)
	}
}

func TestUpgradeTargetsTheLocalToolchain(t *testing.T) {
	dir := existingModule(t, "module example.com/legacy\n\ngo 1.24\n")
	plan := build(t, Options{Dir: dir, UpgradeGo: true, Toolchain: fakeToolchain("go1.26.2")})
	if plan.GoUpgrade == nil || plan.GoUpgrade.To != "1.26.2" || plan.Target.Caps.StdUUID {
		t.Fatalf("upgrade = %+v, caps = %+v", plan.GoUpgrade, plan.Target.Caps)
	}
	dir = existingModule(t, "module example.com/legacy\n\ngo 1.24\n")
	auto := func(context.Context) (goversion.Toolchain, error) {
		return goversion.Toolchain{Local: "go1.25.4", Setting: "auto"}, nil
	}
	plan = build(t, Options{Dir: dir, UpgradeGo: true, Toolchain: auto})
	if plan.GoUpgrade == nil || plan.GoUpgrade.To != goversion.LatestVersion() {
		t.Fatalf("upgrade with an old toolchain = %+v", plan.GoUpgrade)
	}
}

func TestWorkspaceIsChecked(t *testing.T) {
	t.Setenv("GOWORK", "")
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.work"), "go 1.26.0\n\nuse ./other\n")
	dir := filepath.Join(root, "api")
	copySpecs(t, dir)
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/api\n\ngo 1.27.0\n")
	plan, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"below the go 1.27.0 of this module", "does not list this module"} {
		if !slices.ContainsFunc(plan.Warnings, func(w string) bool { return strings.Contains(w, want) }) {
			t.Errorf("missing warning %q in %v", want, plan.Warnings)
		}
	}
	writeTestFile(t, filepath.Join(root, "go.work"), "go 1.27.0\n\ntoolchain go1.28.1\n\nuse ./api\n")
	_, err = BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")})
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "toolchain_too_old" || perr.Details["required"] != "go1.28.1" {
		t.Fatalf("workspace toolchain = %v", err)
	}
	t.Setenv("GOWORK", "off")
	if _, err := BuildPlan(context.Background(), Options{Dir: dir, Toolchain: fakeToolchain("go1.27.0")}); err != nil {
		t.Fatalf("GOWORK=off: %v", err)
	}
}
