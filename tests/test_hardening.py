"""Надёжность VK: очередь, повторная доставка, лимиты и самодиагностика."""
import queue
import tempfile
import threading
import unittest
from pathlib import Path
from unittest.mock import patch

from ai_bot.catalog import refresh_free_models, enabled_models
from ai_bot.core import DEFAULTS, History
from ai_bot.doctor import inspect_vk, enable_vk_events
from ai_bot.providers import ProviderError
from bot import record_incoming, answer_worker, handle_message, send, _SEND_ID


def config():
    return {**DEFAULTS, "group_id": 123, "vk_token": "fake", "api_key": "fake",
            "cooldown_seconds": 0, "free_fallback": True,
            "providers": {"groq": {"enabled": True, "api_key": "fake"},
                          "zai": {"enabled": True, "api_key": "fake"}},
            "enabled_models": ["groq:openai/gpt-oss-20b", "zai:glm-4.7-flash"],
            "default_model": "groq:openai/gpt-oss-20b"}


def item(i=1, sender=42, peer=2000000001, text="бот, как дела?"):
    return {"peer_id": peer, "from_id": sender, "conversation_message_id": i, "text": text}


class Response:
    def __init__(self, data):
        self.data = data
    def json(self):
        return self.data
    def raise_for_status(self):
        pass


class DurableTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.path = Path(self.temp.name) / "history.db"
        self.db = History(self.path)
        self.cfg = config()

    def tearDown(self):
        self.db.db.close()
        self.temp.cleanup()

    def test_cursor_and_pending_survive_restart(self):
        self.db.set_cursor(123, "ts123")
        self.assertEqual(self.db.get_cursor(123), "ts123")
        msg = item()
        self.assertTrue(record_incoming(self.db, msg))
        self.assertTrue(self.db.enqueue(msg))
        self.db.db.close()
        self.db = History(self.path)
        pending = self.db.pending()
        self.assertEqual(pending[0]["text"], msg["text"])
        self.assertTrue(self.db.is_pending(msg))
        self.db.finish_pending(msg)
        self.assertIsNone(self.db.pending())
        self.db.clear_cursor(123)
        self.assertIsNone(self.db.get_cursor(123))

    def test_record_incoming_does_not_duplicate(self):
        msg = item()
        self.assertTrue(record_incoming(self.db, msg))
        self.assertFalse(record_incoming(self.db, msg))
        self.assertEqual(self.db.recent(msg["peer_id"], 10, 24), [(42, msg["text"])])
        self.assertFalse(record_incoming(self.db, {"peer_id": msg["peer_id"],
                                                "from_id": -1, "text": "something", "id": 9}))

    def test_background_worker_resumes_saved_job_without_queue_item(self):
        msg = item()
        record_incoming(self.db, msg)
        self.db.enqueue(msg)
        q = queue.Queue()
        stop = threading.Event()
        # Worker runs until stop and pending queue empty; mocked call sets stop.
        def fake_generate(cfg, messages, selected=None):
            stop.set()
            return "Привет!"
        with patch("bot.generate", side_effect=fake_generate) as generate, \
             patch("bot.send") as send_mock:
            thread = threading.Thread(target=answer_worker,
                                      args=(self.cfg, q, stop, self.path))
            thread.start()
            thread.join(timeout=5)
        self.assertFalse(thread.is_alive())
        generate.assert_called_once()
        send_mock.assert_called_once()
        self.assertFalse(self.db.is_pending(msg))

    def test_answer_worker_deduplicates_replayed_queue(self):
        msg = item()
        record_incoming(self.db, msg)
        self.db.enqueue(msg)
        q = queue.Queue()
        q.put(msg)
        q.put(msg)
        stop = threading.Event()
        stop.set()
        with patch("bot.generate", return_value="Ответ") as ai, patch("bot.send"):
            answer_worker(self.cfg, q, stop, self.path)
        ai.assert_called_once()
        self.assertFalse(self.db.is_pending(msg))

    def test_stable_random_id_for_replay(self):
        ids = []
        def fake_call(cfg, name, **kwargs):
            ids.append(kwargs["random_id"])
        token = _SEND_ID.set(121212)
        try:
            with patch("bot.vk_call", side_effect=fake_call):
                send(self.cfg, 2000000001, "ответ")
                send(self.cfg, 2000000001, "ответ")
        finally:
            _SEND_ID.reset(token)
        self.assertEqual(ids, [121212, 121212])

    def test_hourly_limits_count_fallback_attempts(self):
        with patch("bot.generate", side_effect=[ProviderError("429"), "Ответ"]) as ai, \
             patch("bot.send"):
            handle_message(self.cfg, self.db, item(), {})
        self.assertEqual(ai.call_count, 2)
        self.assertEqual(self.db.count_calls_hour(), 2)
        self.assertEqual(self.db.count_calls_hour(sender_id=42), 2)
        self.assertEqual(self.db.count_calls_hour(peer_id=2000000001), 2)

    def test_user_limit_prevents_generation(self):
        for _ in range(12):
            self.db.track_call(peer_id=2000000001, sender_id=42)
        with patch("bot.generate") as ai, patch("bot.send") as send_mock:
            handle_message(self.cfg, self.db, item(), {})
        ai.assert_not_called()
        self.assertIn("Лимит", send_mock.call_args.args[2])

    def test_manual_dynamic_model_is_verified_on_refresh(self):
        cfg = self.cfg.copy()
        cfg["providers"] = {"openrouter": {"enabled": True, "api_key": "test"}}
        cfg["enabled_models"] = ["openrouter:example/mini:free"]
        cfg["auto_models_providers"] = []
        payload = {"data": [{"id": "example/mini:free",
                             "pricing": {"prompt": "0", "completion": "0"}}]}
        with patch("ai_bot.catalog.requests.get", return_value=Response(payload)):
            status = refresh_free_models(cfg)
        self.assertEqual(status["openrouter"][0], 1)
        self.assertIn(cfg["enabled_models"][0], enabled_models(cfg))


class VkDiagnostics(unittest.TestCase):
    def test_finds_missing_buttons_permission(self):
        cfg = config()
        outputs = [Response({"response": {"is_enabled": 1, "events": {
                   "message_new": 1, "message_event": 0}}}),
                   Response({"response": {"server": "https://lp.vk.com"}})]
        with patch("requests.post", side_effect=outputs):
            ok, details = inspect_vk(cfg)
        self.assertFalse(ok)
        self.assertIn("НЕ разрешены", " ".join(details))

    def test_vk_events_are_changed_only_on_explicit_call(self):
        cfg = config()
        with patch("requests.post", return_value=Response({"response": 1})) as call:
            ok, message = enable_vk_events(cfg)
        self.assertTrue(ok)
        self.assertEqual(call.call_args.kwargs["data"]["message_event"], 1)
        self.assertEqual(call.call_args.kwargs["data"]["message_new"], 1)


if __name__ == "__main__":
    unittest.main()
