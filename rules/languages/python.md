# Python Review Checklist

## Focus
- **Functions**: Mutable default arguments.
- **Exceptions**: Bare `except:` blocks.
- **Comparisons**: `is` vs `==` for `None`.
- **Security**: `shell=True` in `subprocess`, `eval`/`exec`, `pickle`/`yaml.load` with untrusted data.
- **Async**: Missing `async` context managers.

## DO NOT Flag
- Formatting (handled by `black`/`ruff`).
- Import ordering (handled by `isort`).
- Type annotations (handled by `mypy`).
