# Changelog

All notable changes to this project are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- `SECURITY.md` with the supported versions and private vulnerability reporting, issue forms and a pull request template.
- A weekly workflow that fails when a newer version exists of a module written into generated projects (database drivers, gRPC, protobuf, `github.com/google/uuid`), and Dependabot updates for the modules of the tool and the GitHub Actions.

### Changed

- Microservice workspaces are documented as experimental: their layout and generated code may change between minor releases.

## [0.2.0] - 2026-10-19

### Added

- Authentication generated from the specs with the standard library only. The entity marker `auth=session|jwt[|apikey]` turns an entity into the account entity, with the field markers `identity`, `password` and `role` and the entity marker `register`. Sessions are opaque tokens stored as SHA-256 hashes; JWT uses HS256 access tokens with rotating refresh tokens and reuse detection; API keys are hashed and cannot manage other keys. Passwords use PBKDF2-SHA256 with 600,000 iterations and constant-time, timing-equalized verification.
- Routes are protected by default once a project has an account entity. The entity markers `public`, `roles` and `writeroles` open or restrict operations, and relation routes combine the rules of both entities.
- The field marker `owner` scopes every query of an entity to the caller, takes the owner as an explicit repository and service parameter, and rejects references to rows of other owners.
- `POST /auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `GET /auth/me`, `PUT /auth/password` and `/auth/api-keys`, with `HttpOnly` `SameSite=Strict` cookies checked by `http.CrossOriginProtection`, an in-memory login rate limiter, `WWW-Authenticate` and `Retry-After` headers, and configuration through `AUTH_*` variables.
- A user-owned `AuthService` composed from generated `Accounts` primitives, scaffolded method by method like `Service`, and a `cmd/accounts` command to set roles, reset passwords and revoke credentials.
- `httpx.Error` carries response headers.
- `docs/authentication.md`, the `secure` and `secure-jwt` golden projects and their end-to-end HTTP tests; with `CODEBUILD_E2E_DOCKER=1` the end-to-end test also builds and starts the image of SQLite projects.
- Microservice workspaces. A spec may declare a `<Name>Service` struct whose fields are entities and whose `database=own|shared` marker chooses the database. The project becomes a Go workspace with one module per service exposing gRPC, a `contracts` module, a `databases/shared` module for the shared database and a `gateway` module that is the only REST API, with the same routes, bodies and problem details as a monolith.
- codebuild writes the `.proto` files, the protobuf Go code and the gRPC stubs itself, without `protoc`. `contracts/codebuild.contracts.json` keeps field numbers stable and reserves the numbers of removed fields.
- Services only accept mutual TLS and an Ed25519-signed internal token, valid for one minute, that the gateway mints after authenticating the caller through `AuthService.Authenticate` and that services forward to each other. `gateway/cmd/devcerts` creates a development CA, certificates and the token key pair.
- References to an entity in another database have no foreign key and are checked through the gRPC `Exists` call of the owning service (422 when missing); services sharing a database keep their foreign keys. Each database has its own migrations, snapshot and migrate command.
- Workspaces get a generated `docker-compose.yml` (certificates, databases, migrations, services and the gateway with health checks), a Dockerfile per module, a GitHub Actions workflow per module with path filters, a `workspace.yml` workflow and a workspace `AGENTS.md`. `codebuild plan` and `codebuild build` find the workspace root from any module, maintain `go.work` and the `go.mod` of every module, and refuse to convert a monolith into a workspace or back.
- `docs/microservices.md`, the `shop` specs, golden workspaces for every dialect and an end-to-end test that builds every module, runs the services and the gateway and exercises them over HTTP; with `CODEBUILD_E2E_DOCKER=1` it also runs the Compose stack.

### Changed

- `app.RegisterRoutes` returns an error and new `main.go` files check it. `codebuild plan` warns when an authenticated project still ignores it.
- The manual wiring hint uses `NewService(db)`.
- Golden projects live in `testdata/golden/<project>/go1.NN/<dialect>`.
- Spec files start with `//go:build ignore`. `go mod tidy` reads every build tag except `ignore`, so `//go:build spec` made it resolve the imports of the specs: it added `github.com/google/uuid` back on Go 1.27 and failed on the `uuid` import on Go 1.26. Other constraints are now reported with code `build_tag`.
- The project root is the directory of the nearest `go.mod`; a `codebuild.json` in a parent directory no longer captures builds in a monorepo.
- Exit code 6 also covers an outdated plan and a concurrent build.

### Fixed

- Migrations that share a version number, which made one of them disappear silently, now fail both the build and the migrator. The snapshot records the migrations it incorporates, so a migration merged from another branch with a number below the newest one fails the build, and the migrator keeps `schema_migrations_history` next to `schema_migrations` to refuse older migrations that never ran instead of skipping them forever.
- Builds write every file to a temporary file before renaming, write the snapshot last, refuse to apply a plan when files changed since it was computed, and hold `.codebuild.lock` so the CLI and the MCP server never write at the same time. A migration left without its snapshot by an interrupted build is recognized and reused instead of being generated twice.
- The MCP tools only accept directories inside the directory the server was started in (after resolving symbolic links), and they refuse projects whose root lies above it; `generate` is annotated as destructive.
- Non-nullable `[]byte` fields that the client omits are stored as empty values instead of NULL, and not-null violations become HTTP 422 with the field name in every dialect.
- MySQL string columns without `max=` use an expression default (`DEFAULT ('x')`), which `LONGTEXT` requires.
- Soft delete honors the foreign keys that point at the entity: `restrict` answers 409 while live rows reference it, `setnull` clears the references, `cascade` hides the referencing rows and their cascading children from every query, and rows cannot reference deleted ones.
- `PUT` requires every non-pointer member instead of resetting omitted members to their defaults, so a `PUT` without `role` no longer demotes an admin.
- SQL Server inserts and updates use `OUTPUT ... INTO` a table variable, so tables with triggers keep working.
- JWT refresh runs in a transaction, and a refresh token presented again within `AUTH_REFRESH_REUSE_GRACE` (30 seconds) issues new tokens instead of revoking the session, so two tabs refreshing at once stay logged in.
- `go.work` (or `GOWORK`) counts for the required toolchain and is checked against the module, and the upgrade for projects below Go 1.26 targets the installed toolchain when it is supported.

## [0.1.0] - 2026-10-18

### Added

- Spec files in `specs/<domain>.go` with `//go:build spec`, parsed with `go/parser` and `go/types`, supporting the entity markers `table`, `softdelete`, `noroutes` and `readonly` and the field markers `pk`, `auto`, `required`, `notempty`, `unique`, `index`, `min`, `max`, `email`, `enum`, `default`, `omit`, `column`, `rel`, `ondelete`, `fk` and `via`.
- Positioned diagnostics with stable codes and "did you mean" suggestions.
- Go version detection from `go.mod`, a capability table for Go 1.26 (`encoding/json`, `github.com/google/uuid`) and Go 1.27 (`encoding/json/v2`, standard library `uuid`), consent flow for projects below Go 1.26 and toolchain checks that honor `GOTOOLCHAIN`.
- PostgreSQL, SQL Server, MySQL and SQLite dialects with dialect-specific queries, column types, constraint error mapping that works with localized server messages, and incremental migrations derived from a schema snapshot.
- Generated projects with three layers per domain: generated handlers, user-owned services with explicit methods, and generated repositories that accept a transaction through `WithTx`. When a spec change needs a new service method, codebuild writes it in a new `service_<method>.go` file instead of editing existing services.
- `net/http` handlers with RFC 9457 problem details, validation without reflection, pagination and filters, soft delete, has-many and many-to-many routes, request IDs propagated to logs, `/livez` and `/readyz` probes, graceful shutdown with a drain delay and environment configuration.
- `database.DBTX`, `database.InTx` and an embedded migrator compatible with the golang-migrate table layout.
- A multi-stage distroless `Dockerfile`, `.dockerignore`, a GitHub Actions workflow with a database service container and a `.gitattributes` that keeps LF line endings in every starter project, plus `AGENTS.md`.
- Generated validation, repository and transaction tests.
- File ownership rules: regenerated `*_gen.go` files, user files created once, refusal to overwrite generated files whose header was removed, stale file detection and `--prune`.
- CLI commands `build`, `check`, `plan`, `mcp` and `version` with `--json`, `--upgrade-go` and documented exit codes.
- MCP server with the `grammar` resource and the `list_domains`, `validate`, `plan` and `generate` tools.
- Golden tests for every Go version and dialect, end-to-end tests against real databases, CI and GoReleaser configuration.

[Unreleased]: https://github.com/WilkerSL/go-codebuild-tool/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/WilkerSL/go-codebuild-tool/releases/tag/v0.2.0
[0.1.0]: https://github.com/WilkerSL/go-codebuild-tool/tree/8f14877
