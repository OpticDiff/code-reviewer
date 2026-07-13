# Roadmap

Current status: **v0.3.0 — Production-ready with incremental review, SARIF, and multi-model consensus**

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
- [ ] **Config validation tests** — Unit tests for flags > env > yaml precedence (partially covered)
- [ ] **Integration tests** — Mock model responses → verify GitLab API payloads

## ✅ v0.3 — Reviewer Powers (Done)

- [x] **Incremental review** — Only review files changed in latest push via MR versions API (`--incremental`)
- [x] **SARIF output** — Write findings in SARIF 2.1.0 format for CI security tabs (`--sarif`)
- [x] **Cost/token tracking** — Log input/output tokens per call in terminal, JSON, and CI output
- [x] **GitLab hardening** — 429 retry for pagination, generic callback-based pagination, SSRF protection, context cancellation
- [x] **Custom prompts** — `--custom-prompt` for full prompt override (security audit, architecture, etc.)

## 🔜 v0.4 — Platform Expansion & Observability

- [x] **LLM proxy support** — `--proxy-url` / `REVIEW_PROXY_URL` for routing model calls through observability proxies (e.g., Candela)
- [ ] **VCS interface abstraction** — Platform-agnostic `internal/vcs` types, enabling GitHub/Bitbucket support
- [ ] **GitHub support** — New `internal/github/` client implementing same posting interface. Core engine unchanged
- [ ] **GitHub Actions integration** — Native `action.yml` for GitHub-hosted repos
- [ ] **Auto-approve / block MR** — Add `Approve()`/`Unapprove()` to GitLab client + `--approve-mode` flag

## 🧠 v0.5 — Smarter Reviews

- [ ] **Advanced chunk strategies** — Semantic chunking (group related files), AST-aware splitting, dependency-ordered review
- [ ] **Reply to bot comments** — Monitor MR note webhooks, respond to follow-up questions ("why is this a problem?")
- [ ] **Caching** — Hash file diffs, skip re-review of unchanged files across pushes
- [ ] **Multi-pass review** — First pass with Flash (fast/cheap), escalate flagged files to Pro (deep analysis)

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
