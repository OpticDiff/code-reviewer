# Java Review Checklist

## Focus
- **Null Safety**: NullPointerException (NPE) risks.
- **Thread Safety**: Check-then-act race conditions, unsafe lazy initialization.
- **Resource Management**: Resource leaks (prefer `try-with-resources`).
- **Security**: SQL injection risks (e.g., string concatenation in queries).
- **Performance**: N+1 query problems.
- **Logic Errors**: Missing `break` statements in `switch` blocks.

## DO NOT Flag
- Formatting and indentation (handled by `google-java-format`).
- Unused imports and variables (handled by IDE).
- Trivial accessor naming.
