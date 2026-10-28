#!/usr/bin/env bash
# AI-bot Go: one-command binary installation. Existing Python data is preserved.
set -Eeuo pipefail
if [[ $(id -u) != 0 ]]; then echo "Запустите через sudo"; exit 1; fi
command -v systemctl >/dev/null || { echo "Нужен systemd"; exit 1; }
command -v curl >/dev/null || { echo "Установите curl"; exit 1; }
command -v sha256sum >/dev/null || { echo "Не найден sha256sum"; exit 1; }
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "Поддерживаются только Linux amd64/arm64"; exit 1 ;;
esac
RELEASE="https://github.com/SawaMEN/AI-bot/releases/download/go-v1.1.0"
STAGING=$(mktemp -d)
trap 'rm -rf "$STAGING"' EXIT
echo "Загрузка готового бинарного AI-bot Go для $ARCH..."
curl -fLsS --retry 2 --connect-timeout 12 -o "$STAGING/ai-bot-linux-$ARCH" "$RELEASE/ai-bot-linux-$ARCH" ||
 { echo "Go-релиз ещё не опубликован. Старая версия не изменена."; exit 1; }
curl -fLsS --retry 2 --connect-timeout 12 -o "$STAGING/sha256sums.txt" "$RELEASE/sha256sums.txt" ||
 { echo "Не удалось загрузить контрольные суммы. Старая версия не изменена."; exit 1; }
(cd "$STAGING" && grep " ai-bot-linux-$ARCH$" sha256sums.txt | sha256sum -c -) ||
 { echo "Неверная контрольная сумма! Установка прервана."; exit 1; }
chmod 0755 "$STAGING/ai-bot-linux-$ARCH"
# Check ELF/executable before touching the existing service.
"$STAGING/ai-bot-linux-$ARCH" --help >/dev/null 2>&1 || true

if ! id aibot >/dev/null 2>&1; then
  useradd --system --user-group --home-dir /var/lib/ai-bot --shell /usr/sbin/nologin aibot
fi
install -d -m 2750 -o root -g aibot /etc/ai-bot
install -d -m 0700 -o aibot -g aibot /var/lib/ai-bot
install -d -m 0755 /usr/local/lib/ai-bot
if [[ -f /etc/ai-bot/config.json ]]; then
  cp -a /etc/ai-bot/config.json "$STAGING/config.backup"
  chown root:aibot /etc/ai-bot/config.json
  chmod 0640 /etc/ai-bot/config.json
fi
if [[ -f /etc/systemd/system/ai-bot.service ]]; then
  cp -a /etc/systemd/system/ai-bot.service "$STAGING/ai-bot.service.old"
fi
if [[ -f /usr/local/bin/aibot ]]; then cp -a /usr/local/bin/aibot "$STAGING/aibot.old"; fi
if [[ -f /usr/local/lib/ai-bot/ai-bot ]]; then
  cp -a /usr/local/lib/ai-bot/ai-bot "$STAGING/ai-bot.binary.old"
fi

echo "Установка Go-версии с сохранением базы, API-ключей и настроек..."
systemctl stop ai-bot.service 2>/dev/null || true
install -m 0755 -o root -g root "$STAGING/ai-bot-linux-$ARCH" /usr/local/lib/ai-bot/ai-bot
cat >/usr/local/bin/aibot <<'WRAPPER'
#!/usr/bin/env bash
exec /usr/local/lib/ai-bot/ai-bot "$@"
WRAPPER
chmod 0755 /usr/local/bin/aibot
cat >/etc/systemd/system/ai-bot.service <<'SERVICE'
[Unit]
Description=AI-bot VK — native Go
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=aibot
Group=aibot
WorkingDirectory=/var/lib/ai-bot
ExecStart=/usr/local/lib/ai-bot/ai-bot run
Restart=on-failure
RestartSec=8
Environment=GOMEMLIMIT=180MiB
Environment=GOGC=100
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ai-bot
MemoryMax=300M

[Install]
WantedBy=multi-user.target
SERVICE
systemctl daemon-reload
systemctl enable ai-bot.service >/dev/null
if ! /usr/local/bin/aibot is-configured; then
  echo "Первоначальная настройка..."
  if [[ -r /dev/tty ]] && (: </dev/tty) 2>/dev/null; then
    /usr/local/bin/aibot setup </dev/tty
  else
    echo "Терминал недоступен. Позже выполните sudo aibot setup"
  fi
fi
if /usr/local/bin/aibot is-configured; then
  chown root:aibot /etc/ai-bot/config.json
  chmod 0640 /etc/ai-bot/config.json
  if ! systemctl restart ai-bot.service; then
    echo "Ошибка запуска новой службы. Попытка восстановления прежней версии..."
    [[ ! -f "$STAGING/ai-bot.service.old" ]] || cp -a "$STAGING/ai-bot.service.old" /etc/systemd/system/ai-bot.service
    [[ ! -f "$STAGING/aibot.old" ]] || cp -a "$STAGING/aibot.old" /usr/local/bin/aibot
    [[ ! -f "$STAGING/ai-bot.binary.old" ]] || cp -a "$STAGING/ai-bot.binary.old" /usr/local/lib/ai-bot/ai-bot
    [[ ! -f "$STAGING/config.backup" ]] || cp -a "$STAGING/config.backup" /etc/ai-bot/config.json
    systemctl daemon-reload
    systemctl restart ai-bot.service || true
    exit 1
  fi
  echo "AI-bot Go установлен. Меню: sudo aibot menu. Диагностика: sudo aibot doctor"
else
  systemctl stop ai-bot.service 2>/dev/null || true
  echo "Бинарник установлен, но ключи не настроены. sudo aibot setup"
fi
