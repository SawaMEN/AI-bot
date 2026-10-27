"""Русское меню без веб-сервера: ввод токенов, бесплатных моделей и лимитов."""
import getpass
import os
import subprocess
import sys

from ai_bot.catalog import (MODELS, PROVIDERS, SPECS, default_model,
                            enabled_models, model_key, model_label, refresh_free_models)
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
    """Один мастер вместо ручного ввода десятков названий моделей."""
    spec = SPECS[provider]
    name, url = PROVIDERS[provider]
    print("\n--- " + name + " ---")
    print("Регистрация и получение ключа:", url)
    print("Модели запускаются у провайдера, не на вашем VDS.")
    print("Подключайте ТОЛЬКО бесплатный аккаунт, без карты и оплаты.")
    entry = cfg.setdefault("providers", {}).setdefault(provider, {})
    if spec["free_policy"] == "volunteer":
        print("AI Horde: волонтёры обрабатывают ваши сообщения. НЕ используйте для закрытых бесед.")
        print("Можно использовать анонимный ключ 0000000000 без регистрации (низкий приоритет).")
        entry["api_key"] = secret("Личный ключ AI Horde (необязательно)", entry.get("api_key", ""))
    else:
        entry["api_key"] = secret("API-ключ " + name, entry.get("api_key", ""))
    if not (entry.get("api_key") or spec.get("anonymous_key")):
        print("Ключ не указан; провайдер остаётся отключённым.")
        entry["enabled"] = False
        save_config(cfg)
        return
    if provider == "cloudflare":
        entry["cloudflare_account"] = ask("Cloudflare Account ID",
                                          entry.get("cloudflare_account", ""))
        if not entry["cloudflare_account"]:
            print("Не указан Account ID, модель пока недоступна.")
    print("\n1. Автоматически включать ВСЕ подтверждённые бесплатные модели (рекомендуется)")
    print("2. Выбрать конкретные бесплатные модели вручную")
    print("3. Отключить провайдера")
    choice = integer("Режим", 1, 1, 3)
    auto = cfg.setdefault("auto_models_providers", [])
    if choice == 3:
        entry["enabled"] = False
        cfg["auto_models_providers"] = [p for p in auto if p != provider]
        save_config(cfg)
        print("Провайдер отключён.")
        return
    entry["enabled"] = True
    if choice == 1:
        if provider not in auto:
            auto.append(provider)
        # Ручной выбор для провайдера убираем только у этого провайдера.
        cfg["enabled_models"] = [k for k in cfg["enabled_models"]
                                  if not k.startswith(provider + ":")]
        if spec["free_policy"] == "curated":
            print("Включены безопасные модели из локального белого списка:", len(MODELS[provider]))
        else:
            print("Получаю каталог и проверяю стоимость моделей...")
            status = refresh_free_models(cfg, provider)
            count, err = status.get(provider, (0, "неизвестная ошибка"))
            if err:
                print("Каталог временно недоступен:", err,
                      "— до новой проверки платные/неизвестные модели не включаются.")
            else:
                print("Найдено бесплатных моделей:", count)
        print("Новые бесплатные модели будут проверяться автоматически каждые 12 часов.")
    else:
        cfg["auto_models_providers"] = [p for p in auto if p != provider]
        items = list(MODELS[provider])
        if spec["free_policy"] != "curated":
            print("Проверяю бесплатные модели через API...")
            status = refresh_free_models(cfg, provider)
            if status.get(provider, (0, "ошибка"))[1]:
                print("Каталог не ответил. Попробуйте автоматический режим позже.")
            items = cfg.get("_verified_free_models", {}).get(provider, [])
        if not items:
            print("Сейчас нет подтверждённых бесплатных моделей.")
            entry["enabled"] = False
        else:
            existing = [k.split(":", 1)[1] for k in cfg.get("enabled_models", [])
                        if k.startswith(provider + ":")]
            picked = choose_indices("Номера моделей через запятую", items, existing)
            cfg["enabled_models"] = [k for k in cfg["enabled_models"]
                                      if not k.startswith(provider + ":")] + [
                                          model_key(provider, m) for m in picked]
            print("Разрешено моделей:", len(picked))
    if cfg.get("default_model") not in enabled_models(cfg):
        cfg["default_model"] = default_model(cfg) or ""
    save_config(cfg)
    print("Готово. Изменения вступят после перезапуска бота.")


def configure_ai(cfg):
    while True:
        keys = list(PROVIDERS)
        n = len(keys)
        active = set(cfg.get("auto_models_providers", [])) | set(
            k.split(":", 1)[0] for k in cfg.get("enabled_models", []))
        print("\n====== Бесплатные ИИ — управление ======")
        for i, key in enumerate(keys, 1):
            entry = cfg.get("providers", {}).get(key, {})
            flag = "ВКЛ/авто" if entry.get("enabled") and key in active else "выкл"
            print(f"{i:>2}. {PROVIDERS[key][0]} [{flag}]")
        print(f"{n+1}. Модель по умолчанию")
        print(f"{n+2}. Проверить и обновить бесплатные каталоги")
        print(f"{n+3}. Включить автоподбор у всех УЖЕ подключённых провайдеров")
        print("0. Назад")
        number = integer("Номер пункта", 0, 0, n + 3)
        if number == 0:
            break
        if 1 <= number <= n:
            edit_provider(cfg, keys[number - 1])
        elif number == n + 1:
            refresh_free_models(cfg)
            allowed = enabled_models(cfg)
            if not allowed:
                print("Сначала настройте бесплатных провайдеров.")
                continue
            for i, key in enumerate(allowed, 1):
                print(i, model_label(key))
            index = integer("Выберите номер основной модели", 1, 1, len(allowed))
            cfg["default_model"] = allowed[index - 1]
            save_config(cfg)
        elif number == n + 2:
            for provider, (count, error) in refresh_free_models(cfg).items():
                print(f"{PROVIDERS[provider][0]}: {count} бесплатных моделей" +
                      (f" (ошибка: {error})" if error else ""))
            print("Всего доступно:", len(enabled_models(cfg)))
        elif number == n + 3:
            for provider in keys:
                entry = cfg.get("providers", {}).get(provider, {})
                if entry.get("enabled") and (entry.get("api_key") or SPECS[provider].get("anonymous_key")):
                    if provider not in cfg.setdefault("auto_models_providers", []):
                        cfg["auto_models_providers"].append(provider)
            save_config(cfg)
            refresh_free_models(cfg)
            print("Автоподбор включён для всех подключённых провайдеров.")
    print("Автоматически поддерживаемых провайдеров:",
          len(cfg.get("auto_models_providers", [])))


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
    refresh_free_models(cfg)
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
        connected = any(
            item.get("enabled") and (item.get("api_key") or SPECS[name].get("anonymous_key"))
            for name, item in cfg.get("providers", {}).items() if name in SPECS)
        sys.exit(0 if cfg["vk_token"] and cfg["group_id"] and connected else 1)
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
