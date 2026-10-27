"""ВК: inline callback-кнопки и подсказки без лишних сообщений участников."""
import json

from ai_bot.catalog import enabled_models, model_label


def button(label, command):
    return {"action": {
        "type": "callback", "label": label[:40],
        "payload": json.dumps({"aibot": 1, "command": command}, ensure_ascii=False),
    }}


def keyboard(rows):
    """Inline в беседах: 6 рядов максимум, 5 кнопок в строке."""
    if len(rows) > 6 or any(len(row) > 5 for row in rows):
        raise ValueError("Превышен размер клавиатуры ВК")
    return json.dumps({"inline": True, "buttons": rows}, ensure_ascii=False)


def main_menu():
    return keyboard([
        [button("Выбрать ИИ", "модели 1"), button("Мои настройки", "настройки")],
        [button("Поиск", "выбор поиска"), button("Стиль ответа", "выбор стиля")],
        [button("Контекст", "выбор контекста"), button("Настройки беседы", "диалог")],
        [button("Права и подключение", "права"), button("Помощь", "помощь")],
    ])


def model_menu(allowed, page=1, chat=False):
    """Номера в списке не меняются между страницами."""
    size = 5
    pages = max(1, (len(allowed) + size - 1) // size)
    page = max(1, min(int(page), pages))
    offset = (page - 1) * size
    rows = []
    for idx, key in enumerate(allowed[offset:offset + size], offset + 1):
        try:
            label = model_label(key).split(" • ", 1)[-1]
        except ValueError:
            label = key[:30]
        command = ("диалог " if chat else "") + f"модель {idx}"
        rows.append([button(f"{idx}. {label}", command)])
    nav = []
    target = "диалог модели " if chat else "модели "
    if page > 1:
        nav.append(button("Назад", target + str(page - 1)))
    if page < pages:
        nav.append(button("Далее", target + str(page + 1)))
    if nav:
        rows.append(nav)
    rows.append([button("Меню", "меню")])
    return keyboard(rows)


def search_menu(chat=False):
    prefix = "диалог " if chat else ""
    return keyboard([
        [button("Авто", prefix + "поиск авто"),
         button("Всегда искать", prefix + "поиск вкл")],
        [button("Без поиска", prefix + "поиск выкл")],
        [button("Меню", "меню")],
    ])


def style_menu(chat=False):
    prefix = "диалог " if chat else ""
    return keyboard([
        [button("Кратко", prefix + "стиль кратко"), button("Обычно", prefix + "стиль обычно")],
        [button("Подробно", prefix + "стиль подробно")],
        [button("Меню", "меню")],
    ])


def context_menu(cfg, chat=False):
    prefix = "диалог " if chat else ""
    top = max(1, int(cfg.get("context_messages", 30)))
    values = sorted(set(min(top, n) for n in [5, 10, 20, 30, 50, 100]))
    rows = [[button(f"{n} сообщений", prefix + f"контекст {n}")] for n in values[:5]]
    rows.append([button("Меню", "меню")])
    return keyboard(rows)


def for_command(command, cfg):
    """Клавиатура к ответу команды в VK; None = без кнопок."""
    normalized = command.lower().strip()
    allowed = enabled_models(cfg)
    if normalized in ("меню", "помощь", "help", "старт", "права", "настройки", "диалог"):
        return main_menu() if normalized not in ("диалог",) else chat_menu()
    if normalized in ("выбор поиска", "диалог выбор поиска"):
        return search_menu(normalized.startswith("диалог "))
    if normalized in ("выбор стиля", "диалог выбор стиля"):
        return style_menu(normalized.startswith("диалог "))
    if normalized in ("выбор контекста", "диалог выбор контекста"):
        return context_menu(cfg, normalized.startswith("диалог "))
    parts = normalized.split()
    if parts and parts[0] == "модели":
        page = int(parts[1]) if len(parts) == 2 and parts[1].isdigit() else 1
        return model_menu(allowed, page)
    if len(parts) >= 2 and parts[0] == "диалог" and parts[1] == "модели":
        page = int(parts[2]) if len(parts) == 3 and parts[2].isdigit() else 1
        return model_menu(allowed, page, chat=True)
    if "модель" in parts:
        return main_menu()
    if "поиск" in parts:
        return search_menu(normalized.startswith("диалог "))
    if "стиль" in parts:
        return style_menu(normalized.startswith("диалог "))
    if "контекст" in parts:
        return context_menu(cfg, normalized.startswith("диалог "))
    return None


def chat_menu():
    return keyboard([
        [button("Модель для беседы", "диалог модели 1"),
         button("Поиск беседы", "диалог выбор поиска")],
        [button("Стиль беседы", "диалог выбор стиля"),
         button("Контекст беседы", "диалог выбор контекста")],
        [button("Сбросить беседу", "диалог сброс"),
         button("Права доступа", "права")],
        [button("Меню", "меню")],
    ])


def callback_command(event):
    payload = event.get("payload") if isinstance(event, dict) else None
    if isinstance(payload, str):
        try:
            payload = json.loads(payload)
        except ValueError:
            return None
    if not isinstance(payload, dict) or payload.get("aibot") != 1:
        return None
    command = payload.get("command")
    # Выдача прав и контроль модели остаются в основном обработчике.
    if not isinstance(command, str) or len(command) > 90:
        return None
    if command not in ("меню", "помощь", "настройки", "диалог", "права",
                       "выбор поиска", "выбор стиля", "выбор контекста",
                       "диалог выбор поиска", "диалог выбор стиля",
                       "диалог выбор контекста") and not (
        command.startswith(("модели ", "модель ", "поиск ", "стиль ", "контекст ",
                            "диалог модели ", "диалог модель ", "диалог поиск ",
                            "диалог стиль ", "диалог контекст "))
        or command == "диалог сброс"
    ):
        return None
    return command
