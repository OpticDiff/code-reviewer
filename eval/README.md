# Persona Evaluation Suite

This directory contains a curated set of diffs and platform governance configs
used to validate that the dual-review persona system correctly isolates findings
between platform and product profiles.

## Structure

```
eval/
├── README.md               # This file
├── eval_test.go             # Test harness (go test -tags=eval)
├── platform-rules.yaml      # Platform governance rules for eval
├── platform-guidelines.md   # Platform guidelines for eval
└── testdata/diffs/          # Curated test diffs
    ├── sql-injection.diff          # SQL injection vulnerability (platform)
    ├── hardcoded-secret.diff       # Hardcoded API key (platform)
    ├── missing-auth-check.diff     # Missing authorization (platform)
    ├── insecure-tls.diff           # Insecure TLS config (platform)
    ├── path-traversal.diff         # Path traversal vulnerability (platform)
    ├── xss-vulnerability.diff      # Cross-site scripting (platform)
    ├── race-condition.diff         # Data race in concurrent code (platform)
    ├── error-not-checked.diff      # Unchecked error return (product)
    ├── naming-convention.diff      # Poor variable naming (product)
    ├── dead-code.diff              # Unreachable code (product)
    ├── magic-numbers.diff          # Magic numbers / no constants (product)
    ├── missing-test.diff           # New public API without tests (product)
    ├── long-function.diff          # Function too long (product)
    ├── missing-docs.diff           # Missing godoc (product)
    ├── mixed-security-style.diff   # Both security + style issues (both)
```

## Running

```bash
# Requires GOOGLE_CLOUD_PROJECT set (uses real LLM inference)
nix develop -c go test -tags=eval ./eval/ -v -timeout=600s
```

## What It Validates

1. **Platform profile**: Should surface security/compliance findings but NOT
   style, naming, or documentation issues
2. **Product profile**: Should surface code quality, style, and best practice
   findings but NOT platform-rule-attributed findings
3. **All profile**: Should surface everything
4. **Category isolation**: Platform findings must have security/bug category;
   product findings must not reference platform rule names
