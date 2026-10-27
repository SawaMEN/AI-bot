"""Подсказки по доступным моделям; доступ определяет администратор.

Каталог не обещает доступность модели на конкретном аккаунте.
У OpenRouter автоматически подбираются ТОЛЬКО модели с нулевой ценой.
"""
import requests

PROVIDERS = {
    "groq": ("GroqCloud", "https://console.groq.com/keys"),
    "gemini": ("Google Gemini", "https://aistudio.google.com/apikey"),
    "openrouter": ("OpenRouter (free)", "https://openrouter.ai/settings/keys"),
    "cloudflare": ("Cloudflare Workers AI", "https://dash.cloudflare.com/"),
    "mistral": ("Mistral Studio (Free)", "https://console.mistral.ai/"),
    "huggingface": ("Hugging Face ($0.10/мес.)", "https://huggingface.co/settings/tokens"),
    "custom": ("Совместимый API (проверить тариф!)", ""),
}
# Включены только текстовые диалоговые модели. Некоторые тарифы, страны и
# провайдеры могут не позволять запуск — проверяйте на своём аккаунте.
MODELS = {
    "groq": [
        ("openai/gpt-oss-20b", "GPT-OSS 20B — быстрый"),
        ("openai/gpt-oss-120b", "GPT-OSS 120B — умнее"),
        ("llama-3.1-8b-instant", "Llama 3.1 8B — экономный"),
        ("llama-3.3-70b-versatile", "Llama 3.3 70B"),
    ],
    "gemini": [
        ("gemini-3.5-flash-lite", "Gemini 3.5 Flash-Lite — рекомендован"),
        ("gemini-2.5-flash-lite", "Gemini 2.5 Flash-Lite — только для действующих проектов"),
        ("gemini-2.5-flash", "Gemini 2.5 Flash — только для действующих проектов"),
    ],
    "openrouter": [("openrouter/free", "Автовыбор бесплатной модели")],
    "cloudflare": [
        ("@cf/zai-org/glm-4.7-flash", "GLM 4.7 Flash"),
        ("@cf/google/gemma-4-26b-a4b-it", "Gemma 4 26B"),
        ("@cf/nvidia/nemotron-3-120b-a12b", "Nemotron 3 120B"),
    ],
    "mistral": [
        ("mistral-small-latest", "Mistral Small (бесплатный план Explore)"),
        ("mistral-large-latest", "Mistral Large (доступность зависит от квоты)"),
    ],
    "huggingface": [
        ("openai/gpt-oss-120b:cheapest", "GPT-OSS 120B (расходует $0.10/мес.)"),
    ],
    "custom": [],
}


def model_key(provider, model):
    return provider + ":" + model


def split_model(key):
    if ":" not in str(key):
        raise ValueError("Неверный идентификатор модели")
    provider, model = key.split(":", 1)
    if provider not in PROVIDERS or not model or len(model) > 150:
        raise ValueError("Неверный идентификатор модели")
    if provider == "openrouter" and model != "openrouter/free" and not model.endswith(":free"):
        raise ValueError("OpenRouter: только модели :free")
    return provider, model


def model_label(key):
    provider, model = split_model(key)
    title = next((label for ident, label in MODELS.get(provider, []) if ident == model), model)
    return PROVIDERS[provider][0] + " • " + title


def enabled_models(cfg):
    """Серверный белый список. Модели без API-ключа не предлагаются."""
    result = []
    for key in cfg.get("enabled_models", []):
        try:
            provider, model = split_model(key)
        except ValueError:
            continue
        details = cfg.get("providers", {}).get(provider, {})
        if details.get("enabled") and details.get("api_key") and key not in result:
            if provider == "cloudflare" and not details.get("cloudflare_account"):
                continue
            if provider == "custom" and not details.get("custom_url", "").startswith("https://"):
                continue
            result.append(key)
    return result


def default_model(cfg):
    allowed = enabled_models(cfg)
    selected = cfg.get("default_model", "")
    return selected if selected in allowed else (allowed[0] if allowed else None)


def discover_openrouter(key):
    """Актуальный бесплатный список из официального API, без платных fallback."""
    response = requests.get("https://openrouter.ai/api/v1/models",
                            headers={"Authorization": "Bearer " + key}, timeout=(5, 12))
    response.raise_for_status()
    result = []
    for item in response.json().get("data", []):
        ident = str(item.get("id", ""))
        pricing = item.get("pricing") or {}
        if ident == "openrouter/free":
            continue
        if not ident.endswith(":free") or pricing.get("prompt") not in ("0", 0, "0.0"):
            continue
        if pricing.get("completion") not in ("0", 0, "0.0"):
            continue
        if not (item.get("architecture", {}).get("output_modalities") or ["text"]):
            continue
        if "text" not in (item.get("architecture", {}).get("output_modalities") or ["text"]):
            continue
        result.append((ident, str(item.get("name") or ident)[:90]))
    return sorted(result, key=lambda item: item[1].lower())
