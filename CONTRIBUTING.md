# Contributing to code-reviewer

Thanks for your interest in contributing! Here's how to get started.

## Dev Environment

The repo includes a `flake.nix` for reproducible tooling:

```bash
nix develop            # Go + golangci-lint + git
go build ./...
go test ./... -race
```

For detailed setup, config-field checklists, provider patterns, and test
conventions, see [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).

## Code Style

- **gofmt** — all code must be formatted with `gofmt` (enforced by CI).
- **Go idioms** — follow [Effective Go](https://go.dev/doc/effective_go) and the
  [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) wiki.
- **Doc comments** — all exported types, functions, and methods must have doc
  comments starting with the identifier name.
- **Error wrapping** — use `fmt.Errorf("doing X: %w", err)` for context.

## PR Workflow

1. Fork the repo and create a feature branch from `main`.
2. Make your changes. Keep commits focused and use
   [Conventional Commits](https://www.conventionalcommits.org/) style
   (e.g., `feat: add --timeout flag`, `fix: handle nil diff`).
3. Run tests and lint locally:
   ```bash
   nix develop -c go test ./... -race
   nix develop -c golangci-lint run ./...
   ```
4. Push and open a PR against `main`. CI must pass (build, test, lint).
5. Respond to review feedback. We aim for quick, collaborative reviews.

## Architecture

- [`docs/DEVELOPMENT.md`](docs/DEVELOPMENT.md) — how-to guides and patterns
- [`ROADMAP.md`](ROADMAP.md) — planned features and direction
- [`README.md`](README.md) — user-facing docs, flags reference, config format

## Releases

Releases are automated via [GoReleaser](https://goreleaser.com/). To cut a
release, a maintainer tags `main` with a semver tag (e.g., `v0.5.0`) and pushes.
The [release workflow](.github/workflows/release.yml) builds binaries for all
platforms and publishes the GitHub release. The Nix flake also builds from source
with `nix build`.

## Code of Conduct

Be kind, be constructive. We follow the
[Contributor Covenant](https://www.contributor-covenant.org/) v2.1. Treat
everyone with respect and assume good intentions.
