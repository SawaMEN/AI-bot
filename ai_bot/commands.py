"""Русские настройки непосредственно в беседе ВК.

Команды отвечают локально и НЕ отправляются облачному ИИ.
В беседе изменять общие настройки могут только её администраторы.
"""
from ai_bot.catalog import enabled_models, model_label
from ai_bot.preferences import effective, get_pref, reset_pref, set_pref

SEARCH = {"вкл": "on", "да": "on", "выкл": "off", "нет": "off", "авто": "auto"}
STYLES = {"кратко": "short", "обычно": "normal", "подробно": "detailed"}
STYLE_LABEL = {"short": "кратко", "normal": "обычно", "detailed": "подробно"}
SEARCH_LABEL = {"on": "всегда", "off": "выключен", "auto": "по запросу"}


def model_list(allowed, page=1):
    if not allowed:
        return "Администратор сервера ещё не включил ни одну ИИ-модель."
    per_page = 4
    pages = (len(allowed) + per_page - 1) // per_page
    page = max(1, min(page, pages))
    start = (page - 1) * per_page
    section = allowed[start:start + per_page]
    return (f"Доступные модели (страница {page}/{pages}):\n" + "\n".join(
        f"{start + i}. {model_label(key)}" for i, key in enumerate(section, 1)
    ) + "\n\nВыбор: «бот модель 2». Следующая страница: «бот модели 2».")


def help_text():
    return (
        "AI-bot — команды без кавычек:\n"
        "• бот модели — доступные модели (можно выбрать кнопкой)\n"
        "• бот модель 1 — выбрать модель для себя\n"
        "• бот модель сброс — использовать модель беседы или сервера\n"
        "• бот поиск вкл / выкл / авто\n"
        "• бот стиль кратко / обычно / подробно\n"
        "• бот контекст 20 — сколько сообщений учитывать\n"
        "• бот резерв вкл / выкл — использовать ли другую ИИ при сбое\n"
        "• бот настройки — мои действующие настройки\n"
        "• бот сброс — сбросить личные настройки\n\n"
        "Администратору беседы:\n"
        "• бот диалог — настройки этой беседы\n"
        "• бот диалог модель 1\n"
        "• бот диалог поиск авто\n"
        "• бот диалог стиль кратко\n"
        "• бот диалог контекст 20\n"
        "• бот диалог резерв выкл\n"
        "• бот диалог сброс\n\n"
        "Можно и без слова «бот»: упомяните сообщество ВК и задайте вопрос.\n"
        "Для кнопок: упоминание сообщества + «меню».\n"
        "Для выдачи прав: упоминание сообщества + «права»."
    )



def permission_text(cfg):
    group = int(cfg.get("group_id", 0))
    return (
        "Как подключить AI-bot к беседе ВК:\n"
        "1. Откройте своё сообщество → Управление → Сообщения: включите «Сообщения сообщества».\n"
        "2. Там же → Настройки для бота: включите «Возможности ботов» и "
        "«Разрешать добавлять сообщество в беседы».\n"
        "3. Управление → Настройки → Работа с API → Ключи доступа: создайте ключ с правами на сообщения. "
        "Не публикуйте его в ВК, вводите только на своём сервере в sudo aibot.\n"
        "4. В разделе Long Poll API включите его, затем в типах событий отметьте "
        "«Входящее сообщение» (message_new) и «Нажатие на callback-кнопку» (message_event).\n"
        "5. Добавьте сообщество в нужную беседу. Администратор беседы должен открыть "
        "её название → Участники → сообщество/бот → настройки прав и дать "
        "«Доступ ко всей переписке» или права администратора.\n"
        "6. Проверка: один участник пишет обычное сообщение, другой спрашивает "
        f"«[club{group}|AI-bot], а как ты думаешь?». "
        "Без прав на всю переписку бот увидит лишь часть разговора.\n\n"
        "Названия разделов в разных версиях ВК могут отличаться. "
        "Бот не увидит сообщения, отправленные до предоставления доступа.\n"
        "Если ВК не показывает выбор прав, их должен изменить создатель/администратор беседы."
    )


def apply_setting(db, cfg, scope, ident, parts, allowed):
    field = parts[0].lower()
    if field in ("сброс", "сбросить") and len(parts) == 1:
        reset_pref(db, scope, ident)
        return "Настройки сброшены. Теперь используются значения более высокого уровня."
    if field == "модель":
        if len(parts) != 2:
            return "Пример: бот модель 1. Список: бот модели"
        if parts[1].lower() in ("сброс", "авто"):
            set_pref(db, scope, ident, "model", None)
            return "Выбор модели сброшен."
        try:
            number = int(parts[1])
        except ValueError:
            return "Укажите номер из списка: бот модели"
        if not 1 <= number <= len(allowed):
            return "Такой модели нет. Напишите: бот модели"
        key = allowed[number - 1]
        set_pref(db, scope, ident, "model", key)
        return "Модель выбрана: " + model_label(key)
    if field == "поиск":
        if len(parts) != 2 or parts[1].lower() not in SEARCH:
            return "Пример: бот поиск авто (варианты: вкл, выкл, авто)"
        set_pref(db, scope, ident, "search", SEARCH[parts[1].lower()])
        return "Режим поиска: " + SEARCH_LABEL[SEARCH[parts[1].lower()]]
    if field == "стиль":
        if len(parts) != 2 or parts[1].lower() not in STYLES:
            return "Пример: бот стиль кратко (кратко, обычно, подробно)"
        set_pref(db, scope, ident, "style", STYLES[parts[1].lower()])
        return "Стиль ответа: " + parts[1].lower()
    if field == "резерв":
        if len(parts) != 2 or parts[1].lower() not in ("вкл", "выкл", "авто"):
            return "Пример: бот резерв выкл (вкл, выкл, авто)"
        value = {"вкл": "on", "выкл": "off", "авто": None}[parts[1].lower()]
        set_pref(db, scope, ident, "fallback", value)
        return ("Резервный ИИ: " + ("наследовать настройки" if value is None
                                   else ("разрешён" if value == "on" else "запрещён")))
    if field == "контекст":
        try:
            count = int(parts[1]) if len(parts) == 2 else -1
        except ValueError:
            count = -1
        if count < 1 or count > int(cfg["context_messages"]):
            return f"Укажите число от 1 до {cfg['context_messages']}: бот контекст 20"
        set_pref(db, scope, ident, "context_messages", count)
        return f"Контекст: {count} последних сообщений."
    return None


def process_command(cfg, db, peer, sender, question, chat_admin=None):
    """None = обычный вопрос, строка = ответ команды.

    chat_admin вычисляется лениво для безопасного изменения общих настроек.
    """
    parts = question.strip().split()
    if not parts:
        return None
    cmd = parts[0].lower().rstrip("?!.,:")
    args = parts[1:]
    allowed = enabled_models(cfg)
    if cmd in ("меню", "помощь", "help", "старт") and not args:
        return help_text()
    if cmd in ("права", "разрешения", "подключение") and not args:
        return permission_text(cfg)
    if cmd == "выбор" and " ".join(args).lower() in ("поиска", "стиля", "контекста", "резерва"):
        return "Выберите значение кнопками под этим сообщением. Изменение касается только вас."

    if cmd == "модели":
        if not args:
            return model_list(allowed)
        if len(args) == 1 and args[0].isdigit():
            return model_list(allowed, int(args[0]))
        return "Пример: бот модели 2"
    if cmd == "настройки" and not args:
        state = effective(db.db, cfg, peer, sender, allowed)
        return (
            "Ваши действующие настройки:\n"
            "Модель: " + (model_label(state["model"]) if state["model"] else "пока нет доступной") + "\n"
            "Поиск: " + SEARCH_LABEL[state["search"]] + "\n"
            "Стиль: " + STYLE_LABEL[state["style"]] + "\n"
            "Контекст: " + str(state["context_messages"]) + " сообщений\n"
            "Резервный ИИ: " + ("разрешён" if state["fallback"] else "выключен") + "\n"
            "Изменение: бот меню"
        )
    if cmd == "диалог":
        if int(peer) < 2000000000:
            return "Команда «диалог» доступна только в беседе ВК."
        if args[:1] == ["модели"]:
            if len(args) == 1 or (len(args) == 2 and args[1].isdigit()):
                return model_list(allowed, int(args[1]) if len(args) == 2 else 1).replace(
                    "бот модель", "бот диалог модель")
            return "Пример: меню выбора моделей беседы"
        if args[:1] == ["выбор"] and len(args) == 2 and args[1] in ("поиска", "стиля", "контекста"):
            return "Выберите значение кнопками. Настройки беседы меняет только её администратор."
        if not args:
            chat = get_pref(db.db, "chat", peer)
            state = effective(db.db, cfg, peer, sender, allowed)
            return (
                "Настройки беседы (для всех, кто не выбрал личные):\n"
                "Модель: " + (model_label(chat["model"]) if chat.get("model") in allowed else
                              "по умолчанию") + "\n"
                "Поиск: " + SEARCH_LABEL.get(chat.get("search"), "по умолчанию") + "\n"
                "Стиль: " + STYLE_LABEL.get(chat.get("style"), "по умолчанию") + "\n"
                "Контекст: " + str(chat.get("context_messages") or "по умолчанию") + "\n"
                "Резерв: " + {"on": "включён", "off": "выключен"}.get(chat.get("fallback"), "по умолчанию") +
                "\nМенять может только администратор беседы. Пример: бот диалог модель 2"
            )
        if args[0].lower() not in ("модель", "поиск", "стиль", "контекст", "резерв", "сброс", "сбросить"):
            return "Пример: бот диалог модель 2. Команды: бот меню"
        if not chat_admin or not chat_admin():
            return "Настройки беседы может менять только администратор. Проверить права через ВК не удалось или прав недостаточно."
        result = apply_setting(db.db, cfg, "chat", peer, args, allowed)
        return "Настройки беседы. " + result if result else "Пример: бот диалог модель 2"
    if cmd in ("модель", "поиск", "стиль", "контекст", "резерв", "сброс", "сбросить"):
        result = apply_setting(db.db, cfg, "user", sender, [cmd] + args, allowed)
        return "Ваши настройки. " + result if result else "Список команд: бот меню"
    return None
