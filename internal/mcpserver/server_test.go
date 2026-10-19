package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
)

func connect(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server := New(Options{Version: "v0.0.0-test", Dir: dir, Toolchain: func(context.Context) (goversion.Toolchain, error) {
		return goversion.Toolchain{Local: "go1.27.0", Setting: "local"}, nil
	}})
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

func newProject(t *testing.T, gomod string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	src := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "specs", "saas")
	if err := os.MkdirAll(filepath.Join(dir, "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"orgs.go", "users.go", "tags.go", "projects.go"} {
		data, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "specs", name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func call(t *testing.T, session *mcp.ClientSession, name string, args map[string]any, out any) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if out != nil && res.StructuredContent != nil {
		data, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s: decode structured content: %v\n%s", name, err, data)
		}
	}
	return res
}

func TestToolsAndGrammar(t *testing.T) {
	session := connect(t, t.TempDir())
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"generate", "list_domains", "plan", "validate"}) {
		t.Fatalf("tools = %v", names)
	}
	res, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: GrammarURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, "### `rel`") {
		t.Fatalf("grammar resource = %+v", res.Contents)
	}
}

func TestValidateAndListDomains(t *testing.T) {
	dir := newProject(t, "module example.com/app\n\ngo 1.27.0\n")
	session := connect(t, dir)
	var validated validateOutput
	res := call(t, session, "validate", map[string]any{
		"file":    "comments.go",
		"content": "//go:build ignore\n\npackage specs\n\nimport \"uuid\"\n\ntype Comment struct {\n\tID     uuid.UUID `gen:\"pk,auto\"`\n\tUserID uuid.UUID `gen:\"rel=belongs_to:Usr\"`\n\tBody   string    `gen:\"requird\"`\n}\n",
	}, &validated)
	if res.IsError || validated.Valid || len(validated.Diagnostics) != 2 {
		t.Fatalf("validate = %+v", validated)
	}
	suggestions := []string{validated.Diagnostics[0].Suggestion, validated.Diagnostics[1].Suggestion}
	slices.Sort(suggestions)
	if !slices.Equal(suggestions, []string{"User", "required"}) || validated.Diagnostics[0].File != "specs/comments.go" {
		t.Fatalf("diagnostics = %+v", validated.Diagnostics)
	}
	call(t, session, "validate", map[string]any{
		"file":    "specs/comments.go",
		"content": "//go:build ignore\n\npackage specs\n\nimport \"uuid\"\n\ntype Comment struct {\n\tID     uuid.UUID `gen:\"pk,auto\"`\n\tUserID uuid.UUID `gen:\"rel=belongs_to:User\"`\n}\n",
	}, &validated)
	if !validated.Valid {
		t.Fatalf("valid spec rejected: %+v", validated)
	}
	var info struct {
		Dialect      string
		GoVersion    string
		Capabilities goversion.Capabilities
		Domains      []struct{ Domain, Path string }
	}
	call(t, session, "list_domains", nil, &info)
	if info.Dialect != "postgres" || info.GoVersion != "1.27.0" || !info.Capabilities.JSONv2 || len(info.Domains) != 4 || info.Domains[0].Domain != "orgs" {
		t.Fatalf("list_domains = %+v", info)
	}
}

func TestPlanAndGenerate(t *testing.T) {
	dir := newProject(t, "module example.com/app\n\ngo 1.26.0\n")
	session := connect(t, dir)
	var planned planOutput
	res := call(t, session, "plan", nil, &planned)
	if res.IsError || planned.Plan == nil || planned.Plan.Migration == nil || len(planned.Plan.Domains) != 4 {
		t.Fatalf("plan = %+v", planned)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal")); err == nil {
		t.Fatal("plan wrote files")
	}
	var generated generateOutput
	res = call(t, session, "generate", nil, &generated)
	if res.IsError || generated.Result == nil || len(generated.Result.Written) == 0 {
		t.Fatalf("generate = %+v", generated)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal", "users", "repository_gen.go")); err != nil {
		t.Fatal(err)
	}
	call(t, session, "plan", map[string]any{"dir": dir}, &planned)
	if planned.Plan.HasChanges() {
		t.Fatal("plan after generate still has changes")
	}
}

func TestGenerateRequiresUpgradeConsent(t *testing.T) {
	dir := newProject(t, "module example.com/legacy\n\ngo 1.23\n")
	session := connect(t, dir)
	var blocked generateOutput
	res := call(t, session, "generate", nil, &blocked)
	if !res.IsError || blocked.Error == nil || blocked.Error.Code != "unsupported_go_version" || blocked.Error.Details["detected"] != "1.23" {
		t.Fatalf("generate without consent = %+v", blocked)
	}
	if text := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "upgradeGo") {
		t.Fatalf("error text = %s", text)
	}
	if _, err := os.Stat(filepath.Join(dir, "internal")); err == nil {
		t.Fatal("blocked generate wrote files")
	}
	var generated generateOutput
	res = call(t, session, "generate", map[string]any{"upgradeGo": true}, &generated)
	if res.IsError || generated.Plan.GoUpgrade == nil || generated.Plan.GoUpgrade.To != "1.27.0" {
		t.Fatalf("generate with consent = %+v", generated)
	}
	gomod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))
	if !strings.Contains(string(gomod), "go 1.27.0") {
		t.Fatalf("go.mod = %s", gomod)
	}
}

func TestToolsStayInsideTheServerDirectory(t *testing.T) {
	dir := newProject(t, "module example.com/app\n\ngo 1.27.0\n")
	other := newProject(t, "module example.com/other\n\ngo 1.27.0\n")
	session := connect(t, dir)
	for _, target := range []string{"..", "../other", other, filepath.Join(dir, "..")} {
		var generated generateOutput
		res := call(t, session, "generate", map[string]any{"dir": target}, &generated)
		if !res.IsError || generated.Error == nil || generated.Error.Code != "outside_root" {
			t.Fatalf("generate %s = %+v", target, generated)
		}
	}
	var planned planOutput
	real, _ := filepath.EvalSymlinks(dir)
	if res := call(t, session, "plan", map[string]any{"dir": "specs"}, &planned); res.IsError || planned.Plan == nil || planned.Plan.Root != real {
		t.Fatalf("plan inside the project = %+v", planned)
	}
	sub := filepath.Join(dir, "specs")
	nested := connect(t, sub)
	res := call(t, nested, "generate", nil, nil)
	if !res.IsError {
		t.Fatal("generate wrote to a project above the server directory")
	}
	if _, err := os.Stat(filepath.Join(dir, "internal")); err == nil {
		t.Fatal("files were written outside the server directory")
	}
}
