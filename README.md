# codebuild

codebuild turns small Go spec files into a complete REST API: domain types, request validation, `net/http` handlers, `database/sql` repositories with SQL written for your database, migrations, and the application bootstrap.

```go
//go:build spec

package specs

type User struct {
	_         struct{}  `gen:"table=users,softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Email     string    `gen:"required,unique,email,max=254"`
	Role      string    `gen:"enum=admin|operator|viewer,default=viewer"`
	OrgID     uuid.UUID `gen:"rel=belongs_to:Org"`
	CreatedAt time.Time `gen:"auto=create"`
}
```

From that file codebuild generates `GET/POST /users` and `GET/PUT/DELETE /users/{id}` (plus `GET /orgs/{orgId}/users` once `Org` declares the `has_many` side), validation with field errors, RFC 9457 problem responses, paginated and filtered listing, soft delete, a migration, and repository tests.

## Principles

- **Zero runtime dependency.** codebuild is a development tool, like `sqlc` or `stringer`. Generated code imports only the standard library, the database driver and, on Go 1.26, `github.com/google/uuid`. Delete codebuild and the project still builds.
- **Standard library first.** `net/http` routing patterns, `database/sql`, `log/slog`, `encoding/json` or `encoding/json/v2`. No reflection in generated code, no DI container, no framework.
- **Deterministic.** The same specs, Go version and dialect always produce byte-identical files. Output is sorted and gofmt-formatted, so diffs stay reviewable.
- **Version-aware.** The `go` directive of your `go.mod` decides which language and library features the generated code uses. A Go 1.26 project never receives Go 1.27 symbols; `go vet`'s `stdversion` check passes.
- **Your code stays yours.** `*_gen.go` files are regenerated; services, `main.go`, configuration and applied migrations are never overwritten.

## Install

```bash
go install github.com/WilkerSL/go-codebuild-tool/cmd/codebuild@latest
```

To pin codebuild per project, add it as a tool dependency and run it with `go tool`:

```bash
go get -tool github.com/WilkerSL/go-codebuild-tool/cmd/codebuild@latest
go tool codebuild version
```

codebuild needs Go 1.26 or later.

## Five-minute quickstart

```bash
mkdir shop && cd shop
mkdir specs
```

Create `specs/orgs.go` and `specs/users.go`:

```go
//go:build spec

package specs

import (
	"time"

	"github.com/google/uuid"
)

type Org struct {
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Users     []User    `gen:"rel=has_many:User"`
	CreatedAt time.Time `gen:"auto=create"`
}
```

```go
//go:build spec

package specs

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	_         struct{}  `gen:"softdelete"`
	ID        uuid.UUID `gen:"pk,auto"`
	Name      string    `gen:"required,notempty,max=120"`
	Email     string    `gen:"required,unique,email,max=254"`
	OrgID     uuid.UUID `gen:"rel=belongs_to:Org"`
	CreatedAt time.Time `gen:"auto=create"`
	UpdatedAt time.Time `gen:"auto=update"`
}
```

Generate the project, fetch dependencies and start PostgreSQL:

```bash
codebuild build --starter --module example.com/shop --dialect postgres
go mod tidy
docker run --rm -d --name shop-db -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=app -p 5432:5432 postgres:17
export DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable'
go run ./cmd/migrate up
go run ./cmd/api
```

Call the API:

```bash
curl -s -X POST localhost:8080/orgs -d '{"name":"Acme"}'
curl -s -X POST localhost:8080/users -d '{"name":"Ann","email":"ann@example.com","orgId":"<org id>"}'
curl -s 'localhost:8080/users?email=ann@example.com'
curl -s -X POST localhost:8080/users -d '{"name":"","email":"nope"}'
```

The last request answers `422` with a problem document listing every invalid field. Change a spec and run `codebuild build` again: generated files are refreshed, your files are left alone and a new migration describes the schema change.

## Commands

| Command | Purpose |
|---|---|
| `codebuild build --starter --module <path> [--dialect d]` | Create a project in the current directory. |
| `codebuild build` | Regenerate `*_gen.go`, create missing files, add a migration when the schema changed. |
| `codebuild check` | Validate specs, configuration and the Go version without writing. |
| `codebuild plan [--diff] [--exit-code]` | Dry run listing files to create, regenerate or skip, with a diff summary. |
| `codebuild mcp` | Start the Model Context Protocol server over stdio. |
| `codebuild version` | Print the version, supported Go versions and dialects. |

Global flags: `--json` for machine-readable output, `--upgrade-go` to authorize raising a `go` directive below 1.26 without a prompt, `--dir` to point at another project. `build` and `plan` also accept `--dialect`, `--change-dialect` and `--prune`.

Exit codes: `0` success, `1` error, `2` usage, `3` invalid spec, `4` unsupported Go version, `5` toolchain too old, `6` file ownership conflict, `7` configuration conflict, `8` pending changes (`plan --exit-code`).

## Dialects

`postgres` (default), `sqlserver`, `mysql` and `sqlite`. The repositories use plain `database/sql` with SQL written for the selected database: placeholders, `RETURNING` or `OUTPUT INSERTED`, pagination syntax, column types, timestamps, identifier quoting and constraint error detection all differ per dialect. The choice is stored in `codebuild.json`. See [docs/dialects.md](docs/dialects.md).

## Go versions

| Target `go` directive | JSON | UUID |
|---|---|---|
| 1.26 | `encoding/json` | `github.com/google/uuid` |
| 1.27 and later | `encoding/json/v2` | standard library `uuid` |

Below 1.26 codebuild stops and asks before raising the `go` directive. See [docs/go-versions.md](docs/go-versions.md).

## How codebuild compares

| | codebuild | sqlc | ent | Goa | goctl (go-zero) |
|---|---|---|---|---|---|
| Source of truth | Go spec structs | SQL queries and schema | Go schema code | Go design DSL | `.api` files and SQL |
| HTTP layer | yes, `net/http` | no | no | yes, plus gRPC | yes |
| Persistence | yes, generated SQL | yes, your SQL | yes, ORM client | no | yes |
| Migrations | generated, incremental | no (bring your own) | via Atlas | no | no |
| Runtime library in your binary | none | none | `entgo.io/ent` | `goa.design/goa/v3` | `github.com/zeromicro/go-zero` |
| OpenAPI | not yet | no | via extension | yes | yes |

Choose **sqlc** when you want to write every query yourself and keep full control of SQL; codebuild writes CRUD and relation SQL for you and is less flexible for complex reporting queries. Choose **ent** when you need graph traversals, eager loading, hooks and privacy policies and accept a runtime library. Choose **Goa** when the API contract, OpenAPI and gRPC matter more than persistence. Choose **goctl** when you are adopting the go-zero framework. Choose **codebuild** when you want a conventional CRUD service in plain standard-library Go that you can keep evolving by hand.

## Working with AI agents

`codebuild mcp` exposes the spec grammar and the `list_domains`, `validate`, `plan` and `generate` tools. Every starter project contains an `AGENTS.md` describing the safe workflow: read the grammar, write a spec, validate, plan, get the user's approval, generate. See [docs/mcp.md](docs/mcp.md).

## Documentation

- [Spec reference](docs/spec-reference.md): every marker with an example and its common error.
- [Go versions](docs/go-versions.md): capability matrix and upgrade behavior.
- [Dialects](docs/dialects.md): SQL differences side by side.
- [File ownership](docs/file-ownership.md): what is regenerated and what is never touched.
- [MCP](docs/mcp.md): client configuration and recommended flow.
- [Architecture](docs/architecture.md): internals for contributors.
- [Example project](examples/saas): organizations, users, tags and projects.

## License

MIT. See [LICENSE](LICENSE).
