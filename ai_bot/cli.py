"""Русское меню без веб-сервера: ввод токенов, бесплатных моделей и лимитов."""
import getpass
import os
import subprocess
import sys

from ai_bot.catalog import (MODELS, PROVIDERS, default_model, discover_openrouter,
                            enabled_models, model_key, model_label)
from ai_bot.core import History, load_config, save_config


def require_root():
    if os.geteuid() != 0:
        raise SystemExit("Для изменения настроек используйте sudo aibot")


def ask(label, default=""):
    suffix = " [" + str(default) + "]" if default != "" else ""
    value = input(f"{label}{suffix}: ").strip()
    return value if value else str(default)


def secret(label, existing=""):
    value = getpass.getpass(f"{label} (Enter — оставить прежний, '-' — стереть): ").strip()
    return "" if value == "-" else (value or existing)


def integer(label, default, minimum, maximum):
    while True:
        try:
            number = int(ask(label, default))
            if minimum <= number <= maximum:
                return number
        except ValueError:
            pass
        print(f"Допустимое значение: от {minimum} до {maximum}.")


def configure_vk(cfg):
    print("\n--- Подключение ВКонтакте ---")
    print("ВК: создайте токен СООБЩЕСТВА с разрешением «Сообщения».")
    print("Разрешите Bots Long Poll: входящие сообщения. Добавьте сообщество в беседу.")
    print("Чтобы читать контекст, выдайте сообществу доступ ко всей переписке.")
    print("В беседе: название → участники → сообщество → права → «Доступ ко всей переписке».")
    print("Для кнопок в ВК включите событие message_event в Bots Long Poll.")
    print("Помощь прямо в ВК: упомяните сообщество и напишите «права».")
    cfg["vk_token"] = secret("Ключ сообщества ВК", cfg.get("vk_token", ""))
    cfg["group_id"] = integer("Числовой ID сообщества", cfg["group_id"], 1, 999999999999)
    save_config(cfg)
    print("ВК настроен. Секреты не выводятся в меню.")


def choose_indices(label, items, current=()):
    if not items:
        return []
    for number, (ident, description) in enumerate(items, 1):
        selected = " [включена]" if ident in current else ""
        print(f" {number:>2}. {description} ({ident}){selected}")
    while True:
        raw = ask(label, ",".join(str(i) for i, (ident, _) in enumerate(items, 1)
                                   if ident in current) or "1").replace(" ", "")
        try:
            indexes = [int(part) for part in raw.split(",") if part]
            if indexes and all(1 <= i <= len(items) for i in indexes):
                return list(dict.fromkeys(items[i-1][0] for i in indexes))
        except ValueError:
            pass
        print("Введите номера через запятую, например 1,3,5")


def edit_provider(cfg, provider):
    print("\n--- " + PROVIDERS[provider][0] + " ---")
    print("Как получить API-ключ:", PROVIDERS[provider][1] or "ваш поставщик API")
    print("Это внешний API, никакие модели на VDS устанавливать не нужно.")
    print("НЕ добавляйте карту и НЕ переходите на платный тариф; бесплатные лимиты ограничены.")
    entry = cfg.setdefault("providers", {}).setdefault(provider, {})
    entry["api_key"] = secret("API-ключ", entry.get("api_key", ""))
    if not entry["api_key"]:
        entry["enabled"] = False
        cfg["enabled_models"] = [k for k in cfg.get("enabled_models", [])
                                  if not k.startswith(provider + ":")]
        save_config(cfg)
        print("API-ключ не задан, провайдер выключен.")
        return
    if provider == "cloudflare":
        entry["cloudflare_account"] = ask("Cloudflare Account ID",
                                           entry.get("cloudflare_account", ""))
        if not entry["cloudflare_account"]:
            print("Без Account ID этот провайдер будет недоступен.")
    choices = list(MODELS[provider])
    if provider == "openrouter":
            print("Автоматически запрашиваю актуальные бесплатные модели OpenRouter…")
            try:
                dynamic = discover_openrouter(entry["api_key"])
                choices += [pair for pair in dynamic if pair[0] not in {c[0] for c in choices}]
                print("Обнаружено бесплатных моделей:", len(dynamic))
            except Exception as exc:
                print("Каталог сейчас недоступен; доступен бесплатный автоподбор.")
    if provider == "mistral":
        print("Используйте план Studio Free без привязанной карты. Доступные модели зависят от вашего аккаунта.")
    if provider == "siliconflow":
        print("Для бесплатных моделей SiliconFlow может требоваться проверка личности.")
    if provider == "modelscope":
        print("Выберите модели, у которых указан бесплатный провайдер ModelScope.")
    print("ВНИМАНИЕ: только бесплатные модели из проверенного списка, без пользовательских платных ID.")
    print("Отметьте только модели, которые хотите разрешить участникам ВК.")
    current = [k.split(":", 1)[1] for k in cfg.get("enabled_models", [])
               if k.startswith(provider + ":")]
    ids = choose_indices("Номера разрешённых моделей через запятую", choices, current) if choices else []
    cfg["enabled_models"] = [k for k in cfg.get("enabled_models", [])
                              if not k.startswith(provider + ":")] + [model_key(provider, m) for m in ids]
    entry["enabled"] = bool(ids)
    if cfg.get("default_model") not in enabled_models(cfg):
        cfg["default_model"] = default_model(cfg) or ""
    save_config(cfg)
    print(f"Сохранено моделей: {len(ids)}. Изменения вступят после перезапуска бота.")


def configure_ai(cfg):
    while True:
        print("\n--- Провайдеры и модели ---")
        keys = list(PROVIDERS)
        active = set(k.split(":", 1)[0] for k in enabled_models(cfg))
        for i, key in enumerate(keys, 1):
            flag = "ВКЛ" if key in active else "выкл"
            print(f"{i}. {PROVIDERS[key][0]} — {flag}")
        print("9. Модель по умолчанию для новых участников")
        print("10. Отключить провайдера")
        print("0. Назад / сохранить")
        number = integer("Выберите пункт", 0, 0, 10)
        if number == 0:
            break
        if 1 <= number <= len(keys):
            edit_provider(cfg, keys[number-1])
        elif number == 9:
            allowed = enabled_models(cfg)
            if not allowed:
                print("Сначала настройте хотя бы один провайдер.")
                continue
            for index, key in enumerate(allowed, 1):
                print(f"{index}. {model_label(key)}")
            default = allowed.index(default_model(cfg)) + 1
            index = integer("Номер модели по умолчанию", default, 1, len(allowed))
            cfg["default_model"] = allowed[index-1]
            save_config(cfg)
        elif number == 10:
            for i, key in enumerate(keys, 1):
                print(i, PROVIDERS[key][0])
            n = integer("Кого отключить", 1, 1, len(keys))
            provider = keys[n-1]
            cfg.setdefault("providers", {}).setdefault(provider, {})["enabled"] = False
            if cfg.get("default_model") not in enabled_models(cfg):
                cfg["default_model"] = default_model(cfg) or ""
            save_config(cfg)
            print("Отключено. Личные предпочтения переключатся на доступную модель.")
    if enabled_models(cfg):
        print("Активных моделей:", len(enabled_models(cfg)))
    else:
        print("Внимание: нет активных моделей. Бот пока не сможет отвечать.")


def configure_behavior(cfg):
    print("\n--- Основные настройки и ограничения ---")
    print("У каждого пользователя и беседы будут свои настройки в ВК.")
    print("Эти значения — пределы, которые участники не смогут превысить.")
    cfg["trigger"] = ask("Обращение к боту", cfg["trigger"]).lower()
    cfg["context_messages"] = integer("Максимум сообщений контекста",
                                       cfg["context_messages"], 1, 100)
    cfg["context_hours"] = integer("Контекст за последние часы",
                                    cfg["context_hours"], 1, 168)
    cfg["retention_days"] = integer("Хранить историю (дней)",
                                    cfg["retention_days"], 1, 30)
    cfg["search_enabled"] = ask("Разрешить веб-поиск вообще? (д/н)",
                                "д" if cfg["search_enabled"] else "н").lower() in ("д", "да", "y", "yes")
    cfg["cooldown_seconds"] = integer("Пауза между ответами одной беседе (сек)",
                                      cfg["cooldown_seconds"], 0, 3600)
    cfg["max_requests_hour"] = integer("Максимум запросов к ИИ за час",
                                       cfg["max_requests_hour"], 1, 10000)
    save_config(cfg)


def service(action):
    subprocess.run(["systemctl", action, "ai-bot"], check=False)


def show_status(cfg):
    allowed = enabled_models(cfg)
    print("\n--- AI-bot — состояние ---")
    print("ВКонтакте:", "настроен" if cfg["vk_token"] and cfg["group_id"] else "нужна настройка")
    print("Разрешено моделей:", len(allowed))
    for key in allowed:
        print(" •", model_label(key))
    print("Основная модель:", model_label(default_model(cfg)) if allowed else "нет")
    print("Контекст:", cfg["context_messages"], "сообщений за", cfg["context_hours"], "ч.")
    print("Веб-поиск:", "разрешён" if cfg["search_enabled"] else "запрещён")
    service("status")


def menu():
    while True:
        cfg = load_config()
        print("\n====== AI-bot — русское меню управления ======")
        print("1. Подключить ВКонтакте")
        print("2. Разрешённые провайдеры и модели")
        print("3. Контекст, поиск и лимиты")
        print("4. Посмотреть состояние")
        print("5. Перезапустить бота")
        print("6. Последние ошибки / журнал")
        print("7. Очистить историю бесед")
        print("8. Обновить программу")
        print("0. Выход")
        choice = input("Пункт меню: ").strip()
        if choice == "0":
            return
        if choice == "1":
            configure_vk(cfg)
            service("restart")
        elif choice == "2":
            configure_ai(cfg)
            service("restart")
        elif choice == "3":
            configure_behavior(cfg)
            service("restart")
        elif choice == "4":
            show_status(cfg)
        elif choice == "5":
            service("restart")
        elif choice == "6":
            subprocess.run(["journalctl", "-u", "ai-bot", "-n", "35", "--no-pager"], check=False)
        elif choice == "7":
            if ask("Стереть всю историю? Напишите ДА", "нет") == "ДА":
                history = History()
                history.clear()
                history.db.close()
                print("История очищена.")
        elif choice == "8":
            subprocess.run(["bash", "/opt/ai-bot/install.sh", "--update"], check=False)


def main():
    action = sys.argv[1] if len(sys.argv) > 1 else "menu"
    cfg = load_config()
    if action == "is-configured":
        sys.exit(0 if cfg["vk_token"] and cfg["group_id"] and enabled_models(cfg) else 1)
    if action in ("setup", "menu", "vk", "api", "behavior", "clear", "restart"):
        require_root()
    if action == "setup":
        configure_vk(cfg)
        configure_ai(cfg)
        configure_behavior(cfg)
    elif action == "menu":
        menu()
    elif action == "vk":
        configure_vk(cfg)
        service("restart")
    elif action == "api":
        configure_ai(cfg)
        service("restart")
    elif action == "behavior":
        configure_behavior(cfg)
        service("restart")
    elif action == "clear":
        History().clear()
        print("История очищена.")
    elif action == "restart":
        service("restart")
    elif action == "status":
        show_status(cfg)
    elif action == "logs":
        subprocess.run(["journalctl", "-u", "ai-bot", "-n", "60", "--no-pager"], check=False)
    else:
        raise SystemExit("Команды: setup, vk, api, behavior, status, logs, clear, restart")


if __name__ == "__main__":
    main()
