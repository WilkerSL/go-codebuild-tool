package mcpserver

import (
	"context"
	"errors"
	"io"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/project"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
)

const GrammarURI = "codebuild://grammar"

const instructions = `codebuild generates a REST API (net/http handlers, database/sql repositories, migrations) from spec files in specs/<domain>.go.
Recommended workflow:
1. Read the grammar resource (codebuild://grammar).
2. Write or edit one spec file per domain.
3. Call validate with the file content and fix every error.
4. Call plan and show the user which files will be created, regenerated or skipped and which migration will be added.
5. Wait for the user's approval, then call generate.
6. Ask the user to run go mod tidy, go build ./... and go run ./cmd/migrate up.
If plan or generate report unsupported_go_version, explain that the go directive in go.mod must be raised and only pass upgradeGo: true after the user explicitly agrees.
Never edit *_gen.go files; business rules belong in internal/<domain>/service.go.`

type Options struct {
	Version   string
	Dir       string
	Toolchain func(context.Context) (goversion.Toolchain, error)
}

type handler struct {
	dir       string
	toolchain func(context.Context) (goversion.Toolchain, error)
}

func New(opts Options) *mcp.Server {
	dir := opts.Dir
	if dir == "" {
		dir = "."
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	h := handler{dir: dir, toolchain: opts.Toolchain}
	server := mcp.NewServer(&mcp.Implementation{Name: "codebuild", Version: opts.Version}, &mcp.ServerOptions{Instructions: instructions})
	server.AddResource(&mcp.Resource{
		URI:         GrammarURI,
		Name:        "grammar",
		Title:       "codebuild spec grammar",
		Description: "Complete reference of the spec file format, every gen marker with an example and its common error, request semantics, generated routes and diagnostic codes.",
		MIMEType:    "text/markdown",
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: GrammarURI, MIMEType: "text/markdown", Text: spec.Reference()}}}, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_domains",
		Description: "List the domains defined in specs/, the configured SQL dialect, the target Go version from go.mod and the code generation capabilities it enables.",
		Annotations: &mcp.ToolAnnotations{Title: "List domains", ReadOnlyHint: true},
	}, h.listDomains)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "validate",
		Description: "Validate the content of one spec file together with the other specs of the project, without writing it. Returns positioned diagnostics with codes and suggestions.",
		Annotations: &mcp.ToolAnnotations{Title: "Validate spec", ReadOnlyHint: true},
	}, h.validate)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "plan",
		Description: "Dry run of generate: lists every file that would be created, regenerated, skipped or flagged, the new migration, warnings and next steps. Writes nothing.",
		Annotations: &mcp.ToolAnnotations{Title: "Plan generation", ReadOnlyHint: true},
	}, h.plan)
	destructive := false
	mcp.AddTool(server, &mcp.Tool{
		Name:        "generate",
		Description: "Apply the plan: regenerate *_gen.go files, create missing user files, write the new migration and update go.mod with the allowed dependencies. Never overwrites user files or existing migrations. Call it only after the user approved the plan.",
		Annotations: &mcp.ToolAnnotations{Title: "Generate code", DestructiveHint: &destructive, IdempotentHint: true},
	}, h.generate)
	return server
}

func Serve(version string) func(ctx context.Context, dir string, in io.Reader, out io.Writer) error {
	return func(ctx context.Context, dir string, in io.Reader, out io.Writer) error {
		server := New(Options{Version: version, Dir: dir})
		err := server.Run(ctx, &mcp.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{out}})
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error {
	return nil
}

func (h handler) resolve(dir string) string {
	switch {
	case dir == "":
		return h.dir
	case filepath.IsAbs(dir):
		return dir
	}
	return filepath.Join(h.dir, dir)
}

func toolError(err error) (*mcp.CallToolResult, *project.Error) {
	var perr *project.Error
	if !errors.As(err, &perr) {
		perr = &project.Error{Code: "error", Message: err.Error()}
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: perr.Error()}}}, perr
}

type dirInput struct {
	Dir string `json:"dir,omitempty" jsonschema:"project directory, absolute or relative to the server directory; defaults to the server directory"`
}

func (h handler) listDomains(_ context.Context, _ *mcp.CallToolRequest, in dirInput) (*mcp.CallToolResult, *project.Info, error) {
	info, err := project.Inspect(h.resolve(in.Dir))
	if err != nil {
		return nil, nil, err
	}
	return nil, info, nil
}

type validateInput struct {
	Dir     string `json:"dir,omitempty" jsonschema:"project directory; defaults to the server directory"`
	File    string `json:"file" jsonschema:"spec file name such as users.go or specs/users.go; it may be a new file"`
	Content string `json:"content" jsonschema:"complete Go source of the spec file"`
}

type validateOutput struct {
	Valid       bool               `json:"valid"`
	Diagnostics schema.Diagnostics `json:"diagnostics"`
}

func (h handler) validate(_ context.Context, _ *mcp.CallToolRequest, in validateInput) (*mcp.CallToolResult, validateOutput, error) {
	diags, err := project.ValidateSpec(h.resolve(in.Dir), in.File, []byte(in.Content))
	if err != nil {
		return nil, validateOutput{}, err
	}
	if diags == nil {
		diags = schema.Diagnostics{}
	}
	return nil, validateOutput{Valid: !diags.HasErrors(), Diagnostics: diags}, nil
}

type planInput struct {
	Dir       string `json:"dir,omitempty" jsonschema:"project directory; defaults to the server directory"`
	UpgradeGo bool   `json:"upgradeGo,omitempty" jsonschema:"plan as if the go directive were raised to the latest supported version; only after the user agreed"`
}

type planOutput struct {
	Plan  *project.Plan  `json:"plan,omitempty"`
	Error *project.Error `json:"error,omitempty"`
}

func (h handler) plan(ctx context.Context, _ *mcp.CallToolRequest, in planInput) (*mcp.CallToolResult, planOutput, error) {
	plan, err := project.BuildPlan(ctx, project.Options{Dir: h.resolve(in.Dir), UpgradeGo: in.UpgradeGo, Toolchain: h.toolchain})
	if err != nil {
		res, perr := toolError(err)
		return res, planOutput{Error: perr}, nil
	}
	return nil, planOutput{Plan: plan}, nil
}

type generateInput struct {
	Dir       string `json:"dir,omitempty" jsonschema:"project directory; defaults to the server directory"`
	UpgradeGo bool   `json:"upgradeGo,omitempty" jsonschema:"raise the go directive in go.mod to the latest supported version when it is below 1.26; send true only after the user explicitly confirmed"`
	Prune     bool   `json:"prune,omitempty" jsonschema:"delete generated files that the specs no longer produce"`
}

type generateOutput struct {
	Plan   *project.Plan        `json:"plan,omitempty"`
	Result *project.ApplyResult `json:"result,omitempty"`
	Error  *project.Error       `json:"error,omitempty"`
}

func (h handler) generate(ctx context.Context, _ *mcp.CallToolRequest, in generateInput) (*mcp.CallToolResult, generateOutput, error) {
	plan, err := project.BuildPlan(ctx, project.Options{Dir: h.resolve(in.Dir), UpgradeGo: in.UpgradeGo, Prune: in.Prune, Toolchain: h.toolchain})
	if err != nil {
		res, perr := toolError(err)
		return res, generateOutput{Error: perr}, nil
	}
	result, err := project.Apply(plan)
	if err != nil {
		res, perr := toolError(err)
		return res, generateOutput{Plan: plan, Error: perr}, nil
	}
	return nil, generateOutput{Plan: plan, Result: result}, nil
}
