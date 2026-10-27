"""Русское меню управления ботом (запускать от root для изменения настроек)."""
import getpass
import os
import subprocess
import sys

from ai_bot.core import DEFAULTS, PROVIDERS, History, load_config, save_config


def require_root():
    if os.geteuid() != 0:
        raise SystemExit("Для изменения настроек используйте: sudo aibot")


def ask(label, default=""):
    suffix = f" [{default}]" if default else ""
    value = input(f"{label}{suffix}: ").strip()
    return value or str(default)


def secret(label, existing=""):
    value = getpass.getpass(f"{label} (Enter — оставить текущий): ").strip()
    return value or existing


def integer(label, default, minimum, maximum):
    while True:
        value = ask(label, default)
        try:
            num = int(value)
            if minimum <= num <= maximum:
                return num
        except ValueError:
            pass
        print(f"Введите целое число от {minimum} до {maximum}")


def configure_vk(cfg):
    print("\n--- Подключение ВКонтакте ---")
    print("Используйте токен СООБЩЕСТВА с правом messages, не личный токен.")
    cfg["vk_token"] = secret("Ключ доступа сообщества ВК", cfg["vk_token"])
    cfg["group_id"] = integer("Числовой ID сообщества", cfg["group_id"], 1, 999999999999)
    save_config(cfg)
    print("Настройки ВК сохранены.")


def configure_ai(cfg):
    print("\n--- Выбор ИИ API ---")
    keys = list(PROVIDERS)
    for i, key in enumerate(keys, 1):
        print(f"{i}. {PROVIDERS[key][0]}")
    current = keys.index(cfg["provider"]) + 1
    provider = keys[integer("Номер провайдера", current, 1, len(keys)) - 1]
    old = cfg["provider"]
    cfg["provider"] = provider
    cfg["model"] = ask("Модель", PROVIDERS[provider][1] if old != provider else cfg["model"])
    cfg["api_key"] = secret("API-ключ " + PROVIDERS[provider][0],
                             cfg["api_key"] if old == provider else "")
    if provider == "cloudflare":
        cfg["cloudflare_account"] = ask("Cloudflare Account ID", cfg.get("cloudflare_account", ""))
    if provider == "custom":
        cfg["custom_url"] = ask("HTTPS URL /chat/completions", cfg.get("custom_url", ""))
    save_config(cfg)
    print("ИИ-провайдер сохранён. Для применения перезапустите бота.")


def configure_behavior(cfg):
    print("\n--- Контекст и поиск ---")
    cfg["trigger"] = ask("Обращение к боту в беседе", cfg["trigger"]).lower()
    cfg["context_messages"] = integer("Сколько последних сообщений учитывать",
                                      cfg["context_messages"], 1, 100)
    cfg["context_hours"] = integer("Окно контекста в часах", cfg["context_hours"], 1, 168)
    cfg["retention_days"] = integer("Хранить историю (дни)", cfg["retention_days"], 1, 30)
    cfg["search_enabled"] = ask("Искать сайты по запросам? (д/н)",
                                "д" if cfg["search_enabled"] else "н").lower() in ("д", "да", "y", "yes")
    cfg["cooldown_seconds"] = integer("Пауза между ИИ-ответами в беседе (сек)",
                                      cfg["cooldown_seconds"], 0, 3600)
    cfg["max_requests_hour"] = integer("Общий предел ИИ-запросов за час",
                                       cfg["max_requests_hour"], 1, 10000)
    save_config(cfg)
    print("Настройки поведения сохранены.")


def service(action):
    subprocess.run(["systemctl", action, "ai-bot"], check=False)


def show_status(cfg):
    print("\n--- Текущие настройки ---")
    print("ВК:", "настроен" if cfg["vk_token"] and cfg["group_id"] else "не настроен")
    print("API:", PROVIDERS[cfg["provider"]][0], "—", cfg["model"])
    print("API-ключ:", "задан" if cfg["api_key"] else "не задан")
    print("Контекст:", cfg["context_messages"], "сообщений за", cfg["context_hours"], "ч.")
    print("Поиск:", "включён" if cfg["search_enabled"] else "выключен")
    print("Хранение:", cfg["retention_days"], "дней")
    service("status")


def menu():
    while True:
        cfg = load_config()
        print("\n====== AI-bot — управление ======")
        print("1. Настроить ВКонтакте")
        print("2. Выбрать ИИ API и модель")
        print("3. Настроить контекст, поиск и лимиты")
        print("4. Состояние бота")
        print("5. Перезапустить бота")
        print("6. Посмотреть последние логи")
        print("7. Очистить историю сообщений")
        print("8. Обновить AI-bot")
        print("0. Выход")
        choice = input("Выберите пункт: ").strip()
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
            subprocess.run(["journalctl", "-u", "ai-bot", "-n", "35", "--no-pager"],
                           check=False)
        elif choice == "7":
            if ask("Удалить ВСЮ сохранённую переписку? Напишите ДА", "нет") == "ДА":
                History().clear()
                print("История удалена.")
        elif choice == "8":
            subprocess.run(["bash", "/opt/ai-bot/install.sh", "--update"], check=False)


def main():
    action = sys.argv[1] if len(sys.argv) > 1 else "menu"
    if action == "is-configured":
        cfg = load_config()
        sys.exit(0 if cfg["vk_token"] and cfg["group_id"] and cfg["api_key"] else 1)
    if action in ("setup", "menu", "vk", "api", "behavior", "clear", "restart"):
        require_root()
    cfg = load_config()
    if action == "setup":
        print("Первоначальная настройка AI-bot")
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
        print("История удалена.")
    elif action == "restart":
        service("restart")
    elif action == "status":
        show_status(cfg)
    elif action == "logs":
        subprocess.run(["journalctl", "-u", "ai-bot", "-n", "60", "--no-pager"],
                       check=False)
    else:
        raise SystemExit("Команды: setup, vk, api, behavior, status, logs, clear, restart")


if __name__ == "__main__":
    main()
