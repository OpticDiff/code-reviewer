# Roadmap

Current status: **v0.1 — Core functionality complete** (Phase 1 done)

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

## 🔜 v0.2 — Production Hardening

- [ ] **Retry with backoff** — Exponential backoff + jitter for Vertex AI rate limits (429/503)
- [ ] **`--json` output** — Machine-readable output for downstream tooling
- [ ] **Config validation tests** — Unit tests for flags > env > yaml precedence
- [ ] **Integration tests** — Mock model responses → verify GitLab API payloads
- [ ] **Goreleaser** — Multi-platform binary releases via GitHub Actions

## 🔮 v0.3 — Reviewer Powers

- [ ] **Auto-approve / block MR** — Add `Approve()`/`Unapprove()` to GitLab client + `--approve-mode` flag. Enables security gating ("changes requested" workflow)
- [ ] **Incremental review** — Track last-reviewed commit SHA. Only review new changes since last run (avoid re-reviewing entire MR on every push)
- [ ] **Cost/token tracking** — Log input/output tokens per call. Aggregate in CI job output for budget visibility

## 🌱 v0.4 — Platform Expansion

- [ ] **GitHub support** — New `internal/github/` client implementing same posting interface. Core engine unchanged
- [ ] **GitHub Actions integration** — Native `action.yml` for GitHub-hosted repos
- [ ] **Bitbucket support** — PR comments via Bitbucket REST API

## 🧠 v0.5 — Smarter Reviews

- [ ] **Advanced chunk strategies** — Semantic chunking (group related files), AST-aware splitting, dependency-ordered review
- [ ] **Reply to bot comments** — Monitor MR note webhooks, respond to follow-up questions ("why is this a problem?")
- [ ] **Caching** — Hash file diffs, skip re-review of unchanged files across pushes
- [ ] **Custom model prompts** — Allow full prompt override via config for teams with specialized review needs
- [ ] **Multi-pass review** — First pass with Flash (fast/cheap), escalate flagged files to Pro (deep analysis)

## 💡 Ideas (Unplanned)

These are ideas we might explore, not committed:

- **Proto output schema** — Define `ReviewResult` as `.proto`, use `protojson` for serialization
- **VS Code extension** — Review current branch diff inline in the editor
- **Slack/Teams notifications** — Post review summaries to team channels
- **Metrics dashboard** — Track review coverage, common issue categories, team trends
- **Fine-tuned models** — Train on team-specific review patterns for higher-quality feedback
- **Test generation** — Suggest missing test cases for changed code paths

---

## Contributing

Want to work on any of these? Open an issue to discuss the approach before submitting a PR. See [LICENSE](LICENSE) for terms.
