"""ИИ-провайдеры; никакой модельной нагрузки на сервере."""
import requests


class ProviderError(Exception):
    pass


def generate(cfg, messages):
    provider = cfg["provider"]
    key = cfg.get("api_key", "")
    if not key:
        raise ProviderError("Не указан API-ключ. Выполните sudo aibot")
    model = cfg["model"]
    if provider == "gemini":
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
        except (KeyError, IndexError, ValueError) as exc:
            raise ProviderError("Gemini вернул ответ без текста") from exc

    if provider == "cloudflare":
        account = cfg.get("cloudflare_account", "").strip()
        if not account:
            raise ProviderError("Не указан Cloudflare Account ID")
        url = ("https://api.cloudflare.com/client/v4/accounts/" + account +
               "/ai/run/" + model)
        body = {"messages": messages, "max_tokens": 650}
    else:
        urls = {
            "groq": "https://api.groq.com/openai/v1/chat/completions",
            "openrouter": "https://openrouter.ai/api/v1/chat/completions",
            "custom": cfg.get("custom_url", ""),
        }
        url = urls.get(provider, "")
        if provider == "custom" and not url.startswith("https://"):
            raise ProviderError("Для пользовательского API нужен HTTPS URL")
        if not url:
            raise ProviderError("Неизвестный провайдер")
        body = {"model": model, "messages": messages, "max_tokens": 650,
                "temperature": 0.5}

    try:
        response = requests.post(url, headers={"Authorization": "Bearer " + key},
                                 json=body, timeout=(8, 50))
        _check(response)
        payload = response.json()
        if provider == "cloudflare":
            result = payload.get("result", {})
            return str(result.get("response") or result.get("output") or "").strip()
        return str(payload["choices"][0]["message"]["content"] or "").strip()
    except (ValueError, KeyError, IndexError, TypeError) as exc:
        raise ProviderError("ИИ-сервис вернул неожиданный формат ответа") from exc


def _check(response):
    if response.status_code == 429:
        raise ProviderError("Бесплатная квота ИИ временно исчерпана (HTTP 429)")
    if response.status_code in (401, 403):
        raise ProviderError("Неверный API-ключ или сервис недоступен (HTTP 401/403)")
    if not response.ok:
        raise ProviderError("Ошибка ИИ API: HTTP " + str(response.status_code))
