# code-reviewer

AI-powered code review CLI for GitLab merge requests. Uses Vertex AI (Gemini, Claude, Mistral) to analyze diffs and post actionable findings as inline comments or summary notes.

## Install

```bash
# Homebrew
brew install OpticDiff/tap/code-reviewer

# Nix
nix run github:OpticDiff/code-reviewer
nix profile install github:OpticDiff/code-reviewer

# mise
mise use -g ubi:OpticDiff/code-reviewer

# Go
go install github.com/OpticDiff/code-reviewer/cmd/code-reviewer@latest

# Pre-built binary from GitHub Releases
# https://github.com/OpticDiff/code-reviewer/releases

# Build from source
git clone https://github.com/OpticDiff/code-reviewer.git
cd code-reviewer && go build -o code-reviewer ./cmd/code-reviewer
```

## Features

- **Incremental review** — Only review files changed in the latest push, not the entire MR (v0.3.0)
- **SARIF output** — Write findings in SARIF 2.1.0 format for CI security tabs (v0.3.0)
- **Multi-model consensus** — Run multiple models in parallel (e.g. Gemini + Claude), only keep findings that meet the configured consensus threshold
- **Custom prompts** — Bring your own system prompt for specialized reviews (security audits, architecture checks)
- **Focus modes** — `bugs`, `security`, `performance`, `style`, `docs`, or `all`
- **Severity filtering** — `low` (default), `medium`, `high`, `critical`
- **Rich terminal output** — ANSI-colored findings with severity badges, file grouping, and suggestion blocks
- **GitLab integration** — Inline diff discussions or simple MR notes, with idempotent cleanup on re-push
- **Context-aware** — Modular chunking strategies for large MRs
- **Repo-aware context** — Tree-sitter extracts changed symbols from diffs; grep finds usages in unchanged files to give the reviewer cross-file awareness
- **REVIEW.md** — Drop a `REVIEW.md` in your repo root to inject team-specific review instructions at the highest priority
- **Configurable** — CLI flags, env vars, per-repo `.code-reviewer.yaml`, or `REVIEW.md`

## Quick Start

### Local Usage

```bash
# Review your branch against origin/HEAD (colored terminal output)
export GOOGLE_CLOUD_PROJECT=my-gcp-project
code-reviewer --diff

# Review against a specific ref
code-reviewer --diff HEAD~3

# Review specific files
code-reviewer --files internal/handler.go,internal/service.go

# Security-focused review with a custom prompt
code-reviewer --diff --focus security --custom-prompt examples/prompts/security-audit.md

# Only show high/critical issues
code-reviewer --diff --min-severity high

# JSON output for tooling integration
code-reviewer --diff --json

# SARIF output for CI security tabs (GitLab, GitHub)
code-reviewer --diff --sarif results.sarif

# Disable colors (or set NO_COLOR env var)
code-reviewer --diff --no-color
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
    SARIF_OUTPUT: "results.sarif"
  script:
    - code-reviewer --ci --incremental
  allow_failure: true
  artifacts:
    reports:
      sast: results.sarif
    when: always
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
| `--models` | Comma-separated models for multi-model consensus | — |
| `--consensus-threshold` | Min models that must agree on a finding | `2` (when `--models` is set) |
| `--focus` | Review focus (comma-separated) | `all` |
| `--min-severity` | Minimum severity to report | `low` |
| `--comment-mode` | `notes` or `discussions` | `notes` |
| `--chunk-strategy` | `fail` or `split` | `fail` |
| `--extra-rules` | Additional prompt rules | — |
| `--custom-prompt` | Path to custom system prompt file | — |
| `--dry-run` | Analyze without posting | `false` |
| `--json` | Output results as JSON | `false` |
| `--sarif` | Write SARIF 2.1.0 output to file | — |
| `--no-color` | Disable ANSI color output | `false` |
| `--no-context` | Disable repo-aware cross-file context injection | `false` |
| `--incremental` | Only review files changed in latest push (CI mode) | `false` |
| `--proxy-url` | Route model calls through an LLM proxy (e.g. Candela) | — |
| `--version` | Print version and exit | — |

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `GOOGLE_CLOUD_PROJECT` | GCP project for Vertex AI | **Required** |
| `GOOGLE_CLOUD_LOCATION` | GCP region | `us-central1` |
| `GITLAB_TOKEN` | GitLab API token | Required in CI |
| `GITLAB_BASE_URL` | GitLab API base URL | `https://gitlab.com` |
| `REVIEW_MODEL` | Model ID | `gemini-2.5-flash` |
| `REVIEW_MODELS` | Comma-separated models for consensus | — |
| `REVIEW_FOCUS` | Focus areas | `all` |
| `REVIEW_MIN_SEVERITY` | Min severity | `low` |
| `REVIEW_COMMENT_MODE` | Comment mode | `notes` |
| `REVIEW_CHUNK_STRATEGY` | Chunk strategy | `fail` |
| `REVIEW_CUSTOM_PROMPT` | Path to custom system prompt | — |
| `REVIEW_OUTPUT_JSON` | Output results as JSON (`true`/`false`) | `false` |
| `SARIF_OUTPUT` | Write SARIF output to this file path | — |
| `INCREMENTAL` | Only review changed files in latest push (`true`/`false`) | `false` |
| `EXCLUDED_PATTERNS` | Glob patterns to skip | `go.sum,*.lock,vendor/*` |
| `NO_COLOR` | Disable ANSI colors ([no-color.org](https://no-color.org)) | — |

### Per-Repo Config

Create `.code-reviewer.yaml` in your repo root:

```yaml
model: gemini-2.5-flash
focus: [bugs, security]
min_severity: low
comment_mode: discussions
custom_prompt: prompts/team-rules.md
excluded_patterns:
  - "*.pb.go"
  - "generated/*"
extra_rules: |
  Always flag raw SQL string concatenation.
  Check that zerolog is used instead of log/fmt.
```

See [`.code-reviewer.example.yaml`](.code-reviewer.example.yaml) for all options.

### REVIEW.md

Create a `REVIEW.md` in your repo root to inject team-specific review instructions. Its contents are treated as the **highest priority** instruction in the system prompt — above the built-in rules, focus overlays, and extra rules.

```markdown
## Our Review Standards

- All exported functions MUST have doc comments.
- Never use `fmt.Errorf` without `%w` for wrapping.
- Prefer table-driven tests over sequential assertions.
- Flag any use of `context.TODO()` — replace with a real context.
```

The file is discovered by walking up from the working directory, the same way `.code-reviewer.yaml` is found. Presence is logged at startup (`review_md=true`).

### Repo-Aware Context

By default, code-reviewer uses [Tree-sitter](https://tree-sitter.github.io/) (pure-Go, no CGo) to extract symbols defined or modified in the diff, then searches the rest of the repo for usages of those symbols in unchanged files. The matched snippets are injected into the prompt as **Related Unchanged Code**, giving the model cross-file awareness without sending the entire repo.

Supported languages: **Go**, **Kotlin**, **Java**, **Python**, **TypeScript**.

Noise mitigation is built in:
- Symbol names shorter than 4 characters are skipped.
- If a symbol appears in more than 20 files, it is treated as too common and excluded.
- Import statements and comments are filtered out.

Disable with `--no-context` or the `disable_context: true` config field.

## Models

All models are accessed via Vertex AI using Application Default Credentials (ADC). No separate API keys needed.

| Model | Flag Value | Best For |
|---|---|---|
| Gemini 2.5 Flash | `gemini-2.5-flash` | Fast CI reviews (default) |
| Gemini 2.5 Pro | `gemini-2.5-pro` | Deep analysis |
| Claude Sonnet 4 | `claude-sonnet-4` | Code-focused reviews |
| Mistral Medium | `mistral-medium-3` | Alternative perspective |

### Multi-Model Consensus

Run multiple models in parallel and only keep findings that multiple models agree on — dramatically reducing false positives:

```bash
# Run Gemini + Claude, keep findings both agree on
code-reviewer --diff --models gemini-2.5-flash,claude-sonnet-4

# Require all 3 models to agree
code-reviewer --diff --models gemini-2.5-flash,gemini-2.5-pro,claude-sonnet-4 --consensus-threshold 3
```

Findings are deduplicated by file + category + line proximity (±3 lines). The finding with the most detailed explanation is kept as the canonical result.

## Custom Prompts

Replace the built-in system prompt with your own for specialized reviews:

```bash
code-reviewer --diff --custom-prompt path/to/my-prompt.md
```

The custom prompt replaces the built-in base prompt, but **focus overlays** (`--focus`) and **extra rules** (`--extra-rules`) are still appended automatically.

### Example Prompts

Four example prompts are included in [`examples/prompts/`](examples/prompts/):

| Prompt | Use Case |
|---|---|
| [`security-audit.md`](examples/prompts/security-audit.md) | Deep security review: injection, auth, crypto, PII |
| [`strict.md`](examples/prompts/strict.md) | Zero-tolerance review that flags everything |
| [`quick.md`](examples/prompts/quick.md) | Fast review focused only on critical/high issues |
| [`architecture.md`](examples/prompts/architecture.md) | Architecture and design pattern review |

### Writing Custom Prompts

A custom prompt is a Markdown file with instructions for the AI reviewer. It should include:

1. **Persona** — who the reviewer should act as
2. **Objective** — what to focus on
3. **Output format** — must instruct the model to return JSON matching the findings schema

See the [example prompts](examples/prompts/) for reference.

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

## Security

- **Input validation** — Diff refs and file paths are validated to prevent command injection via `git` arguments.
- **SSRF protection** — GitLab pagination URLs are validated against the configured base URL to prevent token exfiltration.
- **Prompt injection resistance** — The system prompt includes adversarial content detection instructions.

## Development

```bash
# Enter dev shell
nix develop

# Build
go build ./cmd/code-reviewer

# Test
go test ./... -race -count=1 -cover

# Lint
golangci-lint run

# Check version
./code-reviewer --version
```

CI runs **build**, **test**, and **lint** as 3 parallel jobs. [CodeRabbit](https://coderabbit.ai) provides automated PR reviews on GitHub.

### Releasing

Releases are automated via [GoReleaser](https://goreleaser.com). Tag a version to publish binaries to GitHub Releases:

```bash
git tag -a v0.3.0 -m "v0.3.0"
git push origin v0.3.0
# → GitHub Actions: test → build 6 binaries → publish to Releases
```

## License

Apache 2.0
