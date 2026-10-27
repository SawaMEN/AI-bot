"""Безопасный каталог бесплатных ИИ.

Новые OpenAI-совместимые провайдеры описываются в providers.json.
Только официально разрешённые статические модели либо модели с
положительной проверкой бесплатности. Неизвестная цена = запрещено.
"""
import json
import logging
import re
from pathlib import Path

import requests

LOG = logging.getLogger(__name__)
REGISTRY_PATH = Path(__file__).with_name("providers.json")
with REGISTRY_PATH.open(encoding="utf-8") as handle:
    _data = json.load(handle)
SPECS = {spec["id"]: spec for spec in _data["providers"]}
PROVIDERS = {k: (v["name"], v["key_url"]) for k, v in SPECS.items()}
MODELS = {k: [tuple(model) for model in v.get("models", [])] for k, v in SPECS.items()}
REMOTE = {"priced_openrouter", "priced_pollinations", "volunteer"}
_SAFE = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_.:/@+-]{0,179}$")


def model_key(provider, model):
    return provider + ":" + model


def split_model(key):
    if ":" not in str(key):
        raise ValueError("Неверный идентификатор модели")
    provider, model = key.split(":", 1)
    spec = SPECS.get(provider)
    if not spec or not _SAFE.fullmatch(model):
        raise ValueError("Недопустимый провайдер или ID модели")
    policy = spec["free_policy"]
    if policy == "curated" and model not in {m for m, _ in MODELS[provider]}:
        raise ValueError("Модель отсутствует в белом списке бесплатных")
    if policy == "priced_openrouter" and model != "openrouter/free" and not model.endswith(":free"):
        raise ValueError("OpenRouter поддерживает только модели :free")
    return provider, model


def model_label(key):
    provider, model = split_model(key)
    label = next((title for ident, title in MODELS.get(provider, []) if ident == model), model)
    return PROVIDERS[provider][0] + " • " + label


def enabled_models(cfg):
    """Настройки администратора + только модели, проверенные живым каталогом."""
    result = []
    verified = cfg.get("_verified_free_models", {})
    for provider, spec in SPECS.items():
        settings = cfg.get("providers", {}).get(provider, {})
        token = settings.get("api_key") or spec.get("anonymous_key")
        if not settings.get("enabled") or not token:
            continue
        if provider == "cloudflare" and not settings.get("cloudflare_account"):
            continue
        for key in cfg.get("enabled_models", []):
            try:
                p, model = split_model(key)
            except ValueError:
                continue
            if p != provider:
                continue
            if spec["free_policy"] in REMOTE and model not in {v[0] for v in verified.get(provider, [])}:
                # Единственное исключение — спецроутер, который гарантирует бесплатный fallback.
                if not (provider == "openrouter" and model == "openrouter/free"):
                    continue
            if key not in result:
                result.append(key)
        if provider in cfg.get("auto_models_providers", []):
            for model, _ in verified.get(provider, []):
                key = model_key(provider, model)
                if key not in result:
                    result.append(key)
            if spec["free_policy"] == "curated":
                for model, _ in MODELS[provider]:
                    key = model_key(provider, model)
                    if key not in result:
                        result.append(key)
    return result


def default_model(cfg):
    allowed = enabled_models(cfg)
    selected = cfg.get("default_model", "")
    return selected if selected in allowed else (allowed[0] if allowed else None)


def _price_zero(value):
    if isinstance(value, bool) or value is None:
        return False
    try:
        # Float == 0 is only true if explicitly zero; missing price rejects.
        return float(value) == 0.0
    except (ValueError, TypeError):
        return False


def _free_openrouter(item):
    mid = str(item.get("id") or "")
    price = item.get("pricing") or {}
    modalities = (item.get("architecture") or {}).get("output_modalities") or ["text"]
    return (mid.endswith(":free") and isinstance(price, dict) and
            _price_zero(price.get("prompt")) and _price_zero(price.get("completion")) and
            "text" in modalities)


def _free_pollinations(item):
    price = item.get("pricing") or {}
    if not isinstance(price, dict):
        return False
    # У каталогов разных версий используются разные названия тарифов.
    candidates = [
        (price.get("prompt"), price.get("completion")),
        (price.get("input"), price.get("output")),
        ((price.get("text") or {}).get("input"), (price.get("text") or {}).get("output"))
        if isinstance(price.get("text"), dict) else (None, None),
    ]
    return any(_price_zero(p) and _price_zero(c) for p, c in candidates)


def _unwrap_items(payload):
    if isinstance(payload, list):
        return payload
    if isinstance(payload, dict):
        for name in ("data", "models", "items"):
            if isinstance(payload.get(name), list):
                return payload[name]
    raise ValueError("API не вернул список моделей")


def _remote_models(provider, key):
    spec = SPECS[provider]
    headers = {"Authorization": "Bearer " + key} if key else {}
    response = requests.get(spec["catalog_url"], headers=headers, timeout=(5, 15))
    response.raise_for_status()
    data = _unwrap_items(response.json())
    found = []
    seen = set()
    for item in data:
        if not isinstance(item, dict):
            continue
        mid = str(item.get("id") or item.get("name") or "")
        if not _SAFE.fullmatch(mid) or mid in seen:
            continue
        if provider == "openrouter" and not _free_openrouter(item):
            continue
        if provider == "pollinations" and not _free_pollinations(item):
            continue
        # AI Horde: волонтёрская бесплатная инфраструктура, без платных моделей.
        # У него нет тарифов за токен. Модели доступны, пока онлайн работники.
        if provider == "aihorde" and item.get("is_premium", False):
            continue
        seen.add(mid)
        found.append((mid, str(item.get("display_name") or item.get("name") or mid)[:85]))
        if len(found) >= 150:
            break
    return sorted(found, key=lambda row: row[1].casefold())


def discover_openrouter(key):
    """Совместимость со старым API каталога."""
    return _remote_models("openrouter", key)


def refresh_free_models(cfg, provider=None):
    """Проверяет реальные каталоги; при сбое динамический список отзывается.

    Отказ по умолчанию: если тариф не содержит положительного признака $0,
    модель не получает право на вызов. Не записывает ключи/каталог на диск.
    """
    checked = cfg.setdefault("_verified_free_models", {})
    status = {}
    targets = [provider] if provider is not None else list(cfg.get("auto_models_providers", []))
    for name in targets:
        if name not in SPECS:
            continue
        spec = SPECS[name]
        settings = cfg.get("providers", {}).get(name, {})
        if not settings.get("enabled"):
            checked.pop(name, None)
            continue
        if spec["free_policy"] == "curated":
            checked[name] = list(MODELS[name])
            status[name] = (len(checked[name]), "")
            continue
        key = settings.get("api_key") or spec.get("anonymous_key", "")
        try:
            if not key:
                raise ValueError("Не указан ключ")
            checked[name] = _remote_models(name, key)
            status[name] = (len(checked[name]), "")
        except (requests.RequestException, ValueError, TypeError, KeyError) as exc:
            checked.pop(name, None)  # не используем непроверенный устаревший каталог
            status[name] = (0, type(exc).__name__)
            LOG.warning("Не удалось обновить бесплатные модели %s: %s", name, type(exc).__name__)
    return status
