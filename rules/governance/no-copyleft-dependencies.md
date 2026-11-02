# no-copyleft-dependencies

## Focus
Flag any introduction of AGPL-3.0, SSPL, GPL (v2/v3), LGPL without static-linking exception, or non-commercial (`CC-BY-NC`, research-only) dependencies, packages, containers, or model weights in code imports, dependency manifests (`pyproject.toml`, `uv.lock`, `requirements*.txt`, `go.mod`, `package.json`, `Cargo.toml`, `build.gradle.kts`, `pom.xml`, `flake.nix`), or build scripts (`Dockerfile*`, `justfile`).

High-risk Python/AI/PDF/OCR traps and their approved permissive (`MIT` / `Apache-2.0` / `BSD`) drop-in replacements:
- **PDF Rendering & Parsing**: Flag `pymupdf`, `fitz`, `pymupdf4llm` (`AGPL-3.0`), `ghostscript` (`AGPL-3.0`), `pdf2image` (`poppler` `GPL`), `unpaper` (`GPL`). Recommend `pypdfium2` (`Apache-2.0`/`BSD-3-Clause`), `pypdf` (`BSD-3-Clause`), `pdfplumber` (`MIT`), `reportlab` (`BSD-3-Clause`), or `docling-parse` (`MIT`).
- **Document Layout & OCR**: Flag `magic-pdf` (`AGPL-3.0`), `mineru < 3.1.0` (`AGPL-3.0`), `surya-ocr` (`GPL-3.0` / restricted weights), `marker-pdf` (`GPL-3.0`). Note that `mineru >= 3.1.0` relicensed from `AGPL-3.0` to `LicenseRef-MinerU-Open-Source-License` (Apache-2.0 base with 100M MAU / $20M monthly revenue threshold and UI/docs attribution obligation; `PyMuPDF` and AGPL/CC-BY-NC models were removed in `3.0.0` in favor of `pypdfium2` and `pdftext`). Recommend `mineru >= 3.1.0` (with attribution), `docling` (`MIT`), `clinical-ocr-go`, `rapidocr-onnxruntime` (`Apache-2.0`), or `pytesseract` (`Apache-2.0`).
- **Computer Vision / Detection**: Flag `ultralytics` (`YOLOv8`/`YOLOv11`, `AGPL-3.0`). Recommend `docling-ibm-models` / `RT-DETR` (`MIT`/`Apache-2.0`), `torchvision` (`BSD-3-Clause`), or `timm` (`Apache-2.0`).
- **Utilities & Drivers**: Flag `chardet` (`LGPL-2.1` — use `charset-normalizer` [`MIT`]), `mysqlclient` / `mysql-connector-python` (`GPL-2.0` — use `pymysql` [`MIT`] or `asyncmy` [`Apache-2.0`]), `gnureadline` (`GPL-3.0`).

## DO NOT Flag
Standard exclusions apply (do not flag comments or documentation discussing licenses, removal of prohibited dependencies, or `mineru >= 3.1.0` when not importing legacy `magic_pdf`).

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/governance/open-source-licensing
