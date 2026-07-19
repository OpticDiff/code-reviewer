# CI Setup & Operations Guide

How to run code-reviewer in CI/CD pipelines, authenticate with models and VCS, and configure every option.

## GitLab CI

### Standard Review Job

The simplest setup — uses `CI_JOB_TOKEN` (zero configuration), posts findings as MR notes:

```yaml
code-review:
  stage: review
  image: gcr.io/$PROJECT/code-reviewer:latest  # Pin to a specific version in production
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GOOGLE_CLOUD_PROJECT: "my-gcp-project"
    GITLAB_TOKEN: $CI_JOB_TOKEN
    REVIEW_COMMENT_MODE: "notes"
  script:
    - code-reviewer --ci
  allow_failure: true
```

### Incremental Review (Latest Push Only)

Only review files changed in the latest push, not the entire MR. Dramatically faster on long-lived MRs:

```yaml
code-review:
  stage: review
  image: gcr.io/$PROJECT/code-reviewer:latest  # Pin to a specific version in production
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GOOGLE_CLOUD_PROJECT: "my-gcp-project"
    GITLAB_TOKEN: $CI_JOB_TOKEN
    REVIEW_COMMENT_MODE: "notes"
  script:
    - code-reviewer --ci --incremental
  allow_failure: true
```

### Security Gate with SARIF

Blocks the MR on high/critical findings. SARIF output appears in GitLab's Security tab:

```yaml
security-review:
  stage: review
  image: gcr.io/$PROJECT/code-reviewer:latest  # Pin to a specific version in production
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GOOGLE_CLOUD_PROJECT: "my-gcp-project"
    GITLAB_TOKEN: $CODE_REVIEWER_TOKEN   # PAT with api scope
    REVIEW_MODEL: "gemini-2.5-pro"
    REVIEW_FOCUS: "security"
    REVIEW_MIN_SEVERITY: "high"
    REVIEW_COMMENT_MODE: "discussions"
    SARIF_OUTPUT: "gl-sast-report.json"
  script:
    - code-reviewer --ci --incremental
  allow_failure: false
  artifacts:
    reports:
      sast: gl-sast-report.json
    when: always
```

### Multi-Model Consensus

Run Gemini + Claude in parallel. Only keep findings both models agree on — dramatically reduces false positives:

```yaml
consensus-review:
  stage: review
  image: gcr.io/$PROJECT/code-reviewer:latest  # Pin to a specific version in production
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    GOOGLE_CLOUD_PROJECT: "my-gcp-project"
    GITLAB_TOKEN: $CODE_REVIEWER_TOKEN
    REVIEW_MODELS: "gemini-2.5-flash,claude-sonnet-4"
    REVIEW_COMMENT_MODE: "discussions"
  script:
    - code-reviewer --ci --incremental
  allow_failure: true
```

### Inline Diff Comments

For inline diff-anchored comments (instead of simple MR notes), use a [Project Access Token](https://docs.gitlab.com/ee/user/project/settings/project_access_tokens.html) with `api` scope:

```yaml
code-review:
  variables:
    GITLAB_TOKEN: $CODE_REVIEWER_TOKEN    # PAT with api scope
    REVIEW_COMMENT_MODE: "discussions"
  script:
    - code-reviewer --ci
```

> **Note:** `CI_JOB_TOKEN` only supports the Notes API (simple comments). For inline diff discussions, you need a Project Access Token or Personal Access Token with `api` scope.

### CI Environment Variables

GitLab CI automatically sets these variables in MR pipelines:

| Variable | Description |
|---|---|
| `CI_PIPELINE_SOURCE` | Pipeline trigger (filter with `merge_request_event`) |
| `CI_PROJECT_ID` | Numeric project ID (auto-detected) |
| `CI_MERGE_REQUEST_IID` | MR number within the project (auto-detected) |
| `CI_MERGE_REQUEST_DIFF_BASE_SHA` | Base commit SHA for the MR diff |
| `CI_COMMIT_BEFORE_SHA` | Previous commit SHA (for incremental review) |

## GitHub Actions

While code-reviewer's VCS integration targets GitLab, the `--diff` mode works anywhere. Use it in GitHub Actions to get review output as a PR comment:

```yaml
name: Code Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      contents: read
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for accurate diffs

      - name: Install code-reviewer
        run: go install github.com/OpticDiff/code-reviewer/cmd/code-reviewer@latest  # Pin to a specific version in production

      - name: Run review
        env:
          GOOGLE_CLOUD_PROJECT: ${{ secrets.GCP_PROJECT }}
        run: |
          code-reviewer --diff origin/${{ github.base_ref }} --json > review.json

      - name: Post results
        if: always()
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const review = JSON.parse(fs.readFileSync('review.json', 'utf8'));
            if (review.findings.length === 0) return;
            let body = `## 🔍 Code Review — ${review.findings.length} finding(s)\n\n`;
            body += `${review.summary}\n\n`;
            for (const f of review.findings) {
              body += `### ${f.severity} — ${f.title}\n`;
              body += `📁 \`${f.file}:${f.line}\` | Category: ${f.category}\n\n`;
              body += `${f.body}\n\n`;
              if (f.suggestion) body += `\`\`\`suggestion\n${f.suggestion}\n\`\`\`\n\n`;
            }
            await github.rest.issues.createComment({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: context.issue.number,
              body: body.slice(0, 65536)
            });
```

> **Note:** This uses `--diff` mode (not `--ci`), which doesn't require GitLab-specific environment variables. The `--json` flag produces machine-parseable output for scripting.

## Authentication

### Authentication Matrix

| Mode | Credential | How to Set | Capabilities |
|---|---|---|---|
| **Vertex AI (ADC)** | Application Default Credentials | `gcloud auth application-default login` (local) or Workload Identity Federation (CI) | Model calls via Vertex AI |
| **GitLab CI_JOB_TOKEN** | `CI_JOB_TOKEN` | `GITLAB_TOKEN: $CI_JOB_TOKEN` in job variables | MR notes (simple comments only) |
| **GitLab PAT** | Project Access Token | Create in Settings → Access Tokens with `api` scope, store as masked CI/CD variable | MR notes + inline diff discussions |
| **OpenAI-compatible API key** | API key string | `--api-key` flag, `REVIEW_API_KEY` env, or `api_key` in yaml | Model calls via HTTP provider (no GCP needed) |
| **No auth (local Ollama)** | None | Just set `--api-url` | Model calls to local endpoint |

### Local Development Auth

```bash
# Authenticate with GCP for Vertex AI model calls
gcloud auth application-default login

# Set your project
export GOOGLE_CLOUD_PROJECT=my-gcp-project

# Run a local review
code-reviewer --diff
```

### CI/CD Auth (Workload Identity Federation)

For production CI, use [Workload Identity Federation](https://cloud.google.com/iam/docs/workload-identity-federation) instead of service account keys:

```yaml
# In your CI job
- id: auth
  uses: google-github-actions/auth@v2
  with:
    workload_identity_provider: projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/$POOL/providers/$PROVIDER
    service_account: code-reviewer@$PROJECT.iam.gserviceaccount.com
```

## Self-Hosted Models

Use `--api-url` to point at any OpenAI-compatible endpoint. No GCP project or ADC required.

### Ollama (Local)

```bash
# Start Ollama
ollama serve

# Pull a model
ollama pull qwen3:32b

# Run review
code-reviewer --diff --api-url http://localhost:11434/v1 --model qwen3:32b
```

### vLLM

```bash
# Start vLLM server
python -m vllm.entrypoints.openai.api_server --model meta-llama/Llama-4-Scout-17B-16E

# Run review
code-reviewer --diff --api-url http://gpu-server:8000/v1 --model meta-llama/Llama-4-Scout-17B-16E
```

### Gemma on Cloud Run

```bash
code-reviewer --diff --api-url https://gemma-review-xyz.run.app/v1 --model gemma-3-27b
```

### Candela Proxy

Route Vertex AI calls through [Candela](https://github.com/candelahq/candela) for observability, cost tracking, and routing:

```bash
# Via CLI flag
code-reviewer --diff --proxy-url http://localhost:8181/proxy/google/

# Via .code-reviewer.yaml
# proxy_url: http://localhost:8181/proxy/google/

# Via environment variable
export REVIEW_PROXY_URL=http://localhost:8181/proxy/google/
```

The proxy URL is passed to the Vertex AI client as a custom base URL. All model calls are routed through it transparently.

## Configuration Reference

Settings are applied in priority order: **CLI flags > env vars > `.code-reviewer.yaml` > defaults**.

### Complete Configuration Table

| Field (yaml) | Flag | Env Var | Default | Description |
|---|---|---|---|---|
| — | `--ci` | — | `false` | Run in GitLab CI mode (auto-detect MR from env) |
| — | `--diff [ref]` | — | — | Review local git diff (default ref: `origin/HEAD`) |
| — | `--files f1,f2` | — | — | Review specific files |
| `model` | `--model` | `REVIEW_MODEL` | `gemini-2.5-flash` | Vertex AI model ID |
| — | `--models` | `REVIEW_MODELS` | — | Comma-separated models for multi-model consensus |
| — | `--consensus-threshold` | — | `2` | Min models that must agree on a finding |
| `focus` | `--focus` | `REVIEW_FOCUS` | `all` | Review focus areas (comma-separated) |
| `min_severity` | `--min-severity` | `REVIEW_MIN_SEVERITY` | `low` | Minimum severity to report |
| `comment_mode` | `--comment-mode` | `REVIEW_COMMENT_MODE` | `notes` | GitLab comment mode: `notes` or `discussions` |
| `chunk_strategy` | `--chunk-strategy` | `REVIEW_CHUNK_STRATEGY` | `fail` | How to handle large diffs: `fail` or `split` |
| `extra_rules` | `--extra-rules` | `REVIEW_EXTRA_RULES` | — | Additional prompt rules (free text) |
| `custom_prompt` | `--custom-prompt` | `REVIEW_CUSTOM_PROMPT` | — | Path to custom system prompt file |
| — | `--incremental` | `INCREMENTAL` | `false` | Only review files changed in latest push (CI) |
| — | `--dry-run` | — | `false` | Analyze without posting to GitLab |
| `output_json` | `--json` | `REVIEW_OUTPUT_JSON` | `false` | Output results as JSON to stdout |
| — | `--no-color` | `NO_COLOR` | `false` | Disable ANSI color output |
| — | `--sarif` | `SARIF_OUTPUT` | — | Write SARIF 2.1.0 output to file path |
| — | `--no-context` | — | `false` | Disable repo-aware cross-file context |
| `max_tokens` | `--max-tokens` | `REVIEW_MAX_TOKENS` | `0` (unlimited) | Maximum total tokens per review |
| `api_url` | `--api-url` | `REVIEW_API_URL` | — | OpenAI-compatible API endpoint URL |
| — | `--api-key` | `REVIEW_API_KEY` | — | API key for HTTP provider |
| `proxy_url` | `--proxy-url` | `REVIEW_PROXY_URL` | — | LLM proxy URL (e.g., Candela) |
| `excluded_patterns` | — | `EXCLUDED_PATTERNS` | `go.sum,*.lock,vendor/*` | Glob patterns to exclude from review |
| — | — | `GOOGLE_CLOUD_PROJECT` | **Required**† | GCP project for Vertex AI |
| — | — | `GOOGLE_CLOUD_LOCATION` | `us-central1` | GCP region |
| — | — | `GITLAB_TOKEN` | Required in CI | GitLab API token |
| — | — | `GITLAB_BASE_URL` | `https://gitlab.com` | GitLab API base URL |

† Not required when using `--api-url` (self-hosted models).

### Per-Repo Config File

Create `.code-reviewer.yaml` (or `.code-reviewer.yml`) in your repo root:

```yaml
model: gemini-2.5-flash
focus: [bugs, security]
min_severity: low
comment_mode: discussions
chunk_strategy: fail
custom_prompt: prompts/team-rules.md
excluded_patterns:
  - "*.pb.go"
  - "generated/*"
extra_rules: |
  Always flag raw SQL string concatenation.
  Check that zerolog is used instead of log/fmt.
max_tokens: 50000
api_url: http://localhost:11434/v1
proxy_url: http://localhost:8181/proxy/google/
```

The file is discovered by walking up from the working directory to the filesystem root. See [`.code-reviewer.example.yaml`](../.code-reviewer.example.yaml) for a fully commented example.

## Troubleshooting

### Missing `GOOGLE_CLOUD_PROJECT`

```text
Error: GOOGLE_CLOUD_PROJECT is required for Vertex AI, or use --api-url for an OpenAI-compatible endpoint
```

**Fix:** Set the environment variable:

```bash
export GOOGLE_CLOUD_PROJECT=my-gcp-project
```

Or use `--api-url` for self-hosted models (no GCP project needed):

```bash
code-reviewer --diff --api-url http://localhost:11434/v1 --model qwen3:32b
```

### Token / Auth Errors

```text
Error: CI mode requires GITLAB_TOKEN env var
```

**Fix:** Add the token to your CI job variables:

```yaml
variables:
  GITLAB_TOKEN: $CI_JOB_TOKEN        # Simple notes
  # or
  GITLAB_TOKEN: $CODE_REVIEWER_TOKEN  # PAT for discussions
```

For Vertex AI auth errors, verify ADC:

```bash
gcloud auth application-default login
gcloud auth application-default print-access-token > /dev/null && echo 'ADC OK'
```

### Rate Limiting (429)

```text
Error: generating content: 429 Resource has been exhausted
```

code-reviewer has built-in retry with exponential backoff for 429, 502, 503, and 504 errors. If you're consistently hitting limits:

- Switch to a less contended model region (`GOOGLE_CLOUD_LOCATION`)
- Use `--max-tokens` to limit total tokens per review
- Use `--incremental` to review fewer files per run
- Add a rate limit to your CI pipeline (e.g., `resource_group` in GitLab)

### Context Window Exceeded

```text
Error: diff exceeds model context window (estimated: 245,000 tokens, limit: 128,000)
```

**Fix options:**

1. Use `--chunk-strategy split` to auto-split large diffs:
   ```bash
   code-reviewer --diff --chunk-strategy split
   ```
2. Use `--max-tokens` to cap total token usage:
   ```bash
   code-reviewer --diff --max-tokens 100000
   ```
3. Exclude noisy files:
   ```yaml
   excluded_patterns:
     - "*.pb.go"
     - "generated/*"
     - "testdata/*"
   ```
4. Use `--incremental` in CI to review only the latest push
5. Scope your MRs smaller (the default `fail` strategy intentionally encourages this)

### Empty Response from Model

```text
Error: empty response from model
```

This typically means the model returned no text content. Possible causes:

- The model's safety filters blocked the response (try a different model)
- The request was too large and the model couldn't generate a response
- Network timeout or transient error (retries should handle this)

**Fix:** Try with a different model or reduce the diff size:

```bash
code-reviewer --diff --model gemini-2.5-pro --chunk-strategy split
```

### HTTPS Required for GitLab

```text
Error: GITLAB_BASE_URL must use HTTPS to protect tokens
```

code-reviewer enforces HTTPS for GitLab API calls to prevent token leakage over plain HTTP. To override for local development:

```bash
export CODE_REVIEWER_ALLOW_INSECURE=true
```

> **Warning:** Never use `CODE_REVIEWER_ALLOW_INSECURE=true` in production CI — it exposes your GitLab token to network interception.

### CI Mode Outside of MR Pipeline

```text
Error: CI mode requires CI_PROJECT_ID and CI_MERGE_REQUEST_IID env vars
```

`--ci` mode only works inside GitLab MR pipelines where `CI_PROJECT_ID` and `CI_MERGE_REQUEST_IID` are automatically set. If running locally, use `--diff` instead:

```bash
code-reviewer --diff        # against origin/HEAD
code-reviewer --diff main   # against main branch
```
