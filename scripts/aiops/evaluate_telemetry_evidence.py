#!/usr/bin/env python3
"""Evaluate blind AIOps2025 telemetry evidence after extraction completes."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

if __package__:
    from scripts.aiops.build_telemetry_evidence import load_case_manifest
else:
    from build_telemetry_evidence import load_case_manifest


ARTIFACT_SCHEMA_VERSION = "aiops2025-recorded-eval-v1"
FORBIDDEN_LABEL_FIELDS = {"fault_type", "fault_category", "fault_description", "groundtruth"}
GENERIC_TARGET_PARTS = {"service", "aiops", "k8s"}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Evaluate label-free telemetry evidence against held-out labels.")
    parser.add_argument("--evidence-root", required=True)
    parser.add_argument("--groundtruth", required=True)
    parser.add_argument("--case-manifest", required=True)
    parser.add_argument("--case-role", choices=["development", "holdout"], required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--input", help="AIOps2025 input.json; required with --gos-eval-output")
    parser.add_argument("--gos-eval-output", help="Optional gos-eval-v2 dataset generated after blind extraction")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    artifact = evaluate_artifact(
        evidence_root=Path(args.evidence_root).resolve(),
        groundtruth_path=Path(args.groundtruth).resolve(),
        manifest_path=Path(args.case_manifest).resolve(),
        role=args.case_role,
    )
    output = Path(args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(artifact, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    if bool(args.input) != bool(args.gos_eval_output):
        raise ValueError("--input and --gos-eval-output must be provided together")
    if args.gos_eval_output:
        dataset = build_gos_eval_dataset(
            input_path=Path(args.input).resolve(),
            groundtruth_path=Path(args.groundtruth).resolve(),
            manifest_path=Path(args.case_manifest).resolve(),
            role=args.case_role,
        )
        dataset_output = Path(args.gos_eval_output).resolve()
        dataset_output.parent.mkdir(parents=True, exist_ok=True)
        dataset_output.write_text(json.dumps(dataset, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(artifact, ensure_ascii=False, indent=2))


def evaluate_artifact(
    evidence_root: Path,
    groundtruth_path: Path,
    manifest_path: Path,
    role: str,
) -> dict[str, Any]:
    expected_ids = load_case_manifest(manifest_path, role)
    summary_path = evidence_root / "telemetry" / "case_evidence_summary.jsonl"
    metadata_path = evidence_root / "telemetry" / "doc_metadata.jsonl"
    report_path = evidence_root / "telemetry" / "telemetry_report.json"
    summaries = load_jsonl(summary_path)
    metadata = load_jsonl(metadata_path)
    report = json.loads(report_path.read_text(encoding="utf-8"))
    groundtruth = load_groundtruth(groundtruth_path)

    actual_ids = [str(item.get("case_id", "")) for item in summaries]
    if actual_ids != expected_ids:
        raise ValueError("evidence case ids or order do not match the selected manifest role")
    if [str(item.get("case_id", "")) for item in metadata] != expected_ids:
        raise ValueError("metadata case ids or order do not match the selected manifest role")
    if report.get("case_role") != role or report.get("extraction_profile") != "blind":
        raise ValueError("telemetry report is not a blind artifact for the selected role")

    anti_leak = validate_anti_leak_contract(summaries, metadata, evidence_root / "docs_evidence_telemetry")
    case_results = [evaluate_case(summary, groundtruth[summary["case_id"]]) for summary in summaries]
    total_key_metrics = sum(item["key_metric_total"] for item in case_results)
    matched_key_metrics = sum(item["key_metric_matches"] for item in case_results)
    metrics = {
        "cases": len(case_results),
        "empty_cases": sum(1 for item in case_results if item["empty"]),
        "exact_entity_hits": sum(1 for item in case_results if item["exact_entity_hit"]),
        "subsystem_hits": sum(1 for item in case_results if item["subsystem_hit"]),
        "key_metric_case_hits": sum(1 for item in case_results if item["key_metric_matches"] > 0),
        "key_metric_matches": matched_key_metrics,
        "key_metric_total": total_key_metrics,
        "anti_leak_contract": anti_leak,
    }
    count = metrics["cases"]
    metrics["exact_entity_recall"] = metrics["exact_entity_hits"] / count if count else 0.0
    metrics["subsystem_recall"] = metrics["subsystem_hits"] / count if count else 0.0
    metrics["key_metric_case_recall"] = metrics["key_metric_case_hits"] / count if count else 0.0
    metrics["key_metric_coverage"] = matched_key_metrics / total_key_metrics if total_key_metrics else 0.0

    repo_root = Path(__file__).resolve().parents[2]
    return {
        "schema_version": ARTIFACT_SCHEMA_VERSION,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "git_commit": current_commit(repo_root),
        "case_role": role,
        "provenance_profile": "recorded_blind",
        "evaluation_eligibility": "development_only",
        "target_selection": "input_time_window_only",
        "manifest_path": str(manifest_path),
        "manifest_sha256": sha256_file(manifest_path),
        "summary_sha256": sha256_file(summary_path),
        "metadata_sha256": sha256_file(metadata_path),
        "builder_sha256": sha256_file(repo_root / "scripts" / "aiops" / "build_telemetry_evidence.py"),
        "evaluator_sha256": sha256_file(Path(__file__).resolve()),
        "extraction_config": {
            "extraction_profile": report.get("extraction_profile"),
            "max_metric_signals": report.get("max_metric_signals"),
            "case_manifest": report.get("case_manifest"),
            "case_role": report.get("case_role"),
        },
        "metrics": metrics,
        "cases": case_results,
    }


def evaluate_case(summary: dict[str, Any], groundtruth: dict[str, Any]) -> dict[str, Any]:
    observed = observed_text(summary)
    exact_tokens = target_tokens(groundtruth)
    subsystem_tokens = list(exact_tokens)
    for token in exact_tokens:
        subsystem_tokens.extend(
            part for part in re.split(r"[-_]", token) if len(part) >= 3 and part not in GENERIC_TARGET_PARTS
        )
    key_metrics = [str(item).lower().strip() for item in groundtruth.get("key_metrics", []) if str(item).strip()]
    matches = [metric for metric in key_metrics if metric in observed]
    return {
        "case_id": summary["case_id"],
        "empty": not any(summary.get(key) for key in ("metric_signals", "log_signals", "trace_signals")),
        "exact_entity_hit": any(token in observed for token in exact_tokens),
        "subsystem_hit": any(token in observed for token in subsystem_tokens),
        "key_metric_matches": len(matches),
        "key_metric_total": len(key_metrics),
    }


def build_gos_eval_dataset(
    input_path: Path,
    groundtruth_path: Path,
    manifest_path: Path,
    role: str,
) -> dict[str, Any]:
    expected_ids = load_case_manifest(manifest_path, role)
    inputs = {str(item["uuid"]): item for item in json.loads(input_path.read_text(encoding="utf-8"))}
    groundtruth = load_groundtruth(groundtruth_path)
    cases: list[dict[str, Any]] = []
    for case_id in expected_ids:
        if case_id not in inputs or case_id not in groundtruth:
            raise ValueError(f"missing input or groundtruth for case {case_id}")
        label = groundtruth[case_id]
        symptom = str(inputs[case_id].get("Anomaly Description", "")).strip()
        if not symptom:
            raise ValueError(f"missing anomaly description for case {case_id}")
        expected_keywords = diagnosis_keywords(label)
        evidence_keywords = evidence_contract_keywords(label)
        affected = expected_keywords[0] if expected_keywords else "unknown"
        fault_type = str(label.get("fault_type", "unknown fault")).strip()
        descriptions = [str(item).strip() for item in label.get("fault_description", []) if str(item).strip()]
        ground_truth = f"{fault_type}; affected entity {affected}"
        if descriptions:
            ground_truth += f"; {descriptions[0]}"
        cases.append(
            {
                "id": case_id,
                "domain": normalized_domain(label),
                "scenario": "recorded_blind_root_cause",
                "symptom": symptom,
                "ground_truth": ground_truth,
                "expected_keywords": expected_keywords,
                "expected_evidence_keywords": evidence_keywords,
                "expected_status": "succeeded",
                "notes": "AIOps2025 recorded_blind; labels are evaluator-only and never passed to the agent.",
            }
        )
    return {
        "schema_version": "gos-eval-v2",
        "role": role,
        "description": (
            "AIOps2025 recorded_blind case-scoped replay. Agent input contains only the anomaly time window; "
            "ground truth and expected keywords are evaluator-only."
        ),
        "cases": cases,
    }


def diagnosis_keywords(label: dict[str, Any]) -> list[str]:
    instances = label.get("instance", [])
    if isinstance(instances, str):
        instances = [instances]
    values = [label.get("service", "")]
    values.extend(instances)
    values.append(label.get("fault_type", ""))
    return unique_non_empty(str(value).strip() for value in values)


def evidence_contract_keywords(label: dict[str, Any]) -> list[str]:
    values: list[str] = [str(item).strip() for item in label.get("key_metrics", []) if str(item).strip()]
    for observation in label.get("key_observations", []):
        keywords = observation.get("keyword", []) if isinstance(observation, dict) else []
        if isinstance(keywords, str):
            keywords = [keywords]
        values.extend(str(item).strip() for item in keywords if str(item).strip())
    return unique_non_empty(values)


def normalized_domain(label: dict[str, Any]) -> str:
    value = str(label.get("instance_type") or label.get("fault_category") or "recorded").strip().lower()
    normalized = re.sub(r"[^a-z0-9]+", "_", value).strip("_")
    return normalized or "recorded"


def validate_anti_leak_contract(
    summaries: list[dict[str, Any]], metadata: list[dict[str, Any]], docs_dir: Path
) -> bool:
    for item in summaries:
        if forbidden_keys(item) or item.get("targets") != {}:
            raise ValueError(f"blind summary {item.get('case_id')} contains label-derived fields")
        if item.get("provenance_profile") != "recorded_blind" or item.get("target_selection") != "input_time_window_only":
            raise ValueError(f"blind summary {item.get('case_id')} has invalid provenance")
    for item in metadata:
        if forbidden_keys(item) or item.get("service") != "unknown" or item.get("instance") != []:
            raise ValueError(f"blind metadata {item.get('case_id')} contains label-derived targets")
    label_field_pattern = re.compile(r"(?im)^\s*-?\s*(fault_type|fault_category|fault_description|groundtruth)\s*:")
    for path in docs_dir.glob("*.md"):
        if label_field_pattern.search(path.read_text(encoding="utf-8")):
            raise ValueError(f"blind evidence document {path.name} contains a forbidden label field")
    return True


def forbidden_keys(value: Any) -> set[str]:
    if isinstance(value, dict):
        found = {str(key).lower() for key in value if str(key).lower() in FORBIDDEN_LABEL_FIELDS}
        for item in value.values():
            found.update(forbidden_keys(item))
        return found
    if isinstance(value, list):
        found: set[str] = set()
        for item in value:
            found.update(forbidden_keys(item))
        return found
    return set()


def observed_text(summary: dict[str, Any]) -> str:
    values: list[str] = []
    for item in summary.get("metric_signals", []):
        values.extend([item.get("entity", ""), item.get("metric", ""), item.get("source_file", "")])
    for item in summary.get("log_signals", []):
        values.extend([item.get("pod", ""), item.get("node", ""), item.get("pattern", "")])
    for item in summary.get("trace_signals", []):
        values.extend([item.get("service", ""), item.get("operation", ""), item.get("peer", "")])
    return " ".join(str(value) for value in values).lower()


def target_tokens(groundtruth: dict[str, Any]) -> list[str]:
    instances = groundtruth.get("instance", [])
    if isinstance(instances, str):
        instances = [instances]
    values = [groundtruth.get("service", ""), groundtruth.get("source", ""), groundtruth.get("destination", "")]
    values.extend(instances)
    return unique_non_empty(str(value).lower().strip() for value in values)


def load_groundtruth(path: Path) -> dict[str, dict[str, Any]]:
    rows = load_jsonl(path)
    return {str(row["uuid"]): row for row in rows}


def load_jsonl(path: Path) -> list[dict[str, Any]]:
    return [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]


def unique_non_empty(values: Any) -> list[str]:
    result: list[str] = []
    for value in values:
        if value and value not in result:
            result.append(value)
    return result


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def current_commit(repo_root: Path) -> str:
    completed = subprocess.run(
        ["git", "rev-parse", "HEAD"],
        cwd=repo_root,
        check=False,
        text=True,
        capture_output=True,
    )
    return completed.stdout.strip() if completed.returncode == 0 else "unknown"


if __name__ == "__main__":
    main()
