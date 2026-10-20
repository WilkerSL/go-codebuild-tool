// Codebuild turns small Go spec files into a complete REST API, or into a workspace of
// gRPC services behind a REST gateway. See the README for the commands and flags.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/WilkerSL/go-codebuild-tool/internal/cli"
	"github.com/WilkerSL/go-codebuild-tool/internal/mcpserver"
)

var version = ""

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "devel"
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	app := &cli.App{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: cli.IsTerminal(os.Stdin),
		Version:     buildVersion(),
		ServeMCP:    mcpserver.Serve(buildVersion()),
	}
	code := app.Run(ctx, os.Args[1:])
	stop()
	os.Exit(code)
}
