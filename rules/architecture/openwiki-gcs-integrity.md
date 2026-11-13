# openwiki-gcs-integrity

## Focus
OpenWiki architecture artifacts (`openwiki/**`: `index.md`, `log.md`, `pages/*.md`,
`.claims/*.json`, `.last-update.json`, `AGENTS.snippet.md`, `manifest.json`) are
published out-of-band to object storage (`<repo_tag>/openwiki/`) and symlinked
locally from `~/.cache/azra/openwiki/<repo_tag>/`. They must never be committed
directly to Git repositories, as doing so causes bot MR noise, `CODEOWNERS`
bottlenecks, and unnecessary container build or GitOps deploy pipeline triggers.

Flag violations including:
- Committing `openwiki/**` artifacts directly to Git.
- Removing `/openwiki` (or `openwiki/`) from `.gitignore` or `.dockerignore`.
- Deleting an existing `<!-- OPENWIKI:START -->...<!-- OPENWIKI:END -->` marker block
  from repository guidance or skill documentation (`SKILL.md`, `conventions.md`, `AGENTS.md`).

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/openwiki-gcs-integrity
