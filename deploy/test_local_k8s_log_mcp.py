import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


MODULE_PATH = Path(__file__).with_name("local-k8s-log-mcp.py")
SPEC = importlib.util.spec_from_file_location("opscaptain_log_adapter", MODULE_PATH)
log_adapter = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(log_adapter)


class FakeResponse:
    def __init__(self, payload):
        self.payload = json.dumps(payload).encode()

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, traceback):
        return False

    def read(self, _limit):
        return self.payload


class LogAdapterTest(unittest.TestCase):
    def test_parse_window_seconds_supports_chinese_and_caps_range(self):
        self.assertEqual(log_adapter.parse_window_seconds("最近30分钟"), 1800)
        self.assertEqual(log_adapter.parse_window_seconds("2h"), 7200)
        self.assertEqual(log_adapter.parse_window_seconds("1d"), log_adapter.MAX_WINDOW_SECONDS)
        self.assertEqual(log_adapter.parse_window_seconds("invalid"), 1800)

    def test_build_loki_query_escapes_service_and_query(self):
        query = log_adapter.build_loki_query(
            {
                "service": 'payment"} |= "panic',
                "query": "timeout.*",
            }
        )

        self.assertTrue(query.startswith("{service_name=~"))
        self.assertNotIn('} |= "panic', query)
        self.assertIn("(?i)(timeout)", query)
        self.assertNotIn(".*", query)

    def test_query_loki_logs_normalizes_streams(self):
        payload = {
            "status": "success",
            "data": {
                "resultType": "streams",
                "result": [
                    {
                        "stream": {"service_name": "payment", "level": "error"},
                        "values": [
                            [
                                "1710000000000000000",
                                json.dumps({"service": "payment", "level": "error", "message": "upstream timeout"}),
                            ]
                        ],
                    }
                ],
            },
        }

        with mock.patch.object(log_adapter, "urlopen", return_value=FakeResponse(payload)):
            result = log_adapter.query_loki_logs({"query": "timeout", "service": "payment", "window": "30m"})

        self.assertTrue(result["success"])
        self.assertEqual(result["backend"], "loki")
        self.assertEqual(result["logs"][0]["service"], "payment")
        self.assertEqual(result["logs"][0]["level"], "error")
        self.assertEqual(result["logs"][0]["message"], "upstream timeout")

    def test_build_loki_push_payload_keeps_correlation_labels(self):
        record = log_adapter.SYNTHETIC_LOGS[0]
        payload = log_adapter.build_loki_push_payload(record, timestamp_ns=123)

        stream = payload["streams"][0]
        self.assertEqual(stream["stream"]["service_name"], "checkout")
        self.assertEqual(stream["stream"]["incident"], "payment-timeout")
        self.assertEqual(stream["values"][0][0], "123")
        self.assertEqual(json.loads(stream["values"][0][1])["trace_id"], "demo-timeout")

    def test_query_file_logs_filters_measured_records_and_keeps_provenance(self):
        now = log_adapter.datetime.now(log_adapter.timezone.utc).isoformat().replace("+00:00", "Z")
        records = [
            {
                "timestamp": now,
                "namespace": "opscaptain-eval",
                "service": "eval-risk-client",
                "level": "error",
                "case_id": "real-controlled-003",
                "message": "dependency request exceeded the client deadline",
                "provenance": {"source": "controlled_fault_injection", "measured": True},
            },
            {
                "timestamp": now,
                "namespace": "opscaptain-eval",
                "service": "eval-cpu-checkout",
                "level": "warn",
                "case_id": "real-controlled-001",
                "message": "worker consumed one CPU core",
                "provenance": {"source": "controlled_fault_injection", "measured": True},
            },
        ]
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telemetry.jsonl"
            path.write_text("".join(json.dumps(item) + "\n" for item in records), encoding="utf-8")
            old_path = log_adapter.LOG_FILE
            log_adapter.LOG_FILE = str(path)
            try:
                result = log_adapter.query_file_logs({"query": "eval-risk-client dependency", "limit": 5})
            finally:
                log_adapter.LOG_FILE = old_path

        self.assertTrue(result["success"])
        self.assertFalse(result["degraded"])
        self.assertEqual(result["backend"], "file")
        self.assertEqual(result["provenance"], "controlled_fault_injection_v1")
        self.assertEqual(len(result["logs"]), 1)
        self.assertEqual(result["logs"][0]["case_id"], "real-controlled-003")
        self.assertTrue(result["logs"][0]["provenance"]["measured"])


if __name__ == "__main__":
    unittest.main()
