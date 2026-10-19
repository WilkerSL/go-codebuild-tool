# Go versions

codebuild generates code for the Go version your project declares, not for the toolchain codebuild was built with.

## Supported versions

| Project `go` directive | Status | Capability set |
|---|---|---|
| below 1.26 | blocked until you authorize an upgrade | none |
| 1.26.x | supported | go1.26 |
| 1.27.x | supported | go1.27 |
| newer than 1.27 | supported with a warning | go1.27, the newest set this release knows |

The codebuild module itself declares `go 1.26.0`, so it installs and runs with Go 1.26 and never uses Go 1.27 APIs internally.

## Capability matrix

Templates never compare version numbers. They read a `Capabilities` value derived from the target version. The table is embedded in the binary; codebuild never looks anything up online, which keeps output deterministic.

| Capability | go1.26 | go1.27 | Effect on generated code |
|---|---|---|---|
| `JSONv2` | false | true | `encoding/json` with `Decoder.DisallowUnknownFields` versus `encoding/json/v2` with `json.RejectUnknownMembers(true)`; decode errors are mapped to the same problem responses in both cases |
| `StdUUID` | false | true | `github.com/google/uuid` (`uuid.NewV7()` returns `(UUID, error)`) versus the standard library `uuid` package (`uuid.NewV7()` returns `UUID`) |

Features available in both versions are used unconditionally: `net/http` routing patterns and `Request.PathValue`, `log/slog`, `errors.AsType`, `new(expr)`, range over integers.

The standard library `uuid` package does not implement `sql.Scanner` or `driver.Valuer`, so generated repositories use small adapters (`database.ScanUUID`, `database.UUIDArg`) for both UUID packages. The adapters also reorder the bytes of SQL Server `UNIQUEIDENTIFIER` values.

`encoding/json/v2` ships in Go 1.27 behind the `jsonv2` experiment, which is enabled by default. If `GOEXPERIMENT` contains `nojsonv2`, `codebuild check` warns that generated code for Go 1.27 will not compile.

## How the target is detected

- **Existing project**: the `go` directive in `go.mod`, read with `golang.org/x/mod/modfile`. The `toolchain` directive does not change the capability set; it only raises the toolchain that must be available.
- **`--starter`**: the version reported by `go env GOVERSION` for the effective toolchain (honoring `GOTOOLCHAIN`) becomes the `go` directive of the new `go.mod`.

## Toolchain checks

codebuild compares the effective toolchain with the version the project requires, the higher of the `go` directive and the `toolchain` directive:

| `GOTOOLCHAIN` | Toolchain older than required |
|---|---|
| `local` | error, exit code 5 |
| a pinned version such as `go1.26.2` | error, exit code 5 |
| `auto`, `path`, `<version>+auto`, `<version>+path` | warning: the go command will switch toolchains, possibly downloading one |

## Projects below Go 1.26

There is no code generation for older versions. When `go.mod` declares, for example, `go 1.24`:

- **Target**: the installed toolchain when it is Go 1.26 or newer and known to codebuild, otherwise the latest known version (1.27.0), which the go command downloads when `GOTOOLCHAIN` allows it.
- **Interactive terminal**: codebuild prints the detected version and asks whether it may generate for the target, warning that the `go` directive will be raised because the generated code does not compile otherwise. The default answer is no. If you accept, `go.mod` is updated, the code is generated and the summary shows `upgrade: go directive 1.24 -> 1.27.0`.
- **No terminal** (CI, scripts, `--json`): the command fails with exit code 4 unless `--upgrade-go` is passed.
- **MCP**: `plan` and `generate` return a structured error with code `unsupported_go_version` and the details `detected`, `minimum` and `upgradeTo`. They only proceed when called with `upgradeGo: true`, which an agent must send only after the user confirms.

A `toolchain` directive older than the new `go` directive is removed during the upgrade.

## Workspaces

When a `go.work` applies to the project (found above the module, or named by `GOWORK`; `GOWORK=off` disables it), its `go` and `toolchain` directives also count for the required toolchain. codebuild warns when the workspace declares a lower `go` version than the module or does not list the module in `use`, because go commands fail in both cases. The generated code still follows the `go` directive of the module.

## Versions newer than the latest known

A project on, for example, Go 1.29 is generated with the go1.27 capability set and a warning. The result compiles because Go keeps backward compatibility; newer features are simply not used until a codebuild release adds them.

## How codebuild evolves with each Go release

1. Add a row to the capability table in `internal/goversion/capabilities.go` with the new flags.
2. Teach the templates to consult the new flags, never the version number.
3. Add the new version to the golden matrix (`testdata/golden/<project>/go1.NN/<dialect>`) and regenerate with `go test ./internal/project -update`.
4. Add the version to the CI matrix.
5. Document the change in this file and in `CHANGELOG.md`.

The minimum version only moves in a major release, together with an entry in the changelog.
