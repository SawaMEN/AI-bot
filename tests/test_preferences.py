"""Настройки пользователей/бесед, ограничения доступа и выбор моделей."""
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ai_bot.catalog import (default_model, discover_openrouter, enabled_models,
                            model_key, split_model)
from ai_bot.commands import process_command
from ai_bot.core import DEFAULTS, History
from ai_bot.preferences import effective, get_pref, set_pref
from ai_bot.providers import ProviderError, generate
from bot import handle_message


class SettingsTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = History(Path(self.tmp.name) / "h.db")
        self.groq = "groq:openai/gpt-oss-20b"
        self.gemini = "gemini:gemini-3.5-flash-lite"
        self.cfg = {**DEFAULTS, "group_id": 5,
                    "providers": {"groq": {"api_key": "fake", "enabled": True},
                                  "gemini": {"api_key": "another", "enabled": True}},
                    "enabled_models": [self.groq, self.gemini],
                    "default_model": self.groq, "cooldown_seconds": 0}

    def tearDown(self):
        self.db.db.close()
        self.tmp.cleanup()

    def test_per_user_overrides_chat_and_chat_overrides_server(self):
        peer = 2000000001
        set_pref(self.db.db, "chat", peer, "model", self.gemini)
        self.assertEqual(effective(self.db.db, self.cfg, peer, 10,
                                   enabled_models(self.cfg))["model"], self.gemini)
        set_pref(self.db.db, "user", 10, "model", self.groq)
        self.assertEqual(effective(self.db.db, self.cfg, peer, 10,
                                   enabled_models(self.cfg))["model"], self.groq)
        self.assertEqual(effective(self.db.db, self.cfg, peer, 11,
                                   enabled_models(self.cfg))["model"], self.gemini)
        self.assertEqual(effective(self.db.db, self.cfg, 11, 10,
                                   enabled_models(self.cfg))["model"], self.groq)

    def test_disabled_model_cannot_be_selected_after_admin_disables_it(self):
        set_pref(self.db.db, "user", 10, "model", self.gemini)
        self.cfg["providers"]["gemini"]["enabled"] = False
        allowed = enabled_models(self.cfg)
        self.assertEqual(allowed, [self.groq])
        self.assertEqual(effective(self.db.db, self.cfg, 1, 10, allowed)["model"], self.groq)

    def test_settings_are_isolated_and_inherited(self):
        peer = 2000000001
        set_pref(self.db.db, "chat", peer, "search", "off")
        set_pref(self.db.db, "user", 10, "style", "short")
        set_pref(self.db.db, "user", 10, "context_messages", 5)
        self.assertEqual(effective(self.db.db, self.cfg, peer, 10,
                                   enabled_models(self.cfg))["search"], "off")
        self.assertEqual(effective(self.db.db, self.cfg, peer, 10,
                                   enabled_models(self.cfg))["style"], "short")
        self.assertEqual(effective(self.db.db, self.cfg, 2, 11,
                                   enabled_models(self.cfg))["style"], "normal")
        self.assertEqual(effective(self.db.db, self.cfg, peer, 11,
                                   enabled_models(self.cfg))["context_messages"], 30)

    def test_context_never_exceeds_server_maximum(self):
        self.cfg["context_messages"] = 8
        set_pref(self.db.db, "user", 10, "context_messages", 100)
        self.assertEqual(effective(self.db.db, self.cfg, 1, 10,
                                   enabled_models(self.cfg))["context_messages"], 8)

    def test_unauthorized_chat_member_cannot_change_chat_model(self):
        peer = 2000000001
        text = process_command(self.cfg, self.db, peer, 10, "диалог модель 2",
                               chat_admin=lambda: False)
        self.assertIn("только администратор", text)
        self.assertEqual(get_pref(self.db.db, "chat", peer), {})
        text = process_command(self.cfg, self.db, peer, 10, "диалог модель 2",
                               chat_admin=lambda: True)
        self.assertIn("Gemini", text)
        self.assertEqual(get_pref(self.db.db, "chat", peer)["model"], self.gemini)

    def test_personal_change_does_not_need_chat_admin(self):
        response = process_command(self.cfg, self.db, 2000000001, 42, "модель 2")
        self.assertIn("Gemini", response)
        self.assertEqual(get_pref(self.db.db, "user", 42)["model"], self.gemini)
        process_command(self.cfg, self.db, 2000000001, 42, "модель сброс")
        self.assertIsNone(get_pref(self.db.db, "user", 42)["model"])

    def test_user_models_route_to_different_providers(self):
        peer = 2000000001
        with patch("bot.send"), patch("bot.generate", return_value="Тест") as generate_mock:
            handle_message(self.cfg, self.db,
                           {"peer_id": peer, "from_id": 10,
                            "conversation_message_id": 1, "text": "бот модель 2"}, {})
            handle_message(self.cfg, self.db,
                           {"peer_id": peer, "from_id": 10,
                            "conversation_message_id": 2, "text": "бот, привет"}, {})
            self.assertEqual(generate_mock.call_args.kwargs["selected"], self.gemini)
            handle_message(self.cfg, self.db,
                           {"peer_id": peer, "from_id": 11,
                            "conversation_message_id": 3, "text": "бот, привет"}, {})
            self.assertEqual(generate_mock.call_args.kwargs["selected"], self.groq)

    def test_free_openrouter_model_filter(self):
        with self.assertRaises(ValueError):
            split_model("openrouter:openai/gpt-4o")
        self.assertEqual(split_model("openrouter:openrouter/free"),
                         ("openrouter", "openrouter/free"))

    def test_disabled_provider_prevents_network_call(self):
        cfg = {**self.cfg, "providers": {**self.cfg["providers"],
                                         "groq": {"enabled": False, "api_key": "fake"}}}
        with patch("ai_bot.providers.requests.post") as fake:
            with self.assertRaises(ProviderError):
                generate(cfg, [{"role": "user", "content": "hi"}], selected=self.groq)
            fake.assert_not_called()

    def test_dynamic_catalog_rejects_paid_openrouter_fallback(self):
        class Response:
            def raise_for_status(self):
                pass
            def json(self):
                return {"data": [
                    {"id": "free/model:free", "name": "Free",
                     "pricing": {"prompt": "0", "completion": "0"}},
                    {"id": "paid/model", "name": "Paid",
                     "pricing": {"prompt": "0.1", "completion": "0.1"}},
                    {"id": "suspicious/model:free", "name": "Actually paid",
                     "pricing": {"prompt": "0", "completion": "0.1"}},
                ]}
        with patch("ai_bot.catalog.requests.get", return_value=Response()):
            models = discover_openrouter("test")
        self.assertEqual(models, [("free/model:free", "Free")])


if __name__ == "__main__":
    unittest.main()
