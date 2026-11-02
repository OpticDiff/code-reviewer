# Python Review Checklist

## Focus
- **Functions**: Mutable default arguments.
- **Exceptions**: Bare `except:` blocks.
- **Comparisons**: `is` vs `==` for `None`.
- **Security**: `shell=True` in `subprocess`, `eval`/`exec`, `pickle`/`yaml.load` with untrusted data.
- **Async**: Missing `async` context managers.
- **Licensing (AGPL/GPL Gate)**: `import fitz` / `pymupdf` / `pymupdf4llm`, `mineru` / `magic_pdf`, `ultralytics`, `surya`, `marker`, `chardet`, and `mysqlclient` in Python imports or `pyproject.toml` / `requirements*.txt`; recommend permissive `MIT` / `Apache-2.0` / `BSD-3-Clause` replacements (`pypdfium2`, `pypdf`, `pdfplumber`, `docling`, `charset-normalizer`).

## DO NOT Flag
- Formatting (handled by `black`/`ruff`).
- Import ordering (handled by `isort`).
- Type annotations (handled by `mypy`).
