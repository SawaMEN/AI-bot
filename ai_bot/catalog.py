"""Белый список только моделей с бесплатным доступом.

Никаких произвольных model-id (кроме OpenRouter :free, проверенных по цене).
Сервис с только промокредитами/пробным периодом сюда НЕ добавляем.
Доступность бесплатного тарифа зависит от региона/аккаунта.
"""
import requests

PROVIDERS = {
    "groq": ("GroqCloud — бесплатная квота", "https://console.groq.com/keys"),
    "gemini": ("Google Gemini API — бесплатная квота", "https://aistudio.google.com/apikey"),
    "openrouter": ("OpenRouter — только бесплатные модели", "https://openrouter.ai/settings/keys"),
    "cloudflare": ("Cloudflare Workers AI — бесплатная квота", "https://dash.cloudflare.com/"),
    "mistral": ("Mistral Studio Free — лимиты бесплатного плана", "https://console.mistral.ai/"),
    "zai": ("Z.ai — полностью бесплатные GLM Flash", "https://z.ai/manage-apikey/apikey-list"),
    "siliconflow": ("SiliconFlow — бесплатные Qwen/DeepSeek", "https://cloud.siliconflow.cn/account/ak"),
    "modelscope": ("ModelScope — бесплатная API Inference", "https://modelscope.cn/my/myaccesstoken"),
}

MODELS = {
    "groq": [
        ("openai/gpt-oss-20b", "GPT-OSS 20B"),
        ("openai/gpt-oss-120b", "GPT-OSS 120B"),
        ("llama-3.1-8b-instant", "Llama 3.1 8B"),
        ("llama-3.3-70b-versatile", "Llama 3.3 70B"),
    ],
    "gemini": [
        ("gemini-3.5-flash-lite", "Gemini 3.5 Flash Lite"),
        ("gemini-2.5-flash-lite", "Gemini 2.5 Flash Lite"),
        ("gemini-2.5-flash", "Gemini 2.5 Flash"),
    ],
    "openrouter": [("openrouter/free", "Автовыбор среди бесплатных моделей")],
    "cloudflare": [
        ("@cf/zai-org/glm-4.7-flash", "GLM 4.7 Flash"),
        ("@cf/google/gemma-4-26b-a4b-it", "Gemma 4 26B"),
        ("@cf/nvidia/nemotron-3-120b-a12b", "Nemotron 3 120B"),
    ],
    "mistral": [
        ("mistral-small-latest", "Mistral Small — только в рамках Studio Free"),
    ],
    "zai": [
        ("glm-4.7-flash", "GLM 4.7 Flash — $0"),
        ("glm-4.5-flash", "GLM 4.5 Flash — $0"),
    ],
    "siliconflow": [
        ("Qwen/Qwen3.5-4B", "Qwen 3.5 4B — бесплатная"),
        ("Qwen/Qwen3-8B", "Qwen 3 8B — бесплатная"),
        ("Qwen/Qwen2.5-7B-Instruct", "Qwen 2.5 7B — бесплатная"),
        ("deepseek-ai/DeepSeek-R1-0528-Qwen3-8B", "DeepSeek R1 Distill Qwen3 8B — бесплатная"),
    ],
    "modelscope": [
        ("Qwen/Qwen3.5-35B-A3B", "Qwen 3.5 35B (ModelScope API Inference)"),
    ],
}


def model_key(provider, model):
    return provider + ":" + model


def split_model(key):
    if ":" not in str(key):
        raise ValueError("Неверный идентификатор модели")
    provider, model = key.split(":", 1)
    if provider not in PROVIDERS or not model or len(model) > 180:
        raise ValueError("Недопустимый ИИ-провайдер или модель")
    if provider == "openrouter":
        if model != "openrouter/free" and not model.endswith(":free"):
            raise ValueError("OpenRouter: разрешены только :free")
    elif model not in (m for m, _ in MODELS[provider]):
        raise ValueError("Модель не подтверждена как бесплатная")
    return provider, model


def model_label(key):
    provider, model = split_model(key)
    label = next((name for ident, name in MODELS[provider] if ident == model), model)
    return PROVIDERS[provider][0].split(" — ")[0] + " • " + label


def enabled_models(cfg):
    allowed = []
    for key in cfg.get("enabled_models", []):
        try:
            provider, _ = split_model(key)
        except ValueError:
            continue
        settings = cfg.get("providers", {}).get(provider, {})
        if not settings.get("enabled") or not settings.get("api_key"):
            continue
        if provider == "cloudflare" and not settings.get("cloudflare_account"):
            continue
        if key not in allowed:
            allowed.append(key)
    return allowed


def default_model(cfg):
    allowed = enabled_models(cfg)
    candidate = cfg.get("default_model", "")
    return candidate if candidate in allowed else (allowed[0] if allowed else None)


def discover_openrouter(api_key):
    """Список :free моделей с нулевыми ценами, без платных fallback."""
    response = requests.get("https://openrouter.ai/api/v1/models",
                            headers={"Authorization": "Bearer " + api_key}, timeout=(5, 15))
    response.raise_for_status()
    choices = []
    for item in response.json().get("data", []):
        mid = str(item.get("id", ""))
        pricing = item.get("pricing") or {}
        if not mid.endswith(":free"):
            continue
        try:
            free = float(pricing["prompt"]) == 0 and float(pricing["completion"]) == 0
        except (KeyError, ValueError, TypeError):
            continue
        if not free:
            continue
        output_modalities = (item.get("architecture") or {}).get("output_modalities") or ["text"]
        if "text" not in output_modalities:
            continue
        choices.append((mid, str(item.get("name") or mid)[:85]))
    return sorted(choices, key=lambda p: p[1].lower())
