# temporal-determinism

## Focus
Temporal workflows must be deterministic. Flag non-deterministic
operations inside workflow definitions including:
- Direct goroutine creation (use workflow.Go instead)
- System clock access (use workflow.Now instead of time.Now)
- Random number generation (use workflow.SideEffect)
- Direct HTTP/network calls (must be wrapped in Activities)
- Direct file I/O (must be wrapped in Activities)
- Map iteration where order matters (non-deterministic in Go)

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/temporal-workflows
