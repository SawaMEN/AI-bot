"""Лёгкое ядро: конфигурация, триггеры, история и ограничения."""
import json
import grp
import os
import re
import sqlite3
import time
from pathlib import Path

CONFIG_FILE = Path(os.getenv("AIBOT_CONFIG", "/etc/ai-bot/config.json"))
DATA_FILE = Path(os.getenv("AIBOT_DATA", "/var/lib/ai-bot/history.db"))
PROVIDERS = {
    "groq": ("GroqCloud", "openai/gpt-oss-20b"),
    "gemini": ("Google Gemini", "gemini-2.5-flash-lite"),
    "openrouter": ("OpenRouter", "openrouter/free"),
    "cloudflare": ("Cloudflare Workers AI", "@cf/meta/llama-3.1-8b-instruct"),
    "custom": ("OpenAI-совместимый API", ""),
}
DEFAULTS = {
    "vk_token": "", "group_id": 0, "provider": "groq", "api_key": "",
    "model": PROVIDERS["groq"][1], "custom_url": "", "cloudflare_account": "",
    "trigger": "бот", "context_messages": 30, "context_hours": 24,
    "retention_days": 7, "search_enabled": True, "cooldown_seconds": 15,
    "max_requests_hour": 50, "providers": {}, "enabled_models": [], "default_model": "",
    "auto_models_providers": [],
}


def load_config():
    if not CONFIG_FILE.is_file():
        return DEFAULTS.copy()
    with CONFIG_FILE.open(encoding="utf-8") as file:
        loaded = json.load(file)
    if not isinstance(loaded, dict):
        raise ValueError("Неверный формат конфигурации")
    cfg = {**DEFAULTS, **loaded}
    # Автоматическая миграция конфигурации v1: не теряем старый API-ключ.
    from ai_bot.catalog import PROVIDERS as NEW_PROVIDERS, model_key
    if cfg.get("provider") not in NEW_PROVIDERS:
        # Старый custom/huggingface теперь исключён: без аварийного завершения
        # отбрасываем выбор и просим настроить бесплатный провайдер.
        cfg["provider"] = "groq"
        cfg["api_key"] = ""
        cfg["model"] = ""

    cfg["providers"] = dict(cfg.get("providers") or {})
    cfg["enabled_models"] = list(cfg.get("enabled_models") or [])
    cfg["auto_models_providers"] = list(cfg.get("auto_models_providers") or [])
    if not cfg["providers"] and cfg.get("api_key"):
        provider = cfg["provider"]
        cfg["providers"] = {
            provider: {"api_key": cfg["api_key"], "enabled": True,
                       "cloudflare_account": cfg.get("cloudflare_account", ""),
                       "custom_url": cfg.get("custom_url", "")}
        }
        selected = model_key(provider, cfg.get("model", ""))
        cfg["enabled_models"] = [selected]
        cfg["default_model"] = selected
    return cfg


def save_config(cfg):
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
    tmp = CONFIG_FILE.with_name(CONFIG_FILE.name + ".tmp")
    with tmp.open("w", encoding="utf-8") as file:
        json.dump({k: v for k, v in cfg.items() if not k.startswith("_")},
                  file, indent=2, ensure_ascii=False)
    os.chmod(tmp, 0o640)
    try:
        os.chown(tmp, -1, grp.getgrnam("aibot").gr_gid)
    except (KeyError, PermissionError):
        pass
    os.replace(tmp, CONFIG_FILE)


def trigger_question(text, group_id, trigger="бот", replied_to_bot=False):
    """Обращение по упоминанию в любой части фразы, даже без слова «бот»."""
    if not isinstance(text, str):
        return None
    raw = text.strip()
    if not raw:
        return None
    for expression in (
        rf"^{re.escape(trigger)}(?=$|[\s,.:;!?])[\s,.:;!?]*",
        r"^/ai(?=$|[\s,.:;!?])[\s,.:;!?]*",
    ):
        found = re.match(expression, raw, flags=re.IGNORECASE)
        if found:
            return raw[found.end():].strip()
    group_id = int(group_id)
    found = re.search(
        rf"\[(?:club|public){group_id}\|[^\]]+\]|@(?:club|public){group_id}(?!\d)",
        raw, flags=re.IGNORECASE
    )
    if found:
        before = raw[:found.start()].rstrip(" ,.:;!?")
        after = raw[found.end():].lstrip(" ,.:;!?")
        # Убираем знаки рядом с упоминанием, сохраняем пунктуацию самого вопроса.
        return (before + " " + after).strip() if after else before
    return raw if replied_to_bot else None


def needs_search(question):
    return bool(re.search(
        r"\b(найди|поищи|проверь|погугли|новости|источники|ссылки|"
        r"в интернете|за сегодня|за вчера|последние новости|актуальн\w*)\b",
        question, re.IGNORECASE
    ))


class History:
    def __init__(self, path=DATA_FILE):
        Path(path).parent.mkdir(parents=True, exist_ok=True)
        self.db = sqlite3.connect(str(path), timeout=15)
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("""CREATE TABLE IF NOT EXISTS messages (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            peer_id INTEGER NOT NULL,
            sender_id INTEGER NOT NULL,
            event_id TEXT NOT NULL UNIQUE,
            text TEXT NOT NULL,
            created_at INTEGER NOT NULL
        )""")
        self.db.execute("CREATE INDEX IF NOT EXISTS idx_peer ON messages(peer_id, id)")
        self.db.execute("""CREATE TABLE IF NOT EXISTS calls (
            id INTEGER PRIMARY KEY AUTOINCREMENT, created_at INTEGER NOT NULL
        )""")
        from ai_bot.preferences import init_preferences
        init_preferences(self.db)
        self.db.commit()

    def add(self, peer_id, sender_id, event_id, text):
        if not text or not event_id:
            return False
        with self.db:
            row = self.db.execute(
                "INSERT OR IGNORE INTO messages(peer_id,sender_id,event_id,text,created_at) VALUES (?,?,?,?,?)",
                (int(peer_id), int(sender_id), str(event_id), str(text)[:3000], int(time.time()))
            )
        return row.rowcount == 1

    def recent(self, peer_id, count, hours):
        rows = self.db.execute("""SELECT sender_id, text FROM messages
            WHERE peer_id=? AND created_at>=? ORDER BY id DESC LIMIT ?""",
            (int(peer_id), int(time.time()) - int(hours) * 3600, max(1, min(int(count), 100)))
        ).fetchall()
        return list(reversed(rows))

    def count_calls_hour(self):
        return self.db.execute("SELECT COUNT(*) FROM calls WHERE created_at>=?",
                               (int(time.time()) - 3600,)).fetchone()[0]

    def track_call(self):
        with self.db:
            self.db.execute("INSERT INTO calls(created_at) VALUES (?)", (int(time.time()),))

    def cleanup(self, days):
        cutoff = int(time.time()) - max(1, int(days)) * 86400
        with self.db:
            self.db.execute("DELETE FROM messages WHERE created_at < ?", (cutoff,))
            self.db.execute("DELETE FROM calls WHERE created_at < ?", (int(time.time()) - 86400,))

    def clear(self, peer_id=None):
        with self.db:
            if peer_id is None:
                self.db.execute("DELETE FROM messages")
            else:
                self.db.execute("DELETE FROM messages WHERE peer_id=?", (int(peer_id),))
