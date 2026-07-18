import importlib.util
from pathlib import Path
import unittest


MODULE_PATH = Path(__file__).with_name("isolated_real_fixture.py")
SPEC = importlib.util.spec_from_file_location("isolated_real_fixture", MODULE_PATH)
fixture = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fixture)


class IsolatedRealFixtureTest(unittest.TestCase):
    def test_build_fixture_produces_measured_case_scoped_telemetry(self):
        records, samples = fixture.build_fixture()

        self.assertEqual(len(records), 5)
        self.assertEqual(len({item["case_id"] for item in records}), 5)
        self.assertTrue(all(item["namespace"] == "opscaptain-eval" for item in records))
        self.assertTrue(all(item["provenance"]["measured"] for item in records))
        self.assertTrue(all(item["measurements"] for item in records))
        self.assertGreaterEqual(len(samples), 10)

        metrics = fixture.prometheus_text(samples)
        self.assertIn("opscaptain_eval_cpu_ratio", metrics)
        self.assertIn("opscaptain_eval_dependency_timeouts_total", metrics)
        self.assertIn('provenance="controlled_fault_injection_v1"', metrics)

    def test_development_fixture_is_disjoint_and_measured(self):
        holdout_records, _ = fixture.build_fixture("holdout_v1")
        records, samples = fixture.build_fixture("development_v1")

        self.assertEqual(len(records), 3)
        self.assertTrue({item["case_id"] for item in records}.isdisjoint({item["case_id"] for item in holdout_records}))
        self.assertTrue(all(item["provenance"]["measured"] for item in records))
        metrics = fixture.prometheus_text(samples)
        self.assertIn("opscaptain_eval_queue_peak_depth", metrics)
        self.assertIn("opscaptain_eval_retry_attempts_total", metrics)
        self.assertIn("opscaptain_eval_db_pool_wait_seconds", metrics)

    def test_fixture_can_isolate_one_case(self):
        records, samples = fixture.build_fixture("development_v1", "real-development-002")

        self.assertEqual([record["case_id"] for record in records], ["real-development-002"])
        self.assertTrue(samples)
        self.assertEqual({sample[1] for sample in samples}, {"real-development-002"})

    def test_fixture_rejects_case_from_another_suite(self):
        with self.assertRaisesRegex(ValueError, "does not belong"):
            fixture.build_fixture("development_v1", "real-holdout-001")


if __name__ == "__main__":
    unittest.main()
