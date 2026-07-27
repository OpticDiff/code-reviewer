# Roadmap

Current status: **v0.6.0 — GitHub support, multi-line comments, code suggestions, and review lifecycle**

## ✅ v0.1 — Foundation (Done)

- Multi-model Vertex AI support (Gemini, Claude, Mistral via ADC)
- Configurable focus modes (bugs, security, performance, style, docs)
- Severity filtering (low → critical)
- GitLab integration (notes + inline discussions)
- Context window chunking (fail/split strategies)
- Multi-layer config (flags > env > `.code-reviewer.yaml` > defaults)
- `--diff`, `--files`, `--ci` input modes
- `--dry-run` for testing
- Idempotent bot comments with cleanup on re-push
- Clear error messages for missing credentials/config

## ✅ v0.2 — Production Hardening (Done)

- [x] **Retry with backoff** — Exponential backoff + jitter for Vertex AI rate limits (429/503)
- [x] **`--json` output** — Machine-readable output for downstream tooling
- [x] **Multi-model consensus** — Run multiple models in parallel, deduplicate by file+category+line proximity
- [x] **Goreleaser** — Multi-platform binary releases via GitHub Actions
- [x] **Config validation tests** — Unit tests for flags > env > yaml precedence, table-driven flag/env coverage
- [x] **Integration tests** — 10 end-to-end tests: mock model → verify GitLab API payloads, SARIF output, summary/explain/intent pipelines (PR #33)

## ✅ v0.3 — Reviewer Powers (Done)

- [x] **Incremental review** — Only review files changed in latest push via MR versions API (`--incremental`)
- [x] **SARIF output** — Write findings in SARIF 2.1.0 format for CI security tabs (`--sarif`)
- [x] **Cost/token tracking** — Log input/output tokens per call in terminal, JSON, and CI output
- [x] **GitLab hardening** — 429 retry for pagination, generic callback-based pagination, SSRF protection, context cancellation
- [x] **Custom prompts** — `--custom-prompt` for full prompt override (security audit, architecture, etc.)

## ✅ v0.4 — Smarter Reviews & Observability (Done)

- [x] **LLM proxy support** — `--proxy-url` / `REVIEW_PROXY_URL` for routing model calls through observability proxies (e.g., Candela)
- [x] **VCS interface abstraction** — Platform-agnostic `internal/vcs` types, enabling GitHub/Bitbucket support
- [x] **REVIEW.md** — Drop a `REVIEW.md` in your repo root; contents injected as highest-priority system prompt instruction (PR #19)
- [x] **Repo-aware context** — Tree-sitter extracts changed symbols, grep finds usages in unchanged files, injected as _Related Unchanged Code_. Supports Go, Kotlin, Java, Python, TypeScript. Opt-out via `--no-context` / `disable_context` (PR #20)

## ✅ v0.5 — Auto-Summary, Intent & Developer Tools (Done)

- [x] **Auto-summary** — `--summarize` generates structured MR descriptions from diffs: classification, intent, risk level, scope areas, breaking changes
- [x] **SummarizeProvider interface** — Both Vertex AI and HTTP providers support summarize mode via shared `generateRaw()` refactor
- [x] **Rich output** — Colored terminal display, JSON output, GitLab markdown comments
- [x] **Intent-aware review** — `--intent` enables two-pass review: infer intent (pass 1), review against it (pass 2). Auto-enabled in CI.
- [x] **Explain mode** — `--explain` generates a plain-language walkthrough of the diff instead of a review
- [x] **Fix mode** — `--fix` applies suggested code changes directly to the working tree
- [x] **Prompt quality pass** — 5 improvements to reduce false positives: precision penalty, zero-is-fine, focus-on-additions, tightened MEDIUM, suggestion guardrails (v0.5.1)
- [x] **Pre-push hook** — `code-reviewer hook install` sets up automatic review before `git push`; `.pre-commit-hooks.yaml` for pre-commit framework (v0.5.1)

## 🔜 v0.6 — Platform Expansion

- [x] **GitHub support** — `internal/github/` client implementing `VCSClient` interface, PR review comments, GitHub Actions integration
- [x] **Code suggestions** — Platform-specific suggestion rendering
- [x] **Resolve discussions mode** — Opt-in `cleanup_mode`
- [x] **MR/PR description update** — Update with review summary
- [x] **Multi-line comments and suggestions**
- [x] **GitLab Draft Notes API** — For single-notification reviews
- [x] **SSRF hardening and tech debt cleanup**
- [ ] **GitHub Actions workflow** — Drop-in `.github/workflows/code-review.yml` example
- [ ] **Pre-commit.com listing** — Register in the pre-commit hook registry for discovery

## 🏢 v1.0 — Compliance & Audit

- [ ] **Policy engine** — Define path-based policies in `.code-reviewer.yaml` (e.g., auth changes require security focus)
- [ ] **Scope enforcement** — Block MRs with scope creep above configurable threshold
- [ ] **Audit trail** — Structured JSONL log of all reviews: intent, classification, findings, token usage, timing
- [ ] **Multi-pass review** — First pass with Flash (fast/cheap), escalate flagged files to Pro (deep analysis)
- [ ] **RAG-based context** — Embed repo into a vector store for enterprise-scale cross-file context (beyond grep)
- [ ] **Import-aware resolution** — Per-language import graph traversal to find transitive dependencies of changed symbols
- [ ] **Reply to bot comments** — Monitor MR note webhooks, respond to follow-up questions ("why is this a problem?")
- [ ] **Caching** — Hash file diffs, skip re-review of unchanged files across pushes

## 💡 Ideas (Unplanned)

These are ideas we might explore, not committed:

- **VS Code extension** — Review current branch diff inline in the editor
- **Slack/Teams notifications** — Post review summaries to team channels
- **Metrics dashboard** — Track review coverage, common issue categories, team trends
- **Fine-tuned models** — Train on team-specific review patterns for higher-quality feedback
- **Test generation** — Suggest missing test cases for changed code paths
- **Bitbucket support** — PR comments via Bitbucket REST API

---

## Contributing

Want to work on any of these? Open an issue to discuss the approach before submitting a PR. See [LICENSE](LICENSE) for terms.
