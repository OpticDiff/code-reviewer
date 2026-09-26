# C/C++ Review Checklist

## Focus
- **Memory Safety**: Buffer overflows (`strcpy`, `sprintf`, `gets`), use-after-free, double-free, null pointer dereference.
- **Resource Leaks**: `malloc`/`new` without corresponding `free`/`delete`, missing RAII wrappers in C++, unclosed file descriptors.
- **Integer Safety**: Integer overflow/underflow, signed/unsigned comparison, truncation on cast.
- **Concurrency**: Data races on shared state, missing locks, lock ordering violations, deadlocks.
- **Undefined Behavior**: Signed overflow, strict aliasing violations, accessing uninitialized memory, out-of-bounds access.
- **Modern C++**: Raw `new`/`delete` where smart pointers (`unique_ptr`, `shared_ptr`) are appropriate, `const` correctness violations.
- **Input Validation**: Format string vulnerabilities (`printf(user_input)`), path traversal, command injection via `system()` / `popen()`.

## DO NOT Flag
- Code style/formatting (handled by `clang-format`).
- Naming conventions (handled by linters).
- Header include order (handled by `include-what-you-use`).
- Compiler warnings already caught by `-Wall -Wextra`.
