package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
)

type result struct {
	code   int
	stdout string
	stderr string
}

func run(t *testing.T, stdin string, interactive bool, local string, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	app := &App{
		Stdin:       strings.NewReader(stdin),
		Stdout:      &stdout,
		Stderr:      &stderr,
		Interactive: interactive,
		Version:     "v0.0.0-test",
		Toolchain: func(context.Context) (goversion.Toolchain, error) {
			return goversion.Toolchain{Local: local, Setting: "local"}, nil
		},
		ServeMCP: func(ctx context.Context, dir string, in io.Reader, out io.Writer) error {
			_, err := io.WriteString(out, "mcp:"+dir)
			return err
		},
	}
	code := app.Run(context.Background(), args)
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func saasSpecs(t *testing.T, dir string) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "specs", "saas")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "specs"), 0o755)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "specs", e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVersion(t *testing.T) {
	r := run(t, "", false, "go1.27.0", "version")
	if r.code != ExitOK || !strings.Contains(r.stdout, "codebuild v0.0.0-test") || !strings.Contains(r.stdout, "encoding/json/v2") {
		t.Fatalf("version = %+v", r)
	}
	r = run(t, "", false, "go1.27.0", "--json", "version")
	var info versionInfo
	if err := json.Unmarshal([]byte(r.stdout), &info); err != nil || info.Minimum != "1.26" || len(info.Dialects) != 4 {
		t.Fatalf("version --json = %s %v", r.stdout, err)
	}
}

func TestUsageErrors(t *testing.T) {
	if r := run(t, "", false, "go1.27.0"); r.code != ExitUsage {
		t.Fatalf("no command: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "deploy"); r.code != ExitUsage || !strings.Contains(r.stderr, "unknown command") {
		t.Fatalf("unknown command: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "build", "--nope"); r.code != ExitUsage {
		t.Fatalf("unknown flag: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "build", "--starter", "--dir", t.TempDir()); r.code != ExitUsage || !strings.Contains(r.stderr, "--module") {
		t.Fatalf("missing module: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "build", "--dialect", "oracle", "--starter", "--module", "example.com/x", "--dir", t.TempDir()); r.code != ExitUsage {
		t.Fatalf("unknown dialect: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "help"); r.code != ExitOK || !strings.Contains(r.stdout, "Exit codes") {
		t.Fatalf("help: %+v", r)
	}
}

func TestStarterBuildPlanAndCheck(t *testing.T) {
	dir := t.TempDir()
	saasSpecs(t, dir)
	r := run(t, "", false, "go1.27.0", "build", "--starter", "--module", "example.com/saas", "--dialect", "sqlserver", "--dir", dir)
	if r.code != ExitOK || !strings.Contains(r.stdout, "create") || !strings.Contains(r.stdout, "go mod tidy") {
		t.Fatalf("starter: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "plan", "--exit-code", "--dir", dir); r.code != ExitOK {
		t.Fatalf("plan after build: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "check", "--dir", dir); r.code != ExitOK || !strings.Contains(r.stdout, "ok:") {
		t.Fatalf("check: %+v", r)
	}
	usersSpec := filepath.Join(dir, "specs", "users.go")
	data, _ := os.ReadFile(usersSpec)
	os.WriteFile(usersSpec, []byte(strings.Replace(string(data), "max=120", "max=150", 1)), 0o644)
	r = run(t, "", false, "go1.27.0", "plan", "--exit-code", "--diff", "--dir", dir)
	if r.code != ExitPlanChanges || !strings.Contains(r.stdout, "+++ b/migrations/0002_alter_users.up.sql") || !strings.Contains(r.stdout, "NVARCHAR(150)") {
		t.Fatalf("plan with changes: %+v", r)
	}
	r = run(t, "", false, "go1.27.0", "--json", "plan", "--dir", dir)
	var plan struct {
		Dialect   string
		Migration struct{ Name string }
		Files     []struct{ Path, Action string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &plan); err != nil || plan.Dialect != "sqlserver" || plan.Migration.Name != "0002_alter_users" {
		t.Fatalf("plan --json: %v %s", err, r.stdout)
	}
	if r := run(t, "", false, "go1.27.0", "build", "--dir", dir, "--dialect", "postgres"); r.code != ExitConfig {
		t.Fatalf("dialect change without flag: %+v", r)
	}
}

func TestInvalidSpecExitCode(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "specs"), 0o755)
	os.WriteFile(filepath.Join(dir, "specs", "items.go"), []byte("//go:build spec\n\npackage specs\n\ntype Item struct {\n\tID int64 `gen:\"pk,auto,uniqe\"`\n}\n"), 0o644)
	r := run(t, "", false, "go1.27.0", "build", "--starter", "--module", "example.com/x", "--dir", dir)
	if r.code != ExitInvalidSpec || !strings.Contains(r.stderr, `did you mean "unique"?`) {
		t.Fatalf("invalid spec: %+v", r)
	}
	r = run(t, "", false, "go1.27.0", "--json", "build", "--starter", "--module", "example.com/x", "--dir", dir)
	var out struct {
		Error struct {
			Code        string
			Diagnostics []struct{ Code, Suggestion string }
		}
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil || out.Error.Code != "invalid_spec" || out.Error.Diagnostics[0].Suggestion != "unique" {
		t.Fatalf("invalid spec --json: %v %s", err, r.stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		t.Fatal("a failed build wrote files")
	}
}

func legacyModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/legacy\n\ngo 1.22\n"), 0o644)
	saasSpecs(t, dir)
	return dir
}

func TestUnsupportedGoVersion(t *testing.T) {
	dir := legacyModule(t)
	r := run(t, "", false, "go1.27.0", "build", "--dir", dir)
	if r.code != ExitUnsupportedGo || !strings.Contains(r.stderr, "--upgrade-go") {
		t.Fatalf("no TTY: %+v", r)
	}
	r = run(t, "n\n", true, "go1.27.0", "build", "--dir", dir)
	if r.code != ExitUnsupportedGo || !strings.Contains(r.stderr, "[y/N]") {
		t.Fatalf("declined: %+v", r)
	}
	r = run(t, "\n", true, "go1.27.0", "build", "--dir", dir)
	if r.code != ExitUnsupportedGo {
		t.Fatalf("default answer must be no: %+v", r)
	}
	r = run(t, "", false, "go1.27.0", "check", "--dir", dir)
	if r.code != ExitUnsupportedGo {
		t.Fatalf("check: %+v", r)
	}
	r = run(t, "y\n", true, "go1.27.0", "build", "--dir", dir)
	if r.code != ExitOK || !strings.Contains(r.stdout, "go directive 1.22 -> 1.27.0") {
		t.Fatalf("accepted: %+v", r)
	}
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(gomod), "go 1.27.0") {
		t.Fatalf("go.mod = %s", gomod)
	}
	dir = legacyModule(t)
	if r := run(t, "", false, "go1.27.0", "build", "--upgrade-go", "--dir", dir); r.code != ExitOK {
		t.Fatalf("--upgrade-go: %+v", r)
	}
	dir = legacyModule(t)
	r = run(t, "", false, "go1.27.0", "--json", "build", "--dir", dir)
	var out struct{ Error struct{ Code string } }
	if json.Unmarshal([]byte(r.stdout), &out); r.code != ExitUnsupportedGo || out.Error.Code != "unsupported_go_version" {
		t.Fatalf("--json: %+v", r)
	}
}

func TestToolchainAndOwnershipExitCodes(t *testing.T) {
	dir := t.TempDir()
	saasSpecs(t, dir)
	if r := run(t, "", false, "go1.27.0", "build", "--starter", "--module", "example.com/saas", "--dir", dir); r.code != ExitOK {
		t.Fatalf("starter: %+v", r)
	}
	if r := run(t, "", false, "go1.26.0", "build", "--dir", dir); r.code != ExitToolchain {
		t.Fatalf("old toolchain: %+v", r)
	}
	path := filepath.Join(dir, "internal", "orgs", "model_gen.go")
	data, _ := os.ReadFile(path)
	os.WriteFile(path, []byte(strings.Replace(string(data), "// Code generated by codebuild. DO NOT EDIT.\n", "", 1)), 0o644)
	if r := run(t, "", false, "go1.27.0", "build", "--dir", dir); r.code != ExitOwnership || !strings.Contains(r.stderr, "internal/orgs/model_gen.go") {
		t.Fatalf("conflict: %+v", r)
	}
	if r := run(t, "", false, "go1.27.0", "check", "--dir", dir); r.code != ExitOwnership {
		t.Fatalf("check conflict: %+v", r)
	}
}

func TestMCPCommand(t *testing.T) {
	r := run(t, "", false, "go1.27.0", "mcp", "--dir", "somewhere")
	if r.code != ExitOK || r.stdout != "mcp:somewhere" {
		t.Fatalf("mcp: %+v", r)
	}
}
