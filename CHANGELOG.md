# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Findings whose thread was replied to (GitHub, GitLab) or resolved (GitLab) by someone other than the tool are no longer re-posted while the code under them is unchanged. Only threads opened by the tool's own account count, author-only dismissals never silence HIGH/CRITICAL findings, and suppressed HIGH/CRITICAL findings still count toward the exit status when the model emits them. Inline comments carry a hidden fingerprint, also exported to SARIF `partialFingerprints`.
- Optional `temperature` configuration field, `--temperature` CLI flag, and `REVIEW_TEMPERATURE` environment variable to set the sampling temperature of review calls. Unset keeps the built-in 0.2.
- Findings on removed lines. The model can report a defect introduced by a deletion (a removed guard or check) with `old_line`; the comment is anchored on the old side only (`position.old_line` on GitLab, `side: LEFT` on GitHub), without suggestions or ranges, and falls back to a merge request note naming the file and old line when GitLab refuses the position. Removed-line findings are left out of the SARIF and GitLab SAST reports, which need a location in the new file. `agent_verdicts` audit entries carry an optional `old_line` for such findings.

## [0.23.0] — 2026-10-08

### Added
- `confidence_floor` configuration field, `--confidence-floor` CLI flag, and `REVIEW_CONFIDENCE_FLOOR` environment variable to change the 80% confidence threshold in the built-in system prompt. Unset keeps the prompt unchanged.
- `intent_checks` configuration to set the severity (or turn off) of each intent-aware rule, choose where breaking changes count as documented, and optionally downgrade `missing_tests` on stacked merge requests (`stack_aware`).
- Explicit `token_limit` configuration field, `--token-limit` CLI flag, and `REVIEW_TOKEN_LIMIT` environment variable to override context window limits for diff chunking without code changes.

- Review coverage reporting: files with no reviewable patch (`empty_patch`, `collapsed`, `too_large`), files dropped by the token budget (`budget`), and parse failures (`parse_error`) are now listed in the audit log (`files_skipped`, plus `files_skipped_detail` with a reason per file), block `--auto-approve` (except `empty_patch`, see below), and add a "Reviewed N of M files. Not reviewed: ..." line to the summary. Files matching `excluded_patterns` are not counted. `empty_patch` is reported but does not block auto-approve, because binary and truncated patches are indistinguishable on GitLab's legacy `/changes` endpoint; `too_large` and `collapsed` (GitLab diffs endpoint) block. On GitHub, oversized files are detected through the files API `changes` count and block.

### Changed
- Suggestions are checked against the diff before posting. A suggestion that repeats lines directly above or below its range, or whose range does not contain the code the finding quotes, is dropped; the finding is still posted and the count is recorded as `suggestions_dropped` in the audit log.
- When a diff is split into chunks, every chunk now starts with the list of all changed files and their added/removed line counts, and the review summary combines the summaries of all chunks instead of keeping only the first.
- Default model is now `gemini-3.8-flash` and the default `GOOGLE_CLOUD_LOCATION` is `global`. `gemini-2.5-flash` retires on Vertex AI on 2026-10-20, and Gemini 3.x is not served from `us-central1`.
- Token limits for the current Gemini 3.x and Claude models; `gemini-2.0-flash` (shut down) removed.

### Fixed
- Non-ASCII source text (ligatures, soft hyphens, emoji, CJK) in findings and suggestions was corrupted by the model-response JSON repair step, which re-encoded UTF-8 bytes as Latin-1. Suggestions that still re-emit source text as Latin-1 mojibake are now dropped (the finding is kept).
- GitLab merge requests are read from the paginated `/diffs` endpoint instead of the deprecated `/changes`, which returned empty patches for large files. Collapsed files are recovered from `/raw_diffs`; files GitLab cannot supply are reported as not reviewed (and block auto-approval) instead of being reviewed as empty, and files missing relative to the merge request's `changes_count` are reported as unreviewed too. The `collapsed`/`too_large` flags need GitLab 18.4 or newer; on older instances empty diffs of modified files are checked against `/raw_diffs` and a warning is logged.
- Vertex AI snapshot IDs such as `claude-haiku-4-5@20251001` resolve to their base model's token limit instead of the 128k fallback.
- GitLab inline comments on unchanged diff lines now send both `old_line` and `new_line`. Previously only `new_line` was sent, the draft was accepted, and `bulk_publish` then failed, so the whole review was lost.
- The "Config File Modified" finding is anchored to the first added line of the config file instead of an unchanged context line.
- GitLab inline comments on renamed files send the pre-rename path as `old_path`, and findings replayed from the review cache get their `old_line` like fresh ones.

## [0.7.0] — 2026-08-24

### Added

### Changed

### Fixed

## [0.6.0] — 2026-07-27

### Added
- Multi-line comment and suggestion support (#46)
- Inject review summary into MR/PR description (#45)
- Platform-specific code suggestion rendering (#42)
- Opt-in resolve mode for previous review cleanup (`cleanup_mode`) (#43)
- GitLab Draft Notes for single-notification reviews
- GitHub VCS client and platform auto-detection (#35)
- GitHub Review API hardening against production edge cases (#41)

### Changed
- Add `SubmitReview` to `VCSClient`, move orchestration into client
- Address tech debt from CodeRabbit reviews (#36, #37, #38, #39)

### Fixed
- Clear `GITHUB_ACTIONS` in `ci_without_project_id` test

## [0.5.2] — 2026-07-25

### Added
- 10 integration tests for end-to-end pipeline verification (#33)

## [0.5.1] — 2026-07-25

### Added
- Pre-push hook with install/uninstall commands
- Core.hooksPath test coverage

### Changed
- Reduce false positives with 5 prompt quality improvements

### Fixed
- CI lint failures and CodeRabbit review findings

## [0.5.0] — 2026-07-20

### Added
- `--fix` mode — auto-apply suggestions to working tree
- `--explain` mode — explain diffs instead of reviewing
- Two-pass intent-aware review (v0.6 preview)
- Auto-summary mode (`--summarize`)
- `REVIEW.md` — repo-level review instructions with highest prompt priority

### Changed
- Consolidate fix tests into table-driven format

### Fixed
- Improve suggestion quality with prompt rules and sanitization
- Critical bugs in suggestion sanitizer
- Unconditional count assertions and fail on `ReadFile` error
- Security: adversarial input guardrails + terminal sanitization
