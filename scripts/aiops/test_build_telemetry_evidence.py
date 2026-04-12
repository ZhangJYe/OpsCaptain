import json
import tempfile
import unittest
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq

from scripts.aiops import build_telemetry_evidence as mod


class BuildTelemetryEvidenceTest(unittest.TestCase):
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
            self.assertIn("checkoutservice latency spike", doc)
            self.assertIn("rrt", doc)
            self.assertIn("context canceled", doc)
            self.assertIn("CheckoutService/PlaceOrder", doc)

            metadata = json.loads((output_root / "docs_evidence_telemetry" / "case-a.metadata.json").read_text(encoding="utf-8"))
            self.assertEqual("case-a", metadata["case_id"])
            self.assertEqual("telemetry_evidence", metadata["doc_kind"])
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
