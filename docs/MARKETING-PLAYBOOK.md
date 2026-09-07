# OpticDiff & code-reviewer: Launch & Developer Growth Playbook

This playbook provides copy-paste promotional posts, launch announcements, and community listing submissions to accelerate developer adoption for **OpticDiff** and **`code-reviewer`**.

---

## 1. Hacker News: Show HN

**Timing**: Tuesday or Wednesday at 6:30 AM – 8:00 AM PT (peak traffic window for technical launches).

**Title**: 
`Show HN: code-reviewer – Local-first, sovereign AI code review CLI and pre-push hook`

**Post Body**:

```markdown
Hi HN! I’m Austin, the creator of code-reviewer (https://github.com/OpticDiff/code-reviewer).

Like many teams, we wanted the benefits of AI code reviews, but existing SaaS solutions (CodeRabbit, GitHub Copilot PR review, etc.) presented three dealbreakers:
1. Data sovereignty & security: Corporate policies forbid sending proprietary code diffs to third-party cloud bots.
2. Noise and hallucinations: AI bots posting 20 pedantic comments per PR, causing developers to ignore all notifications.
3. CI-only latency: Having to push to remote and wait 5 minutes in CI just to discover basic bugs.

We built `code-reviewer` to be an open-source, local-first alternative that you fully control:

- 100% Local & Sovereign: Runs completely offline using Ollama or vLLM on your workstation or private GPU runner. If you do use cloud models (Vertex AI via Application Default Credentials / Workload Identity, or AWS Bedrock via LiteLLM), requests go straight to your VPC. Zero code is ever retained by an external SaaS vendor.
- Multi-Model Consensus: Run multiple models in parallel (e.g., Gemini Flash + Claude Sonnet). The CLI cross-references findings and only surfaces issues that multiple independent models agree on (deduplicating by AST symbol and line proximity). This drops hallucinated nits close to zero.
- Shift-Left: Run `code-reviewer --diff` directly in your terminal while coding, or install the pre-push hook (`code-reviewer hook install`) to catch security issues and nil pointer dereferences before your code ever leaves your machine.
- Repo-Aware Tree-Sitter Context: Built with pure-Go Tree-sitter (no CGo). When a function signature changes in your diff, it searches unchanged files for usages and injects those call sites into the prompt for semantic cross-file context.
- Smart Diff Caching: File diffs and prompts are content-hashed. Iterative PR pushes only review newly touched files, skipping LLM calls for unchanged files and drastically reducing token costs and latency.
- Dual Parity (GitHub & GitLab): Native support for inline diff comments, multi-line code suggestions, GitLab draft notes, SARIF 2.1.0 output for GitHub Code Scanning, GitLab SAST, and SHA-pinned `--auto-approve` with 9 automated safety checks.
- Single Static Go Binary: Packaged for Homebrew (`brew install OpticDiff/tap/code-reviewer`), Nix Flakes, Mise, Docker, or direct GitHub releases.

GitHub Action: https://github.com/OpticDiff/code-reviewer-action
Homebrew Tap: https://github.com/OpticDiff/homebrew-tap

The project is Apache 2.0. I'd love your feedback on the consensus engine, local review workflows, and prompt layers!
```

---

## 2. Reddit Playbook

### A. r/golang
**Target**: Go community, focusing on clean engineering, pure Go Tree-sitter, concurrency, and developer tooling.

**Title**: 
`I built an open-source, local-first AI code review CLI in Go (Tree-sitter, multi-model consensus, pre-push hook)`

**Post Body**:

```markdown
Hey r/golang!

Over the past few months I've been building `code-reviewer` (https://github.com/OpticDiff/code-reviewer), an Apache 2.0 CLI and pre-push hook for automated code reviews.

A few technical details that might interest this community:

1. Pure-Go Tree-Sitter (No CGo):
To provide cross-file awareness without requiring heavy vector databases or language servers, we use pure-Go tree-sitter bindings (`smacker/go-tree-sitter`) to parse diffs across Go, Kotlin, Java, Python, and TypeScript. It extracts defined/modified symbols and searches the rest of the repository for usages in unchanged files, feeding relevant call sites to the LLM.

2. Multi-Model Consensus:
One of the biggest issues with LLM code reviews is noise. You can pass `--models gemini-2.5-flash,claude-sonnet-4` and `--consensus-threshold 2`. The tool executes the reviews concurrently via goroutines, normalizes findings, and deduplicates by file path, category, and line proximity (±3 lines). Only findings confirmed by both models survive.

3. Shift-Left & Pre-push Hook:
Instead of waiting for GitHub Actions or GitLab CI, `code-reviewer hook install` installs a Git pre-push hook (or integrates with `.pre-commit-config.yaml`). If an uncommitted bug is high/critical, it halts the push locally.

4. Fast Diff Caching:
We hash the file diff, model ID, prompt schema, and rules. Subsequent runs skip unchanged files, making reviews nearly instantaneous on incremental pushes.

Available via `brew install OpticDiff/tap/code-reviewer` or `nix run github:OpticDiff/code-reviewer`.

Code: https://github.com/OpticDiff/code-reviewer

Would love to hear your thoughts or see PRs/issues!
```

### B. r/selfhosted
**Target**: Privacy-conscious developers, Ollama users, homelab runners.

**Title**: 
`code-reviewer: 100% self-hosted AI code review CLI and GitHub/GitLab action (Ollama / vLLM)`

**Summary**:
Highlight that zero code touches any external SaaS. Walk through running `ollama run qwen3:8b` and reviewing diffs completely offline with zero API keys or monthly subscription fees.

### C. r/devops
**Target**: Platform engineers, CI/CD maintainers, GitLab & GitHub admins.

**Title**: 
`Bringing local-first, consensus-based AI code reviews to GitHub Actions and GitLab CI (with SARIF & SAST reporting)`

**Summary**:
Highlight dual-platform parity (GitLab Draft Notes API, GitHub PR Suggestions), Workload Identity Federation (no static API keys), SARIF 2.1.0 for GitHub Code Scanning, GitLab SAST artifacts, and the 9 safety guards in `--auto-approve`.

---

## 3. Curated Lists (Awesome Lists)

### A. awesome-go
Submit PR to `avelino/awesome-go` under **Development Tools / Code Analysis and Linters**:
```markdown
* [code-reviewer](https://github.com/OpticDiff/code-reviewer) - Fast, local-first AI code review CLI & pre-push hook for GitHub PRs and GitLab MRs with Tree-sitter context, multi-model consensus, and SARIF export.
```

### B. pre-commit.com Hook Registry
Submit PR or register `OpticDiff/code-reviewer` in the pre-commit hook repository:
```yaml
- repo: https://github.com/OpticDiff/code-reviewer
  rev: v0.8.0
  hooks:
    - id: code-review
      stages: [pre-push]
      args: [--min-severity, high]
```

### C. awesome-github-actions
Submit PR to `sdras/awesome-actions` under **Utilities / Code Quality**:
```markdown
* [code-reviewer-action](https://github.com/OpticDiff/code-reviewer-action) - Reusable action for AI-powered code review with Ollama, Vertex AI, AWS Bedrock, native suggestions, and SARIF upload.
```

---

## 4. Twitter / X Announcement Thread

**Tweet 1**:
> 🚀 Introducing code-reviewer v0.8.0 by @OpticDiff:
> 
> The local-first, noise-free AI code review CLI & pre-push hook for GitHub PRs & GitLab MRs.
> 
> 🔒 100% offline with Ollama
> 🎯 Multi-model consensus (zero hallucinations)
> 🌳 Tree-sitter repo context
> ⚡ Content-addressed diff caching
> 
> 100% open source. Thread 👇
> https://github.com/OpticDiff/code-reviewer

**Tweet 2**:
> 1/ Why another AI reviewer?
> 
> Most AI review bots are closed SaaS that require sending proprietary code to 3rd-party servers, only run *after* you push to CI, and spam your PR with pedantic nits.
> 
> We wanted something private, shift-left, and with actual signal.

**Tweet 3**:
> 2/ Multi-Model Consensus 🎯
> 
> Run Gemini + Claude in parallel. `code-reviewer` analyzes both ASTs and only keeps findings that both models independently verify.
> 
> Pedantic hallucinations drop to near zero.

**Tweet 4**:
> 3/ Catch bugs BEFORE pushing ⚡
> 
> Run `code-reviewer --diff` in your terminal, or `code-reviewer hook install` to add a pre-push hook.
> 
> Catch nil pointer dereferences and auth flaws on your machine before anyone else sees them.

**Tweet 5**:
> 4/ Install now in 1 second:
> 
> `brew install OpticDiff/tap/code-reviewer`
> Or `nix run github:OpticDiff/code-reviewer`
> 
> ⭐ Star the repo and try it on your current branch:
> https://github.com/OpticDiff/code-reviewer
