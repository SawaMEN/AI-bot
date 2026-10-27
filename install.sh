#!/usr/bin/env bash
# Установка AI-bot на Debian/Ubuntu: curl -fsSL URL | sudo bash
set -Eeuo pipefail
APP="/opt/ai-bot"
REPO="https://github.com/SawaMEN/AI-bot.git"
UPDATE=0
if [[ "$*" == *"--update"* ]]; then UPDATE=1; fi

if [[ "$(id -u)" -ne 0 ]]; then
  echo "Запустите от root: curl -fsSL https://raw.githubusercontent.com/SawaMEN/AI-bot/main/install.sh | sudo bash"
  exit 1
fi
if ! command -v apt-get >/dev/null 2>&1; then
  echo "Пока поддерживаются только Ubuntu/Debian (systemd + apt)."
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive
echo "Установка зависимостей AI-bot..."
apt-get update -qq
apt-get install -y -qq --no-install-recommends git python3 python3-venv python3-pip ca-certificates
python3 -c 'import sys; assert sys.version_info >= (3,10), "Нужен Python 3.10+"'

if ! id aibot >/dev/null 2>&1; then
  useradd --system --user-group --home-dir /var/lib/ai-bot --shell /usr/sbin/nologin aibot
fi
install -d -m 2750 -o root -g aibot /etc/ai-bot
install -d -m 0700 -o aibot -g aibot /var/lib/ai-bot

if [[ -d "$APP/.git" ]]; then
  echo "Обновление исходников..."
  systemctl stop ai-bot.service 2>/dev/null || true
  git -C "$APP" fetch --depth 1 origin main
  git -C "$APP" reset --hard FETCH_HEAD
else
  git clone --depth 1 --branch main "$REPO" "$APP"
fi
python3 -m venv "$APP/.venv"
"$APP/.venv/bin/python" -m pip install --disable-pip-version-check --no-cache-dir -r "$APP/requirements.txt"
cat > /usr/local/bin/aibot <<'EOF'
#!/usr/bin/env bash
cd /opt/ai-bot
exec /opt/ai-bot/.venv/bin/python -m ai_bot.cli "$@"
EOF
chmod 0755 /usr/local/bin/aibot
cat > /etc/systemd/system/ai-bot.service <<'EOF'
[Unit]
Description=AI-bot — помощник для ВКонтакте
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=aibot
Group=aibot
WorkingDirectory=/opt/ai-bot
ExecStart=/opt/ai-bot/.venv/bin/python /opt/ai-bot/bot.py run
Restart=always
RestartSec=8
Environment=PYTHONUNBUFFERED=1
Environment=PYTHONDONTWRITEBYTECODE=1
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ai-bot
MemoryMax=300M

[Install]
WantedBy=multi-user.target
EOF
systemctl daemon-reload
systemctl enable ai-bot.service >/dev/null
if [[ "$UPDATE" -eq 0 && ! -f /etc/ai-bot/config.json ]]; then
  echo "Первоначальная настройка..."
  if [[ -e /dev/tty ]] && (echo >/dev/tty) 2>/dev/null; then
    /usr/local/bin/aibot setup </dev/tty
  else
    echo "Нет интерактивного терминала. Выполните позже: sudo aibot setup"
  fi
fi
if /usr/local/bin/aibot is-configured; then
  chown root:aibot /etc/ai-bot/config.json
  chmod 0640 /etc/ai-bot/config.json
  systemctl restart ai-bot.service
  echo "AI-bot установлен и запущен! Меню: sudo aibot"
else
  systemctl stop ai-bot.service 2>/dev/null || true
  echo "AI-bot установлен. Укажите токены через sudo aibot setup и запустите: sudo systemctl start ai-bot"
fi
