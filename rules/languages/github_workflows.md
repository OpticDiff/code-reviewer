# GitHub Workflows Review Checklist

## Focus
- **Injection**: Expression injection via `${{ github.event.*.body }}` in `run:` steps, untrusted `pull_request_target` with checkout of PR HEAD.
- **Permissions**: Overly broad `permissions:` (write-all), missing `permissions:` block (defaults to write), `contents: write` when only `read` is needed.
- **Pinning**: Unpinned third-party actions (`uses: actions/checkout@main` vs `@v4` or SHA pin), supply chain risk from mutable tags.
- **Secrets**: Secrets in `if:` conditions or outputs (logged), secrets passed to untrusted actions, `GITHUB_TOKEN` with excessive scope.
- **Triggers**: `pull_request_target` + `actions/checkout` of PR ref (code injection), `workflow_dispatch` without input validation.
- **Caching**: Cache poisoning via `actions/cache` with mutable keys, restore-keys matching stale entries.
- **Concurrency**: Missing `concurrency:` group for expensive workflows, redundant matrix builds.

## DO NOT Flag
- YAML formatting (handled by linters).
- Action version being latest stable (only flag unpinned `@main`/`@master`).
- Workflow naming conventions.
