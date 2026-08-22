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
for var in HOMEBASE_SIGN_P12_PATH HOMEBASE_SIGN_P12_PASSWORD_FILE HOMEBASE_SIGN_TEAM_ID HOMEBASE_SIGN_API_KEY_JSON; do
  [ -n "${!var:-}" ] || missing+=("$var")
done
if [ "${#missing[@]}" -gt 0 ]; then
  echo "codesign-darwin.sh: skipping $bin, unset: ${missing[*]}" >&2
  exit 0
fi

# rcodesign auto-merges any RCODESIGN_* environment variable into its config,
# validated against the same profile schema as config files (only "sign" and
# "remote-sign" tables) — hence our own env vars use a HOMEBASE_SIGN_ prefix
# to avoid colliding with that mechanism (RCODESIGN_TEAM_ID / _API_KEY_JSON
# etc. were being rejected as UnknownField("team"/"api", ...)).
rcodesign sign \
  --p12-file "$HOMEBASE_SIGN_P12_PATH" \
  --p12-password-file "$HOMEBASE_SIGN_P12_PASSWORD_FILE" \
  --team-name "$HOMEBASE_SIGN_TEAM_ID" \
  --for-notarization \
  "$bin"

# Apple's notary service only accepts a container (.zip/.dmg/.pkg/.app), not
# a bare Mach-O, and a bare binary can't be stapled either — so zip it up,
# submit the zip (--wait, no --staple), and ship the signed-but-unstapled
# binary. Gatekeeper checks Apple's notarization ticket database online on
# first run, which is sufficient once the notarization above has completed.
zip_path="$(mktemp -u).zip"
zip -j "$zip_path" "$bin" >/dev/null
rcodesign notary-submit \
  --api-key-file "$HOMEBASE_SIGN_API_KEY_JSON" \
  --wait \
  "$zip_path"
rm -f "$zip_path"
