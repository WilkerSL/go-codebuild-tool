# Changelog

All notable changes to this project are documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-18

### Added

- Spec files in `specs/<domain>.go` with `//go:build spec`, parsed with `go/parser` and `go/types`, supporting the entity markers `table`, `softdelete`, `noroutes` and `readonly` and the field markers `pk`, `auto`, `required`, `notempty`, `unique`, `index`, `min`, `max`, `email`, `enum`, `default`, `omit`, `column`, `rel`, `ondelete`, `fk` and `via`.
- Positioned diagnostics with stable codes and "did you mean" suggestions.
- Go version detection from `go.mod`, a capability table for Go 1.26 (`encoding/json`, `github.com/google/uuid`) and Go 1.27 (`encoding/json/v2`, standard library `uuid`), consent flow for projects below Go 1.26 and toolchain checks that honor `GOTOOLCHAIN`.
- PostgreSQL, SQL Server, MySQL and SQLite dialects with dialect-specific queries, column types, constraint error mapping that works with localized server messages, and incremental migrations derived from a schema snapshot.
- Generated projects with three layers per domain: generated handlers, user-owned services with explicit methods, and generated repositories that accept a transaction through `WithTx`. When a spec change needs a new service method, codebuild writes it in a new `service_<method>.go` file instead of editing existing services.
- `net/http` handlers with RFC 9457 problem details, validation without reflection, pagination and filters, soft delete, has-many and many-to-many routes, request IDs propagated to logs, `/livez` and `/readyz` probes, graceful shutdown with a drain delay and environment configuration.
- `database.DBTX`, `database.InTx` and an embedded migrator compatible with the golang-migrate table layout.
- A multi-stage distroless `Dockerfile`, `.dockerignore` and a GitHub Actions workflow with a database service container in every starter project, plus `AGENTS.md`.
- Generated validation, repository and transaction tests.
- File ownership rules: regenerated `*_gen.go` files, user files created once, refusal to overwrite generated files whose header was removed, stale file detection and `--prune`.
- CLI commands `build`, `check`, `plan`, `mcp` and `version` with `--json`, `--upgrade-go` and documented exit codes.
- MCP server with the `grammar` resource and the `list_domains`, `validate`, `plan` and `generate` tools.
- Examples: `examples/saas` on PostgreSQL and Go 1.26, and `example/`, a library API on SQLite and Go 1.27 with transactions, a custom endpoint, manual wiring, a seed migration and tests of business rules.
- Golden tests for every Go version and dialect, end-to-end tests against real databases, CI and GoReleaser configuration.

[Unreleased]: https://github.com/WilkerSL/go-codebuild-tool/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/WilkerSL/go-codebuild-tool/releases/tag/v0.1.0
