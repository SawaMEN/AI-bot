"""Раздельные настройки пользователя и беседы в SQLite.

None у поля означает наследование от беседы / сервера.
Команды диалога может менять только администратор беседы.
"""
import sqlite3


def init_preferences(db):
    db.execute("""CREATE TABLE IF NOT EXISTS preferences (
        scope TEXT NOT NULL CHECK(scope IN ('user', 'chat')),
        scope_id INTEGER NOT NULL,
        model TEXT,
        search TEXT,
        style TEXT,
        context_messages INTEGER,
        PRIMARY KEY (scope, scope_id)
    )""")
    db.commit()


def get_pref(db, scope, ident):
    row = db.execute("""SELECT model, search, style, context_messages
        FROM preferences WHERE scope=? AND scope_id=?""", (scope, int(ident))).fetchone()
    if not row:
        return {}
    return dict(zip(("model", "search", "style", "context_messages"), row))


def set_pref(db, scope, ident, field, value):
    if scope not in ("user", "chat") or field not in ("model", "search", "style", "context_messages"):
        raise ValueError("Недопустимый параметр")
    with db:
        db.execute("INSERT OR IGNORE INTO preferences(scope,scope_id) VALUES (?,?)",
                   (scope, int(ident)))
        db.execute(f"UPDATE preferences SET {field}=? WHERE scope=? AND scope_id=?",
                   (value, scope, int(ident)))


def reset_pref(db, scope, ident):
    if scope not in ("user", "chat"):
        raise ValueError("Недопустимая область настроек")
    with db:
        db.execute("DELETE FROM preferences WHERE scope=? AND scope_id=?",
                   (scope, int(ident)))


def effective(db, cfg, peer_id, sender_id, allowed):
    """Личные -> диалог -> сервер. Недоступная модель наследуется автоматически."""
    user = get_pref(db, "user", sender_id)
    chat = get_pref(db, "chat", peer_id) if int(peer_id) >= 2000000000 else {}
    base = cfg.get("default_model")
    fallback = base if base in allowed else (allowed[0] if allowed else None)
    model = next((value for value in (user.get("model"), chat.get("model"), fallback)
                  if value in allowed), None)
    search = next((v for v in (user.get("search"), chat.get("search"), "auto")
                   if v in ("on", "off", "auto")), "auto")
    style = next((v for v in (user.get("style"), chat.get("style"), "normal")
                  if v in ("short", "normal", "detailed")), "normal")
    count = next((v for v in (user.get("context_messages"), chat.get("context_messages"),
                              cfg["context_messages"]) if isinstance(v, int)
                  and not isinstance(v, bool) and v > 0), 30)
    return {"model": model, "search": search, "style": style,
            "context_messages": min(int(cfg["context_messages"]), int(count))}
