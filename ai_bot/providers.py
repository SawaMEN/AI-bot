"""Несколько облачных API. Сервер выполняет только HTTP-запросы."""
import requests

from ai_bot.catalog import enabled_models, split_model


class ProviderError(Exception):
    pass


ENDPOINTS = {
    "groq": "https://api.groq.com/openai/v1/chat/completions",
    "openrouter": "https://openrouter.ai/api/v1/chat/completions",
    "mistral": "https://api.mistral.ai/v1/chat/completions",
    "huggingface": "https://router.huggingface.co/v1/chat/completions",
}


def generate(cfg, messages, selected=None):
    """Не допускает вызовы моделей вне белого списка администратора."""
    allowed = enabled_models(cfg)
    chosen = selected or cfg.get("default_model")
    if chosen not in allowed:
        # Обратная совместимость для старых тестов и миграции без выбора модели.
        if not selected and allowed:
            chosen = allowed[0]
        else:
            raise ProviderError("Выбранная модель отключена администратором")
    provider, model = split_model(chosen)
    settings = cfg["providers"][provider]
    key = settings["api_key"]
    if provider == "gemini":
        # Здесь model берётся только из белого списка; key передаём заголовком.
        url = "https://generativelanguage.googleapis.com/v1beta/models/" + model + ":generateContent"
        prompt = "\n\n".join(m["role"].upper() + ":\n" + m["content"] for m in messages)
        body = {"contents": [{"parts": [{"text": prompt}]}],
                "generationConfig": {"maxOutputTokens": 700}}
        try:
            response = requests.post(url, headers={"x-goog-api-key": key},
                                     json=body, timeout=(8, 50))
            _check(response)
            parts = response.json()["candidates"][0]["content"]["parts"]
            return "".join(p.get("text", "") for p in parts).strip()
        except (KeyError, IndexError, ValueError, TypeError) as exc:
            raise ProviderError("Gemini не вернул текст") from exc

    if provider == "cloudflare":
        account = settings.get("cloudflare_account", "").strip()
        if not account:
            raise ProviderError("Cloudflare Account ID не задан")
        url = "https://api.cloudflare.com/client/v4/accounts/" + account + "/ai/run/" + model
        body = {"messages": messages, "max_tokens": 650}
    else:
        if provider == "custom":
            url = settings.get("custom_url", "")
            if not url.startswith("https://"):
                raise ProviderError("Пользовательский API должен использовать HTTPS")
        else:
            url = ENDPOINTS.get(provider)
        if not url:
            raise ProviderError("ИИ API не поддерживается")
        if provider == "openrouter" and model != "openrouter/free" and not model.endswith(":free"):
            raise ProviderError("Запрещено использование платных моделей OpenRouter")
        body = {"model": model, "messages": messages, "max_tokens": 650}

    try:
        response = requests.post(url, headers={"Authorization": "Bearer " + key},
                                 json=body, timeout=(8, 50))
        _check(response)
        payload = response.json()
        if provider == "cloudflare":
            output = payload.get("result") or {}
            answer = output.get("response") or output.get("output") or ""
            return str(answer).strip()
        answer = payload["choices"][0]["message"]["content"]
        return str(answer or "").strip()
    except (ValueError, KeyError, IndexError, TypeError) as exc:
        raise ProviderError("ИИ-сервис вернул неверный формат") from exc


def _check(response):
    if response.status_code == 429:
        raise ProviderError("Бесплатная квота ИИ временно исчерпана")
    if response.status_code in (401, 403):
        raise ProviderError("Неверный API-ключ или доступ запрещён")
    if not response.ok:
        raise ProviderError("Ошибка ИИ API: HTTP " + str(response.status_code))
