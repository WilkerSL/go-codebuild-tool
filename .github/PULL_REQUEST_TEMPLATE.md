## What and why

## Checklist

- [ ] Tests cover the change, and `go test -short ./...` and `golangci-lint run` pass.
- [ ] Golden files were regenerated with `-update` and the diff of `testdata/` was reviewed, if generated output changed.
- [ ] `CHANGELOG.md` describes user-visible changes under "Unreleased".
- [ ] The change follows the ground rules in `CONTRIBUTING.md`: Go 1.26 APIs only and deterministic output.
