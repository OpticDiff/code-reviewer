# Kotlin Review Checklist

## Focus
- **Null Safety**: Platform type (`!`) misuse, unchecked casts to non-null, `!!` operator without justification.
- **Coroutines**: Missing `CoroutineExceptionHandler`, unstructured concurrency (`GlobalScope`), `runBlocking` on main thread.
- **Resource Management**: Missing `use {}` for `Closeable` resources, unclosed streams.
- **Collections**: Mutable collections exposed from public APIs, `ConcurrentModificationException` risks.
- **Data Classes**: Mutable properties in data classes used as map keys, missing `copy()` for defensive copies.
- **Java Interop**: Nullable returns from Java APIs consumed without null checks, `@JvmStatic`/`@JvmOverloads` misuse.

## DO NOT Flag
- Code style (handled by `ktlint` / `detekt`).
- Import ordering.
- Named argument style preferences.
