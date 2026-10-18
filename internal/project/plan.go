package project

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/version"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/gen"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
	"github.com/WilkerSL/go-codebuild-tool/internal/spec"
	"github.com/WilkerSL/go-codebuild-tool/internal/textdiff"
)

const googleUUIDModule = "github.com/google/uuid"

const googleUUIDVersion = "v1.6.0"

type Options struct {
	Dir           string
	Starter       bool
	Module        string
	Dialect       string
	ChangeDialect bool
	UpgradeGo     bool
	Prune         bool
	Confirm       func(question string) (bool, error)
	Toolchain     func(ctx context.Context) (goversion.Toolchain, error)
}

type Action string

const (
	ActionCreate    Action = "create"
	ActionUpdate    Action = "update"
	ActionUnchanged Action = "unchanged"
	ActionSkip      Action = "skip"
	ActionConflict  Action = "conflict"
	ActionStale     Action = "stale"
	ActionDelete    Action = "delete"
)

type Owner string

const (
	OwnerGenerated Owner = "generated"
	OwnerUser      Owner = "user"
	OwnerMigration Owner = "migration"
	OwnerState     Owner = "state"
	OwnerConfig    Owner = "config"
	OwnerGoMod     Owner = "gomod"
)

type FileChange struct {
	Path    string `json:"path"`
	Action  Action `json:"action"`
	Owner   Owner  `json:"owner"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
	Reason  string `json:"reason,omitempty"`
	content []byte
	old     []byte
}

type GoUpgrade struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type MigrationInfo struct {
	Version uint64 `json:"version"`
	Name    string `json:"name"`
	Up      string `json:"up"`
	Down    string `json:"down"`
}

type ToolchainStatus struct {
	Local    string `json:"local"`
	Setting  string `json:"setting"`
	Required string `json:"required"`
}

type Plan struct {
	Root            string             `json:"root"`
	Module          string             `json:"module"`
	Starter         bool               `json:"starter"`
	Dialect         string             `json:"dialect"`
	PreviousDialect string             `json:"previousDialect,omitempty"`
	Target          goversion.Target   `json:"target"`
	GoUpgrade       *GoUpgrade         `json:"goUpgrade,omitempty"`
	Toolchain       *ToolchainStatus   `json:"toolchain,omitempty"`
	Domains         []string           `json:"domains"`
	Files           []FileChange       `json:"files"`
	Migration       *MigrationInfo     `json:"migration,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
	Diagnostics     schema.Diagnostics `json:"diagnostics,omitempty"`
	NextSteps       []string           `json:"nextSteps,omitempty"`
}

func (p *Plan) HasChanges() bool {
	return slices.ContainsFunc(p.Files, func(f FileChange) bool {
		return f.Action == ActionCreate || f.Action == ActionUpdate || f.Action == ActionDelete
	})
}

func (p *Plan) Conflicts() []FileChange {
	var out []FileChange
	for _, f := range p.Files {
		if f.Action == ActionConflict {
			out = append(out, f)
		}
	}
	return out
}

func (p *Plan) UnifiedDiff() string {
	var b strings.Builder
	for _, f := range p.Files {
		switch f.Action {
		case ActionCreate, ActionUpdate, ActionDelete:
			b.WriteString(textdiff.Unified(f.Path, string(f.old), string(f.content), 3))
		}
	}
	return b.String()
}

type planner struct {
	opts       Options
	plan       *Plan
	root       string
	modData    []byte
	mod        *goversion.ModFile
	cfg        Config
	cfgExisted bool
	dialect    dialect.Dialect
	model      *schema.Model
	files      []FileChange
	hasSQL     bool
}

func BuildPlan(ctx context.Context, opts Options) (*Plan, error) {
	p := &planner{opts: opts, plan: &Plan{Starter: opts.Starter, Domains: []string{}, Files: []FileChange{}}}
	steps := []func(context.Context) error{
		p.locate, p.loadConfig, p.resolveGo, p.loadSpecs, p.planMigrations, p.generate, p.planGoMod, p.classify,
	}
	for _, step := range steps {
		if err := step(ctx); err != nil {
			return nil, err
		}
	}
	p.nextSteps()
	return p.plan, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findUp(dir, name string) (string, bool) {
	for {
		if exists(filepath.Join(dir, name)) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func (p *planner) locate(context.Context) error {
	dir := p.opts.Dir
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return newError(KindInternal, "invalid_dir", err.Error())
	}
	if p.opts.Starter {
		if p.opts.Module == "" {
			return newError(KindUsage, "missing_module", "--starter requires --module, for example --module github.com/acme/shop")
		}
		if err := module.CheckPath(p.opts.Module); err != nil {
			return newError(KindUsage, "invalid_module", fmt.Sprintf("invalid module path %q: %v", p.opts.Module, err))
		}
		if exists(filepath.Join(abs, "go.mod")) {
			return newError(KindConfig, "module_exists", fmt.Sprintf("%s already contains a go.mod; run codebuild build without --starter to generate into the existing module", abs))
		}
		p.root = abs
		p.plan.Root = abs
		p.plan.Module = p.opts.Module
		return nil
	}
	if p.opts.Module != "" {
		return newError(KindUsage, "unexpected_module", "--module is only valid together with --starter")
	}
	root, ok := findUp(abs, ConfigFile)
	if !ok {
		root, ok = findUp(abs, "go.mod")
	}
	if !ok {
		return newError(KindConfig, "no_module", fmt.Sprintf("no go.mod found in %s or its parents; create a project with codebuild build --starter --module <path>", abs))
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return newError(KindConfig, "no_module", fmt.Sprintf("%s has %s but no go.mod next to it", root, ConfigFile))
	}
	mf, err := goversion.ParseModFile("go.mod", data)
	if err != nil {
		return newError(KindConfig, "invalid_go_mod", err.Error())
	}
	if mf.Module == "" {
		return newError(KindConfig, "invalid_go_mod", "go.mod has no module directive")
	}
	p.root, p.modData, p.mod = root, data, mf
	p.plan.Root = root
	p.plan.Module = mf.Module
	return nil
}

func (p *planner) loadConfig(context.Context) error {
	data, err := os.ReadFile(filepath.Join(p.root, ConfigFile))
	switch {
	case err == nil:
		cfg, err := ParseConfig(data)
		if err != nil {
			return newError(KindConfig, "invalid_config", err.Error())
		}
		p.cfg, p.cfgExisted = cfg, true
	case errors.Is(err, fs.ErrNotExist):
		name := p.opts.Dialect
		if name == "" {
			name = dialect.Default
		}
		p.cfg = DefaultConfig(name)
		if !p.opts.Starter {
			p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf("%s was not found; it will be created with dialect %s", ConfigFile, name))
		}
	default:
		return newError(KindInternal, "read_config", err.Error())
	}
	if p.opts.Dialect != "" {
		if _, err := dialect.Get(p.opts.Dialect); err != nil {
			return newError(KindUsage, "unknown_dialect", err.Error())
		}
	}
	if p.opts.ChangeDialect && p.opts.Dialect == "" {
		return newError(KindUsage, "missing_dialect", "--change-dialect requires --dialect <name>")
	}
	if p.cfgExisted && p.opts.Dialect != "" && p.opts.Dialect != p.cfg.Dialect {
		if !p.opts.ChangeDialect {
			e := newError(KindConfig, "dialect_change", fmt.Sprintf(
				"the project uses dialect %s but --dialect %s was given. Switching regenerates the repositories, queries and platform code for %s, "+
					"while the existing migrations stay written for %s and will not run on a %s database. Re-run with --change-dialect to confirm; "+
					"to start the new database from a fresh baseline, also delete migrations/*.sql and %s.",
				p.cfg.Dialect, p.opts.Dialect, p.opts.Dialect, p.cfg.Dialect, p.opts.Dialect, SnapshotFile))
			e.Details = map[string]string{"current": p.cfg.Dialect, "requested": p.opts.Dialect}
			return e
		}
		p.plan.PreviousDialect = p.cfg.Dialect
		p.cfg.Dialect = p.opts.Dialect
	}
	d, err := dialect.Get(p.cfg.Dialect)
	if err != nil {
		return newError(KindConfig, "invalid_config", err.Error())
	}
	p.dialect = d
	p.plan.Dialect = d.Name()
	return nil
}

func (p *planner) detectToolchain(ctx context.Context) (goversion.Toolchain, error) {
	if p.opts.Toolchain != nil {
		return p.opts.Toolchain(ctx)
	}
	return goversion.DetectToolchain(ctx)
}

func (p *planner) resolveGo(ctx context.Context) error {
	tc, tcErr := p.detectToolchain(ctx)
	var goVersion, toolchainDirective string
	if p.opts.Starter {
		if tcErr != nil {
			return newError(KindInternal, "toolchain_unknown", fmt.Sprintf("cannot detect the installed Go toolchain, which --starter needs to choose the go directive: %v", tcErr))
		}
		goVersion = strings.TrimPrefix(tc.Local, "go")
	} else {
		goVersion, toolchainDirective = p.mod.Go, p.mod.Toolchain
	}
	target, err := goversion.Resolve(goVersion)
	if err != nil {
		return newError(KindConfig, "invalid_go_version", err.Error())
	}
	if target.Status == goversion.StatusBelowMinimum {
		to := goversion.LatestVersion()
		source := "go.mod declares go " + goVersion
		if p.opts.Starter {
			source = "the installed toolchain is go " + goVersion
		}
		question := fmt.Sprintf("%s, but codebuild only generates code for Go %s or later. Generate for go %s, the latest supported version? "+
			"The go directive in go.mod will be set to %s; without it the generated code does not compile.", source, goversion.MinimumRelease, to, to)
		authorized := p.opts.UpgradeGo
		if !authorized && p.opts.Confirm != nil {
			if authorized, err = p.opts.Confirm(question); err != nil {
				return newError(KindInternal, "prompt_failed", err.Error())
			}
		}
		if !authorized {
			e := newError(KindUnsupportedGo, "unsupported_go_version", fmt.Sprintf(
				"%s, but codebuild only generates code for Go %s or later. Generation would require changing the go directive to %s. "+
					"Re-run with --upgrade-go to authorize the change (MCP: upgradeGo: true after the user confirms).", source, goversion.MinimumRelease, to))
			e.Details = map[string]string{"detected": goVersion, "minimum": goversion.MinimumRelease, "upgradeTo": to}
			return e
		}
		p.plan.GoUpgrade = &GoUpgrade{From: goVersion, To: to}
		goVersion = to
		if target, err = goversion.Resolve(goVersion); err != nil {
			return newError(KindInternal, "invalid_go_version", err.Error())
		}
		if version.Compare(toolchainDirective, "go"+goVersion) < 0 {
			toolchainDirective = ""
		}
	}
	target.Toolchain = toolchainDirective
	p.plan.Target = target
	p.plan.Warnings = append(p.plan.Warnings, target.Warnings...)
	if tcErr != nil {
		p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf("could not detect the Go toolchain: %v", tcErr))
		return nil
	}
	required := goversion.Max("go"+goVersion, toolchainDirective)
	check := tc.Check(required)
	p.plan.Toolchain = &ToolchainStatus{Local: tc.Local, Setting: tc.Setting, Required: required}
	if check.Fatal {
		e := newError(KindToolchain, "toolchain_too_old", check.Message)
		e.Details = map[string]string{"local": tc.Local, "required": required, "gotoolchain": tc.Setting}
		return e
	}
	if !check.OK {
		p.plan.Warnings = append(p.plan.Warnings, check.Message)
	}
	if w := tc.ExperimentWarning(target.Caps); w != "" {
		p.plan.Warnings = append(p.plan.Warnings, w)
	}
	return nil
}

func (p *planner) loadSpecs(context.Context) error {
	sources, err := spec.LoadDir(p.root, SpecsDir)
	if err != nil {
		return newError(KindInternal, "read_specs", err.Error())
	}
	model, diags := analyze(sources, p.dialect, p.plan.Target.Caps, p.cfg)
	if diags.HasErrors() {
		e := newError(KindInvalidSpec, "invalid_spec", fmt.Sprintf("the specs contain %d error(s)", len(diags.Errors())))
		e.Diagnostics = diags
		return e
	}
	p.plan.Diagnostics = diags
	p.model = model
	for _, e := range model.Entities {
		p.plan.Domains = append(p.plan.Domains, e.Domain)
	}
	slices.Sort(p.plan.Domains)
	return nil
}

func analyze(sources []spec.Source, d dialect.Dialect, caps goversion.Capabilities, cfg Config) (*schema.Model, schema.Diagnostics) {
	model, diags := spec.Parse(sources)
	if !diags.HasErrors() {
		diags = append(diags, dialect.Validate(d, model)...)
	}
	for _, e := range model.Entities {
		if e.UUIDImport == "uuid" && !caps.StdUUID {
			diags = append(diags, schema.Diagnostic{
				Pos:      schema.Pos{File: e.File, Line: e.Pos.Line},
				Severity: schema.SeverityWarning,
				Code:     "toolchain_feature",
				Message:  "the spec imports the standard library uuid package, which requires Go 1.27; use github.com/google/uuid in specs of Go 1.26 projects",
			})
		}
	}
	domains := map[string]bool{}
	for _, e := range model.Entities {
		domains[e.Domain] = true
	}
	for _, name := range cfg.ManualWiring {
		if !domains[name] {
			diags = append(diags, schema.Diagnostic{
				Pos:      schema.Pos{File: ConfigFile},
				Severity: schema.SeverityWarning,
				Code:     "unknown_domain",
				Message:  fmt.Sprintf("manualWiring lists %q, which is not a domain in specs/", name),
			})
		}
	}
	diags.Sort()
	return model, diags
}

var migrationFile = regexp.MustCompile(`^([0-9]+)_(.+)\.(up|down)\.sql$`)

func (p *planner) planMigrations(context.Context) error {
	entries, err := os.ReadDir(filepath.Join(p.root, MigrationsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return newError(KindInternal, "read_migrations", err.Error())
	}
	var maxVersion uint64
	width := 4
	for _, entry := range entries {
		m := migrationFile.FindStringSubmatch(entry.Name())
		if entry.IsDir() || m == nil {
			continue
		}
		p.hasSQL = true
		v, err := strconv.ParseUint(m[1], 10, 64)
		if err != nil {
			continue
		}
		maxVersion = max(maxVersion, v)
		width = max(width, len(m[1]))
	}
	var old *schema.Database
	snapshotPath := filepath.Join(p.root, filepath.FromSlash(SnapshotFile))
	data, err := os.ReadFile(snapshotPath)
	switch {
	case err == nil:
		snap, err := schema.UnmarshalSnapshot(data)
		if err != nil {
			return newError(KindConfig, "invalid_snapshot", fmt.Sprintf("%s: %v", SnapshotFile, err))
		}
		old = snap.Database()
	case errors.Is(err, fs.ErrNotExist):
		if p.hasSQL {
			return newError(KindConfig, "snapshot_missing", fmt.Sprintf(
				"%s contains SQL migrations but %s is missing, so codebuild cannot tell which schema they produce. "+
					"Restore the snapshot from version control, or delete the generated migrations to start from a new baseline.", MigrationsDir, SnapshotFile))
		}
	default:
		return newError(KindInternal, "read_snapshot", err.Error())
	}
	next := schema.Build(p.model)
	diff := schema.Diff(old, next, schema.DiffOptions{RecreateDependents: p.dialect.RecreateDependents()})
	if !diff.Empty() {
		version := maxVersion + 1
		base := fmt.Sprintf("%0*d_%s", width, version, schema.MigrationName(old, diff.Changes))
		up := path.Join(MigrationsDir, base+".up.sql")
		down := path.Join(MigrationsDir, base+".down.sql")
		p.add(up, []byte(p.dialect.Migration(diff.Changes, old, next)), OwnerMigration)
		p.add(down, []byte(p.dialect.Migration(diff.Down(), next, old)), OwnerMigration)
		p.plan.Migration = &MigrationInfo{Version: version, Name: base, Up: up, Down: down}
		p.plan.Warnings = append(p.plan.Warnings, diff.Warnings...)
	}
	if p.plan.PreviousDialect != "" && p.hasSQL {
		p.plan.Warnings = append(p.plan.Warnings, fmt.Sprintf(
			"existing migrations were written for %s and will not run on %s; new migrations use %s", p.plan.PreviousDialect, p.dialect.Name(), p.dialect.Name()))
	}
	snapshot, err := schema.NewSnapshot(p.dialect.Name(), next).Marshal()
	if err != nil {
		return newError(KindInternal, "snapshot", err.Error())
	}
	p.add(SnapshotFile, snapshot, OwnerState)
	return nil
}

func (p *planner) add(filePath string, content []byte, owner Owner) {
	p.files = append(p.files, FileChange{Path: filePath, Owner: owner, content: content})
}

func (p *planner) generate(context.Context) error {
	files, err := gen.Generate(gen.Options{
		Module:        p.plan.Module,
		GoVersion:     p.plan.Target.GoVersion,
		Caps:          p.plan.Target.Caps,
		Dialect:       p.dialect,
		Model:         p.model,
		Pagination:    p.cfg.Pagination,
		ManualWiring:  p.cfg.ManualWiring,
		HasMigrations: p.hasSQL || p.plan.Migration != nil,
	})
	if err != nil {
		return newError(KindInternal, "generate", err.Error())
	}
	for _, f := range files {
		owner := OwnerUser
		if f.Owner == gen.OwnerGenerated {
			owner = OwnerGenerated
		}
		p.add(f.Path, f.Content, owner)
	}
	if !p.cfgExisted || p.plan.PreviousDialect != "" {
		p.add(ConfigFile, p.cfg.Marshal(), OwnerConfig)
	}
	return nil
}

func (p *planner) requiredModules() [][2]string {
	driver := p.dialect.Driver()
	mods := [][2]string{{driver.Module, driver.Version}}
	if !p.plan.Target.Caps.StdUUID {
		mods = append(mods, [2]string{googleUUIDModule, googleUUIDVersion})
	}
	slices.SortFunc(mods, func(a, b [2]string) int { return strings.Compare(a[0], b[0]) })
	return mods
}

func (p *planner) planGoMod(context.Context) error {
	mods := p.requiredModules()
	data := p.modData
	if p.opts.Starter {
		data = []byte(fmt.Sprintf("module %s\n\ngo %s\n", p.plan.Module, p.plan.Target.GoVersion))
	}
	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return newError(KindConfig, "invalid_go_mod", err.Error())
	}
	modified := p.opts.Starter
	if up := p.plan.GoUpgrade; up != nil && !p.opts.Starter {
		if err := f.AddGoStmt(up.To); err != nil {
			return newError(KindInternal, "go_mod", err.Error())
		}
		if f.Toolchain != nil && version.Compare(f.Toolchain.Name, "go"+up.To) < 0 {
			f.DropToolchainStmt()
		}
		modified = true
	}
	for _, m := range mods {
		if !slices.ContainsFunc(f.Require, func(r *modfile.Require) bool { return r.Mod.Path == m[0] }) {
			f.AddNewRequire(m[0], m[1], false)
			modified = true
		}
	}
	if modified {
		f.Cleanup()
		data = modfile.Format(f.Syntax)
	}
	p.add("go.mod", data, OwnerGoMod)
	if p.plan.Target.Caps.StdUUID && slices.ContainsFunc(f.Require, func(r *modfile.Require) bool { return r.Mod.Path == googleUUIDModule }) {
		p.plan.Warnings = append(p.plan.Warnings, "the generated code uses the standard library uuid package; github.com/google/uuid stays in go.mod until go mod tidy removes it")
	}
	return nil
}

func hasHeader(content []byte) bool {
	line, _, _ := bytes.Cut(content, []byte("\n"))
	return string(bytes.TrimRight(line, "\r")) == gen.Header
}

func normalize(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
}

func isGeneratedName(name string) bool {
	return strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, "_gen_test.go")
}

func (p *planner) classify(context.Context) error {
	planned := map[string]bool{}
	for i := range p.files {
		f := &p.files[i]
		planned[f.Path] = true
		old, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(f.Path)))
		exists := err == nil
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return newError(KindInternal, "read_file", err.Error())
		}
		if exists {
			f.old = normalize(old)
		}
		switch {
		case !exists:
			f.Action = ActionCreate
		case f.Owner == OwnerUser:
			f.Action, f.Reason = ActionSkip, "user-owned file already exists"
		case f.Owner == OwnerMigration:
			f.Action, f.Reason = ActionConflict, "a migration with this name already exists"
		case f.Owner == OwnerGenerated && !hasHeader(old):
			f.Action = ActionConflict
			f.Reason = "the \"" + gen.Header + "\" header was removed, so the file may contain manual changes; move them to a non-generated file such as service.go and delete this file, or restore the header"
		case bytes.Equal(f.old, f.content):
			f.Action = ActionUnchanged
		default:
			f.Action = ActionUpdate
		}
		if f.Action == ActionCreate || f.Action == ActionUpdate {
			f.Added, f.Removed = textdiff.Stats(string(f.old), string(f.content))
		}
	}
	for _, dir := range []string{"internal", MigrationsDir} {
		err := filepath.WalkDir(filepath.Join(p.root, dir), func(full string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if d.IsDir() || !isGeneratedName(d.Name()) {
				return nil
			}
			rel, err := filepath.Rel(p.root, full)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if planned[rel] {
				return nil
			}
			content, err := os.ReadFile(full)
			if err != nil {
				return err
			}
			if !hasHeader(content) {
				return nil
			}
			change := FileChange{Path: rel, Owner: OwnerGenerated, old: normalize(content), Action: ActionStale,
				Reason: "no longer produced by the specs; run codebuild build --prune to delete it"}
			if p.opts.Prune {
				change.Action, change.Reason = ActionDelete, "no longer produced by the specs"
				_, change.Removed = textdiff.Stats(string(change.old), "")
			}
			p.files = append(p.files, change)
			return nil
		})
		if err != nil {
			return newError(KindInternal, "scan_files", err.Error())
		}
	}
	slices.SortFunc(p.files, func(a, b FileChange) int { return strings.Compare(a.Path, b.Path) })
	p.plan.Files = p.files
	return nil
}

func (p *planner) nextSteps() {
	var steps []string
	changed := map[string]Action{}
	for _, f := range p.plan.Files {
		changed[f.Path] = f.Action
	}
	if a := changed["go.mod"]; a == ActionCreate || a == ActionUpdate {
		steps = append(steps, "go mod tidy")
	}
	if p.plan.HasChanges() {
		steps = append(steps, "go build ./... && go vet ./... && go test ./...")
	}
	if p.plan.Migration != nil {
		steps = append(steps, "go run ./cmd/migrate up")
	}
	for _, name := range p.cfg.ManualWiring {
		if slices.Contains(p.plan.Domains, name) {
			steps = append(steps, fmt.Sprintf("wire %s by hand in cmd/api/main.go: %s.RegisterRoutes(mux, %s.NewHandler(%s.NewService(%s.NewRepository(db)), logger))", name, name, name, name, name))
		}
	}
	if p.opts.Starter {
		if len(p.plan.Domains) == 0 {
			steps = append(steps, "write a spec in specs/<domain>.go and run codebuild build")
		}
		steps = append(steps, "go get -tool github.com/WilkerSL/go-codebuild-tool/cmd/codebuild (optional, pins codebuild for go tool codebuild)")
	}
	p.plan.NextSteps = steps
}
