# Python Review Checklist

## Focus
- **Functions**: Mutable default arguments.
- **Exceptions**: Bare `except:` blocks.
- **Comparisons**: `is` vs `==` for `None`.
- **Security**: `shell=True` in `subprocess`, `eval`/`exec`, `pickle`/`yaml.load` with untrusted data.
- **Async**: Missing `async` context managers.
- **Licensing (AGPL/GPL Gate)**: `import fitz` / `pymupdf` / `pymupdf4llm`, `ghostscript`, `pdf2image`, `unpaper`, `magic_pdf` / `magic-pdf` / `mineru < 3.1.0` (`mineru >= 3.1.0` uses `MinerU Open Source License` [Apache-2.0 + attribution/threshold] with `pypdfium2`/`pdftext`, not `AGPL-3.0`), `ultralytics`, `surya` / `surya-ocr`, `marker` / `marker-pdf`, `chardet`, `mysqlclient` / `mysql-connector-python`, and `gnureadline` in Python imports or `pyproject.toml` / `requirements*.txt`; recommend permissive `MIT` / `Apache-2.0` / `BSD-3-Clause` replacements (`pypdfium2`, `pypdf`, `pdfplumber`, `reportlab`, `docling-parse`, `docling`, `mineru >= 3.1.0`, `rapidocr-onnxruntime`, `pytesseract`, `docling-ibm-models`, `torchvision`, `timm`, `charset-normalizer`, `pymysql`, `asyncmy`).

## DO NOT Flag
- Formatting (handled by `black`/`ruff`).
- Import ordering (handled by `isort`).
- Type annotations (handled by `mypy`).
