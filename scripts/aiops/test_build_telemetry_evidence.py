import json
import tempfile
import unittest
from contextlib import redirect_stdout
from io import StringIO
from pathlib import Path
from unittest.mock import patch

import pyarrow as pa
import pyarrow.parquet as pq

from scripts.aiops import build_telemetry_evidence as mod
from scripts.aiops import evaluate_telemetry_evidence as eval_mod
from scripts.aiops import freeze_recorded_holdout as freeze_mod


class BuildTelemetryEvidenceTest(unittest.TestCase):
    def test_freeze_recorded_holdout_is_input_only_deterministic_and_disjoint(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            input_path = root / "input.json"
            prior = root / "prior.json"
            prior.write_text(
                json.dumps(
                    {
                        "development_case_ids": ["used-development"],
                        "holdout_case_ids": ["used-holdout"],
                    }
                ),
                encoding="utf-8",
            )
            cases = []
            for index in range(5):
                hour = 16 + index
                cases.append(
                    {
                        "uuid": f"case-{index}",
                        "Anomaly Description": (
                            f"A fault occurred from 2025-06-08T{hour:02d}:10:00Z "
                            f"to 2025-06-08T{hour:02d}:20:00Z."
                        ),
                    }
                )
            cases.extend(
                [
                    {
                        "uuid": "used-development",
                        "Anomaly Description": (
                            "A fault occurred from 2025-06-08T16:10:00Z to 2025-06-08T16:20:00Z."
                        ),
                    },
                    {
                        "uuid": "used-holdout",
                        "Anomaly Description": (
                            "A fault occurred from 2025-06-08T17:10:00Z to 2025-06-08T17:20:00Z."
                        ),
                    },
                ]
            )
            input_path.write_text(json.dumps(cases), encoding="utf-8")

            first = freeze_mod.freeze_manifest(input_path, prior, ["2025-06-09"], 3, "fixed-seed")
            second = freeze_mod.freeze_manifest(input_path, prior, ["2025-06-09"], 3, "fixed-seed")

            self.assertEqual(first, second)
            self.assertEqual(3, len(first["holdout_case_ids"]))
            self.assertFalse({"used-development", "used-holdout"} & set(first["holdout_case_ids"]))
            self.assertEqual(5, first["consumed_case_count"])
            self.assertEqual(
                {"used-development", "used-holdout", *first["holdout_case_ids"]},
                set(first["consumed_case_ids"]),
            )
            self.assertNotIn("groundtruth", json.dumps(first).lower())
            self.assertNotIn("fault_type", json.dumps(first).lower())

    def test_freeze_recorded_holdout_excludes_multiple_prior_generations(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            input_path = root / "input.json"
            first_prior = root / "prior-v1.json"
            second_prior = root / "prior-v2.json"
            first_prior.write_text(
                json.dumps({"development_case_ids": ["dev"], "holdout_case_ids": ["holdout-v1"]}),
                encoding="utf-8",
            )
            second_prior.write_text(
                json.dumps(
                    {
                        "development_case_ids": ["dev"],
                        "holdout_case_ids": ["holdout-v2"],
                        "consumed_case_ids": ["dev", "holdout-v1", "holdout-v2"],
                    }
                ),
                encoding="utf-8",
            )
            input_path.write_text(
                json.dumps(
                    [
                        {
                            "uuid": case_id,
                            "Anomaly Description": (
                                "A fault occurred from 2025-06-08T16:10:00Z "
                                "to 2025-06-08T16:20:00Z."
                            ),
                        }
                        for case_id in ["dev", "holdout-v1", "holdout-v2", "new-a", "new-b"]
                    ]
                ),
                encoding="utf-8",
            )

            manifest = freeze_mod.freeze_manifest(
                input_path,
                [first_prior, second_prior],
                ["2025-06-09"],
                1,
                "fixed-seed",
            )

            self.assertEqual(2, manifest["prior_manifest_count"])
            self.assertFalse({"dev", "holdout-v1", "holdout-v2"} & set(manifest["holdout_case_ids"]))
            self.assertTrue({"dev", "holdout-v1", "holdout-v2"}.issubset(manifest["consumed_case_ids"]))

    def test_balanced_quotas_redistribute_a_capacity_shortfall(self) -> None:
        candidates = {
            "2025-06-09": ["a"] * 4,
            "2025-06-17": ["b"] * 15,
            "2025-06-19": ["c"] * 17,
        }

        self.assertEqual(
            {"2025-06-09": 4, "2025-06-17": 7, "2025-06-19": 7},
            freeze_mod.balanced_quotas(candidates, list(candidates), 18),
        )

    def test_build_gos_eval_dataset_keeps_labels_out_of_agent_input(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            manifest = root / "split.json"
            input_path = root / "input.json"
            groundtruth = root / "groundtruth.jsonl"
            manifest.write_text(
                json.dumps(
                    {
                        "schema_version": "aiops2025-recorded-split-v1",
                        "development_case_ids": ["case-a"],
                        "holdout_case_ids": ["case-b"],
                    }
                ),
                encoding="utf-8",
            )
            input_path.write_text(
                json.dumps(
                    [
                        {
                            "uuid": "case-b",
                            "Anomaly Description": "A fault occurred from 2025-06-16T18:10:05Z to 2025-06-16T18:24:05Z.",
                        }
                    ]
                ),
                encoding="utf-8",
            )
            groundtruth.write_text(
                json.dumps(
                    {
                        "uuid": "case-b",
                        "service": "cartservice",
                        "instance": "cartservice",
                        "instance_type": "service",
                        "fault_type": "code error",
                        "fault_description": ["database error", "cartservice"],
                        "key_metrics": ["pod_processes"],
                        "key_observations": [{"type": "log", "keyword": ["FailedPrecondition"]}],
                    }
                )
                + "\n",
                encoding="utf-8",
            )

            dataset = eval_mod.build_gos_eval_dataset(input_path, groundtruth, manifest, "holdout")

            self.assertEqual("gos-eval-v2", dataset["schema_version"])
            self.assertEqual("holdout", dataset["role"])
            self.assertEqual(["case-b"], [case["id"] for case in dataset["cases"]])
            self.assertNotIn("cartservice", dataset["cases"][0]["symptom"])
            self.assertEqual(["cartservice", "code error"], dataset["cases"][0]["expected_keywords"])
            self.assertEqual(["cartservice"], dataset["cases"][0]["expected_entity_keywords"])
            self.assertIn("code error", dataset["cases"][0]["expected_cause_keywords"])
            self.assertIn("database error", dataset["cases"][0]["expected_cause_keywords"])
            self.assertIn("代码错误", dataset["cases"][0]["expected_cause_keywords"])
            self.assertNotIn("cartservice", dataset["cases"][0]["expected_cause_keywords"])
            self.assertEqual(
                ["pod_processes", "FailedPrecondition"],
                dataset["cases"][0]["expected_evidence_keywords"],
            )

    def test_evaluator_builds_hashed_blind_artifact(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            evidence = root / "evidence"
            telemetry = evidence / "telemetry"
            docs = evidence / "docs_evidence_telemetry"
            telemetry.mkdir(parents=True)
            docs.mkdir(parents=True)
            manifest = root / "split.json"
            groundtruth = root / "groundtruth.jsonl"
            manifest.write_text(
                json.dumps(
                    {
                        "schema_version": "aiops2025-recorded-split-v1",
                        "development_case_ids": ["case-a"],
                        "holdout_case_ids": ["case-b"],
                    }
                ),
                encoding="utf-8",
            )
            groundtruth.write_text(
                json.dumps(
                    {
                        "uuid": "case-b",
                        "service": "emailservice",
                        "instance": "emailservice",
                        "source": "",
                        "destination": "",
                        "key_metrics": ["request", "response"],
                    }
                )
                + "\n",
                encoding="utf-8",
            )
            summary = {
                "case_id": "case-b",
                "provenance_profile": "recorded_blind",
                "target_selection": "input_time_window_only",
                "targets": {},
                "metric_signals": [
                    {"entity": "emailservice", "metric": "request", "source_file": "service_emailservice.parquet"}
                ],
                "log_signals": [],
                "trace_signals": [],
            }
            metadata = {"case_id": "case-b", "service": "unknown", "instance": []}
            (telemetry / "case_evidence_summary.jsonl").write_text(json.dumps(summary) + "\n", encoding="utf-8")
            (telemetry / "doc_metadata.jsonl").write_text(json.dumps(metadata) + "\n", encoding="utf-8")
            (telemetry / "telemetry_report.json").write_text(
                json.dumps(
                    {
                        "case_role": "holdout",
                        "extraction_profile": "blind",
                        "max_metric_signals": 16,
                        "case_manifest": str(manifest),
                    }
                ),
                encoding="utf-8",
            )
            (docs / "case-b.md").write_text("# Telemetry Evidence Case\n", encoding="utf-8")

            artifact = eval_mod.evaluate_artifact(evidence, groundtruth, manifest, "holdout")

            self.assertEqual("aiops2025-recorded-eval-v1", artifact["schema_version"])
            self.assertEqual(1.0, artifact["metrics"]["exact_entity_recall"])
            self.assertEqual(0.5, artifact["metrics"]["key_metric_coverage"])
            self.assertTrue(artifact["metrics"]["anti_leak_contract"])
            self.assertEqual(eval_mod.sha256_file(manifest), artifact["manifest_sha256"])

    def test_recorded_blind_artifacts_are_comparable_and_current(self) -> None:
        root = Path(__file__).resolve().parents[2]
        development = json.loads(
            (root / "evals" / "aiops2025" / "recorded_blind_development.json").read_text(encoding="utf-8")
        )
        holdout = json.loads(
            (root / "evals" / "aiops2025" / "recorded_blind_holdout.json").read_text(encoding="utf-8")
        )

        self.assertEqual("development", development["case_role"])
        self.assertEqual("holdout", holdout["case_role"])
        self.assertEqual(development["manifest_sha256"], holdout["manifest_sha256"])
        self.assertEqual(development["builder_sha256"], holdout["builder_sha256"])
        self.assertEqual(development["evaluator_sha256"], holdout["evaluator_sha256"])
        self.assertEqual(
            eval_mod.sha256_file(root / "scripts" / "aiops" / "build_telemetry_evidence.py"),
            holdout["builder_sha256"],
        )
        self.assertEqual(
            eval_mod.sha256_file(root / "scripts" / "aiops" / "evaluate_telemetry_evidence.py"),
            holdout["evaluator_sha256"],
        )
        self.assertEqual(18, development["metrics"]["cases"])
        self.assertEqual(18, holdout["metrics"]["cases"])
        self.assertTrue(development["metrics"]["anti_leak_contract"])
        self.assertTrue(holdout["metrics"]["anti_leak_contract"])
        self.assertFalse(
            {item["case_id"] for item in development["cases"]}
            & {item["case_id"] for item in holdout["cases"]}
        )

    def test_recorded_split_manifest_is_disjoint_and_label_free(self) -> None:
        path = Path(__file__).resolve().parents[2] / "evals" / "aiops2025" / "recorded_split.json"
        data = json.loads(path.read_text(encoding="utf-8"))
        development = data["development_case_ids"]
        holdout = data["holdout_case_ids"]

        self.assertEqual("aiops2025-recorded-split-v1", data["schema_version"])
        self.assertEqual(18, len(development))
        self.assertEqual(18, len(holdout))
        self.assertEqual(18, len(set(development)))
        self.assertEqual(18, len(set(holdout)))
        self.assertFalse(set(development) & set(holdout))
        self.assertNotIn("fault_type", json.dumps(data))
        self.assertNotIn("groundtruth", json.dumps(data))

    def test_recorded_holdout_v2_is_new_frozen_and_anti_leak(self) -> None:
        root = Path(__file__).resolve().parents[2]
        old_manifest = json.loads((root / "evals" / "aiops2025" / "recorded_split.json").read_text(encoding="utf-8"))
        manifest_path = root / "evals" / "aiops2025" / "recorded_split_v2.json"
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
        quality = json.loads(
            (root / "evals" / "aiops2025" / "recorded_blind_holdout_v2.json").read_text(encoding="utf-8")
        )
        dataset = json.loads(
            (root / "evals" / "aiops2025" / "recorded_holdout_v2.json").read_text(encoding="utf-8")
        )

        consumed = set(old_manifest["development_case_ids"] + old_manifest["holdout_case_ids"])
        selected = manifest["holdout_case_ids"]
        self.assertEqual(18, len(selected))
        self.assertEqual(18, len(set(selected)))
        self.assertFalse(consumed & set(selected))
        self.assertEqual(old_manifest["development_case_ids"], manifest["development_case_ids"])
        self.assertEqual({"2025-06-09": 4, "2025-06-17": 7, "2025-06-19": 7}, manifest["selected_counts_by_archive_date"])
        self.assertNotRegex(json.dumps(manifest).lower(), r"fault_type|fault_category|groundtruth")
        self.assertEqual(eval_mod.sha256_file(manifest_path), quality["manifest_sha256"])
        self.assertTrue(quality["metrics"]["anti_leak_contract"])
        self.assertEqual(selected, [item["case_id"] for item in quality["cases"]])
        self.assertEqual(selected, [item["id"] for item in dataset["cases"]])
        self.assertEqual("holdout", dataset["role"])

    def test_load_case_manifest_selects_role_and_rejects_overlap(self) -> None:
        path = Path(__file__).resolve().parents[2] / "evals" / "aiops2025" / "recorded_split.json"
        development = mod.load_case_manifest(path, "development")
        holdout = mod.load_case_manifest(path, "holdout")

        self.assertEqual(18, len(development))
        self.assertEqual(18, len(holdout))
        self.assertFalse(set(development) & set(holdout))

        with tempfile.TemporaryDirectory() as tmp:
            invalid = Path(tmp) / "split.json"
            invalid.write_text(
                json.dumps(
                    {
                        "schema_version": "aiops2025-recorded-split-v1",
                        "development_case_ids": ["case-a"],
                        "holdout_case_ids": ["case-a"],
                    }
                ),
                encoding="utf-8",
            )
            with self.assertRaisesRegex(ValueError, "must be disjoint"):
                mod.load_case_manifest(invalid, "holdout")

    def test_select_case_ids_preserves_explicit_order_and_applies_limit(self) -> None:
        available = ["case-a", "case-b", "case-c"]

        self.assertEqual(
            ["case-b", "case-a"],
            mod.select_case_ids(available, ["case-b", "case-b", "case-a"], limit=0),
        )
        self.assertEqual(
            ["case-b"],
            mod.select_case_ids(available, ["case-b", "case-a"], limit=1),
        )
        self.assertEqual(available, mod.select_case_ids(available, [], limit=0))

    def test_select_case_ids_rejects_unknown_ids(self) -> None:
        with self.assertRaisesRegex(ValueError, "unknown case ids: case-missing"):
            mod.select_case_ids(["case-a"], ["case-missing"], limit=0)

    def test_blind_context_uses_only_input_time_window(self) -> None:
        context = mod.build_blind_case_context(
            mod.InputCase(
                uuid="case-blind",
                anomaly_description=(
                    "Determine the root cause from 2025-06-08T17:10:04Z "
                    "to 2025-06-08T17:10:05Z."
                ),
            )
        )

        self.assertEqual(mod.EXTRACTION_PROFILE_BLIND, context.extraction_profile)
        self.assertEqual("2025-06-08T17:10:04Z", context.groundtruth.start_time)
        self.assertEqual("2025-06-08T17:10:05Z", context.groundtruth.end_time)
        self.assertEqual("", context.groundtruth.service)
        self.assertEqual([], context.groundtruth.instance)
        self.assertEqual([], context.entity_tokens)

    def test_blind_context_rejects_ambiguous_input_window(self) -> None:
        with self.assertRaisesRegex(ValueError, "exactly two UTC timestamps"):
            mod.build_blind_case_context(
                mod.InputCase(uuid="case-blind", anomaly_description="fault around 2025-06-08T17:10:04Z")
            )

    def test_blind_metric_extraction_keeps_entity_boundaries(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "pod_request.parquet"
            self._write_parquet(
                path,
                [
                    {"time": "2025-06-08T17:09:00Z", "pod": "pod-a", "request": 100},
                    {"time": "2025-06-08T17:10:00Z", "pod": "pod-a", "request": 100},
                    {"time": "2025-06-08T17:09:00Z", "pod": "pod-b", "request": 100},
                    {"time": "2025-06-08T17:10:00Z", "pod": "pod-b", "request": 10},
                ],
            )
            context = mod.build_blind_case_context(
                mod.InputCase(
                    uuid="case-blind",
                    anomaly_description=(
                        "Determine the root cause from 2025-06-08T17:10:00Z "
                        "to 2025-06-08T17:10:30Z."
                    ),
                )
            )

            signals = mod.metric_signals_for_file(path, context)

            request_signals = [signal for signal in signals if signal.metric == "request"]
            self.assertEqual(["pod-b"], [signal.entity for signal in request_signals])

    def test_metric_selection_prioritizes_entity_diversity(self) -> None:
        signals = [
            mod.MetricSignal("a.parquet", "pod-a", "error", 0, 10, 10, 10, 1),
            mod.MetricSignal("b.parquet", "pod-a", "timeout", 0, 9, 9, 9, 1),
            mod.MetricSignal("c.parquet", "pod-b", "request", 100, 20, 20, 0.8, 1),
        ]

        selected = mod.select_metric_signals(signals, 2)

        self.assertEqual(["pod-a", "pod-b"], [signal.entity for signal in selected])
        self.assertEqual(["error", "request"], [signal.metric for signal in selected])

    def test_metric_selection_preserves_source_family_coverage(self) -> None:
        signals = [
            mod.MetricSignal("service_frontend.parquet", "frontend", "error", 0, 10, 10, 10, 1),
            mod.MetricSignal("pod_checkout-0.parquet", "checkout-0", "timeout", 0, 9, 9, 9, 1),
            mod.MetricSignal("infra_tidb_qps.parquet", "10.0.0.1:10080", "qps", 100, 90, 90, 0.1, 1),
        ]

        selected = mod.select_metric_signals(signals, 2)

        self.assertEqual(
            {"apm", "infra_tidb"},
            {mod.metric_source_family(signal.source_file) for signal in selected},
        )

    def test_blind_main_does_not_require_groundtruth_file(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "aiopschallenge2025"
            output = Path(tmp) / "output"
            metric_dir = root / "extracted" / "2025-06-09" / "metric-parquet" / "apm" / "service"
            metric_dir.mkdir(parents=True)
            (root / "input.json").write_text(
                json.dumps(
                    [
                        {
                            "uuid": "case-blind",
                            "Anomaly Description": (
                                "Determine the root cause from 2025-06-08T17:10:00Z "
                                "to 2025-06-08T17:11:00Z."
                            ),
                        }
                    ]
                ),
                encoding="utf-8",
            )
            self._write_parquet(
                metric_dir / "service_checkoutservice_2025-06-09.parquet",
                [
                    {"time": "2025-06-08T17:09:00Z", "object_id": "checkoutservice", "request": 100},
                    {"time": "2025-06-08T17:10:00Z", "object_id": "checkoutservice", "request": 10},
                ],
            )

            argv = [
                "build_telemetry_evidence.py",
                "--dataset-root",
                str(root),
                "--output-root",
                str(output),
                "--extraction-profile",
                "blind",
                "--case-id",
                "case-blind",
                "--progress-seconds",
                "0",
            ]
            with patch("sys.argv", argv), redirect_stdout(StringIO()):
                mod.main()

            report = json.loads((output / "telemetry" / "telemetry_report.json").read_text(encoding="utf-8"))
            metadata = json.loads(
                (output / "docs_evidence_telemetry" / "case-blind.metadata.json").read_text(encoding="utf-8")
            )
            self.assertEqual("recorded_blind", report["provenance_profile"])
            self.assertEqual("input_time_window_only", report["target_selection"])
            self.assertEqual(0, report["empty_cases"])
            self.assertEqual("recorded_blind", metadata["provenance_profile"])
            self.assertEqual("unknown", metadata["service"])
            self.assertFalse((root / "groundtruth.jsonl").exists())

    def test_tidb_metric_rows_use_tidb_namespace_when_instance_is_not_named(self) -> None:
        case = mod.build_case_context(
            mod.InputCase(uuid="case-tidb", anomaly_description="tidb failure"),
            mod.GroundTruthCase(
                uuid="case-tidb",
                fault_category="io fault",
                fault_type="io fault",
                instance_type="pod",
                service="tidb-tikv",
                instance=["tidb-tikv-0"],
                source="",
                destination="",
                start_time="2025-06-16T22:10:12Z",
                end_time="2025-06-16T22:15:12Z",
            ),
        )
        rows = mod.pd.DataFrame(
            {"namespace": ["tidb"], "instance": ["10.233.79.158:10080"], "cpu_usage": [0.8]}
        )

        filtered = mod.filter_metric_rows(
            rows,
            case,
            Path("/data/metric-parquet/infra/infra_tidb/infra_tidb_cpu_usage.parquet"),
        )

        self.assertEqual(1, len(filtered))

    def test_short_incident_includes_minute_bucket_and_observation_grace(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "service_productcatalogservice.parquet"
            self._write_parquet(
                path,
                [
                    {"time": "2025-06-08T17:09:00Z", "object_id": "productcatalogservice", "request": 100},
                    {"time": "2025-06-08T17:10:00Z", "object_id": "productcatalogservice", "request": 10},
                    {"time": "2025-06-08T17:11:00Z", "object_id": "productcatalogservice", "request": 10},
                ],
            )
            case = mod.build_case_context(
                mod.InputCase(uuid="case-kill", anomaly_description="pod kill"),
                mod.GroundTruthCase(
                    uuid="case-kill",
                    fault_category="pod fault",
                    fault_type="pod kill",
                    instance_type="service",
                    service="productcatalogservice",
                    instance=["productcatalogservice"],
                    source="",
                    destination="",
                    start_time="2025-06-08T17:10:04Z",
                    end_time="2025-06-08T17:10:05Z",
                ),
            )

            signals = mod.metric_signals_for_file(path, case)

            request_signal = next(signal for signal in signals if signal.metric == "request")
            self.assertEqual(100.0, request_signal.baseline_mean)
            self.assertEqual(10.0, request_signal.incident_mean)
            self.assertEqual(2, request_signal.sample_count)

    def test_namespace_tokens_do_not_expand_log_relevance(self) -> None:
        case = mod.build_case_context(
            mod.InputCase(uuid="case-pod", anomaly_description="paymentservice failure"),
            mod.GroundTruthCase(
                uuid="case-pod",
                fault_category="network",
                fault_type="timeout",
                instance_type="pod",
                service="paymentservice",
                instance=["paymentservice-1"],
                source="",
                destination="",
                start_time="2025-06-11T21:10:11Z",
                end_time="2025-06-11T21:10:12Z",
            ),
        )
        self.assertNotIn("hipstershop", case.entity_tokens)
        self.assertTrue(
            mod.is_relevant_log_row(
                case,
                "paymentservice-1",
                "aiops-k8s-01",
                "paymentservice timeout waiting for downstream",
            )
        )
        self.assertFalse(
            mod.is_relevant_log_row(
                case,
                "cartservice-2",
                "aiops-k8s-08",
                "Executed endpoint 'gRPC - /hipstershop.CartService/GetCart'",
            )
        )

    def test_request_latency_millis_does_not_count_as_http_500(self) -> None:
        self.assertLessEqual(
            mod.log_pattern_signal_score(
                "Request finished HTTP/2 POST http://cartservice:7070/hipstershop.CartService/GetCart application/grpc 5009ms"
            ),
            0,
        )
        self.assertGreater(
            mod.log_pattern_signal_score("rpc failed with status 500 and timeout"),
            0,
        )

    def test_generates_telemetry_docs_and_build_only_outputs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp) / "aiopschallenge2025"
            extracted = root / "extracted" / "2025-06-06"
            (extracted / "log-parquet").mkdir(parents=True)
            (extracted / "trace-parquet").mkdir(parents=True)
            (extracted / "metric-parquet" / "apm" / "service").mkdir(parents=True)
            (extracted / "metric-parquet" / "infra" / "infra_pod").mkdir(parents=True)
            (root / "baseline" / "eval").mkdir(parents=True)

            (root / "input.json").write_text(
                json.dumps(
                    [
                        {"uuid": "case-a", "Anomaly Description": "checkoutservice latency spike"},
                        {"uuid": "case-b", "Anomaly Description": "frontend timeout"},
                    ]
                ),
                encoding="utf-8",
            )
            (root / "groundtruth.jsonl").write_text(
                "\n".join(
                    [
                        json.dumps(
                            {
                                "uuid": "case-a",
                                "fault_category": "network",
                                "fault_type": "network delay",
                                "instance_type": "service",
                                "service": "checkoutservice",
                                "instance": "checkoutservice",
                                "source": "frontend",
                                "destination": "checkoutservice",
                                "start_time": "2025-06-05T16:10:00Z",
                                "end_time": "2025-06-05T16:20:00Z",
                            }
                        ),
                        json.dumps(
                            {
                                "uuid": "case-b",
                                "fault_category": "stress",
                                "fault_type": "cpu stress",
                                "instance_type": "service",
                                "service": "frontend",
                                "instance": "frontend",
                                "source": "",
                                "destination": "",
                                "start_time": "2025-06-05T17:10:00Z",
                                "end_time": "2025-06-05T17:20:00Z",
                            }
                        ),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )
            (root / "baseline" / "eval" / "build_split.json").write_text(
                json.dumps({"build_case_ids": ["case-a"]}),
                encoding="utf-8",
            )

            self._write_parquet(
                extracted / "log-parquet" / "log_filebeat-server_2025-06-06_00-00-00.parquet",
                [
                    {
                        "k8_namespace": "hipstershop",
                        "@timestamp": "2025-06-05T16:12:00Z",
                        "agent_name": "filebeat-1",
                        "k8_pod": "checkoutservice-1",
                        "message": "rpc error code canceled desc context canceled",
                        "k8_node_name": "aiops-k8s-01",
                    }
                ],
            )
            self._write_parquet(
                extracted / "trace-parquet" / "trace_jaeger-span_2025-06-06_00-00-00.parquet",
                [
                    {
                        "operationName": "hipstershop.CheckoutService/PlaceOrder",
                        "startTimeMillis": 1749139920000,
                        "duration": 42000,
                        "tags": [
                            {"key": "rpc.service", "type": "string", "value": "checkoutservice"},
                            {"key": "status.code", "type": "int64", "value": "2"},
                        ],
                        "process": {"serviceName": "frontend"},
                    }
                ],
            )
            self._write_parquet(
                extracted / "metric-parquet" / "apm" / "service" / "service_checkoutservice_2025-06-06.parquet",
                [
                    {
                        "time": "2025-06-05T15:50:00Z",
                        "object_id": "checkoutservice",
                        "object_type": "service",
                        "rrt": 100.0,
                        "error_ratio": 0.01,
                        "request": 100,
                        "response": 100,
                    },
                    {
                        "time": "2025-06-05T16:15:00Z",
                        "object_id": "checkoutservice",
                        "object_type": "service",
                        "rrt": 400.0,
                        "error_ratio": 0.20,
                        "request": 100,
                        "response": 60,
                    },
                ],
            )
            self._write_parquet(
                extracted / "metric-parquet" / "infra" / "infra_pod" / "infra_pod_pod_cpu_usage_2025-06-06.parquet",
                [
                    {
                        "time": "2025-06-05T16:15:00Z",
                        "instance": "aiops-k8s-01",
                        "namespace": "hipstershop",
                        "object_type": "pod",
                        "pod": "checkoutservice-1",
                        "pod_cpu_usage": 87.0,
                    }
                ],
            )

            output_root = root / "baseline"
            inputs = mod.load_input_cases(root / "input.json")
            groundtruth = mod.load_groundtruth_cases(root / "groundtruth.jsonl")
            build_ids = mod.load_build_ids(root / "baseline" / "eval" / "build_split.json")

            case_ids = sorted(set(inputs) & set(groundtruth))
            summaries = []
            for case_id in case_ids:
                case = mod.build_case_context(inputs[case_id], groundtruth[case_id])
                summary = mod.summarize_case(root, case)
                mod.write_text(output_root / "docs_evidence_telemetry" / f"{case_id}.md", mod.render_telemetry_doc(case, summary))
                mod.write_json(
                    output_root / "docs_evidence_telemetry" / f"{case_id}.metadata.json",
                    mod.build_doc_metadata(case, summary, split="all").to_json(),
                )
                if case_id in build_ids:
                    mod.write_text(output_root / "docs_evidence_telemetry_build" / f"{case_id}.md", mod.render_telemetry_doc(case, summary))
                    mod.write_json(
                        output_root / "docs_evidence_telemetry_build" / f"{case_id}.metadata.json",
                        mod.build_doc_metadata(case, summary, split="build").to_json(),
                    )
                summaries.append(summary.to_json())
            mod.write_jsonl(output_root / "telemetry" / "case_evidence_summary.jsonl", summaries)

            doc = (output_root / "docs_evidence_telemetry" / "case-a.md").read_text(encoding="utf-8")
            self.assertIn("provenance_profile: recorded_label_assisted", doc)
            self.assertIn("evaluation_eligibility: development_only", doc)
            self.assertIn("checkoutservice latency spike", doc)
            self.assertIn("rrt", doc)
            self.assertIn("context canceled", doc)
            self.assertIn("CheckoutService/PlaceOrder", doc)

            metadata = json.loads((output_root / "docs_evidence_telemetry" / "case-a.metadata.json").read_text(encoding="utf-8"))
            self.assertEqual("case-a", metadata["case_id"])
            self.assertEqual("telemetry_evidence", metadata["doc_kind"])
            self.assertEqual("recorded_label_assisted", metadata["provenance_profile"])
            self.assertEqual("development_only", metadata["evaluation_eligibility"])
            self.assertEqual("all", metadata["split"])
            self.assertIn("checkoutservice", metadata["service_tokens"])

            self.assertTrue((output_root / "docs_evidence_telemetry_build" / "case-a.md").exists())
            self.assertTrue((output_root / "docs_evidence_telemetry_build" / "case-a.metadata.json").exists())
            self.assertFalse((output_root / "docs_evidence_telemetry_build" / "case-b.md").exists())

    @staticmethod
    def _write_parquet(path: Path, rows: list[dict]) -> None:
        path.parent.mkdir(parents=True, exist_ok=True)
        table = pa.Table.from_pylist(rows)
        pq.write_table(table, path)


if __name__ == "__main__":
    unittest.main()
