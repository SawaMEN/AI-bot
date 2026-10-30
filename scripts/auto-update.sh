#!/usr/bin/env bash
# Verified, transactional updater for installations managed by install.sh.
set -Eeuo pipefail
umask 077
[[ $(id -u) == 0 ]] || { echo "Run as root" >&2; exit 1; }
for cmd in curl sha256sum awk flock systemctl mktemp; do
  command -v "$cmd" >/dev/null || { echo "Missing $cmd" >&2; exit 1; }
done
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "Unsupported architecture" >&2; exit 1 ;;
esac
# Migrate an existing hourly timer without re-running the installer.
# Only the unit created by AI-bot itself is modified; custom timers are
# preserved. The updater will perform this migration on its next invocation
# after its own binary/script was automatically updated.
ensure_fast_update_timer() {
  local unit=/etc/systemd/system/ai-bot-update.timer
  [[ -f "$unit" ]] || return 0
  if ! grep -Eq '^Description=Check AI-bot release (updates hourly|every 5 minutes)
UPDATER="$ROOT/ai-bot-update"
RELEASE=https://github.com/SawaMEN/AI-bot/releases/latest/download
BINARY="ai-bot-linux-$ARCH"
[[ -f "$APP" ]] || { echo "AI-bot binary is not installed" >&2; exit 1; }
exec 9>/run/lock/ai-bot-update.lock
if ! flock -n 9; then echo "Another AI-bot update is running"; exit 0; fi
STAGING=$(mktemp -d "$ROOT/.update.XXXXXXXX")
trap 'rm -rf "$STAGING"' EXIT

curl -fLsS --retry 2 --connect-timeout 12 --max-time 90 \
  -o "$STAGING/sha256sums.txt" "$RELEASE/sha256sums.txt"
expected_hash() {
  local value
  value=$(awk -v name="$1" '$2 == name { print $1 }' "$STAGING/sha256sums.txt")
  [[ "$value" =~ ^[a-fA-F0-9]{64}$ ]] || { echo "Missing or malformed checksum for $1" >&2; return 1; }
  printf '%s\n' "${value,,}"
}
local_hash() {
  if [[ -f "$1" ]]; then sha256sum "$1" | awk '{print $1}'; else printf 'missing\n'; fi
}
verify_asset() {
  local asset="$1" expected="$2"
  curl -fLsS --retry 2 --connect-timeout 12 --max-time 120 \
    -o "$STAGING/$asset" "$RELEASE/$asset"
  printf '%s  %s\n' "$expected" "$STAGING/$asset" | sha256sum -c --status -
}
BINARY_HASH=$(expected_hash "$BINARY")
UPDATER_HASH=$(expected_hash ai-bot-update)
CHANGE_BINARY=false
CHANGE_UPDATER=false
[[ "$(local_hash "$APP")" == "$BINARY_HASH" ]] || CHANGE_BINARY=true
[[ "$(local_hash "$UPDATER")" == "$UPDATER_HASH" ]] || CHANGE_UPDATER=true
if [[ "$CHANGE_BINARY" == false && "$CHANGE_UPDATER" == false ]]; then
  echo "AI-bot: already up to date"
  exit 0
fi
if [[ "$CHANGE_BINARY" == true ]]; then
  verify_asset "$BINARY" "$BINARY_HASH"
  chmod 0755 "$STAGING/$BINARY"
  VERSION=$("$STAGING/$BINARY" version)
  [[ "$VERSION" =~ ^AI-bot\ Go\ v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
    echo "Unexpected AI-bot version: $VERSION" >&2; exit 1;
  }
fi
if [[ "$CHANGE_UPDATER" == true ]]; then
  verify_asset ai-bot-update "$UPDATER_HASH"
  bash -n "$STAGING/ai-bot-update"
  chmod 0755 "$STAGING/ai-bot-update"
fi
if [[ "$CHANGE_BINARY" == true ]]; then
  BOT_ACTIVE=false
  PANEL_ACTIVE=false
  systemctl is-active --quiet ai-bot.service && BOT_ACTIVE=true
  systemctl is-active --quiet ai-bot-panel.service && PANEL_ACTIVE=true
  cp -p "$APP" "$STAGING/previous-binary"
  # The temporary directory and destination share a filesystem: rename is atomic.
  mv -f "$STAGING/$BINARY" "$APP"
  FAILED=false
  if [[ "$BOT_ACTIVE" == true ]]; then
    systemctl restart ai-bot.service || FAILED=true
  fi
  if [[ "$PANEL_ACTIVE" == true ]]; then
    systemctl restart ai-bot-panel.service || FAILED=true
  fi
  if [[ "$FAILED" == false && ( "$BOT_ACTIVE" == true || "$PANEL_ACTIVE" == true ) ]]; then
    sleep 3
    if [[ "$BOT_ACTIVE" == true ]] && ! systemctl is-active --quiet ai-bot.service; then FAILED=true; fi
    if [[ "$PANEL_ACTIVE" == true ]] && ! systemctl is-active --quiet ai-bot-panel.service; then FAILED=true; fi
  fi
  if [[ "$FAILED" == true ]]; then
    echo "AI-bot: new version failed to start; rolling back" >&2
    mv -f "$STAGING/previous-binary" "$APP"
    if [[ "$BOT_ACTIVE" == true ]]; then systemctl restart ai-bot.service || true; fi
    if [[ "$PANEL_ACTIVE" == true ]]; then systemctl restart ai-bot-panel.service || true; fi
    exit 1
  fi
  echo "AI-bot upgraded: $VERSION"
fi
# Update the updater itself only after the application has passed its health checks.
if [[ "$CHANGE_UPDATER" == true ]]; then
  mv -f "$STAGING/ai-bot-update" "$UPDATER"
  echo "AI-bot updater refreshed"
fi
 "$unit"; then
    return 0
  fi
  local candidate
  candidate=$(mktemp /etc/systemd/system/.ai-bot-timer.XXXXXX)
  cat >"$candidate" <<'TIMER_UNIT'
[Unit]
Description=Check AI-bot release every 5 minutes

[Timer]
OnBootSec=2min
OnUnitInactiveSec=5min
AccuracySec=15s
RandomizedDelaySec=20s
Unit=ai-bot-update.service

[Install]
WantedBy=timers.target
TIMER_UNIT
  if ! cmp -s "$candidate" "$unit"; then
    install -m 0644 "$candidate" "$unit"
    systemctl daemon-reload
    systemctl try-restart ai-bot-update.timer
    echo "AI-bot: update interval changed to approximately 5 minutes"
  fi
  rm -f "$candidate"
}
ensure_fast_update_timer

ROOT=/usr/local/lib/ai-bot
APP="$ROOT/ai-bot"
UPDATER="$ROOT/ai-bot-update"
RELEASE=https://github.com/SawaMEN/AI-bot/releases/latest/download
BINARY="ai-bot-linux-$ARCH"
[[ -f "$APP" ]] || { echo "AI-bot binary is not installed" >&2; exit 1; }
exec 9>/run/lock/ai-bot-update.lock
if ! flock -n 9; then echo "Another AI-bot update is running"; exit 0; fi
STAGING=$(mktemp -d "$ROOT/.update.XXXXXXXX")
trap 'rm -rf "$STAGING"' EXIT

curl -fLsS --retry 2 --connect-timeout 12 --max-time 90 \
  -o "$STAGING/sha256sums.txt" "$RELEASE/sha256sums.txt"
expected_hash() {
  local value
  value=$(awk -v name="$1" '$2 == name { print $1 }' "$STAGING/sha256sums.txt")
  [[ "$value" =~ ^[a-fA-F0-9]{64}$ ]] || { echo "Missing or malformed checksum for $1" >&2; return 1; }
  printf '%s\n' "${value,,}"
}
local_hash() {
  if [[ -f "$1" ]]; then sha256sum "$1" | awk '{print $1}'; else printf 'missing\n'; fi
}
verify_asset() {
  local asset="$1" expected="$2"
  curl -fLsS --retry 2 --connect-timeout 12 --max-time 120 \
    -o "$STAGING/$asset" "$RELEASE/$asset"
  printf '%s  %s\n' "$expected" "$STAGING/$asset" | sha256sum -c --status -
}
BINARY_HASH=$(expected_hash "$BINARY")
UPDATER_HASH=$(expected_hash ai-bot-update)
CHANGE_BINARY=false
CHANGE_UPDATER=false
[[ "$(local_hash "$APP")" == "$BINARY_HASH" ]] || CHANGE_BINARY=true
[[ "$(local_hash "$UPDATER")" == "$UPDATER_HASH" ]] || CHANGE_UPDATER=true
if [[ "$CHANGE_BINARY" == false && "$CHANGE_UPDATER" == false ]]; then
  echo "AI-bot: already up to date"
  exit 0
fi
if [[ "$CHANGE_BINARY" == true ]]; then
  verify_asset "$BINARY" "$BINARY_HASH"
  chmod 0755 "$STAGING/$BINARY"
  VERSION=$("$STAGING/$BINARY" version)
  [[ "$VERSION" =~ ^AI-bot\ Go\ v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
    echo "Unexpected AI-bot version: $VERSION" >&2; exit 1;
  }
fi
if [[ "$CHANGE_UPDATER" == true ]]; then
  verify_asset ai-bot-update "$UPDATER_HASH"
  bash -n "$STAGING/ai-bot-update"
  chmod 0755 "$STAGING/ai-bot-update"
fi
if [[ "$CHANGE_BINARY" == true ]]; then
  BOT_ACTIVE=false
  PANEL_ACTIVE=false
  systemctl is-active --quiet ai-bot.service && BOT_ACTIVE=true
  systemctl is-active --quiet ai-bot-panel.service && PANEL_ACTIVE=true
  cp -p "$APP" "$STAGING/previous-binary"
  # The temporary directory and destination share a filesystem: rename is atomic.
  mv -f "$STAGING/$BINARY" "$APP"
  FAILED=false
  if [[ "$BOT_ACTIVE" == true ]]; then
    systemctl restart ai-bot.service || FAILED=true
  fi
  if [[ "$PANEL_ACTIVE" == true ]]; then
    systemctl restart ai-bot-panel.service || FAILED=true
  fi
  if [[ "$FAILED" == false && ( "$BOT_ACTIVE" == true || "$PANEL_ACTIVE" == true ) ]]; then
    sleep 3
    if [[ "$BOT_ACTIVE" == true ]] && ! systemctl is-active --quiet ai-bot.service; then FAILED=true; fi
    if [[ "$PANEL_ACTIVE" == true ]] && ! systemctl is-active --quiet ai-bot-panel.service; then FAILED=true; fi
  fi
  if [[ "$FAILED" == true ]]; then
    echo "AI-bot: new version failed to start; rolling back" >&2
    mv -f "$STAGING/previous-binary" "$APP"
    if [[ "$BOT_ACTIVE" == true ]]; then systemctl restart ai-bot.service || true; fi
    if [[ "$PANEL_ACTIVE" == true ]]; then systemctl restart ai-bot-panel.service || true; fi
    exit 1
  fi
  echo "AI-bot upgraded: $VERSION"
fi
# Update the updater itself only after the application has passed its health checks.
if [[ "$CHANGE_UPDATER" == true ]]; then
  mv -f "$STAGING/ai-bot-update" "$UPDATER"
  echo "AI-bot updater refreshed"
fi
