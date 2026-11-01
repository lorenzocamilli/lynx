# Contribution Guidelines

Thank you for taking an interest in Lynx! If you want to contribute to the
project, please read the guidelines below to ensure a smooth development experience.

## Code of conduct

Please first read the [code of conduct](CODE_OF_CONDUCT.md), and abide to it
whenever you interact with the community.

## Issues

Use [issues](https://github.com/lorenzocamilli/lynx/issues) for reporting bugs,
adding feature requests and giving context to PRs you submit. Please use [labels](https://github.com/lorenzocamilli/lynx/labels)
in favor of category prefixes in issue titles. To keep the issue tracker
focused on development, use [discussions](https://github.com/lorenzocamilli/lynx/discussions)
for usage questions and non-code related discourse.

Before submitting new feature requests, check out the Kanban board for the
status of on-going work. There might already be a card/issue.

## Pull requests

Before submitting a pull request that introduces a new feature or significantly
changes the behavior of Lynx, please consider first using [discussions](https://github.com/lorenzocamilli/lynx/discussions)
or commenting on a relevant existing issue to share what you have in mind.
Because the project is in an early stage, this is especially important; there
are still a lot of major design decisions to be made. Until the foundation has
solidified, design and implementation leading up to the first milestone (v1.0)
is highly in flux, and your work might not align/be applicable for what the
maintainers have envisioned.

## Development

Install Go 1.26.10+, Node.js 20+, and a C compiler (required by Go's race
detector), then run:

```bash
make check       # Go build/tests/race/vet and admin lint/typecheck
make build       # static admin export and single embedded Go binary
./lynx
```

For a quicker backend-only test run that doesn't require building the UI:

```bash
go test -tags noembed ./cmd/... ./pkg/...
```

Frontend type-checking also rejects unused locals and parameters. Keep generated
GraphQL files in sync with their schemas rather than editing generated code by
hand. Run `go generate ./pkg/api` from the repository root after schema changes.
The generator preserves the handwritten resolver file; add implementations for
new fields there, then regenerate. Run
`go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -tags noembed ./cmd/... ./pkg/...`
for the blocking Go vulnerability check. See `CODEBASE_REVIEW.md` for the current release-readiness findings.
