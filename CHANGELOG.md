# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
