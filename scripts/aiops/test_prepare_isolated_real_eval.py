import importlib.util
from pathlib import Path
import unittest


MODULE_PATH = Path(__file__).with_name("prepare_isolated_real_eval.py")
SPEC = importlib.util.spec_from_file_location("prepare_isolated_real_eval", MODULE_PATH)
config_tool = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(config_tool)


class PrepareIsolatedRealEvalTest(unittest.TestCase):
    def test_rewrite_config_applies_every_isolated_override(self):
        source = Path("manifest/config/config.yaml").read_text(encoding="utf-8")

        rendered = config_tool.rewrite_config(source)

        self.assertIn('telemetry_profile: "real"', rendered)
        self.assertIn('telemetry_provenance: "controlled_fault_injection_v1"', rendered)
        self.assertIn("call_timeout_ms: 30000", rendered)
        self.assertIn('log_http_url: "http://127.0.0.1:28088/tools/query_logs"', rendered)
        self.assertIn('log_default_window: "6h"', rendered)
        self.assertIn('address: "http://127.0.0.1:19090"', rendered)
        self.assertEqual(rendered.count('telemetry_profile: "real"'), 1)

    def test_rewrite_config_rejects_incomplete_source(self):
        with self.assertRaisesRegex(ValueError, "config overrides not found"):
            config_tool.rewrite_config("aiops:\n  gos:\n    enabled: false\n")

    def test_rewrite_config_accepts_isolated_endpoint_overrides(self):
        source = Path("manifest/config/config.yaml").read_text(encoding="utf-8")
        overrides = dict(config_tool.OVERRIDES)
        overrides["mcp.log_http_url"] = '"http://127.0.0.1:28089/tools/query_logs"'
        overrides["prometheus.address"] = '"http://127.0.0.1:19091"'
        overrides["milvus.address"] = '"127.0.0.1:29530"'

        rendered = config_tool.rewrite_config(source, overrides)

        self.assertIn('log_http_url: "http://127.0.0.1:28089/tools/query_logs"', rendered)
        self.assertIn('address: "http://127.0.0.1:19091"', rendered)
        self.assertIn('address: "127.0.0.1:29530"', rendered)


if __name__ == "__main__":
    unittest.main()
