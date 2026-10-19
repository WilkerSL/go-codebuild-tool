# MCP server

`codebuild mcp` runs a Model Context Protocol server over stdio, built with the official `github.com/modelcontextprotocol/go-sdk`. It lets AI agents write specs and generate code while the user keeps control of every change.

## Configuration

The server works on the directory given with `--dir`, or the directory it was started in. Tools also accept a `dir` argument, absolute or relative, which must stay inside that directory after resolving symbolic links; the project found from it (the nearest `go.mod`) must be inside it too, so a server started in a subdirectory never writes to a module above it. Violations return the error code `outside_root`.

Claude Code:

```bash
claude mcp add codebuild -- codebuild mcp --dir /path/to/project
```

Clients that use a JSON configuration (Claude Desktop, Cursor, VS Code and others):

```json
{
  "mcpServers": {
    "codebuild": {
      "command": "codebuild",
      "args": ["mcp", "--dir", "/path/to/project"]
    }
  }
}
```

When codebuild is a tool dependency of the project, use `"command": "go"` and `"args": ["tool", "codebuild", "mcp"]` instead.

## Resource

| URI | Content |
|---|---|
| `codebuild://grammar` | The full spec reference: file layout, every marker with an example and its common error, request semantics, generated routes and diagnostic codes. It is the same text as [spec-reference.md](spec-reference.md). |

## Tools

| Tool | Writes | Input | Output |
|---|---|---|---|
| `list_domains` | no | `dir` | domains (with their service in a workspace), services and their databases, dialect, Go version from `go.mod`, status and capabilities |
| `validate` | no | `dir`, `file`, `content` | `valid` and positioned diagnostics with codes and suggestions |
| `plan` | no | `dir`, `upgradeGo` | the plan: files with actions, the migration (`migrations`, one per database, in a workspace), warnings, next steps |
| `generate` | yes | `dir`, `upgradeGo`, `prune` | the plan and the written and deleted files |

`validate` checks the given content together with the other specs on disk, so relations to existing entities resolve, without saving the file.

Failures are returned as tool errors (`isError: true`) whose structured content contains an `error` object with a stable `code`, a `message`, optional `details` and `diagnostics`. Codes include `invalid_spec`, `unsupported_go_version`, `toolchain_too_old`, `ownership_conflict`, `dialect_change`, `snapshot_missing` and `layout_change`.

## Go version consent

For projects below Go 1.26, `plan` and `generate` return `unsupported_go_version` with `details.detected`, `details.minimum` and `details.upgradeTo`. The agent must explain that the `go` directive in `go.mod` would be raised and send `upgradeGo: true` only after the user explicitly agrees.

## Recommended flow

1. Read `codebuild://grammar`.
2. Call `list_domains` to learn the existing entities, dialect and Go version.
3. Write or edit `specs/<domain>.go`.
4. Call `validate` with the content and fix every error.
5. Call `plan` and show the user the files that will change, the migration and the warnings.
6. Wait for approval, then call `generate`.
7. Ask the user to run `go mod tidy`, `go build ./...`, `go test ./...` and `go run ./cmd/migrate up`.

Agents must never edit `*_gen.go` files; business rules go in `internal/<domain>/service.go`. Starter projects include an `AGENTS.md` with the same rules.
