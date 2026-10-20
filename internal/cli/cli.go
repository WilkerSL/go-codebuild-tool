// Package cli is the command line front end. It parses flags, asks before risky changes
// and prints plans as text or JSON; the actual work happens in package project.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/WilkerSL/go-codebuild-tool/internal/goversion"
	"github.com/WilkerSL/go-codebuild-tool/internal/project"
)

const (
	ExitOK            = 0
	ExitError         = 1
	ExitUsage         = 2
	ExitInvalidSpec   = 3
	ExitUnsupportedGo = 4
	ExitToolchain     = 5
	ExitOwnership     = 6
	ExitConfig        = 7
	ExitPlanChanges   = 8
)

type App struct {
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool
	Version     string
	Toolchain   func(context.Context) (goversion.Toolchain, error)
	ServeMCP    func(ctx context.Context, dir string, in io.Reader, out io.Writer) error
}

type globalFlags struct {
	json      bool
	upgradeGo bool
	dir       string
}

func (g *globalFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&g.json, "json", g.json, "print machine-readable JSON")
	fs.BoolVar(&g.upgradeGo, "upgrade-go", g.upgradeGo, "authorize raising the go directive to the latest supported version when it is below 1.26")
	fs.StringVar(&g.dir, "dir", g.dir, "project directory (default: current directory)")
}

func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

const usage = `codebuild generates REST APIs from Go spec files.

Usage:
  codebuild <command> [flags]

Commands:
  build     generate the project (create it with --starter)
  check     validate specs, configuration and the Go version without writing
  plan      show what build would create, regenerate or skip
  mcp       run the Model Context Protocol server over stdio
  version   print the codebuild version and supported Go versions

Global flags:
  --json         print machine-readable JSON
  --upgrade-go   authorize raising the go directive when it is below 1.26
  --dir path     project directory (default: current directory)

Build and plan flags:
  --starter              create a new project in the directory
  --module path          module path for --starter
  --dialect name         postgres (default), sqlserver, mysql or sqlite
  --change-dialect       confirm switching the dialect of an existing project
  --prune                delete generated files that the specs no longer produce

Plan flags:
  --diff        print a unified diff of every change
  --exit-code   exit with 8 when the plan contains changes

Exit codes:
  0 success, 1 error, 2 usage, 3 invalid spec, 4 unsupported Go version,
  5 toolchain too old, 6 file conflict (ownership, outdated plan or a concurrent build), 7 configuration conflict,
  8 plan has changes (only with plan --exit-code)
`

func (a *App) Run(ctx context.Context, args []string) int {
	var g globalFlags
	global := flag.NewFlagSet("codebuild", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	g.register(global)
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(a.Stdout, usage)
			return ExitOK
		}
		return a.usageError(g, err)
	}
	rest := global.Args()
	if len(rest) == 0 {
		fmt.Fprint(a.Stderr, usage)
		return ExitUsage
	}
	command, cmdArgs := rest[0], rest[1:]
	switch command {
	case "build", "plan":
		return a.runBuild(ctx, &g, command, cmdArgs)
	case "check":
		return a.runCheck(ctx, &g, cmdArgs)
	case "mcp":
		return a.runMCP(ctx, &g, cmdArgs)
	case "version":
		return a.runVersion(&g, cmdArgs)
	case "help":
		fmt.Fprint(a.Stdout, usage)
		return ExitOK
	}
	return a.usageError(g, fmt.Errorf("unknown command %q", command))
}

func (a *App) usageError(g globalFlags, err error) int {
	if g.json {
		a.writeJSON(map[string]any{"error": map[string]string{"code": "usage", "message": err.Error()}})
	} else {
		fmt.Fprintf(a.Stderr, "codebuild: %v\nrun codebuild help for usage\n", err)
	}
	return ExitUsage
}

func (a *App) parse(g *globalFlags, name string, args []string, extra func(*flag.FlagSet)) (bool, int) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	g.register(fs)
	if extra != nil {
		extra(fs)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(a.Stdout, usage)
			return false, ExitOK
		}
		return false, a.usageError(*g, err)
	}
	if fs.NArg() > 0 {
		return false, a.usageError(*g, fmt.Errorf("unexpected argument %q", fs.Arg(0)))
	}
	return true, ExitOK
}

func (a *App) writeJSON(v any) {
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func exitCode(err error) int {
	var perr *project.Error
	if !errors.As(err, &perr) {
		return ExitError
	}
	switch perr.Kind {
	case project.KindUsage:
		return ExitUsage
	case project.KindInvalidSpec:
		return ExitInvalidSpec
	case project.KindUnsupportedGo:
		return ExitUnsupportedGo
	case project.KindToolchain:
		return ExitToolchain
	case project.KindOwnership:
		return ExitOwnership
	case project.KindConfig:
		return ExitConfig
	}
	return ExitError
}

func (a *App) fail(g *globalFlags, err error) int {
	code := exitCode(err)
	var perr *project.Error
	if !errors.As(err, &perr) {
		perr = &project.Error{Code: "error", Message: err.Error()}
	}
	if g.json {
		a.writeJSON(map[string]any{"error": perr})
		return code
	}
	fmt.Fprintf(a.Stderr, "codebuild: %s\n", perr.Message)
	for _, d := range perr.Diagnostics {
		fmt.Fprintln(a.Stderr, d.String())
	}
	return code
}

func (a *App) confirm(g *globalFlags) func(string) (bool, error) {
	if !a.Interactive || g.json {
		return nil
	}
	reader := bufio.NewReader(a.Stdin)
	return func(question string) (bool, error) {
		fmt.Fprintf(a.Stderr, "%s [y/N]: ", question)
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		return answer == "y" || answer == "yes", nil
	}
}
