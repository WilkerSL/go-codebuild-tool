# Changelog

All notable changes to this project are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Security

- A password change, `accounts set-password` and `accounts revoke` now run in one transaction that holds the account row, and API key creation and logins take the same lock: a key requested with a session that the change revokes is refused instead of surviving it, a login whose password was verified just before a change no longer receives a session or refresh token, and a failure no longer leaves the new password in place with the old credentials still valid. A rehash on login only replaces the hash it verified, so it can no longer restore a password changed meanwhile.
- With the default `AUTH_LIMIT_STORE=memory` the per-client limiter has its own store again, so a flood of new identities cannot evict the per-client counters that throttle it.
- `AUTH_LIMIT_STORE=database` cleans up in the background and in batches of 1,000 rows, so clients that hang up cannot skip the cleanup and it never locks the table for long; under a flood it drops single-attempt counters instead of the counters that block a client. In workspaces the gateway reports limiter errors without gRPC details, rejects an `AUTH_LOGIN_WINDOW` above 24 hours at startup instead of answering 400 to every login, and the account service checks the window before converting it.

## [0.4.0] - 2026-10-23

### Added

- `AUTH_LIMIT_STORE=database` keeps the login, registration and password change rate limiter in a new `auth_rate_limits` table, so every replica shares the counters and they survive restarts. Keys are hashed, IPv6 clients are counted per /64, expired rows are cleaned up every minute and the table is bounded under floods. Existing projects get the table through a new migration. In workspaces the gateway asks the account service through the new `AuthService.HitRateLimit` and `ResetRateLimit` calls. The default stays `memory`.
- Kubernetes manifests: every build writes a kustomize base in `deploy/kubernetes/base` for the monolith or for every service and the gateway of a workspace (Deployments with startup, liveness and readiness probes on `/livez` and `/readyz`, resource requests and limits, non-root read-only containers without service account tokens, Services, PodDisruptionBudgets, generated ConfigMaps, migration Jobs, and Secrets referenced by name only), plus a user-owned overlay in `deploy/kubernetes/overlays/production` for images, namespace and patches. The base files carry the generated header, so `codebuild plan` reports edits as conflicts and removed services as stale. A workflow deploys a monolith and a workspace to a kind cluster.
- gRPC clients in workspaces retry `Get`, `List…`, `Exists`, `Authenticate` and `Me` when a backend is unavailable, with jittered exponential backoff inside the call deadline, and open a circuit breaker per backend after repeated failures. `GRPC_CALL_TIMEOUT`, `GRPC_RETRY_ATTEMPTS`, `GRPC_RETRY_BACKOFF`, `GRPC_BREAKER_FAILURES` and `GRPC_BREAKER_COOLDOWN` configure them, and `/metrics` exports retries and breaker state per backend.
- Services and the gateway reload `TLS_CA_FILE`, `TLS_CERT_FILE` and `TLS_KEY_FILE` every 30 seconds, so certificates from your own PKI (for example cert-manager Secrets) rotate without a restart. A broken file keeps the last working certificate.
- A load test (`CODEBUILD_E2E_LOAD=1`, and a manual `Load test` workflow) measures throughput, latency percentiles and errors of the generated API on PostgreSQL; `docs/production.md` records the results.
- `docs/production.md`: TLS and proxies, secrets and key rotation, backups, migrations in deployments, pool sizing, timeouts, CORS, health checks, metrics, traces and alerts, rate limits, Kubernetes and the remaining limits.
- `AUTH_JWT_CONFIRM=true` makes a monolith check every JWT access token against the database on every route, so logout and revocation take effect immediately everywhere. The default stays `false`.
- OpenAPI 3.1: every build writes `openapi.json` at the project root, built from the same views as the handlers (routes, request and response bodies with their validation rules, filters, cursor and offset pagination, problem responses, security and roles), and the API, or the gateway in a workspace, serves it at `GET /openapi.json`. A test checks that every served route is documented and the reverse. The file opens with an `x-codebuild` member that marks it as generated.
- Cursor pagination: every list response carries `nextCursor`, and `?cursor=` returns the rows after it without a `COUNT(*)`, using the list order as the key, so it stays fast on large tables and correct while rows are added. Offset pagination works as before. In workspaces the gateway forwards the cursor and the contracts gain `cursor` and `next_cursor` fields; existing field numbers do not change.
- Database spans: with tracing enabled, every query of the generated repositories and account code is a child span of its request, following the OpenTelemetry database conventions, without parameter values. `database.Traced` wraps your own `*sql.DB` or `*sql.Tx` the same way.

### Changed

- Microservice workspaces are no longer experimental; `docs/microservices.md` addresses or justifies each of their limits.
- In workspaces `grpcx.Dial` takes a `CallPolicy` (`grpcx.LoadCallPolicy` reads it from the environment) and `grpcx.TLSConfig` must come from `grpcx.LoadTLSConfig`. The generated `internal/app` code already does both; `main.go` files written by earlier versions keep compiling.
- Lists are backed by indexes in their order: `lx_<table>` on the creation time and the primary key, and `lx_<table>_<column>` on each reference followed by them, so first pages, cursor pages and the lists of one parent or owner read rows in order instead of sorting the table. Cursor queries add a range bound the planner can use. Existing projects get the indexes through a new migration.
- `auth.Limiter` takes a scope and a `LimitStore` and its `Allow` and `Reset` take a context; `NewAuthHandler` takes the store. Both live in generated files, so this only matters to code of yours that used the limiter directly.
- `Page.Total` is an `*int64`, nil on cursor pages, and `Page` has a `NextCursor *string`; `ListParams` has a `Cursor *Cursor`. Code of yours that reads `Total` needs a dereference. In workspace contracts `total` is now `optional`.
- Generated SQLite projects use `modernc.org/sqlite` v1.61.0.

### Fixed

- `naming.Snake` and the names derived from it no longer change when applied twice to a name with a capital letter that has no lowercase form, such as `ϒ`. Found by the weekly fuzz run.

### Security

- Changing the password through `PUT /auth/password` also deletes every API key of the account, like `accounts set-password` and `accounts revoke` already did.
- JWT: logout, `accounts revoke`, a password change or reuse detection that ran while a refresh of the same family was committing could leave the refresh token issued by that refresh alive on PostgreSQL. Revocation now repeats its delete until no row is left, and a generated test reproduces the race.

## [0.3.0] - 2026-10-22

### Added

- `SECURITY.md` with the supported versions and private vulnerability reporting, issue forms and a pull request template.
- A weekly workflow that fails when a newer version exists of a module written into generated projects (database drivers, gRPC, protobuf, `github.com/google/uuid`), and Dependabot updates for the modules of the tool and the GitHub Actions.
- Release archives come with an SPDX SBOM each, `checksums.txt` is signed keyless with cosign (Sigstore bundle `checksums.txt.sigstore.json`), and every archive gets a GitHub build provenance attestation. `SECURITY.md` explains how to verify them; CI builds a snapshot release with SBOMs on every push.
- Fuzz tests for the spec parser, struct tags, schema snapshots, the contract registry, `codebuild.json`, `go.mod` and `go.work`, naming and the text diff. Generated projects fuzz password hash verification, JWT parsing and, in workspaces, internal token verification. A weekly workflow runs each fuzz target of the tool for three minutes.
- Optional observability in generated projects, with the standard library only: Prometheus metrics on `METRICS_ADDR` (HTTP requests by route and status, latency histograms, gRPC calls, database pool and Go runtime), OpenTelemetry traces exported over OTLP/HTTP JSON when `OTEL_EXPORTER_OTLP_ENDPOINT` is set, W3C `traceparent` propagation through HTTP and gRPC, and `trace_id` and `span_id` in log lines. See `docs/observability.md`.
- Configurable CORS through `CORS_ALLOWED_ORIGINS`, `CORS_ALLOW_CREDENTIALS` and `CORS_MAX_AGE`, answered before routing and authentication.
- `codebuild plan` warns when a user-owned `main.go` does not start telemetry yet, since codebuild never rewrites it.

### Changed

- `codebuild mcp` is built with `github.com/modelcontextprotocol/go-sdk` v1.8.0.
- Microservice workspaces are documented as experimental: their layout and generated code may change between minor releases.
- Generated endpoints require `Content-Type: application/json` (or another `+json` type) on request bodies and answer 415 otherwise. Clients that post JSON without that header, such as `curl -d` or `fetch` with a string body, must set it.
- Linking and unlinking a many-to-many pair needs read and update access to both entities from either route, so `writeroles=` on one side now also restricts the link routes that start from the other side.
- `auth.NewVerifier` takes any number of public keys and `auth.LoadPublicKeys` reads every key of a PEM file; new service `main.go` files use them so the internal token key can rotate without downtime. Existing `main.go` files keep compiling with `LoadPublicKey`.

### Security

These fixes come from an internal review of the generated authentication code; it was not an independent audit.

- JWT: routes restricted by `roles=` or `writeroles=`, and API key creation, check the access token against the database, so a token whose session was revoked, whose account was deleted or whose role changed can no longer use them or mint a lasting API key. In workspaces the account service checks every JWT this way.
- JWT: a refresh token presented again within `AUTH_REFRESH_REUSE_GRACE` now receives the successor it already produced instead of starting a second token chain, and concurrent rotations of one token can no longer both create successors, so reuse detection catches a stolen token.
- The login rate limiter hashes its keys and keeps at most 100,000 of them, and over-long identities are rejected before it, so distinct identities cannot exhaust memory. IPv6 clients are limited per /64 network.
- `PUT /auth/password` is rate limited per client and per session, so it cannot be used to guess the current password or to burn CPU on password hashing.
- `AUTH_TRUST_FORWARDED_FOR` reads the last `X-Forwarded-For` header line, so a client cannot choose its address behind proxies that add a separate line.
- Requiring JSON bodies blocks login CSRF through `text/plain` HTML forms.
- In workspaces the account service accepts `AuthService` calls and account writes only from the gateway's client certificate (`TLS_GATEWAY_NAME`), so a compromised service cannot replay forwarded user tokens to create API keys or accounts.
- `devcerts -force` resets the permissions of the key files it rewrites.

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

[Unreleased]: https://github.com/WilkerSL/go-codebuild-tool/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/WilkerSL/go-codebuild-tool/releases/tag/v0.4.0
[0.3.0]: https://github.com/WilkerSL/go-codebuild-tool/releases/tag/v0.3.0
[0.2.0]: https://github.com/WilkerSL/go-codebuild-tool/releases/tag/v0.2.0
[0.1.0]: https://github.com/WilkerSL/go-codebuild-tool/tree/8f14877
