# Error Handling Quality

## Focus
- Functions returning `error` MUST NOT silently discard errors with `_ = fn()`. Every error must be handled, logged, or explicitly documented.
- Error messages MUST include context: wrap with `fmt.Errorf("doing X: %w", err)` instead of returning raw errors.
- Sentinel errors (`var ErrNotFound = errors.New(...)`) SHOULD be used for errors that callers need to inspect.
- Panic MUST NOT be used for expected error conditions. Panics are only for programmer bugs (invariant violations).

## DO NOT Flag
- `_ = writer.Close()` in deferred cleanup where the primary operation already succeeded.
- Deliberate panics in `init()` for required configuration.
- Error returns in test helpers (tests should use `t.Fatal`).
