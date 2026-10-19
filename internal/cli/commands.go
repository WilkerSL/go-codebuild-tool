package cli

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/dialect"
	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/project"
	"github.com/WilkerSL/go-codebuild-tool/internal/schema"
)

type buildFlags struct {
	starter       bool
	module        string
	dialect       string
	changeDialect bool
	prune         bool
	diff          bool
	exitCode      bool
}

func (a *App) runBuild(ctx context.Context, g *globalFlags, command string, args []string) int {
	var bf buildFlags
	ok, code := a.parse(g, command, args, func(fs *flag.FlagSet) {
		fs.BoolVar(&bf.starter, "starter", false, "create a new project")
		fs.StringVar(&bf.module, "module", "", "module path for --starter")
		fs.StringVar(&bf.dialect, "dialect", "", "SQL dialect")
		fs.BoolVar(&bf.changeDialect, "change-dialect", false, "confirm a dialect change")
		fs.BoolVar(&bf.prune, "prune", false, "delete stale generated files")
		if command == "plan" {
			fs.BoolVar(&bf.diff, "diff", false, "print a unified diff")
			fs.BoolVar(&bf.exitCode, "exit-code", false, "exit with 8 when there are changes")
		}
	})
	if !ok {
		return code
	}
	opts := project.Options{
		Dir:           g.dir,
		Starter:       bf.starter,
		Module:        bf.module,
		Dialect:       bf.dialect,
		ChangeDialect: bf.changeDialect,
		UpgradeGo:     g.upgradeGo,
		Prune:         bf.prune,
		Toolchain:     a.Toolchain,
	}
	if command == "build" {
		opts.Confirm = a.confirm(g)
	}
	plan, err := project.BuildPlan(ctx, opts)
	if err != nil {
		return a.fail(g, err)
	}
	if command == "plan" {
		if g.json {
			a.writeJSON(plan)
		} else {
			a.printPlan(plan, true)
			if bf.diff {
				fmt.Fprint(a.Stdout, plan.UnifiedDiff())
			}
		}
		if bf.exitCode && plan.HasChanges() {
			return ExitPlanChanges
		}
		return ExitOK
	}
	result, err := project.Apply(plan)
	if err != nil {
		if !g.json {
			a.printPlan(plan, false)
		}
		return a.fail(g, err)
	}
	if g.json {
		a.writeJSON(map[string]any{"plan": plan, "result": result})
	} else {
		a.printPlan(plan, false)
	}
	return ExitOK
}

func (a *App) printHeader(p *project.Plan) {
	w := a.Stdout
	fmt.Fprintf(w, "root:     %s\n", p.Root)
	fmt.Fprintf(w, "module:   %s\n", p.Module)
	fmt.Fprintf(w, "go:       %s, %s\n", p.Target.GoVersion, p.Target.Caps)
	if p.GoUpgrade != nil {
		fmt.Fprintf(w, "upgrade:  go directive %s -> %s\n", p.GoUpgrade.From, p.GoUpgrade.To)
	}
	if p.Toolchain != nil {
		fmt.Fprintf(w, "toolchain: %s (GOTOOLCHAIN=%s, requires %s)\n", p.Toolchain.Local, p.Toolchain.Setting, p.Toolchain.Required)
	}
	dialectLine := p.Dialect
	if p.PreviousDialect != "" {
		dialectLine = p.PreviousDialect + " -> " + p.Dialect
	}
	fmt.Fprintf(w, "dialect:  %s\n", dialectLine)
	domains := "none"
	if len(p.Domains) > 0 {
		domains = strings.Join(p.Domains, ", ")
	}
	fmt.Fprintf(w, "domains:  %s\n", domains)
}

func (a *App) printPlan(p *project.Plan, dryRun bool) {
	w := a.Stdout
	a.printHeader(p)
	fmt.Fprintln(w)
	counts := map[project.Action]int{}
	width := 0
	for _, f := range p.Files {
		width = max(width, len(f.Path))
	}
	for _, f := range p.Files {
		counts[f.Action]++
		if f.Action == project.ActionUnchanged || (f.Action == project.ActionSkip && !dryRun) {
			continue
		}
		line := fmt.Sprintf("  %-9s %-*s", f.Action, width, f.Path)
		switch {
		case f.Added > 0 || f.Removed > 0:
			line += fmt.Sprintf("  +%d -%d", f.Added, f.Removed)
		case f.Reason != "":
			line += "  (" + f.Reason + ")"
		}
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
	fmt.Fprintf(w, "\n%d created, %d updated, %d deleted, %d unchanged, %d skipped, %d stale, %d conflicts\n",
		counts[project.ActionCreate], counts[project.ActionUpdate], counts[project.ActionDelete], counts[project.ActionUnchanged],
		counts[project.ActionSkip], counts[project.ActionStale], counts[project.ActionConflict])
	if p.Migration != nil {
		fmt.Fprintf(w, "migration: %s\n", p.Migration.Name)
	}
	for _, m := range p.Migrations {
		fmt.Fprintf(w, "migration: %s (%s database)\n", m.Name, m.Database)
	}
	a.printNotes(p.Warnings, p.Diagnostics)
	if len(p.NextSteps) > 0 && !dryRun {
		fmt.Fprintln(w, "\nnext steps:")
		for _, s := range p.NextSteps {
			fmt.Fprintln(w, "  "+s)
		}
	}
}

func (a *App) printNotes(warnings []string, diags schema.Diagnostics) {
	if len(warnings) == 0 && len(diags) == 0 {
		return
	}
	fmt.Fprintln(a.Stderr, "\nwarnings:")
	for _, warning := range warnings {
		fmt.Fprintln(a.Stderr, "  "+warning)
	}
	for _, d := range diags {
		fmt.Fprintln(a.Stderr, "  "+d.String())
	}
}

func (a *App) runCheck(ctx context.Context, g *globalFlags, args []string) int {
	if ok, code := a.parse(g, "check", args, nil); !ok {
		return code
	}
	plan, err := project.BuildPlan(ctx, project.Options{Dir: g.dir, UpgradeGo: g.upgradeGo, Toolchain: a.Toolchain})
	if err != nil {
		return a.fail(g, err)
	}
	conflicts := plan.Conflicts()
	if g.json {
		a.writeJSON(map[string]any{
			"ok":          len(conflicts) == 0,
			"root":        plan.Root,
			"dialect":     plan.Dialect,
			"target":      plan.Target,
			"goUpgrade":   plan.GoUpgrade,
			"domains":     plan.Domains,
			"warnings":    plan.Warnings,
			"diagnostics": plan.Diagnostics,
			"conflicts":   conflicts,
		})
	} else {
		a.printHeader(plan)
		a.printNotes(plan.Warnings, plan.Diagnostics)
		for _, c := range conflicts {
			fmt.Fprintf(a.Stderr, "conflict: %s: %s\n", c.Path, c.Reason)
		}
		if len(conflicts) == 0 {
			fmt.Fprintln(a.Stdout, "\nok: specs, configuration and Go version are valid")
		}
	}
	if len(conflicts) > 0 {
		return ExitOwnership
	}
	return ExitOK
}

func (a *App) runMCP(ctx context.Context, g *globalFlags, args []string) int {
	if ok, code := a.parse(g, "mcp", args, nil); !ok {
		return code
	}
	if a.ServeMCP == nil {
		return a.fail(g, fmt.Errorf("the MCP server is not available in this build"))
	}
	dir := g.dir
	if dir == "" {
		dir = "."
	}
	if err := a.ServeMCP(ctx, dir, a.Stdin, a.Stdout); err != nil && ctx.Err() == nil {
		return a.fail(g, err)
	}
	return ExitOK
}

type versionInfo struct {
	Version      string                   `json:"version"`
	Minimum      string                   `json:"minimumGo"`
	Latest       string                   `json:"latestGo"`
	Capabilities []goversion.Capabilities `json:"capabilities"`
	Dialects     []string                 `json:"dialects"`
}

func (a *App) runVersion(g *globalFlags, args []string) int {
	if ok, code := a.parse(g, "version", args, nil); !ok {
		return code
	}
	info := versionInfo{
		Version:      a.Version,
		Minimum:      goversion.MinimumRelease,
		Latest:       goversion.Latest().Release,
		Capabilities: goversion.Known(),
		Dialects:     dialect.Names(),
	}
	if g.json {
		a.writeJSON(info)
		return ExitOK
	}
	w := a.Stdout
	fmt.Fprintf(w, "codebuild %s\n", info.Version)
	fmt.Fprintf(w, "supported Go versions: %s or later (versions newer than %s use the %s capability set)\n", info.Minimum, info.Latest, info.Latest)
	for _, c := range info.Capabilities {
		fmt.Fprintf(w, "  %-24s json: %s, uuid: %s\n", c.String(), c.JSONImport(), c.UUIDImport())
	}
	fmt.Fprintf(w, "dialects: %s\n", strings.Join(info.Dialects, ", "))
	return ExitOK
}
