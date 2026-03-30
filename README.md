# code-reviewer

AI-powered code review CLI for GitLab merge requests. Uses Vertex AI (Gemini, Claude, Mistral) to analyze diffs and post actionable findings as inline comments or summary notes.

## Features

- **Multi-model** — Gemini 2.5 Flash (default), Pro, Claude, Mistral via single Vertex AI ADC credential
- **Focus modes** — `bugs`, `security`, `performance`, `style`, `docs`, or `all`
- **Severity filtering** — `low` (default), `medium`, `high`, `critical`
- **GitLab integration** — Inline diff discussions or simple MR notes, with idempotent cleanup on re-push
- **Context-aware** — Modular chunking strategies for large MRs
- **Configurable** — CLI flags, env vars, or per-repo `.code-reviewer.yaml`

## Quick Start

### Local Usage

```bash
# Review your branch against origin/HEAD
export GOOGLE_CLOUD_PROJECT=my-gcp-project
code-reviewer --diff

# Review against a specific ref
code-reviewer --diff HEAD~3

# Review specific files
code-reviewer --files internal/handler.go,internal/service.go

# Security-focused review
code-reviewer --diff --focus security

# Only show high/critical issues
code-reviewer --diff --min-severity high
```

### GitLab CI

Add to your `.gitlab-ci.yml`:

```yaml
# Quick setup — uses CI_JOB_TOKEN, no PAT needed
code-review:
  stage: review
  image: gcr.io/$PROJECT/code-reviewer:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GITLAB_TOKEN: $CI_JOB_TOKEN
    REVIEW_COMMENT_MODE: "notes"
  script:
    - code-reviewer --ci
  allow_failure: true
```

For inline diff-anchored comments, use a [Project Access Token](https://docs.gitlab.com/ee/user/project/settings/project_access_tokens.html) with `api` scope:

```yaml
code-review:
  variables:
    GITLAB_TOKEN: $CODE_REVIEWER_TOKEN    # PAT with api scope
    REVIEW_COMMENT_MODE: "discussions"
  script:
    - code-reviewer --ci
```

See [`.gitlab-ci.example.yml`](.gitlab-ci.example.yml) for the full setup.

## Configuration

Settings are applied in priority order: **CLI flags > env vars > `.code-reviewer.yaml` > defaults**.

### CLI Flags

| Flag | Description | Default |
|---|---|---|
| `--ci` | Run in GitLab CI mode | — |
| `--diff [ref]` | Review local git diff | `origin/HEAD` |
| `--files f1,f2` | Review specific files | — |
| `--model` | Vertex AI model ID | `gemini-2.5-flash` |
| `--focus` | Review focus (comma-separated) | `all` |
| `--min-severity` | Minimum severity to report | `low` |
| `--comment-mode` | `notes` or `discussions` | `notes` |
| `--chunk-strategy` | `fail` or `split` | `fail` |
| `--extra-rules` | Additional prompt rules | — |
| `--dry-run` | Analyze without posting | `false` |

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `GOOGLE_CLOUD_PROJECT` | GCP project for Vertex AI | **Required** |
| `GOOGLE_CLOUD_LOCATION` | GCP region | `us-central1` |
| `GITLAB_TOKEN` | GitLab API token | Required in CI |
| `GITLAB_BASE_URL` | GitLab API base URL | `https://gitlab.com` |
| `REVIEW_MODEL` | Model ID | `gemini-2.5-flash` |
| `REVIEW_FOCUS` | Focus areas | `all` |
| `REVIEW_MIN_SEVERITY` | Min severity | `low` |
| `REVIEW_COMMENT_MODE` | Comment mode | `notes` |
| `REVIEW_CHUNK_STRATEGY` | Chunk strategy | `fail` |
| `EXCLUDED_PATTERNS` | Glob patterns to skip | `go.sum,*.lock,vendor/*` |

### Per-Repo Config

Create `.code-reviewer.yaml` in your repo root:

```yaml
model: gemini-2.5-flash
focus: [bugs, security]
min_severity: low
comment_mode: discussions
excluded_patterns:
  - "*.pb.go"
  - "generated/*"
extra_rules: |
  Always flag raw SQL string concatenation.
  Check that zerolog is used instead of log/fmt.
```

See [`.code-reviewer.example.yaml`](.code-reviewer.example.yaml) for all options.

## Models

All models are accessed via Vertex AI using Application Default Credentials (ADC). No separate API keys needed.

| Model | Flag Value | Best For |
|---|---|---|
| Gemini 2.5 Flash | `gemini-2.5-flash` | Fast CI reviews (default) |
| Gemini 2.5 Pro | `gemini-2.5-pro` | Deep analysis |
| Claude Sonnet 4 | `claude-sonnet-4` | Code-focused reviews |
| Mistral Medium | `mistral-medium-3` | Alternative perspective |

## Auth

### Vertex AI (Model)

Uses [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials):

```bash
# Local development
gcloud auth application-default login

# CI/CD — use Workload Identity Federation or a service account key
```

### GitLab API

| Token Type | Capabilities | Setup |
|---|---|---|
| `CI_JOB_TOKEN` | Notes API (simple comments) | Automatic, zero config |
| Project Access Token | Notes + Discussions API (inline diff) | Settings → Access Tokens, `api` scope |

## Context Window Handling

Large MRs may exceed the model's context window. The `--chunk-strategy` flag controls behavior:

- **`fail`** (default) — Errors with a helpful message if the diff is too large. Forces teams to scope MRs.
- **`split`** — Auto-splits diffs into file groups, runs separate model calls, merges results.

The chunker interface is modular — custom strategies can be added.

## Development

```bash
# Enter dev shell
nix develop

# Build
go build ./cmd/code-reviewer

# Test
go test ./...

# Lint
golangci-lint run
```

## License

Apache 2.0
