# Rust Review Checklist

## Focus
- **Ownership & Borrowing**: Unnecessary clones, lifetime issues, moves after borrow.
- **Unsafe**: `unsafe` blocks without safety comments, undefined behavior risks.
- **Error Handling**: Unwrapping `Result`/`Option` in non-test code (`unwrap()`, `expect()` without justification).
- **Concurrency**: Data races via `Arc<Mutex>` misuse, deadlocks from lock ordering, `Send`/`Sync` violations.
- **Memory**: Buffer overflows in FFI code, use-after-free in unsafe blocks.
- **Performance**: Unnecessary allocations, `clone()` where references suffice, `Box<dyn>` where generics work.

## DO NOT Flag
- Clippy-level lints (handled by `cargo clippy`).
- Formatting (handled by `rustfmt`).
- Unused imports or variables (handled by compiler warnings).
