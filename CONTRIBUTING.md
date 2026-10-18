# Contributing

Thanks for helping improve codebuild. Read [docs/architecture.md](docs/architecture.md) for an overview of the packages.

## Ground rules

- The tool supports projects on Go 1.26 or later and its own `go.mod` declares `go 1.26.0`: do not use APIs that only exist in newer releases. CI runs every test with Go 1.26 and 1.27.
- Everything is in English: packages, identifiers, flags, messages, file names and templates.
- Output must stay deterministic. Sort before producing output and never iterate maps to generate files.
- Generated code uses the standard library first, no reflection, no dependency injection containers and no interfaces it does not need.
- Dependencies of the tool are limited to `github.com/modelcontextprotocol/go-sdk` and `golang.org/x/mod`. Tests use only the standard library.

## Development

```bash
go build ./...
go vet ./...
go test -short ./...
golangci-lint run
```

Golden files are part of the test suite. After an intended change to generated output, regenerate and review them:

```bash
go test ./internal/dialect -update
go test ./internal/project -update
go test ./internal/spec -update
git diff testdata docs/spec-reference.md
```

## End-to-end tests

The end-to-end test downloads modules and builds generated projects, so it is skipped with `-short`. Start the databases you want to test against:

```bash
docker run --rm -d --name cb-postgres -e POSTGRES_USER=app -e POSTGRES_PASSWORD=app -e POSTGRES_DB=app -p 5432:5432 postgres:17
docker run --rm -d --name cb-sqlserver -e ACCEPT_EULA=Y -e MSSQL_SA_PASSWORD=Codebuild_Passw0rd -p 1433:1433 mcr.microsoft.com/mssql/server:2022-latest
docker run --rm -d --name cb-mysql -e MYSQL_ROOT_PASSWORD=root -e MYSQL_DATABASE=app -e MYSQL_USER=app -e MYSQL_PASSWORD=app -p 3306:3306 mysql:8.4
docker exec cb-sqlserver /opt/mssql-tools18/bin/sqlcmd -S localhost -U sa -P Codebuild_Passw0rd -C -Q "CREATE DATABASE codebuild"
```

```bash
export CODEBUILD_TEST_POSTGRES_DSN='postgres://app:app@localhost:5432/app?sslmode=disable'
export CODEBUILD_TEST_SQLSERVER_DSN='sqlserver://sa:Codebuild_Passw0rd@localhost:1433?database=codebuild&encrypt=disable'
export CODEBUILD_TEST_MYSQL_DSN='app:app@tcp(localhost:3306)/app'
go test -count=1 -run TestEndToEnd -v ./cmd/codebuild
```

Use databases dedicated to tests: the run creates and drops tables. To test against a local SQL Server Express instance on Windows that only accepts shared memory connections, also set `CODEBUILD_TEST_SQLSERVER_IMPORT=github.com/microsoft/go-mssqldb/sharedmemory` and use a DSN such as `sqlserver://localhost/SQLEXPRESS?database=codebuild&protocol=lpc&encrypt=disable`.

To check the Go 1.26 path with a newer toolchain installed:

```bash
GOTOOLCHAIN=go1.26.0 go test -short ./...
GOTOOLCHAIN=go1.26.0 CODEBUILD_E2E_DIALECTS=sqlite go test -run TestEndToEnd ./cmd/codebuild
```

## Pull requests

- One topic per pull request, with tests.
- Describe user-visible changes in `CHANGELOG.md` under "Unreleased".
- Commit messages use the imperative mood ("Add MySQL rebuild of check constraints").

## Releases

Maintainers tag `vX.Y.Z` on `main`; the release workflow runs the tests and publishes binaries for Linux, macOS and Windows on amd64 and arm64 with GoReleaser.
