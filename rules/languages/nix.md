# Nix Review Checklist

## Focus
- **Reproducibility**: Missing `flake.lock` (in flake-based projects only).
- **Dependencies**: Unfree packages without `allowUnfree`, fetchurl without hash, hash mismatches.
- **Safety**: Impure builds.
- **Metadata**: Missing `meta.license`.

## DO NOT Flag
- Formatting (handled by `nixfmt`).
- Attribute ordering.
