# Changelog

All notable changes to this project are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- Authentication generated from the specs with the standard library only. The entity marker `auth=session|jwt[|apikey]` turns an entity into the account entity, with the field markers `identity`, `password` and `role` and the entity marker `register`. Sessions are opaque tokens stored as SHA-256 hashes; JWT uses HS256 access tokens with rotating refresh tokens and reuse detection; API keys are hashed and cannot manage other keys. Passwords use PBKDF2-SHA256 with 600,000 iterations and constant-time, timing-equalized verification.
- Routes are protected by default once a project has an account entity. The entity markers `public`, `roles` and `writeroles` open or restrict operations, and relation routes combine the rules of both entities.
- The field marker `owner` scopes every query of an entity to the caller, takes the owner as an explicit repository and service parameter, and rejects references to rows of other owners.
- `POST /auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout`, `GET /auth/me`, `PUT /auth/password` and `/auth/api-keys`, with `HttpOnly` `SameSite=Strict` cookies checked by `http.CrossOriginProtection`, an in-memory login rate limiter, `WWW-Authenticate` and `Retry-After` headers, and configuration through `AUTH_*` variables.
- A user-owned `AuthService` composed from generated `Accounts` primitives, scaffolded method by method like `Service`, and a `cmd/accounts` command to set roles, reset passwords and revoke credentials.
- `httpx.Error` carries response headers.
- `docs/authentication.md`, the `secure` and `secure-jwt` golden projects and their end-to-end HTTP tests; with `CODEBUILD_E2E_DOCKER=1` the end-to-end test also builds and starts the image of SQLite projects.

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

## [0.1.0] - 2026-10-18

### Added

- Spec files in `specs/<domain>.go` with `//go:build ignore`, parsed with `go/parser` and `go/types`, supporting the entity markers `table`, `softdelete`, `noroutes` and `readonly` and the field markers `pk`, `auto`, `required`, `notempty`, `unique`, `index`, `min`, `max`, `email`, `enum`, `default`, `omit`, `column`, `rel`, `ondelete`, `fk` and `via`.
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

[Unreleased]: https://github.com/WilkerSL/go-codebuild-tool/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/WilkerSL/go-codebuild-tool/releases/tag/v0.1.0
