# Architecture

This document is for contributors. codebuild is a pipeline of small packages under `internal/`, each with one job and tests next to it.

```
specs/*.go ──► spec ──► schema.Model ──┬──► schema.Build ──► schema.Database ──► schema.Diff ──► dialect.Migration ──► migrations/*.sql
                                       │
go.mod ──► goversion ──► Capabilities ─┼──► gen (templates) ──► Go files
                                       │
codebuild.json ──► project ────────────┴──► plan ──► ownership rules ──► apply ──► disk
                                                   ▲
                               cli ────────────────┤
                               mcpserver ──────────┘
```

## Packages

| Package | Responsibility |
|---|---|
| `internal/naming` | Word splitting, snake/kebab/camel/Pascal case, pluralization and "did you mean" suggestions (optimal string alignment distance). |
| `internal/spec` | Parses spec files with `go/parser`, type-checks them with `go/types` through a synthetic importer that only knows `time.Time` and both `uuid.UUID` packages, resolves aliases with `types.Unalias`, interprets `gen` markers, validates combinations and relations, and renders the grammar reference. |
| `internal/schema` | The model (`Entity`, `Field`, `Relation`), positioned diagnostics, the logical database (`Table`, `Column`, `Index`, `ForeignKey`, `Check`), the snapshot format and the diff engine that orders changes and computes inverse (down) changes. |
| `internal/goversion` | Reads `go.mod`, resolves the target version into `Capabilities`, detects the toolchain through `go env` and evaluates `GOTOOLCHAIN`. |
| `internal/dialect` | The `Dialect` interface and its four implementations: column types, quoting, placeholders, query construction, DDL rendering, SQLite table rebuilds and dialect-specific validation. |
| `internal/gen` | Embedded `text/template` files, the view model that precomputes code fragments, import pruning through the AST and `go/format`. |
| `internal/project` | Locates the project, loads `codebuild.json`, resolves the Go version policy, plans migrations from the snapshot, classifies every file by ownership, edits `go.mod` with `modfile` and applies the plan atomically. |
| `internal/textdiff` | Myers line diff for plan summaries and `--diff`. |
| `internal/cli` | Flag parsing with the standard `flag` package, exit codes, prompts, text and JSON output. |
| `internal/mcpserver` | MCP resource and tools on top of `project`. |

## Determinism

- Sources, entities, tables, indexes, foreign keys, checks, files and diagnostics are sorted before use.
- Maps are never iterated to produce output.
- Go output goes through `FormatGoModule`, which removes unused imports, groups standard library, third-party and module imports, and runs `go/format`.
- SQL is produced by string builders with fixed layout; constraint names longer than 63 characters are shortened with an FNV hash.
- The capability table, driver versions and templates are embedded; nothing is fetched at generation time.
- Files are written with LF line endings and compared after normalizing CRLF.

## Generated code rules

- No reflection; validation, scanning and argument binding are spelled out per field.
- Templates consult `.Caps` for version differences and dialect view fields (`SelectAfter`, `ScanTime`, `UUIDSwap`, ...) for database differences; they never compare version numbers.
- Queries are named constants in `queries_gen.go`. Optional filters use the `(param IS NULL OR column = param)` form so every query is constant.
- Relation routes live in the package of the entity they return, so domain packages never import each other.
- Services are user code. `service.go` is rendered once with explicit methods; `internal/project/service.go` parses the non-generated files of a package to find the methods `Service` already has and renders only the missing ones, each into a new `service_<method>.go`.
- Repositories depend on `database.DBTX`, the interface shared by `*sql.DB`, `*sql.Tx` and `*sql.Conn`, so the same code runs inside or outside a transaction.

## Adding a dialect

1. Implement `Dialect` and the unexported `ddl` methods in `internal/dialect/<name>.go` and register it in `registry`.
2. Add `<name>.constraint`, `<name>.connect` and `<name>.migrate` templates in `internal/gen/templates/dialect/<name>.tmpl` and the error classifier imports in `errorImports`.
3. Set the dialect view flags in `newDialectView`.
4. Run `go test ./internal/dialect -update` and `go test ./internal/project -update`, review the golden SQL and projects, and add the dialect to the end-to-end test and the CI services.

## Adding a Go version

See [go-versions.md](go-versions.md#how-codebuild-evolves-with-each-go-release).

## Tests

| Command | What it covers |
|---|---|
| `go test -short ./...` | Unit tests, golden SQL, golden projects, CLI and MCP. |
| `go test ./internal/dialect -update` | Rewrites `testdata/golden/sql`. |
| `go test ./internal/project -update` | Rewrites `testdata/golden/go1.26` and `testdata/golden/go1.27`. |
| `go test ./internal/spec -update` | Rewrites `docs/spec-reference.md` from the marker registry. |
| `go test -run TestEndToEnd ./cmd/codebuild` | Builds the binary, generates a project per dialect, runs `go mod tidy`, `go build`, `go vet`, `gofmt -l`, `go test`, an empty second plan, migrations up and down and an HTTP smoke test. |

The end-to-end test reads `CODEBUILD_TEST_POSTGRES_DSN`, `CODEBUILD_TEST_SQLSERVER_DSN` and `CODEBUILD_TEST_MYSQL_DSN`; without them it still builds and unit-tests the generated projects, and SQLite always runs against a temporary file. `CODEBUILD_E2E_DIALECTS` restricts the dialects, and `CODEBUILD_TEST_<DIALECT>_IMPORT` adds a blank import to the generated database package (for example `github.com/microsoft/go-mssqldb/sharedmemory` to reach a local SQL Server Express instance that only listens on shared memory).
