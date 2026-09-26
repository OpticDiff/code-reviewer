# Markdown Review Checklist

## Focus
- **Links**: Broken relative links, links to non-existent anchors, HTTP links where HTTPS is available.
- **Structure**: Missing or duplicate headings, heading level skips (h1 → h3), empty sections.
- **Code Blocks**: Missing language identifier on fenced code blocks, outdated/incorrect code examples.
- **Security**: Credentials, API keys, or internal URLs in documentation, PII in examples.
- **Accuracy**: README instructions that don't match actual CLI flags or API, outdated version references, stale badges.

## DO NOT Flag
- Prose style, grammar, or tone (subjective).
- Line length (handled by `markdownlint`).
- Trailing whitespace or blank lines (handled by linters).
- Markdown formatting preferences (handled by `prettier` / `markdownlint`).
