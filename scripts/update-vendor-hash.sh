#!/usr/bin/env bash
# Updates the vendorHash in flake.nix to match the current go.sum.
#
# Usage: ./scripts/update-vendor-hash.sh
#
# How it works:
#   1. Saves a backup of flake.nix
#   2. Temporarily sets vendorHash to an empty string
#   3. Runs `nix build` which fails but prints the correct hash
#   4. Patches flake.nix with the correct hash
#   5. Restores flake.nix from backup if anything goes wrong

set -euo pipefail

FLAKE="flake.nix"

if [[ ! -f "$FLAKE" ]]; then
  echo "Error: $FLAKE not found. Run this from the repo root." >&2
  exit 1
fi

# Save the original file so we can always restore it on failure
backup=$(mktemp)
cp "$FLAKE" "$backup"

success=false
cleanup() {
  if [[ "$success" != true ]]; then
    echo "Restoring original $FLAKE..."
    cp "$backup" "$FLAKE"
  fi
  rm -f "$backup"
}
trap cleanup EXIT INT TERM

# Extract the current hash (portable — no grep -P)
current_hash=$(sed -n 's/.*vendorHash = "\([^"]*\)".*/\1/p' "$FLAKE")
echo "Current vendorHash: $current_hash"

# Temporarily blank the hash to force Nix to compute the correct one
sed -i.bak "s|vendorHash = \"$current_hash\"|vendorHash = \"\"|" "$FLAKE"
rm -f "$FLAKE.bak"

# Build and capture the correct hash from the error output
echo "Computing new vendorHash (this will download Go modules)..."
new_hash=$(nix build 2>&1 | sed -n 's/.*got: *\([^ ]*\).*/\1/p' || true)

if [[ -z "$new_hash" ]]; then
  echo "Error: could not determine new hash. Was vendorHash already correct?" >&2
  exit 1
fi

# Patch flake.nix with the correct hash
sed -i.bak "s|vendorHash = \"\"|vendorHash = \"$new_hash\"|" "$FLAKE"
rm -f "$FLAKE.bak"

success=true
echo "Updated vendorHash: $new_hash"
echo "Done! Verify with: nix build"
