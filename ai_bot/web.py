"""Локальная панель администратора без внешних зависимостей.

Запускается вручную: sudo aibot web
Только 127.0.0.1, случайный пароль на сеанс, Basic Auth, CSRF и no-store.
Публично открывать порт НЕ НАДО: используйте SSH -L.
"""
import base64
import hashlib
import hmac
from html import escape
from http.server import BaseHTTPRequestHandler, HTTPServer
import secrets
import subprocess
from urllib.parse import parse_qs, urlsplit

from ai_bot.catalog import PROVIDERS, SPECS, default_model, enabled_models, model_label, refresh_free_models
from ai_bot.core import load_config, save_config


def _num(form, field, minimum, maximum, default):
    try:
        number = int(form.get(field, default))
    except (ValueError, TypeError) as exc:
        raise ValueError("Недопустимое число: " + field) from exc
    if not minimum <= number <= maximum:
        raise ValueError("Недопустимый диапазон: " + field)
    return number


def apply_form(cfg, form):
    """Изменяются только явно разрешённые поля, никогда не URL/API из ВК."""
    action = form.get("action", "")
    if action == "settings":
        cfg["context_messages"] = _num(form, "context_messages", 1, 100, 30)
        cfg["context_hours"] = _num(form, "context_hours", 1, 168, 24)
        cfg["retention_days"] = _num(form, "retention_days", 1, 30, 7)
        cfg["max_requests_hour"] = _num(form, "max_requests_hour", 1, 10000, 50)
        cfg["max_user_requests_hour"] = _num(form, "max_user_requests_hour", 1, 1000, 12)
        cfg["max_chat_requests_hour"] = _num(form, "max_chat_requests_hour", 1, 1000, 25)
        cfg["search_enabled"] = form.get("search_enabled") == "on"
        cfg["free_fallback"] = form.get("free_fallback") == "on"
        return "Ограничения сохранены"
    if action == "provider":
        provider = form.get("provider")
        if provider not in SPECS:
            raise ValueError("Неизвестный провайдер")
        spec = SPECS[provider]
        entry = cfg.setdefault("providers", {}).setdefault(provider, {})
        key = form.get("api_key", "")
        if key:
            if len(key) > 500:
                raise ValueError("Ключ слишком длинный")
            entry["api_key"] = key
        if form.get("remove_key") == "on":
            entry["api_key"] = ""
        if provider == "cloudflare" and form.get("cloudflare_account", ""):
            value = form["cloudflare_account"]
            if len(value) > 100 or not all(c.isalnum() or c in "-_" for c in value):
                raise ValueError("Неверный Account ID")
            entry["cloudflare_account"] = value
        enabled = form.get("enabled") == "on"
        if enabled and not (entry.get("api_key") or spec.get("anonymous_key")):
            raise ValueError("Для этого провайдера нужен API-ключ")
        entry["enabled"] = enabled
        enabled_auto = form.get("auto") == "on"
        auto = cfg.setdefault("auto_models_providers", [])
        cfg["auto_models_providers"] = [p for p in auto if p != provider]
        if enabled and enabled_auto:
            cfg["auto_models_providers"].append(provider)
            # Включаем только список бесплатных моделей из реестра / проверки цен.
            cfg["enabled_models"] = [
                k for k in cfg.get("enabled_models", []) if not k.startswith(provider + ":")]
        return "Настройки " + spec["name"] + " сохранены"
    if action == "default":
        chosen = form.get("model", "")
        if chosen not in enabled_models(cfg):
            raise ValueError("Модель не разрешена сервером")
        cfg["default_model"] = chosen
        return "Основная модель сохранена"
    raise ValueError("Неизвестное действие")


def render_page(cfg, csrf, note=""):
    """Все значения экранируются; токены в HTML никогда не выводятся."""
    esc = lambda value: escape(str(value), quote=True)
    allowed = enabled_models(cfg)
    parts = [
        "<!doctype html><html lang='ru'><head><meta charset='utf-8'>"
        "<meta name='viewport' content='width=device-width,initial-scale=1'>"
        "<title>AI-bot — управление</title>"
        "<style>body{font:15px system-ui,sans-serif;background:#101820;color:#ecf0f5;"
        "max-width:950px;margin:auto;padding:25px}section{padding:18px;margin:16px 0;"
        "background:#1b2934;border-radius:12px}input,select,button{padding:9px;"
        "margin:5px;max-width:100%;border-radius:6px}input{background:#f3f4f6;color:#101820}"
        "label{display:inline-block;margin:4px}button{cursor:pointer}p{line-height:1.5}"
        "small{color:#c1c9d0}a{color:#8fcbff}</style></head><body>",
        "<h1>AI-bot — панель администратора</h1>",
        "<p>Локальная панель через SSH. Пароли и токены в браузере не отображаются."
        " Настройки применяются после перезапуска бота.</p>",
        "<p><small>Внешние бесплатные тарифы ограничены; отключите платный биллинг в аккаунтах провайдеров.</small></p>",
    ]
    if note:
        parts.append("<section><strong>" + esc(note) + "</strong></section>")
    parts.append("<section><h2>Система</h2><p>Разрешено моделей: <b>" +
                 str(len(allowed)) + "</b></p><form method='post'>"
                 "<input type='hidden' name='csrf' value='" + esc(csrf) + "'>"
                 "<input type='hidden' name='action' value='restart'>"
                 "<button type='submit'>Применить настройки и перезапустить бота</button></form></section>")
    parts.append("<section><h2>Основная модель</h2><form method='post'>"
                 "<input type='hidden' name='csrf' value='" + esc(csrf) + "'>"
                 "<input type='hidden' name='action' value='default'><select name='model'>")
    current = default_model(cfg)
    for key in allowed:
        parts.append("<option value='" + esc(key) + "'" + (" selected" if current == key else "") +
                     ">" + esc(model_label(key)) + "</option>")
    parts.append("</select><button" + (" disabled" if not allowed else "") +
                 ">Сохранить модель</button></form></section>")
    parts.append("<section><h2>Провайдеры</h2>"
                 "<p>Включайте автоматический поиск, если хотите получать новые бесплатные модели."
                 " Для индивидуального списка моделей используйте <code>sudo aibot</code>.</p>")
    for provider, spec in SPECS.items():
        detail = cfg.get("providers", {}).get(provider, {})
        on = bool(detail.get("enabled"))
        auto = provider in cfg.get("auto_models_providers", [])
        parts.append("<form method='post'><h3>" + esc(spec["name"]) + "</h3>"
                     "<p><a rel='noreferrer noopener' href='" + esc(spec["key_url"]) +
                     "'>Как получить бесплатный ключ</a></p>"
                     "<input type='hidden' name='csrf' value='" + esc(csrf) + "'>"
                     "<input type='hidden' name='action' value='provider'>"
                     "<input type='hidden' name='provider' value='" + esc(provider) + "'>"
                     "<label><input type='checkbox' name='enabled'" +
                     (" checked" if on else "") + ">Включён</label>"
                     "<label><input type='checkbox' name='auto'" +
                     (" checked" if auto else "") + ">Автоматические бесплатные модели</label>"
                     "<label>API-ключ: <input type='password' name='api_key' maxlength='500'"
                     " placeholder='" + ("Настроен — оставьте пустым" if detail.get("api_key") else "Укажите ключ")
                     + "' autocomplete='new-password'></label>"
                     "<label><input type='checkbox' name='remove_key'>Стереть ключ</label>")
        if provider == "cloudflare":
            parts.append("<label>Account ID: <input name='cloudflare_account' value='" +
                         esc(detail.get("cloudflare_account", "")) + "' maxlength='100'></label>")
        parts.append("<button>Сохранить провайдера</button></form><hr>")
    parts.append("</section>")
    parts.append("<section><h2>Общие ограничения</h2><form method='post'>"
                 "<input type='hidden' name='csrf' value='" + esc(csrf) + "'>"
                 "<input type='hidden' name='action' value='settings'>")
    fields = [
        ("context_messages", "Максимум сообщений контекста"),
        ("context_hours", "Часов контекста"),
        ("retention_days", "Дней хранения"),
        ("max_requests_hour", "Вызовов ИИ за час (все)"),
        ("max_user_requests_hour", "Вызовов одного участника за час"),
        ("max_chat_requests_hour", "Вызовов одной беседы за час"),
    ]
    for key, label in fields:
        parts.append("<label>" + esc(label) + ": <input type='number' name='" + key +
                     "' value='" + esc(cfg.get(key, 0)) + "'></label>")
    for key, label in [("search_enabled", "Разрешить веб-поиск"),
                       ("free_fallback", "Разрешить резервную бесплатную модель")]:
        parts.append("<label><input type='checkbox' name='" + key + "'" +
                     (" checked" if cfg.get(key, False) else "") + ">" + esc(label) + "</label>")
    parts.append("<p><small>Участники могут отключить резерв самостоятельно через кнопки ВК."
                 " Резерв может отправить контекст другому провайдеру.</small></p>"
                 "<button>Сохранить ограничения</button></form></section>")
    parts.append("<p><small>Закройте терминал с панелью, когда закончите. "
                 "Никогда не пробрасывайте этот порт в публичный интернет.</small></p></body></html>")
    return "".join(parts)


def run_web(port=8765):
    """Без запуска root-сервиса в фоне; владелец сам открывает SSH-туннель."""
    password = secrets.token_urlsafe(22)
    csrf = secrets.token_urlsafe(24)
    required = "Basic " + base64.b64encode(("admin:" + password).encode()).decode()
    initial_cfg = load_config()
    refresh_free_models(initial_cfg)
    state = {"status": "", "catalog": initial_cfg.get("_verified_free_models", {})}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass  # не записываем в логи HTTP-заголовки и пароли

        def headers_common(self):
            self.send_header("Cache-Control", "no-store, max-age=0")
            self.send_header("X-Frame-Options", "DENY")
            self.send_header("X-Content-Type-Options", "nosniff")
            self.send_header("Content-Security-Policy",
                             "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")

        def authorized(self):
            host = self.headers.get("Host", "")
            if host not in ("127.0.0.1:" + str(port), "localhost:" + str(port),
                            "127.0.0.1", "localhost"):
                self.send_error(400, "Недопустимый Host")
                return False
            supplied = self.headers.get("Authorization", "")
            if not hmac.compare_digest(supplied, required):
                self.send_response(401)
                self.send_header("WWW-Authenticate", 'Basic realm="AI-bot"')
                self.headers_common()
                self.end_headers()
                return False
            return True

        def do_GET(self):
            if not self.authorized():
                return
            if urlsplit(self.path).path != "/":
                self.send_error(404)
                return
            cfg = load_config()
            cfg["_verified_free_models"] = state["catalog"]
            html = render_page(cfg, csrf, state["status"]).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(html)))
            self.headers_common()
            self.end_headers()
            self.wfile.write(html)

        def do_POST(self):
            if not self.authorized():
                return
            if self.path != "/":
                self.send_error(404)
                return
            try:
                size = int(self.headers.get("Content-Length", "0"))
            except ValueError:
                self.send_error(400)
                return
            if not 1 <= size <= 8192:
                self.send_error(413)
                return
            payload = parse_qs(self.rfile.read(size).decode("utf-8"),
                               keep_blank_values=True)
            fields = {k: v[-1] for k, v in payload.items()}
            if not hmac.compare_digest(fields.get("csrf", ""), csrf):
                self.send_error(403)
                return
            try:
                if fields.get("action") == "restart":
                    subprocess.run(["systemctl", "restart", "ai-bot"], check=True,
                                   capture_output=True, timeout=15)
                    state["status"] = "Бот перезапущен."
                else:
                    cfg = load_config()
                    cfg["_verified_free_models"] = state["catalog"]
                    state["status"] = apply_form(cfg, fields)
                    if fields.get("action") == "provider":
                        refresh_free_models(cfg, fields.get("provider"))
                        state["catalog"] = cfg.get("_verified_free_models", {})
                    save_config(cfg)
            except (ValueError, subprocess.CalledProcessError, subprocess.TimeoutExpired) as exc:
                state["status"] = "Ошибка сохранения: " + str(exc)[:160]
            self.send_response(303)
            self.send_header("Location", "/")
            self.headers_common()
            self.end_headers()

    server = HTTPServer(("127.0.0.1", int(port)), Handler)
    print("\nAI-bot — веб-панель (доступ только через SSH-туннель)")
    print("На вашем компьютере запустите: ssh -L 8765:127.0.0.1:8765 USER@SERVER")
    print("Откройте в браузере http://127.0.0.1:8765")
    print("Логин: admin")
    print("Разовый пароль: " + password)
    print("Нажмите Ctrl+C, чтобы закрыть панель.\n")
    try:
        server.serve_forever(poll_interval=0.2)
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()


if __name__ == "__main__":
    run_web()
