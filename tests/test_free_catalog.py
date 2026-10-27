"""Автокаталог: только $0, новые сервисы без смены логики бота."""
import copy
import tempfile
import unittest
from unittest.mock import patch

from ai_bot.catalog import (
    MODELS, PROVIDERS, SPECS, _free_pollinations, default_model, enabled_models,
    model_label, refresh_free_models, split_model,
)
from ai_bot.providers import generate


def make_config():
    return {"provider": "groq", "model": "openai/gpt-oss-20b",
            "providers": {
                "groq": {"enabled": True, "api_key": "test"},
                "openrouter": {"enabled": True, "api_key": "test"},
                "pollinations": {"enabled": True, "api_key": "test"},
                "aihorde": {"enabled": True, "api_key": ""},
            },
            "auto_models_providers": ["groq", "openrouter", "pollinations", "aihorde"],
            "enabled_models": [], "default_model": ""}


class Response:
    def __init__(self, data, status=200):
        self.payload = data
        self.status_code = status
        self.ok = status < 400

    def json(self):
        return self.payload

    def raise_for_status(self):
        if self.status_code >= 400:
            raise ValueError("HTTP response")


class FreeRegistryTests(unittest.TestCase):
    def test_registry_is_extensible_and_has_known_implementations(self):
        self.assertGreaterEqual(len(PROVIDERS), 10)
        for name, spec in SPECS.items():
            self.assertIn(spec["free_policy"], ("curated", "priced_openrouter",
                                                "priced_pollinations", "volunteer"))
            self.assertIn(spec["type"], ("openai", "gemini", "cloudflare"))
            self.assertIn(name, MODELS)
        self.assertIn("aihorde", PROVIDERS)
        self.assertIn("pollinations", PROVIDERS)

    def test_new_provider_names_are_rejected_if_not_in_registry(self):
        for name in ["openai:gpt-4o", "claude:opus", "cerebras:gpt-oss-120b",
                     "pollinations:some-unknown"]:
            # Pollinations IDs are allowed syntactically, but must be verified
            # and individually approved by the runtime free-only catalog.
            if name.startswith("pollinations:"):
                continue
            with self.assertRaises(ValueError):
                split_model(name)

    def test_auto_curated_needs_admin_enable(self):
        cfg = make_config()
        self.assertIn("groq:openai/gpt-oss-20b", enabled_models(cfg))
        cfg["providers"]["groq"]["enabled"] = False
        self.assertFalse(any(item.startswith("groq:") for item in enabled_models(cfg)))

    def test_pricing_missing_is_not_free(self):
        self.assertFalse(_free_pollinations({"pricing": {}}))
        self.assertFalse(_free_pollinations({"pricing": {"input": "0"}}))
        self.assertFalse(_free_pollinations({"pricing": {"input": 0, "output": "0.1"}}))
        self.assertTrue(_free_pollinations({"pricing": {"input": "0", "output": 0}}))

    def test_remote_filters_paid_and_missing_price(self):
        cfg = make_config()
        payloads = {
            "openrouter": {"data": [
                {"id": "deepseek/model:free", "name": "DeepSeek",
                 "pricing": {"prompt": "0", "completion": "0"}},
                {"id": "paid/hidden:free", "name": "Do not enable",
                 "pricing": {"prompt": "0", "completion": "0.1"}},
                {"id": "unknown/price:free", "name": "Missing",
                 "pricing": {"prompt": "0"}},
                {"id": "paid/model", "pricing": {"prompt": 0, "completion": 0}},
            ]},
            "pollinations": {"data": [
                {"id": "qwen/qwen-free", "name": "Qwen",
                 "pricing": {"input": 0, "output": 0}},
                {"id": "openai/gpt-paid", "name": "Paid",
                 "pricing": {"input": 1, "output": 1}},
                {"id": "openai/unknown", "name": "Unknown"},
            ]},
            "aihorde": {"data": [{"id": "llama-7b", "name": "Llama"},
                                  {"id": "hidden", "is_premium": True}]},
        }

        def mocked_get(url, **kw):
            for name in payloads:
                if SPECS[name]["catalog_url"] == url:
                    return Response(payloads[name])
            raise AssertionError(url)

        with patch("ai_bot.catalog.requests.get", side_effect=mocked_get):
            counts = refresh_free_models(cfg)
        self.assertEqual(counts["openrouter"][0], 1)
        self.assertEqual(counts["pollinations"][0], 1)
        self.assertEqual(counts["aihorde"][0], 1)
        allowed = enabled_models(cfg)
        self.assertIn("openrouter:deepseek/model:free", allowed)
        self.assertIn("pollinations:qwen/qwen-free", allowed)
        self.assertIn("aihorde:llama-7b", allowed)
        self.assertFalse(any("paid" in item or "unknown" in item for item in allowed))
        self.assertEqual(len([item for item in allowed if item.startswith("groq:")]),
                         len(MODELS["groq"]))

    def test_offline_catalog_revokes_unverified_selection(self):
        cfg = make_config()
        cfg["_verified_free_models"] = {"pollinations": [("qwen/free", "Qwen")]}
        self.assertIn("pollinations:qwen/free", enabled_models(cfg))
        with patch("ai_bot.catalog.requests.get", side_effect=ValueError("network")):
            refresh_free_models(cfg, "pollinations")
        self.assertNotIn("pollinations:qwen/free", enabled_models(cfg))
        self.assertNotIn("pollinations", cfg["_verified_free_models"])

    def test_old_paid_models_cannot_be_enabled_from_config(self):
        cfg = make_config()
        cfg["enabled_models"] = ["groq:not-in-catalog", "openrouter:paid/model",
                                 "openrouter:paid/model:free",
                                 "pollinations:premium/paid"]
        cfg["auto_models_providers"] = []
        self.assertEqual(enabled_models(cfg), [])

    def test_provider_from_registry_routes_without_custom_code(self):
        cfg = make_config()
        cfg["_verified_free_models"] = {"aihorde": [("some-llama", "Llama")]}
        cfg["auto_models_providers"] = ["aihorde"]
        response = Response({"choices": [{"message": {"content": "OK"}}]})
        with patch("ai_bot.providers.requests.post", return_value=response) as post:
            answer = generate(cfg, [{"role": "user", "content": "привет"}],
                              selected="aihorde:some-llama")
        self.assertEqual(answer, "OK")
        self.assertEqual(post.call_args.args[0], SPECS["aihorde"]["endpoint"])
        self.assertEqual(post.call_args.kwargs["headers"]["Authorization"],
                         "Bearer 0000000000")

    def test_disabled_paid_model_never_reaches_network(self):
        cfg = make_config()
        cfg["enabled_models"] = ["pollinations:expensive-model"]
        cfg["auto_models_providers"] = []
        with patch("ai_bot.providers.requests.post") as request:
            with self.assertRaises(Exception):
                generate(cfg, [{"role": "user", "content": "привет"}],
                         selected="pollinations:expensive-model")
            request.assert_not_called()


if __name__ == "__main__":
    unittest.main()
