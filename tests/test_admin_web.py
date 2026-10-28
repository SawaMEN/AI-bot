"""Локальная веб-панель не выводит токены и не допускает произвольные API."""
import unittest

from ai_bot.core import DEFAULTS
from ai_bot.web import apply_form, render_page
from ai_bot.catalog import enabled_models


def cfg():
    return {
        **DEFAULTS,
        "providers": {"groq": {"enabled": True, "api_key": "VERY_SECRET_TEST_KEY"}},
        "enabled_models": ["groq:openai/gpt-oss-20b"],
        "default_model": "groq:openai/gpt-oss-20b",
        "auto_models_providers": [],
    }


class AdminWebTests(unittest.TestCase):
    def test_does_not_reveal_keys_in_page(self):
        html = render_page(cfg(), "CSRFSECRET")
        self.assertIn("GroqCloud", html)
        self.assertIn("CSRFSECRET", html)
        self.assertNotIn("VERY_SECRET_TEST_KEY", html)
        self.assertIn("password", html)

    def test_can_enable_existing_provider_automatic_free_models(self):
        conf = cfg()
        message = apply_form(conf, {"action": "provider", "provider": "groq",
                                    "auto": "on", "enabled": "on"})
        self.assertIn("сохранены", message)
        self.assertIn("groq", conf["auto_models_providers"])
        self.assertNotIn("VERY_SECRET_TEST_KEY", message)
        self.assertEqual(conf["providers"]["groq"]["api_key"], "VERY_SECRET_TEST_KEY")

    def test_can_disable_or_clear_key(self):
        conf = cfg()
        apply_form(conf, {"action": "provider", "provider": "groq",
                          "remove_key": "on"})
        self.assertFalse(conf["providers"]["groq"]["enabled"])
        self.assertEqual(conf["providers"]["groq"]["api_key"], "")
        self.assertEqual(enabled_models(conf), [])

    def test_reject_unknown_service_and_paid_model(self):
        conf = cfg()
        with self.assertRaises(ValueError):
            apply_form(conf, {"action": "provider", "provider": "unknown",
                              "enabled": "on", "api_key": "test"})
        with self.assertRaises(ValueError):
            apply_form(conf, {"action": "default", "model": "openrouter:paid-model"})

    def test_reject_out_of_bounds_settings(self):
        conf = cfg()
        with self.assertRaises(ValueError):
            apply_form(conf, {"action": "settings", "context_messages": "9999"})

    def test_limit_changes_do_not_affect_secrets(self):
        conf = cfg()
        apply_form(conf, {"action": "settings",
                          "context_messages": "20", "context_hours": "12",
                          "retention_days": "3", "max_requests_hour": "30",
                          "max_user_requests_hour": "8",
                          "max_chat_requests_hour": "15",
                          "search_enabled": "on"})
        self.assertEqual(conf["context_messages"], 20)
        self.assertEqual(conf["max_user_requests_hour"], 8)
        self.assertFalse(conf["free_fallback"])
        self.assertEqual(conf["providers"]["groq"]["api_key"], "VERY_SECRET_TEST_KEY")


if __name__ == "__main__":
    unittest.main()
