"""Локальные тесты маршрутизации событий без сети и реальных секретов."""
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ai_bot.core import DEFAULTS, History
from ai_bot.providers import ProviderError
from bot import handle_message


def event(peer, user, cmid, text):
    return {"peer_id": peer, "from_id": user,
            "conversation_message_id": cmid, "text": text}


class RuntimeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.db = History(Path(self.tmp.name) / "history.db")
        self.cfg = {**DEFAULTS, "group_id": 123, "vk_token": "fake",
                    "api_key": "fake", "search_enabled": False, "cooldown_seconds": 0,
                    "providers": {"groq": {"enabled": True, "api_key": "fake"}},
                    "enabled_models": ["groq:openai/gpt-oss-20b"],
                    "default_model": "groq:openai/gpt-oss-20b"}
        self.cooldowns = {}

    def tearDown(self):
        self.db.db.close()
        self.tmp.cleanup()

    def test_reads_other_members_and_only_replies_when_called(self):
        peer = 2000000001
        with patch("bot.generate", return_value="Я бы выбрал Debian.") as model, \
             patch("bot.send") as sender:
            handle_message(self.cfg, self.db, event(peer, 10, 1, "Что выбрать: Debian или Ubuntu?"), self.cooldowns)
            handle_message(self.cfg, self.db, event(peer, 20, 2, "Я за Debian"), self.cooldowns)
            sender.assert_not_called()
            handle_message(self.cfg, self.db, event(peer, 30, 3, "бот, а как ты думаешь?"), self.cooldowns)
            sender.assert_called_once_with(self.cfg, peer, "Я бы выбрал Debian.")
            sent_context = model.call_args.args[1][1]["content"]
            self.assertIn("Участник 10: Что выбрать", sent_context)
            self.assertIn("Участник 20: Я за Debian", sent_context)
            self.assertIn("Текущий вопрос к боту", sent_context)

    def test_duplicate_vk_event_is_ignored(self):
        peer = 2000000001
        with patch("bot.generate", return_value="готово") as model, patch("bot.send"):
            item = event(peer, 30, 9, "бот, привет")
            handle_message(self.cfg, self.db, item, self.cooldowns)
            handle_message(self.cfg, self.db, item, self.cooldowns)
            model.assert_called_once()

    def test_search_snippet_fallback(self):
        self.cfg["search_enabled"] = True
        results = [{"title": "Новость", "body": "Найденное описание",
                    "url": "https://example.com/news"}]
        with patch("bot.generate", side_effect=ProviderError("квота")), \
             patch("bot.search_sites", return_value=results), \
             patch("bot.send") as sender:
            handle_message(self.cfg, self.db,
                           event(2000000001, 10, 1, "бот, найди новости"), self.cooldowns)
            self.assertIn("https://example.com/news", sender.call_args.args[2])

    def test_chat_context_is_isolated(self):
        self.db.add(2000000001, 10, "one", "секрет одной беседы")
        with patch("bot.generate", return_value="ответ") as model, patch("bot.send"):
            handle_message(self.cfg, self.db, event(2000000002, 20, 1, "бот, привет"), self.cooldowns)
            self.assertNotIn("секрет одной беседы", model.call_args.args[1][1]["content"])


if __name__ == "__main__":
    unittest.main()
