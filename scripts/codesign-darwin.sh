#!/usr/bin/env bash
# Signs and notarizes a darwin Mach-O binary via rcodesign, invoked as a
# goreleaser post-build hook for every build target. Non-Mach-O artifacts
# (linux binaries) are a no-op. Missing credentials are a warning, not a
# failure, so local/snapshot builds without secrets still succeed.
set -euo pipefail

bin="$1"

if ! file "$bin" | grep -q "Mach-O"; then
  exit 0
fi

missing=()
for var in RCODESIGN_P12_PATH RCODESIGN_P12_PASSWORD_FILE RCODESIGN_TEAM_ID RCODESIGN_API_KEY_JSON; do
  [ -n "${!var:-}" ] || missing+=("$var")
done
if [ "${#missing[@]}" -gt 0 ]; then
  echo "codesign-darwin.sh: skipping $bin, unset: ${missing[*]}" >&2
  exit 0
fi

rcodesign sign \
  --p12-file "$RCODESIGN_P12_PATH" \
  --p12-password-file "$RCODESIGN_P12_PASSWORD_FILE" \
  --team-name "$RCODESIGN_TEAM_ID" \
  --for-notarization \
  "$bin"

rcodesign notary-submit \
  --api-key-file "$RCODESIGN_API_KEY_JSON" \
  --staple \
  "$bin"
