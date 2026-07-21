#!/usr/bin/env bash
# Updates the vendorHash in flake.nix to match the current go.sum.
#
# Usage: ./scripts/update-vendor-hash.sh
#
# How it works:
#   1. Temporarily sets vendorHash to an empty string
#   2. Runs `nix build` which fails but prints the correct hash
#   3. Patches flake.nix with the correct hash
#   4. Restores flake.nix if anything goes wrong

set -euo pipefail

FLAKE="flake.nix"

if [[ ! -f "$FLAKE" ]]; then
  echo "Error: $FLAKE not found. Run this from the repo root." >&2
  exit 1
fi

# Save the current hash so we can restore on failure
current_hash=$(grep -oP 'vendorHash = "\K[^"]+' "$FLAKE")
echo "Current vendorHash: $current_hash"

# Temporarily blank the hash to force Nix to compute the correct one
sed -i.bak "s|vendorHash = \"$current_hash\"|vendorHash = \"\"|" "$FLAKE"

restore() {
  echo "Restoring original vendorHash..."
  mv "$FLAKE.bak" "$FLAKE"
}
trap restore ERR

# Build and capture the correct hash from the error output
echo "Computing new vendorHash (this will download Go modules)..."
new_hash=$(nix build 2>&1 | grep -oP 'got:\s+\K\S+' || true)

if [[ -z "$new_hash" ]]; then
  echo "Error: could not determine new hash. Was vendorHash already correct?" >&2
  restore
  exit 1
fi

# Patch flake.nix with the correct hash
sed -i.bak "s|vendorHash = \"\"|vendorHash = \"$new_hash\"|" "$FLAKE"
rm -f "$FLAKE.bak"

echo "Updated vendorHash: $new_hash"
echo "Done! Verify with: nix build"
