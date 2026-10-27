"""Тесты функций, не требующие токенов, сети и установленного VK."""
import tempfile
import unittest
from pathlib import Path

from ai_bot.core import History, needs_search, trigger_question
from bot import build_prompt


class TriggerTests(unittest.TestCase):
    def test_plain_chat_is_not_a_trigger(self):
        self.assertIsNone(trigger_question("как дела", 123))

    def test_contextual_call(self):
        self.assertEqual(trigger_question("Бот, а как ты думаешь?", 123),
                         "а как ты думаешь?")

    def test_mention(self):
        self.assertEqual(trigger_question("[club123|Бот] найди новости", 123),
                         "найди новости")

    def test_reply(self):
        self.assertEqual(trigger_question("согласен?", 123, replied_to_bot=True),
                         "согласен?")

    def test_word_boundary(self):
        self.assertIsNone(trigger_question("ботаника", 123))

    def test_search(self):
        self.assertTrue(needs_search("найди новости сегодня"))
        self.assertFalse(needs_search("что думаешь по этому поводу"))


class HistoryTests(unittest.TestCase):
    def test_group_isolation_and_duplicate_detection(self):
        with tempfile.TemporaryDirectory() as tmp:
            db = History(Path(tmp) / "h.db")
            self.assertTrue(db.add(2000000001, 10, "e:1", "первое"))
            self.assertFalse(db.add(2000000001, 10, "e:1", "дубликат"))
            self.assertTrue(db.add(2000000001, 11, "e:2", "второе"))
            self.assertTrue(db.add(2000000002, 12, "e:3", "чужая беседа"))
            self.assertEqual(db.recent(2000000001, 30, 24),
                             [(10, "первое"), (11, "второе")])
            self.assertEqual(len(db.recent(2000000002, 30, 24)), 1)
            db.clear(2000000001)
            self.assertEqual(db.recent(2000000001, 30, 24), [])
            self.assertEqual(len(db.recent(2000000002, 30, 24)), 1)

    def test_prompt_includes_all_speakers(self):
        prompt = build_prompt([(100, "Нужно ли брать Linux?"),
                               (200, "Мне больше нравится Debian"),
                               (300, "Бот, как считаешь?")],
                              "как считаешь?")
        self.assertIn("Участник 200: Мне больше нравится Debian",
                      prompt[1]["content"])


if __name__ == "__main__":
    unittest.main()
