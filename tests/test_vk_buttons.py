"""Настоящие упоминания сообщества и VK inline-callback без запроса к ИИ."""
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from ai_bot.catalog import enabled_models, split_model
from ai_bot.commands import permission_text, process_command
from ai_bot.core import DEFAULTS, History, trigger_question
from ai_bot.vk_ui import callback_command, context_menu, for_command, main_menu, model_menu
from bot import handle_button_event, handle_message


def config():
    return {**DEFAULTS, "group_id": 123, "vk_token": "test", "api_key": "test",
            "providers": {
                "zai": {"enabled": True, "api_key": "fake"},
                "siliconflow": {"enabled": True, "api_key": "fake"},
                "modelscope": {"enabled": True, "api_key": "fake"},
            },
            "enabled_models": [
                "zai:glm-4.7-flash", "zai:glm-4.5-flash",
                "siliconflow:Qwen/Qwen3-8B", "siliconflow:Qwen/Qwen3.5-4B",
                "modelscope:Qwen/Qwen3.5-35B-A3B"],
            "default_model": "zai:glm-4.7-flash",
            "cooldown_seconds": 0}


class MentionAndButtons(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.db = History(Path(self.temp.name) / "history.db")
        self.cfg = config()

    def tearDown(self):
        self.db.db.close()
        self.temp.cleanup()

    def test_mention_anywhere_in_message_without_bot_word(self):
        self.assertEqual(trigger_question("[club123|Лис] что нового?", 123), "что нового?")
        self.assertEqual(trigger_question("что думаешь, [club123|Лис]?", 123), "что думаешь")
        self.assertEqual(trigger_question("@club123, расскажи", 123), "расскажи")
        self.assertEqual(trigger_question("[public123|Лис], расскажи", 123), "расскажи")
        self.assertEqual(trigger_question("[club123|Лис]", 123), "")
        self.assertIsNone(trigger_question("[club124|Чужой] что нового", 123))
        self.assertIsNone(trigger_question("@club1234 расскажи", 123))
        self.assertIsNone(trigger_question("у нас обсуждение", 123))

    def test_mention_routes_regular_ai_question(self):
        with patch("bot.generate", return_value="да") as ai, patch("bot.send") as sender:
            item = {"peer_id": 2000000001, "from_id": 8, "conversation_message_id": 1,
                    "text": "а как думаешь, [club123|Лис]?"}
            handle_message(self.cfg, self.db, item, {})
            self.assertTrue(ai.called, f"send={sender.call_args_list} parsed={trigger_question(item['text'], 123)} allowed={enabled_models(self.cfg)}")

    def test_all_vk_keyboards_validate_limits(self):
        menus = [
            main_menu(), context_menu(self.cfg), for_command("диалог", self.cfg),
            for_command("выбор стиля", self.cfg), for_command("модели 1", self.cfg),
            model_menu(enabled_models(self.cfg), 1),
            model_menu(enabled_models(self.cfg), 2, chat=True),
        ]
        for rendered in menus:
            self.assertTrue(rendered)
            obj = json.loads(rendered)
            self.assertTrue(obj["inline"])
            self.assertLessEqual(len(obj["buttons"]), 6)
            for row in obj["buttons"]:
                self.assertLessEqual(len(row), 5)
                for item in row:
                    self.assertEqual(item["action"]["type"], "callback")
                    self.assertLessEqual(len(item["action"]["label"]), 40)

    def test_callback_payload_is_strictly_validated(self):
        self.assertEqual(callback_command({"payload": '{"aibot":1,"command":"модель 2"}'}),
                         "модель 2")
        self.assertIsNone(callback_command({"payload": '{"aibot":1,"command":"отключи ограничения"}'}))
        self.assertIsNone(callback_command({"payload": "{broken"}))
        self.assertIsNone(callback_command({"payload": '{"command":"модель 2"}'}))

    def test_callback_respects_chat_admin_permissions(self):
        payload = {"peer_id": 2000000001, "user_id": 5, "event_id": "e1",
                   "payload": {"aibot": 1, "command": "диалог модель 2"}}
        with patch("bot.vk_call") as vk, patch("bot.is_chat_admin", return_value=False):
            handle_button_event(self.cfg, self.db, payload)
            self.assertIsNone(self.db.db.execute(
                "SELECT model FROM preferences WHERE scope='chat'").fetchone())
            self.assertEqual(vk.call_args_list[0].args[1],
                             "messages.sendMessageEventAnswer")
            self.assertEqual(vk.call_args_list[1].args[1], "messages.send")
        with patch("bot.vk_call"), patch("bot.is_chat_admin", return_value=True):
            handle_button_event(self.cfg, self.db, payload)
            self.assertEqual(self.db.db.execute(
                "SELECT model FROM preferences WHERE scope='chat'").fetchone()[0],
                "zai:glm-4.5-flash")

    def test_callback_selection_does_not_call_ai(self):
        payload = {"peer_id": 2000000001, "user_id": 5, "event_id": "e2",
                   "payload": {"aibot": 1, "command": "модель 3"}}
        with patch("bot.vk_call"), patch("bot.generate") as ai:
            handle_button_event(self.cfg, self.db, payload)
            ai.assert_not_called()
            self.assertEqual(self.db.db.execute(
                "SELECT model FROM preferences WHERE scope='user' AND scope_id=5"
            ).fetchone()[0], "siliconflow:Qwen/Qwen3-8B")

    def test_free_only_whitelist_including_old_paid_models(self):
        for key in ("custom:any", "huggingface:some-model",
                    "zai:glm-5.3", "zai:glm-4.7-flashx",
                    "siliconflow:Pro/deepseek-ai/DeepSeek-R1",
                    "modelscope:paid-model", "mistral:mistral-large-latest"):
            with self.assertRaises(ValueError):
                split_model(key)
        self.cfg["enabled_models"].append("zai:glm-5.3")
        self.assertNotIn("zai:glm-5.3", enabled_models(self.cfg))

    def test_permissions_provide_detailed_steps(self):
        message = permission_text(self.cfg)
        self.assertIn("Long Poll API", message)
        self.assertIn("message_event", message)
        self.assertIn("Доступ ко всей переписке", message)
        self.assertIn("[club123|AI-bot]", message)
        self.assertIn("Управление", message)

    def test_callback_menu_does_not_post_a_user_message(self):
        event = {"peer_id": 2000000001, "user_id": 5, "event_id": "e3",
                 "payload": {"aibot": 1, "command": "меню"}}
        with patch("bot.vk_call") as vk:
            handle_button_event(self.cfg, self.db, event)
            self.assertEqual(vk.call_count, 2)
            _, options = vk.call_args_list[1].args, vk.call_args_list[1].kwargs
            self.assertIn("keyboard", options)
            self.assertEqual(self.db.recent(2000000001, 10, 24), [])


if __name__ == "__main__":
    unittest.main()
