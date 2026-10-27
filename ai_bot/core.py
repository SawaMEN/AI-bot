"""Лёгкое ядро: конфигурация, триггеры, история и ограничения."""
import json
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
    "max_requests_hour": 50,
}


def load_config():
    if not CONFIG_FILE.is_file():
        return DEFAULTS.copy()
    with CONFIG_FILE.open(encoding="utf-8") as file:
        loaded = json.load(file)
    if not isinstance(loaded, dict):
        raise ValueError("Неверный формат конфигурации")
    cfg = {**DEFAULTS, **loaded}
    if cfg["provider"] not in PROVIDERS:
        raise ValueError("Неизвестный ИИ-провайдер")
    return cfg


def save_config(cfg):
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
    tmp = CONFIG_FILE.with_name(CONFIG_FILE.name + ".tmp")
    with tmp.open("w", encoding="utf-8") as file:
        json.dump(cfg, file, indent=2, ensure_ascii=False)
    os.chmod(tmp, 0o640)
    os.replace(tmp, CONFIG_FILE)


def trigger_question(text, group_id, trigger="бот", replied_to_bot=False):
    """Возвращает вопрос без обращения к боту или None."""
    if not isinstance(text, str):
        return None
    raw = text.strip()
    expressions = [
        rf"^{re.escape(trigger)}(?=$|[\s,.:;!?])[\s,.:;!?]*",
        r"^/ai(?=$|[\s,.:;!?])[\s,.:;!?]*",
        rf"^\[club{int(group_id)}\|[^\]]+\][\s,.:;!?]*",
    ]
    for expression in expressions:
        matched = re.match(expression, raw, flags=re.IGNORECASE)
        if matched:
            return raw[matched.end():].strip()
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
