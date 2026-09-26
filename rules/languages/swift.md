# Swift Review Checklist

## Focus
- **Memory Management**: Retain cycles from strong reference closures (missing `[weak self]` or `[unowned self]`), `deinit` never called.
- **Optionals**: Force unwrapping (`!`) without guard, implicit unwrapped optionals in non-IBOutlet contexts.
- **Concurrency**: Data races with Swift Concurrency (`actor` isolation violations), `@Sendable` conformance issues, main actor blocking.
- **Error Handling**: `try!` / `try?` swallowing errors silently, missing `do-catch` for throwing functions.
- **Access Control**: Overly permissive access (`public`/`open` where `internal` suffices), missing `final` on classes not designed for subclassing.
- **Value vs Reference**: Large structs copied frequently (should be class), mutable state in structs shared across threads.

## DO NOT Flag
- Swift style/formatting (handled by `swiftformat` / `swift-format`).
- Unused variables (handled by compiler).
- Import ordering.
