"""Бесплатный резерв при исчерпании лимита, без платных API."""
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ai_bot.core import DEFAULTS, History
from ai_bot.providers import ProviderError
from bot import handle_message


def event(cmid=1):
    return {"peer_id": 2000000001, "from_id": 25,
            "conversation_message_id": cmid, "text": "бот, привет"}


class FreeFallbackTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.db = History(Path(self.temp.name) / "db.sqlite")
        self.cfg = {**DEFAULTS, "group_id": 10, "vk_token": "test",
                    "cooldown_seconds": 0, "free_fallback": True,
                    "providers": {
                        "groq": {"enabled": True, "api_key": "test"},
                        "zai": {"enabled": True, "api_key": "test"}},
                    "enabled_models": ["groq:openai/gpt-oss-20b", "zai:glm-4.7-flash"],
                    "default_model": "groq:openai/gpt-oss-20b",
                    "auto_models_providers": []}

    def tearDown(self):
        self.db.db.close()
        self.temp.cleanup()

    def test_automatically_uses_another_free_provider(self):
        with patch("bot.generate", side_effect=[ProviderError("квота"), "Ответ"]) as generate, \
             patch("bot.send") as send:
            handle_message(self.cfg, self.db, event(), {})
        self.assertEqual(generate.call_count, 2)
        self.assertEqual(generate.call_args_list[0].kwargs["selected"], "groq:openai/gpt-oss-20b")
        self.assertEqual(generate.call_args_list[1].kwargs["selected"], "zai:glm-4.7-flash")
        self.assertIn("Резервная бесплатная модель", send.call_args.args[2])

    def test_admin_can_disable_fallback(self):
        self.cfg["free_fallback"] = False
        with patch("bot.generate", side_effect=ProviderError("квота")) as generate, \
             patch("bot.send") as send:
            handle_message(self.cfg, self.db, event(), {})
        generate.assert_called_once()
        self.assertIn("недоступны", send.call_args.args[2])

    def test_never_tries_disabled_models(self):
        self.cfg["providers"]["zai"]["enabled"] = False
        with patch("bot.generate", side_effect=ProviderError("квота")) as generate, \
             patch("bot.send"):
            handle_message(self.cfg, self.db, event(), {})
        generate.assert_called_once()

    def test_no_more_than_two_attempts(self):
        self.cfg["enabled_models"].append("groq:openai/gpt-oss-120b")
        with patch("bot.generate", side_effect=ProviderError("quota")) as generate, \
             patch("bot.send"):
            handle_message(self.cfg, self.db, event(), {})
        self.assertEqual(generate.call_count, 2)


if __name__ == "__main__":
    unittest.main()
