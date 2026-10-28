"""Безопасная диагностика. Никаких платных генераций и вывода токенов."""
import os
import sqlite3
from pathlib import Path

import requests

from ai_bot.catalog import SPECS, enabled_models, refresh_free_models
from ai_bot.core import DATA_FILE, load_config


def inspect_vk(cfg, request=None):
    """Проверяет разрешения VK; только чтение, без изменения сообщества."""
    request = request or requests.post
    if not cfg.get("vk_token") or not int(cfg.get("group_id") or 0):
        return False, ["Не задан токен или ID сообщества. Откройте sudo aibot → Подключить ВКонтакте."]
    params = {"access_token": cfg["vk_token"], "v": "5.199", "group_id": int(cfg["group_id"])}
    base = "https://api.vk.com/method/"
    messages = []
    try:
        resp = request(base + "groups.getLongPollSettings", data=params, timeout=(5, 15))
        resp.raise_for_status()
        parsed = resp.json()
        if "error" in parsed:
            return False, ["ВК отказал в проверке прав: " + str(
                parsed["error"].get("error_msg", "Ошибка API"))[:150]]
        settings = parsed.get("response") or {}
        events = settings.get("events") or {}
        ok = bool(settings.get("is_enabled") or settings.get("enabled"))
        messages.append(("OK: " if ok else "ОШИБКА: ") + "Bots Long Poll " +
                        ("включён" if ok else "отключён"))
        for code, label in (("message_new", "Входящие сообщения"),
                            ("message_event", "Кнопки ВК")):
            present = bool(events.get(code))
            messages.append(("OK: " if present else "ОШИБКА: ") + label +
                            (" разрешены" if present else " НЕ разрешены (включите " + code + " в типах событий)"))
            ok = ok and present
        server = request(base + "groups.getLongPollServer", data=params, timeout=(5, 15))
        server.raise_for_status()
        server_data = server.json()
        if "error" in server_data:
            messages.append("Ошибка: не удалось получить сервер Long Poll.")
            ok = False
        elif "server" in (server_data.get("response") or {}):
            messages.append("OK: ключ и соединение с API ВК действуют.")
        else:
            messages.append("Ошибка: сервер Long Poll не вернул адрес.")
            ok = False
        messages.append("Важно: «Доступ ко всей переписке» выдаётся администратором каждой беседы вручную.")
        return ok, messages
    except (requests.RequestException, ValueError, TypeError) as exc:
        return False, ["VK API недоступен: " + type(exc).__name__]



def enable_vk_events(cfg, request=None):
    """Пробует включить только ключевые события после явного подтверждения.

    VK может требовать пользовательский токен администратора. Не скрываем отказ.
    У существующего сообщества могут быть и другие события — предупредите
    администратора о возможном изменении их настроек.
    """
    if not cfg.get("vk_token") or not int(cfg.get("group_id") or 0):
        return False, "Сначала задайте токен и ID сообщества."
    request = request or requests.post
    try:
        reply = request("https://api.vk.com/method/groups.setLongPollSettings",
                        data={"access_token": cfg["vk_token"], "v": "5.199",
                              "group_id": int(cfg["group_id"]), "enabled": 1,
                              "message_new": 1, "message_event": 1},
                        timeout=(5, 15))
        reply.raise_for_status()
        payload = reply.json()
        if payload.get("error"):
            return False, str(payload["error"].get("error_msg", "ВК отказал"))[:160]
        if payload.get("response") in (1, True):
            return True, "События включены. Проверьте права сообщества в настройках ВК."
        return False, "Неожиданный ответ ВК. Проверьте настройки вручную."
    except (ValueError, requests.RequestException) as exc:
        return False, "Не удалось настроить VK API: " + type(exc).__name__


def inspect_local(cfg, db_path=None):
    lines = []
    db_path = Path(db_path or DATA_FILE)
    if db_path.exists():
        try:
            connection = sqlite3.connect(str(db_path), timeout=2)
            integrity = connection.execute("PRAGMA quick_check").fetchone()[0]
            connection.close()
            lines.append(("OK" if integrity == "ok" else "ОШИБКА") +
                         ": SQLite — " + str(integrity)[:100])
        except (OSError, sqlite3.Error) as exc:
            lines.append("ОШИБКА: база SQLite: " + type(exc).__name__)
    else:
        lines.append("База SQLite пока не создана (бот ещё не работал).")
    lines.append("Лимит ИИ за час: " + str(cfg.get("max_requests_hour", 50)))
    lines.append("Лимит на пользователя: " + str(cfg.get("max_user_requests_hour", 12)))
    lines.append("Лимит на беседу: " + str(cfg.get("max_chat_requests_hour", 25)))
    return lines


def run_diagnostics(cfg=None, vk=True, catalogs=True, printer=print):
    cfg = cfg or load_config()
    printer("\n====== Проверка AI-bot ======")
    printer("Убедитесь, что у провайдеров отключён платный биллинг.")
    for line in inspect_local(cfg):
        printer(line)
    free = cfg.get("auto_models_providers", [])
    if catalogs:
        status = refresh_free_models(cfg)
        for provider, (count, error) in status.items():
            printer(("OK" if not error and count else "ПРОВЕРЬТЕ") +
                    ": " + SPECS[provider]["name"] + " — " + str(count) +
                    " подтверждённых моделей" + (", ошибка: " + error if error else ""))
    active = enabled_models(cfg)
    printer("Разрешённых сейчас моделей: " + str(len(active)))
    if not active:
        printer("Действие: откройте sudo aibot → Разрешённые провайдеры и модели.")
    success = bool(active)
    if vk:
        connected, messages = inspect_vk(cfg)
        for message in messages:
            printer(message)
        success = success and connected
    printer("Подсказки: меню ВК → «Права и подключение», сервер → sudo aibot.")
    return success


if __name__ == "__main__":
    raise SystemExit(0 if run_diagnostics() else 1)
