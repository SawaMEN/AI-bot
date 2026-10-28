"""Персональные настройки приватности и переключения ИИ."""
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ai_bot.core import DEFAULTS, History
from ai_bot.preferences import effective, get_pref, set_pref
from ai_bot.catalog import enabled_models
from ai_bot.commands import process_command
from ai_bot.vk_ui import main_menu, fallback_menu, for_command, callback_command
from ai_bot.providers import ProviderError
from bot import handle_message


class PrivacyFallbackTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = History(Path(self.tmp.name) / "test.db")
        self.cfg = {**DEFAULTS, "group_id": 123, "cooldown_seconds": 0,
                    "free_fallback": True,
                    "providers": {
                        "groq": {"enabled": True, "api_key": "mock"},
                        "zai": {"enabled": True, "api_key": "mock"}},
                    "enabled_models": ["groq:openai/gpt-oss-20b", "zai:glm-4.7-flash"],
                    "default_model": "groq:openai/gpt-oss-20b"}

    def tearDown(self):
        self.db.db.close()
        self.tmp.cleanup()

    def test_user_can_disable_fallback(self):
        peer = 2000000001
        process_command(self.cfg, self.db, peer, 100, "резерв выкл")
        state = effective(self.db.db, self.cfg, peer, 100, enabled_models(self.cfg))
        self.assertFalse(state["fallback"])
        self.assertTrue(effective(self.db.db, self.cfg, peer, 200,
                                   enabled_models(self.cfg))["fallback"])

    def test_chat_admin_only_controls_chat_fallback(self):
        peer = 2000000001
        denied = process_command(self.cfg, self.db, peer, 100, "диалог резерв выкл",
                                 chat_admin=lambda: False)
        self.assertIn("только администратор", denied)
        self.assertEqual(get_pref(self.db.db, "chat", peer), {})
        allowed = process_command(self.cfg, self.db, peer, 100, "диалог резерв выкл",
                                  chat_admin=lambda: True)
        self.assertIn("запрещён", allowed)
        self.assertFalse(effective(self.db.db, self.cfg, peer, 200,
                                   enabled_models(self.cfg))["fallback"])

    def test_server_disables_fallback_even_if_user_requests_it(self):
        self.cfg["free_fallback"] = False
        set_pref(self.db.db, "user", 1, "fallback", "on")
        self.assertFalse(effective(self.db.db, self.cfg, 2000000001, 1,
                                   enabled_models(self.cfg))["fallback"])

    def test_disable_fallback_prevents_other_provider_from_receiving_context(self):
        self.cfg["free_fallback"] = True
        set_pref(self.db.db, "user", 1, "fallback", "off")
        item = {"peer_id": 2000000001, "from_id": 1,
                "conversation_message_id": 1, "text": "бот, что думаешь?"}
        with patch("bot.generate", side_effect=ProviderError("rate")) as ai, \
             patch("bot.send"):
            handle_message(self.cfg, self.db, item, {})
        ai.assert_called_once()

    def test_vk_buttons_show_fallback_choices(self):
        keyboard = json.loads(main_menu())
        labels = [b["action"]["label"] for row in keyboard["buttons"] for b in row]
        self.assertIn("Резервный ИИ", labels)
        for command in ("выбор резерва", "диалог выбор резерва",
                        "резерв выкл", "диалог резерв вкл"):
            self.assertIsNotNone(for_command(command, self.cfg))
            payload = json.dumps({"aibot": 1, "command": command}, ensure_ascii=False)
            self.assertEqual(callback_command({"payload": payload}), command)


if __name__ == "__main__":
    unittest.main()
