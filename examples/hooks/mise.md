# Managing code-reviewer with mise

[mise](https://mise.jdx.dev/) is a fast, polyglot development environment and task runner. You can use mise to install `code-reviewer` binaries automatically and define convenient review tasks for your team.

---

## 1. Installation via mise

`code-reviewer` is distributed via GitHub Releases and can be installed directly with `mise` using the Universal Binary Installer (`ubi:` backend).

### Global Installation

To install `code-reviewer` globally across all projects:

```bash
mise use -g ubi:OpticDiff/code-reviewer
```

### Project-Specific Version Pinning

To pin a specific version of `code-reviewer` for your repository, add it to `.mise.toml`:

```toml
[tools]
"ubi:OpticDiff/code-reviewer" = "0.7.0"
```

Then install the tools specified for the project:

```bash
mise install
```

---

## 2. Task Configuration in `.mise.toml`

Define standardized review commands under the `[tasks]` table in `.mise.toml` so team members can run identical review workflows:

```toml
[tools]
"ubi:OpticDiff/code-reviewer" = "0.7.0"

[env]
GOOGLE_CLOUD_PROJECT = "my-gcp-project"
GOOGLE_CLOUD_LOCATION = "us-central1"

# Standard branch review against origin/main
[tasks.review]
description = "Review uncommitted or branch diff against origin/main"
run = "code-reviewer --diff origin/main"

# Security-focused review with SARIF output
[tasks."review:security"]
description = "Run security-focused review and export SARIF report"
run = "code-reviewer --diff origin/main --focus security --sarif results.sarif --min-severity medium"

# Generate structured PR / MR summary
[tasks."review:summary"]
description = "Generate structured PR/MR description summary from diff"
run = "code-reviewer --diff origin/main --summarize"

# Apply AI fix suggestions directly to working tree
[tasks."review:fix"]
description = "Analyze diff and apply recommended code fixes"
run = "code-reviewer --diff origin/main --fix"

# Pre-push verification task
[tasks."review:pre-push"]
description = "Pre-push check: block on high or critical severity issues"
run = "code-reviewer --diff @{upstream} --min-severity high"
```

---

## 3. Running Tasks

Execute your configured tasks anywhere in the project root:

```bash
# Run general review
mise run review

# Run security review
mise run review:security

# Generate MR summary
mise run review:summary

# Apply fixes
mise run review:fix
```
