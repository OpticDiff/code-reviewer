# code-reviewer

[![CI](https://github.com/OpticDiff/code-reviewer/actions/workflows/ci.yml/badge.svg)](https://github.com/OpticDiff/code-reviewer/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/OpticDiff/code-reviewer)](https://github.com/OpticDiff/code-reviewer/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/OpticDiff/code-reviewer)](https://goreportcard.com/report/github.com/OpticDiff/code-reviewer)
[![License](https://img.shields.io/github/license/OpticDiff/code-reviewer)](LICENSE)
[![GitHub Action](https://img.shields.io/badge/action-OpticDiff%2Fcode--reviewer--action-blue?logo=github)](https://github.com/OpticDiff/code-reviewer-action)
[![pre-commit](https://img.shields.io/badge/pre--commit-enabled-brightgreen?logo=pre-commit)](https://pre-commit.com)

AI-powered code review CLI for GitHub pull requests and GitLab merge requests. Works with Ollama, Vertex AI (Gemini, Claude, Mistral), AWS Bedrock (via LiteLLM), or any OpenAI-compatible endpoint.

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

# Docker
docker pull ghcr.io/opticdiff/code-reviewer:1.0.0
docker run --rm ghcr.io/opticdiff/code-reviewer:1.0.0 --help

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
- **Auto-summary** — `--summarize` generates structured MR descriptions from diffs: classification, intent, risk level, scope areas, and breaking changes
- **Intent-aware review** — `--intent` enables two-pass review: infer intent, then review against it. Auto-enabled in CI (v0.5.0)
- **Explain mode** — `--explain` generates a plain-language walkthrough of the diff (v0.5.0)
- **Fix mode** — `--fix` applies suggested code fixes directly to the working tree (v0.5.0)
- **Pre-push hook** — `code-reviewer hook install` sets up automatic review before `git push` (v0.5.1)
- **Configurable** — CLI flags, env vars, per-repo `.code-reviewer.yaml`, or `REVIEW.md`
- **GitHub support** — Full PR review integration: inline comments, code suggestions, previous review cleanup (v0.6.0)
- **Code suggestions** — AI-generated fix suggestions rendered as platform-native suggestion blocks (v0.6.0)
- **Multi-line comments** — Findings can span line ranges for more precise feedback (v0.6.0)
- **Description update** — `--update-description` injects review summary into MR/PR description with idempotent markers (v0.6.0)
- **Review cleanup** — `--cleanup-mode` controls how previous bot reviews are handled: `delete` (default) or `resolve` (v0.6.0)
- **GitLab Draft Notes** — Reviews posted as draft notes and published atomically for a single notification (v0.6.0)
- **Smart incremental** — Preserves review findings for unchanged files across pushes, only re-reviews modified files (v0.7.0)
- **Scope enforcement** — `--max-files` and `--scope-action` warn or fail when MRs exceed a file count threshold (v0.7.0)
- **Audit trail** — `--audit-log` writes structured JSONL audit records per review: config, files, findings, token usage, duration (v0.7.0)
- **Auto-approve** — `--auto-approve` automatically approves MR/PR when review finds zero issues, with 9 safety guards including SHA pinning (v0.8.0)

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

```yaml
# With auto-approve on clean reviews
code-review:
  stage: review
  image: gcr.io/$PROJECT/code-reviewer:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GITLAB_TOKEN: $CODE_REVIEWER_TOKEN  # PAT with api scope (required for approvals)
    REVIEW_COMMENT_MODE: "discussions"
  script:
    - code-reviewer --ci --auto-approve --audit-log review.jsonl
  artifacts:
    paths:
      - review.jsonl
    when: always
```

See [`.gitlab-ci.example.yml`](.gitlab-ci.example.yml) for the full setup.

### GitHub Actions

Use the [reusable action](https://github.com/OpticDiff/code-reviewer-action) for the simplest setup:

```yaml
# Self-hosted (Ollama) — zero auth, no cloud
- uses: OpticDiff/code-reviewer-action@v1
  with:
    model: qwen3:8b
    extra-args: --api-url http://ollama:11434/v1
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

```yaml
# Vertex AI with auto-approve
- uses: google-github-actions/auth@v2
  with:
    workload_identity_provider: ${{ secrets.WIF_PROVIDER }}
    service_account: ${{ secrets.WIF_SA }}
- uses: OpticDiff/code-reviewer-action@v1
  with:
    extra-args: --auto-approve --audit-log review.jsonl
  env:
    GOOGLE_CLOUD_PROJECT: ${{ secrets.GCP_PROJECT }}
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

See [`examples/github/`](examples/github/) for complete workflows: [basic](examples/github/basic.yml), [SARIF + Code Scanning](examples/github/sarif.yml), [self-hosted Ollama](examples/github/self-hosted.yml), [multi-model consensus](examples/github/consensus.yml), [AWS Bedrock](examples/github/bedrock.yml).

## Configuration

Settings are applied in priority order: **CLI flags > env vars > `.code-reviewer.yaml` > defaults**.

### CLI Flags

| Flag | Description | Default |
|---|---|---|
| `--ci` | Run in CI mode (auto-detects GitHub/GitLab) | — |
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
| `--max-tokens` | Maximum total tokens per review (0 = unlimited) | `0` |
| `--max-files` | Maximum files before scope enforcement (0 = unlimited) | `0` |
| `--scope-action` | Action when scope exceeded: `warn` or `fail` | `warn` |
| `--audit-log` | Write structured JSONL audit log to file | — |
| `--auto-approve` | Automatically approve MR/PR on clean review (CI mode only) | `false` |
| `--api-url` | OpenAI-compatible API endpoint (e.g., `http://localhost:11434/v1`) | — |
| `--api-key` | API key for HTTP provider (optional for IAM/ADC auth) | — |
| `--incremental` | Only review files changed in latest push (CI mode) | `false` |
| `--proxy-url` | Route model calls through an LLM proxy (e.g. Candela) | — |
| `--summarize` | Generate structured MR summary instead of review | `false` |
| `--summary-update-description` | Update MR description with generated summary | `false` |
| `--intent` | Enable two-pass intent-aware review | `false` (auto in CI) |
| `--no-intent` | Disable intent-aware review (overrides CI default) | `false` |
| `--explain` | Explain the diff instead of reviewing it | `false` |
| `--fix` | Apply suggested fixes to the working tree | `false` |
| `--update-description` | Inject review summary into MR/PR description | `false` |
| `--cleanup-mode` | How to handle previous reviews: `delete` or `resolve` | `delete` |
| `--version` | Print version and exit | — |
| `hook install` | Install a pre-push git hook | — |
| `hook uninstall` | Remove the pre-push git hook | — |

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
| `REVIEW_MAX_TOKENS` | Maximum total tokens per review (0 = unlimited) | `0` |
| `REVIEW_MAX_FILES` | Maximum files before scope enforcement (0 = unlimited) | `0` |
| `REVIEW_SCOPE_ACTION` | Action when scope exceeded: `warn` or `fail` | `warn` |
| `REVIEW_AUDIT_LOG` | Write audit log to this file path | — |
| `REVIEW_AUTO_APPROVE` | Automatically approve on clean review (`true`/`false`) | `false` |
| `REVIEW_API_URL` | OpenAI-compatible API endpoint | — |
| `REVIEW_API_KEY` | API key for HTTP provider | — |
| `NO_COLOR` | Disable ANSI colors ([no-color.org](https://no-color.org)) | — |
| `GITHUB_TOKEN` | GitHub API token (auto-set in GitHub Actions) | Required for GitHub |
| `CODE_REVIEWER_UPDATE_DESCRIPTION` | Update MR/PR description with summary | `false` |
| `CODE_REVIEWER_CLEANUP_MODE` | Previous review cleanup mode | `delete` |

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
max_tokens: 50000  # Optional: cap total tokens per review
api_url: http://localhost:11434/v1  # Optional: use a self-hosted model
update_description: false  # Inject summary into MR/PR description
cleanup_mode: delete       # delete or resolve
auto_approve: false     # Auto-approve MR/PR when zero findings (CI mode)
audit_log: review.jsonl # Write audit trail
max_files: 30           # Scope enforcement: warn when MR exceeds 30 files
scope_action: warn      # warn or fail
```

See [`.code-reviewer.example.yaml`](.code-reviewer.example.yaml) for all options.

### Self-Hosted Models

Use `--api-url` to point at any OpenAI-compatible endpoint. No GCP project required.

```bash
# Ollama (local)
code-reviewer --diff --api-url http://localhost:11434/v1 --model qwen3:32b

# Gemma on Cloud Run
code-reviewer --diff --api-url https://gemma-review-xyz.run.app/v1 --model gemma-3-27b

# vLLM
code-reviewer --diff --api-url http://gpu-server:8000/v1 --model meta-llama/Llama-4-Scout-17B-16E

# With Candela proxy (observability + routing)
code-reviewer --diff --api-url http://candela:8080/v1 --model gemini-2.5-flash
```

### Auto-Approve

When enabled, the tool automatically approves the MR/PR via the platform API if the review finds zero issues. **Opt-in only.**

```bash
code-reviewer --ci --auto-approve
```

All safety guards must pass:

| Guard | What it checks |
|---|---|
| Zero findings | Review found no issues (pre-severity-filter) |
| Files reviewed > 0 | Model actually reviewed something |
| No skipped files | Token budget didn't trim any files |
| Budget not exceeded | Runtime token limit wasn't hit mid-review |
| Scope not oversized | MR doesn't exceed `--max-files` limit |
| Not truncated | Model response wasn't cut short |
| Not draft | MR/PR is not in draft/WIP state |
| CI mode, not dry-run | Running in real CI pipeline |
| SHA pinned | Approval targets the exact reviewed commit |

If any guard fails, the reason is printed and approval is skipped.

Approval failures (e.g. missing token permissions) exit non-zero:

```
GitHub: ensure 'pull-requests: write' permission
GitLab: ensure token has 'api' scope
```

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

### Auto-Summary

Use `--summarize` to generate a structured MR description from the diff instead of a code review. The model analyzes the changes and produces:

- **Classification** — `feat`, `fix`, `refactor`, `chore`, `docs`, `test`, `security`, `config`, `perf`
- **Intent** — What the developer is trying to accomplish
- **Risk level** — `low`, `medium`, `high` based on scope, complexity, and sensitivity
- **Scope areas** — Which parts of the codebase are affected (e.g. `auth`, `api`, `database`)
- **Breaking changes** — Any backward-incompatible changes

```bash
# Local: summarize your branch diff
code-reviewer --summarize --diff

# CI: post summary as MR comment
code-reviewer --summarize --ci

# JSON output for scripting
code-reviewer --summarize --diff --json
```

### Pre-push Hook

Automatic code review before every push — catches issues before CI, before MR creation, before anyone sees your code.

```bash
# Install the hook (one-time setup)
code-reviewer hook install

# Now every git push triggers a review:
$ git push
🔍 code-reviewer: reviewing changes before push...

  ❌ HIGH  internal/auth/handler.go:58
           Nil pointer dereference — token is used before nil check

🚫 Push blocked: 1 HIGH severity finding. Fix or --no-verify to skip.

# Remove the hook
code-reviewer hook uninstall
```

Honors Git's `core.hooksPath` configuration. Won't overwrite foreign hooks.

Also works with the [pre-commit](https://pre-commit.com) framework:

```yaml
# .pre-commit-config.yaml
- repo: https://github.com/OpticDiff/code-reviewer
  rev: v0.7.0
  hooks:
    - id: code-review
      stages: [pre-push]
      args: [--min-severity, high]
```

Other hook managers work too:

```yaml
# lefthook.yml
pre-push:
  commands:
    code-review:
      run: code-reviewer --diff --min-severity high
```

```bash
# Husky (Node/JS projects)
npx husky add .husky/pre-push "code-reviewer --diff --min-severity high"
```

See [`examples/hooks/`](examples/hooks/) for lefthook, Husky, and mise configurations.

## Models

| Model | Tier | Context | Provider | Best For |
|---|---|---|---|---|
| `gemini-2.5-flash` | ⭐ Recommended | 1M | Vertex AI | Fast CI reviews (default) |
| `gemini-2.5-pro` | ⭐⭐ Best | 1M | Vertex AI | Deep analysis |
| `claude-sonnet-4` | ⭐⭐ Best | 200k | Vertex AI | Code-focused reviews |
| `mistral-medium-3` | ⭐ Good | 128k | Vertex AI | Alternative perspective |
| `qwen3:32b` | ⭐ Good | 32k | Ollama / self-hosted | Local, no cloud needed (32GB RAM) |
| `qwen3:8b` | Demo | 32k | Ollama / self-hosted | Quick local demo (8GB RAM) |
| Any model | Varies | Varies | `--api-url` | Any OpenAI-compatible endpoint |

Vertex AI models use [Application Default Credentials](https://cloud.google.com/docs/authentication/application-default-credentials). Self-hosted models use `--api-url` pointed at any OpenAI-compatible endpoint (Ollama, vLLM, Cloud Run, LiteLLM for Bedrock).

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

### GitHub API

| Token Type | Capabilities | Setup |
|---|---|---|
| `GITHUB_TOKEN` (Actions) | PR review comments, suggestions | Automatic in GitHub Actions |
| Personal Access Token | PR reviews outside CI | `repo` scope required |

## Context Window Handling

Large MRs may exceed the model's context window. The `--chunk-strategy` flag controls behavior:

- **`fail`** (default) — Errors with a helpful message if the diff is too large. Forces teams to scope MRs.
- **`split`** — Auto-splits diffs into file groups, runs separate model calls, merges results.

The chunker interface is modular — custom strategies can be added.

## Caching

code-reviewer caches review results per file diff, so unchanged files are never re-reviewed. This dramatically reduces API calls and latency on iterative PRs.

**How it works:**
- Each file diff is hashed (content + model + prompt + custom rules) → deterministic cache key
- Cache hits return findings instantly without an LLM call
- Entries expire after 7 days by default

**Configuration:**

```yaml
# .code-reviewer.yaml
cache_dir: ~/.cache/code-reviewer   # Default location
no_cache: false                      # Set to true to disable
cache_max_age: 7d                    # Auto-expire entries
```

```bash
# CLI flags
code-reviewer --no-cache              # Skip cache for this run
code-reviewer --cache-dir /tmp/cr     # Custom cache location
code-reviewer --cache-max-age 24h     # Custom expiry

# Environment variables
REVIEW_CACHE_DIR=/tmp/cr
REVIEW_NO_CACHE=true
```

**Cache management:**

```bash
code-reviewer cache stats    # Show entry count, size, oldest entry
code-reviewer cache clear    # Remove all cached entries
```

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
git tag -a v0.7.0 -m "v0.7.0"
git push origin v0.7.0
# → GitHub Actions: test → build 6 binaries → publish to Releases
```

## License

Apache 2.0
