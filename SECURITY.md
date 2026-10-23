# Security policy

codebuild writes security-sensitive code into your projects: password hashing, sessions, JWT and API keys, cookies and cross-origin checks, SQL queries, and, in microservice workspaces, mutual TLS and the signed internal token between services. Vulnerabilities in that code matter as much as vulnerabilities in the tool itself.

## Supported versions

| Version | Supported |
|---|---|
| 0.2.x | yes |
| older | no |

Before 1.0, fixes ship only in the latest minor release.

## Reporting a vulnerability

Report privately through GitHub: open the [Security tab](https://github.com/WilkerSL/go-codebuild-tool/security) of the repository and choose "Report a vulnerability". Do not open a public issue, pull request or discussion.

Include:

- the codebuild version, the Go version and the dialect;
- the specs and commands that produce the vulnerable code, or the affected file of the tool;
- the impact you expect and, if you have one, a proof of concept.

The report stays private until a fix is released. Fixed versions are published as a new release with an advisory that credits the reporter, unless you prefer to stay anonymous.

## Verifying a release

Releases from 0.3.0 on are built by GitHub Actions and ship with:

- `checksums.txt` and its Sigstore bundle `checksums.txt.sigstore.json`, signed keyless by the release workflow;
- one SPDX SBOM per archive (`<archive>.sbom.json`);
- a GitHub build provenance attestation for every archive and for `checksums.txt`.

Check the signature of the checksums, then the archive you downloaded:

```bash
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json --certificate-identity-regexp '^https://github.com/WilkerSL/go-codebuild-tool/\.github/workflows/release\.yml@refs/tags/v' --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
```

Or check the provenance of an archive with the GitHub CLI:

```bash
gh attestation verify codebuild_0.3.0_linux_amd64.tar.gz --repo WilkerSL/go-codebuild-tool
```

`go install github.com/WilkerSL/go-codebuild-tool/cmd/codebuild@<version>` builds from source and is verified by the Go checksum database instead.

## Reviews

The generated authentication code had an internal security review in October 2026; it was not an independent audit. Its findings are fixed in 0.3.0 and listed in `CHANGELOG.md`. The parsers of the tool and the token and password parsers of generated projects are fuzz tested.

## Fixing generated projects

Generated `*_gen.go` files are rewritten by every build, so most fixes reach a project by upgrading codebuild and running `codebuild build`. The advisory says when a fix also needs a change in files you own, such as `service.go`, `main.go` or configuration.

## Scope

In scope: the codebuild binary and MCP server (for example writing outside the project directory) and any code or configuration it generates with its default settings.

Out of scope: vulnerabilities in the Go toolchain, database drivers or other dependencies (report them upstream), and changes you made to user-owned files.
