# Go Review Checklist

## Focus
- **Concurrency**: Goroutine leaks, concurrent map writes, `sync.Mutex` copying by value.
- **Memory & Pointers**: Nil pointer dereferences.
- **Loops**: Using `defer` inside loops (can lead to resource exhaustion).
- **Error Handling**: Silent errors (`_ = err`), unchecked returns from functions returning errors.
- **Resource Management**: Timer leaks, unclosed resources (e.g., `sql.Rows.Close()`, `http.Response.Body.Close()`).
- **Unbounded Reads**: `io.ReadAll`, `io.Copy` without `io.LimitReader` on untrusted/variable-size input.
- **Path Traversal**: `os.ReadFile`, `os.Open`, `os.Create` with paths derived from user input or config without `os.OpenRoot` scoping (Go ≥1.24). Note: `filepath.Clean` + prefix check is NOT sufficient — it is purely lexical and does not prevent symlink escapes.
- **Context Propagation**: Functions accepting `context.Context` that spawn goroutines without deriving a child context.
- **Deferred Locks**: `sync.Mutex.Lock()` without corresponding `defer Unlock()` in the same scope, or `RLock/RUnlock` mismatch.

## DO NOT Flag
- Style or formatting choices (handled by `gofmt` / `goimports`).
- Unused variables or imports (handled by `go vet` and compiler).
- Static types and basic syntax (handled by compiler).
- Race conditions that `go test -race` would reliably detect in exercised paths.
- General linting rules already covered by `staticcheck`.
