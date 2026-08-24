# Examples

Quick-start examples for integrating code-reviewer into your CI/CD pipeline and git hooks.

## Where to Start

| I want to... | Start here |
|---|---|
| Review PRs on GitHub | [github/basic.yml](github/basic.yml) |
| Review MRs on GitLab | [gitlab/basic.yml](gitlab/basic.yml) |
| Use a self-hosted model (no cloud) | [github/self-hosted.yml](github/self-hosted.yml) |
| Use AWS Bedrock | [bedrock/litellm.md](bedrock/litellm.md) |
| Add a pre-push hook | [hooks/lefthook.yml](hooks/lefthook.yml) |

---

## Directory Overview

### GitHub Actions (`examples/github/`)

- [**`basic.yml`**](github/basic.yml) — Minimal PR review workflow using the reusable action and Google Cloud Workload Identity Federation (WIF).
- [**`sarif.yml`**](github/sarif.yml) — PR review with SARIF output uploaded to GitHub Code Scanning (Security alerts tab).
- [**`self-hosted.yml`**](github/self-hosted.yml) — Fully local review using an Ollama service sidecar container without cloud credentials.
- [**`consensus.yml`**](github/consensus.yml) — Multi-model consensus review running Gemini + Claude in parallel to reduce false positives.
- [**`bedrock.yml`**](github/bedrock.yml) — AWS Bedrock integration via a LiteLLM proxy sidecar service container.

### GitLab CI (`examples/gitlab/`)

- [**`basic.yml`**](gitlab/basic.yml) — Minimal GitLab CI job using `CI_JOB_TOKEN` (zero configuration) with high-level MR notes.
- [**`discussions.yml`**](gitlab/discussions.yml) — GitLab CI job using a Project Access Token (PAT) for inline line-anchored discussions.
- [**`sarif.yml`**](gitlab/sarif.yml) — Security review generating a SARIF report ingested into GitLab's SAST / Security tab.
- [**`template.yml`**](gitlab/template.yml) — Reusable hidden job (`.code-review`) designed to be shared and extended across projects.

### Git Hooks & Developer Tools (`examples/hooks/`)

- [**`lefthook.yml`**](hooks/lefthook.yml) — Fast pre-push hook configuration with Lefthook to block high-severity issues locally.
- [**`husky.md`**](hooks/husky.md) — Guide for configuring pre-push hooks using Husky in JavaScript/TypeScript/polyglot repositories.
- [**`mise.md`**](hooks/mise.md) — Guide for installing `code-reviewer` and defining review tasks via `.mise.toml`.

### AWS Bedrock Integration (`examples/bedrock/`)

- [**`litellm.md`**](bedrock/litellm.md) — Detailed guide for routing code-reviewer through a LiteLLM proxy to AWS Bedrock models.
- [**`access-gateway.md`**](bedrock/access-gateway.md) — Enterprise guide for using a centralized AWS Bedrock Access Gateway endpoint.

### Custom System Prompts (`examples/prompts/`)

- [**`security-audit.md`**](prompts/security-audit.md) — Security audit prompt targeting injection, auth, crypto, and PII leaks.
- [**`strict.md`**](prompts/strict.md) — Zero-tolerance prompt enforcing strict idiomatic patterns and error handling.
- [**`quick.md`**](prompts/quick.md) — Fast triage prompt reporting only high and critical bugs.
- [**`architecture.md`**](prompts/architecture.md) — Architecture and design pattern review prompt.

---

## Model Quality Tiers

`code-reviewer` supports Google Vertex AI models, AWS Bedrock models (via LiteLLM / Access Gateway), and any self-hosted OpenAI-compatible endpoint (Ollama, vLLM, Cloud Run).

| Tier | Model | Provider / Engine | Recommended For | Strengths & Trade-offs |
|---|---|---|---|---|
| **Tier 1: Recommended** | `gemini-2.5-flash` | Google Vertex AI | Default choice for CI/CD PR/MR reviews | **Fastest, highly cost-effective, 1M+ context window**. Exceptional balance of speed and code reasoning. |
| **Tier 1: Deep Analysis** | `gemini-2.5-pro` | Google Vertex AI | Deep security audits, complex refactoring | **Highest reasoning depth**, unmatched context comprehension across complex multi-file changes. |
| **Tier 1: Code Specialist** | `claude-sonnet-4` | Vertex AI / Bedrock | Code quality, subtle bug detection, consensus pairs | **Premier code analysis quality**, highly accurate suggestion blocks; great pairing with Gemini in consensus mode. |
| **Tier 2: Self-Hosted (Large)** | `qwen3:32b` / `qwen2.5-coder:32b` | Ollama / vLLM (On-Prem) | Air-gapped / Private deployments | **Strong code intelligence without cloud egress**. Requires dedicated GPU host (24GB+ VRAM). |
| **Tier 2: Self-Hosted (Cloud)** | `gemma-3-27b` | Cloud Run / vLLM | Self-hosted cloud instances | Good balance of latency and inference cost for private VPC inference. |
| **Tier 3: Demo & Local Triage** | `qwen3:8b` / `qwen2.5-coder:7b` | Ollama / Local CPU/GPU | Local developer testing, quick CI smoke tests | **Lightweight, runs on laptops / standard CI runners**. Lower reasoning depth; may yield more false positives than 32B+ models. |
