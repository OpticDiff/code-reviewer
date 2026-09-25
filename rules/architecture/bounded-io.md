# Bounded I/O

## Focus
- Every `io.ReadAll` and `io.Copy` on external input MUST use `io.LimitReader` with an explicit byte cap.
- Every `http.Request.Body` read MUST be bounded (use `http.MaxBytesReader` or `io.LimitReader`).
- File reads from user-configurable paths MUST use `os.OpenRoot` (Go ≥1.24) to prevent directory traversal.
- Buffer pools (`sync.Pool`) for large allocations SHOULD cap individual buffer size.

## DO NOT Flag
- Reads from embedded/compiled-in data (`embed.FS`).
- Reads from trusted, fixed-size sources (e.g., reading a known small config from a hardcoded path).
- Test files.
