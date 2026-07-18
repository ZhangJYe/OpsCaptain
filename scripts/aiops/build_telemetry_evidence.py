#!/usr/bin/env python3
"""Build telemetry-driven evidence documents for aiopschallenge2025.

This stage upgrades the baseline from label-derived evidence docs to docs built
from raw parquet telemetry around each fault window.
"""

from __future__ import annotations

import argparse
import json
import math
import re
import time
from collections import Counter, defaultdict
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any, Iterable

import pandas as pd
import pyarrow.parquet as pq
from zoneinfo import ZoneInfo


BEIJING = ZoneInfo("Asia/Shanghai")
UTC = timezone.utc
EXTRACTION_PROFILE_GROUNDTRUTH = "groundtruth_guided"
EXTRACTION_PROFILE_BLIND = "blind"
MIN_OBSERVATION_WINDOW = timedelta(minutes=1)

NUMERIC_TYPES = {"int64", "double", "float", "float64", "int32"}
SKIP_METRIC_COLUMNS = {
    "time",
    "cf",
    "device",
    "instance",
    "kpi_key",
    "kpi_name",
    "kubernetes_node",
    "mountpoint",
    "namespace",
    "object_type",
    "pod",
    "sql_type",
    "type",
    "object_id",
}
LOG_TIMESTAMP_COL = "@timestamp"
TRACE_TIMESTAMP_COL = "startTimeMillis"
DEFAULT_GUIDED_MAX_METRIC_SIGNALS = 8
DEFAULT_BLIND_MAX_METRIC_SIGNALS = 16
MAX_LOG_SIGNALS = 6
MAX_TRACE_SIGNALS = 6
KEYWORD_STOPWORDS = {
    "request",
    "starting",
    "finished",
    "executed",
    "executing",
    "endpoint",
    "application",
    "grpc",
    "http",
    "post",
    "get",
    "with",
    "detail",
    "code",
    "status",
    "raised",
}

UUID_RE = re.compile(r"\b[0-9a-f]{8}-[0-9a-f-]{27}\b", re.IGNORECASE)
HEX_RE = re.compile(r"\b[0-9a-f]{16,}\b", re.IGNORECASE)
IP_RE = re.compile(r"\b(?:\d{1,3}\.){3}\d{1,3}\b")
NUMBER_RE = re.compile(r"\b\d+(?:\.\d+)?\b")
WHITESPACE_RE = re.compile(r"\s+")
POD_SUFFIX_RE = re.compile(r"^(?P<base>[a-z0-9-]+?)-\d+(?:\s+\(deleted\))?$")
ANSI_RE = re.compile(r"\x1B\[[0-?]*[ -/]*[@-~]")
UTC_TIMESTAMP_RE = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z")

HIPSTERSHOP_SERVICES = {
    "adservice",
    "cartservice",
    "checkoutservice",
    "currencyservice",
    "emailservice",
    "frontend",
    "paymentservice",
    "productcatalogservice",
    "recommendationservice",
    "redis-cart",
    "shippingservice",
}

TIDB_HINTS = {"tidb", "tikv", "pd", "tikv", "tidb-pd", "tidb-tikv", "tidb-tidb"}


@dataclass
class InputCase:
    uuid: str
    anomaly_description: str


@dataclass
class GroundTruthCase:
    uuid: str
    fault_category: str
    fault_type: str
    instance_type: str
    service: str
    instance: list[str]
    source: str
    destination: str
    start_time: str
    end_time: str


@dataclass
class CaseContext:
    input_case: InputCase
    groundtruth: GroundTruthCase
    start_utc: datetime
    end_utc: datetime
    start_local: datetime
    end_local: datetime
    service_tokens: list[str]
    pod_tokens: list[str]
    node_tokens: list[str]
    entity_tokens: list[str]
    file_tokens: list[str]
    namespace_tokens: list[str]
    extraction_profile: str


@dataclass
class MetricSignal:
    source_file: str
    entity: str
    metric: str
    baseline_mean: float
    incident_mean: float
    incident_max: float
    score: float
    sample_count: int


@dataclass
class LogSignal:
    pod: str
    node: str
    pattern: str
    count: int
    first_seen: str
    last_seen: str
    signal_score: int


@dataclass
class TraceSignal:
    service: str
    operation: str
    peer: str
    count: int
    error_count: int
    avg_duration_ms: float
    p95_duration_ms: float
    score: float


@dataclass
class TelemetrySummary:
    case_id: str
    provenance_profile: str
    evaluation_eligibility: str
    target_selection: str
    time_window_utc: str
    observation_window_utc: str
    targets: dict[str, Any]
    metric_signals: list[MetricSignal] = field(default_factory=list)
    log_signals: list[LogSignal] = field(default_factory=list)
    trace_signals: list[TraceSignal] = field(default_factory=list)

    def to_json(self) -> dict[str, Any]:
        return {
            "case_id": self.case_id,
            "provenance_profile": self.provenance_profile,
            "evaluation_eligibility": self.evaluation_eligibility,
            "target_selection": self.target_selection,
            "time_window_utc": self.time_window_utc,
            "observation_window_utc": self.observation_window_utc,
            "targets": self.targets,
            "metric_signals": [signal.__dict__ for signal in self.metric_signals],
            "log_signals": [signal.__dict__ for signal in self.log_signals],
            "trace_signals": [signal.__dict__ for signal in self.trace_signals],
        }


@dataclass
class TelemetryDocMetadata:
    case_id: str
    doc_id: str
    doc_kind: str
    split: str
    provenance_profile: str
    evaluation_eligibility: str
    target_selection: str
    service: str
    instance_type: str
    instance: list[str]
    source: str
    destination: str
    start_time: str
    end_time: str
    observation_start_time: str
    observation_end_time: str
    service_tokens: list[str]
    pod_tokens: list[str]
    node_tokens: list[str]
    namespace_tokens: list[str]
    metric_signal_count: int
    log_signal_count: int
    trace_signal_count: int
    metric_names: list[str]
    trace_services: list[str]
    trace_operations: list[str]

    def to_json(self) -> dict[str, Any]:
        return self.__dict__


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Build telemetry evidence docs from aiopschallenge2025 parquet.")
    parser.add_argument("--dataset-root", default="aiopschallenge2025", help="Path to aiopschallenge2025 dataset root")
    parser.add_argument("--output-root", default="", help="Output root; defaults to <dataset-root>/baseline")
    parser.add_argument(
        "--split-manifest",
        default="",
        help="Optional build split manifest; defaults to <output-root>/eval/build_split.json if present",
    )
    parser.add_argument("--limit", type=int, default=0, help="Optional limit on processed cases")
    parser.add_argument(
        "--max-metric-signals",
        type=int,
        default=0,
        help="Maximum metric signals per case; 0 uses the extraction-profile default",
    )
    parser.add_argument(
        "--extraction-profile",
        choices=[EXTRACTION_PROFILE_GROUNDTRUTH, EXTRACTION_PROFILE_BLIND],
        default=EXTRACTION_PROFILE_GROUNDTRUTH,
        help="groundtruth_guided narrows by labels; blind scans all entities in the input time window",
    )
    parser.add_argument(
        "--case-id",
        action="append",
        default=[],
        help="Process only this case id; repeat the option to select multiple cases",
    )
    parser.add_argument("--case-manifest", default="", help="Optional aiops2025-recorded-split-v1 manifest")
    parser.add_argument(
        "--case-role",
        choices=["development", "holdout"],
        default="",
        help="Role to select from --case-manifest",
    )
    parser.add_argument(
        "--progress-seconds",
        type=int,
        default=30,
        help="Emit a progress line and refresh telemetry_report.json every N seconds; 0 disables periodic progress",
    )
    return parser.parse_args()


def provenance_for_profile(extraction_profile: str) -> tuple[str, str, str]:
    if extraction_profile == EXTRACTION_PROFILE_BLIND:
        return "recorded_blind", "development_only", "input_time_window_only"
    if extraction_profile == EXTRACTION_PROFILE_GROUNDTRUTH:
        return "recorded_label_assisted", "development_only", "groundtruth_guided"
    raise ValueError(f"unsupported extraction profile: {extraction_profile}")


def load_case_manifest(path: Path, role: str) -> list[str]:
    data = json.loads(path.read_text(encoding="utf-8"))
    if data.get("schema_version") != "aiops2025-recorded-split-v1":
        raise ValueError("case manifest schema_version must be aiops2025-recorded-split-v1")
    if role not in {"development", "holdout"}:
        raise ValueError("case manifest role must be development or holdout")

    development = unique_non_empty(data.get("development_case_ids", []))
    holdout = unique_non_empty(data.get("holdout_case_ids", []))
    if not development or not holdout:
        raise ValueError("case manifest development and holdout ids must be non-empty")
    if len(development) != len(data.get("development_case_ids", [])) or len(holdout) != len(
        data.get("holdout_case_ids", [])
    ):
        raise ValueError("case manifest contains blank or duplicate ids")
    if set(development) & set(holdout):
        raise ValueError("case manifest development and holdout ids must be disjoint")
    return development if role == "development" else holdout


def select_case_ids(available_ids: list[str], requested_ids: list[str], limit: int) -> list[str]:
    selected: list[str] = []
    seen: set[str] = set()
    for value in requested_ids:
        case_id = str(value).strip()
        if not case_id or case_id in seen:
            continue
        seen.add(case_id)
        selected.append(case_id)

    if selected:
        available = set(available_ids)
        unknown = [case_id for case_id in selected if case_id not in available]
        if unknown:
            raise ValueError(f"unknown case ids: {', '.join(unknown)}")
    else:
        selected = list(available_ids)

    if limit > 0:
        selected = selected[:limit]
    return selected


def main() -> None:
    args = parse_args()
    dataset_root = Path(args.dataset_root).resolve()
    output_root = Path(args.output_root).resolve() if args.output_root else (dataset_root / "baseline").resolve()
    split_manifest = Path(args.split_manifest).resolve() if args.split_manifest else (output_root / "eval" / "build_split.json")

    inputs = load_input_cases(dataset_root / "input.json")
    if args.extraction_profile == EXTRACTION_PROFILE_BLIND:
        groundtruth: dict[str, GroundTruthCase] = {}
        available_ids = sorted(inputs)
    else:
        groundtruth = load_groundtruth_cases(dataset_root / "groundtruth.jsonl")
        available_ids = sorted(set(inputs) & set(groundtruth))
    build_ids = load_build_ids(split_manifest) if split_manifest.exists() else None

    docs_dir = output_root / "docs_evidence_telemetry"
    docs_build_dir = output_root / "docs_evidence_telemetry_build"
    telemetry_dir = output_root / "telemetry"
    docs_dir.mkdir(parents=True, exist_ok=True)
    docs_build_dir.mkdir(parents=True, exist_ok=True)
    telemetry_dir.mkdir(parents=True, exist_ok=True)

    requested_case_ids = args.case_id
    case_manifest_path: Path | None = None
    if args.case_manifest:
        if args.case_id:
            raise SystemExit("--case-manifest cannot be combined with --case-id")
        if not args.case_role:
            raise SystemExit("--case-role is required with --case-manifest")
        case_manifest_path = Path(args.case_manifest).resolve()
        try:
            requested_case_ids = load_case_manifest(case_manifest_path, args.case_role)
        except (OSError, ValueError, json.JSONDecodeError) as exc:
            raise SystemExit(str(exc)) from exc
    elif args.case_role:
        raise SystemExit("--case-role requires --case-manifest")

    try:
        all_ids = select_case_ids(available_ids, requested_case_ids, args.limit)
    except ValueError as exc:
        raise SystemExit(str(exc)) from exc

    provenance_profile, evaluation_eligibility, target_selection = provenance_for_profile(args.extraction_profile)
    if args.max_metric_signals < 0:
        raise SystemExit("--max-metric-signals cannot be negative")
    max_metric_signals = args.max_metric_signals
    if max_metric_signals == 0:
        max_metric_signals = (
            DEFAULT_BLIND_MAX_METRIC_SIGNALS
            if args.extraction_profile == EXTRACTION_PROFILE_BLIND
            else DEFAULT_GUIDED_MAX_METRIC_SIGNALS
        )

    summaries: list[TelemetrySummary] = []
    build_summaries: list[TelemetrySummary] = []
    metadata_rows: list[TelemetryDocMetadata] = []
    build_metadata_rows: list[TelemetryDocMetadata] = []
    stats = {
        "status": "running",
        "cases": 0,
        "total_cases": len(all_ids),
        "build_cases": 0,
        "metric_signals": 0,
        "log_signals": 0,
        "trace_signals": 0,
        "empty_cases": 0,
        "selection_mode": "case_manifest" if case_manifest_path else ("explicit_case_ids" if args.case_id else "all_cases"),
        "case_manifest": str(case_manifest_path) if case_manifest_path else "",
        "case_role": args.case_role,
        "extraction_profile": args.extraction_profile,
        "provenance_profile": provenance_profile,
        "evaluation_eligibility": evaluation_eligibility,
        "target_selection": target_selection,
        "max_metric_signals": max_metric_signals,
        "output_root": str(output_root),
    }

    started_at = datetime.now(UTC).isoformat()
    started_monotonic = time.monotonic()
    last_progress = started_monotonic

    def emit_progress(current_case_id: str, force: bool = False) -> None:
        nonlocal last_progress
        now = time.monotonic()
        if not force:
            if args.progress_seconds <= 0:
                return
            if (now - last_progress) < args.progress_seconds:
                return
        elapsed = now - started_monotonic
        last_progress = now
        report = {
            **stats,
            "started_at": started_at,
            "elapsed_seconds": round(elapsed, 1),
            "current_case_id": current_case_id,
            "docs_dir": str(docs_dir),
            "docs_build_dir": str(docs_build_dir),
            "doc_metadata_path": str(telemetry_dir / "doc_metadata.jsonl"),
            "doc_metadata_build_path": str(telemetry_dir / "doc_metadata_build.jsonl"),
            "split_manifest": str(split_manifest) if split_manifest.exists() else "",
        }
        write_json(telemetry_dir / "telemetry_report.json", report)
        print(
            "[progress] "
            + f"{stats['cases']}/{stats['total_cases']} cases "
            + f"(build={stats['build_cases']}, empty={stats['empty_cases']}) "
            + f"signals(metric/log/trace)="
            + f"{stats['metric_signals']}/{stats['log_signals']}/{stats['trace_signals']} "
            + f"current={current_case_id} elapsed={elapsed:.1f}s",
            flush=True,
        )

    for index, case_id in enumerate(all_ids, start=1):
        if args.extraction_profile == EXTRACTION_PROFILE_BLIND:
            context = build_blind_case_context(inputs[case_id])
        else:
            context = build_case_context(inputs[case_id], groundtruth[case_id])
        summary = summarize_case(dataset_root, context, max_metric_signals=max_metric_signals)
        doc_metadata = build_doc_metadata(context, summary, split="all")
        write_text(docs_dir / f"{case_id}.md", render_telemetry_doc(context, summary))
        write_json(docs_dir / f"{case_id}.metadata.json", doc_metadata.to_json())
        summaries.append(summary)
        metadata_rows.append(doc_metadata)
        stats["cases"] += 1
        stats["metric_signals"] += len(summary.metric_signals)
        stats["log_signals"] += len(summary.log_signals)
        stats["trace_signals"] += len(summary.trace_signals)
        if not summary.metric_signals and not summary.log_signals and not summary.trace_signals:
            stats["empty_cases"] += 1

        if build_ids is not None and case_id in build_ids:
            build_doc_metadata_row = build_doc_metadata(context, summary, split="build")
            write_text(docs_build_dir / f"{case_id}.md", render_telemetry_doc(context, summary))
            write_json(docs_build_dir / f"{case_id}.metadata.json", build_doc_metadata_row.to_json())
            build_summaries.append(summary)
            build_metadata_rows.append(build_doc_metadata_row)
            stats["build_cases"] += 1

        emit_progress(case_id, force=index == 1 or index == len(all_ids))

    write_jsonl(telemetry_dir / "case_evidence_summary.jsonl", [item.to_json() for item in summaries])
    write_jsonl(telemetry_dir / "case_evidence_summary_build.jsonl", [item.to_json() for item in build_summaries])
    write_jsonl(telemetry_dir / "doc_metadata.jsonl", [item.to_json() for item in metadata_rows])
    write_jsonl(telemetry_dir / "doc_metadata_build.jsonl", [item.to_json() for item in build_metadata_rows])
    stats["status"] = "completed"
    write_json(
        telemetry_dir / "telemetry_report.json",
        {
            **stats,
            "started_at": started_at,
            "elapsed_seconds": round(time.monotonic() - started_monotonic, 1),
            "docs_dir": str(docs_dir),
            "docs_build_dir": str(docs_build_dir),
            "doc_metadata_path": str(telemetry_dir / "doc_metadata.jsonl"),
            "doc_metadata_build_path": str(telemetry_dir / "doc_metadata_build.jsonl"),
            "split_manifest": str(split_manifest) if split_manifest.exists() else "",
        },
    )
    print(json.dumps(stats, ensure_ascii=False, indent=2))


def load_input_cases(path: Path) -> dict[str, InputCase]:
    items = json.loads(path.read_text(encoding="utf-8"))
    out: dict[str, InputCase] = {}
    for item in items:
        case_id = str(item.get("uuid", "")).strip()
        if not case_id:
            continue
        out[case_id] = InputCase(
            uuid=case_id,
            anomaly_description=str(item.get("Anomaly Description", "")).strip(),
        )
    return out


def load_groundtruth_cases(path: Path) -> dict[str, GroundTruthCase]:
    out: dict[str, GroundTruthCase] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        if not line.strip():
            continue
        item = json.loads(line)
        case_id = str(item.get("uuid", "")).strip()
        if not case_id:
            continue
        instance = item.get("instance", [])
        if isinstance(instance, str):
            instance_list = [instance]
        else:
            instance_list = [str(v).strip() for v in instance if str(v).strip()]
        out[case_id] = GroundTruthCase(
            uuid=case_id,
            fault_category=str(item.get("fault_category", "")).strip(),
            fault_type=str(item.get("fault_type", "")).strip(),
            instance_type=str(item.get("instance_type", "")).strip(),
            service=str(item.get("service", "")).strip(),
            instance=instance_list,
            source=str(item.get("source", "")).strip(),
            destination=str(item.get("destination", "")).strip(),
            start_time=str(item.get("start_time", "")).strip(),
            end_time=str(item.get("end_time", "")).strip(),
        )
    return out


def load_build_ids(path: Path) -> set[str]:
    data = json.loads(path.read_text(encoding="utf-8"))
    return {str(item).strip() for item in data.get("build_case_ids", []) if str(item).strip()}


def extract_input_time_window(input_case: InputCase) -> tuple[str, str]:
    timestamps = UTC_TIMESTAMP_RE.findall(input_case.anomaly_description)
    if len(timestamps) != 2:
        raise ValueError(f"case {input_case.uuid} must contain exactly two UTC timestamps")
    start = parse_utc(timestamps[0])
    end = parse_utc(timestamps[1])
    if end < start:
        raise ValueError(f"case {input_case.uuid} has an inverted UTC time window")
    return timestamps[0], timestamps[1]


def build_blind_case_context(input_case: InputCase) -> CaseContext:
    start_time, end_time = extract_input_time_window(input_case)
    return build_case_context(
        input_case,
        GroundTruthCase(
            uuid=input_case.uuid,
            fault_category="",
            fault_type="",
            instance_type="",
            service="",
            instance=[],
            source="",
            destination="",
            start_time=start_time,
            end_time=end_time,
        ),
        extraction_profile=EXTRACTION_PROFILE_BLIND,
    )


def build_case_context(
    input_case: InputCase,
    groundtruth: GroundTruthCase,
    extraction_profile: str = EXTRACTION_PROFILE_GROUNDTRUTH,
) -> CaseContext:
    provenance_for_profile(extraction_profile)
    start_utc = parse_utc(groundtruth.start_time)
    end_utc = parse_utc(groundtruth.end_time)
    services = unique_non_empty([groundtruth.service, groundtruth.source, groundtruth.destination])
    pod_tokens: list[str] = []
    node_tokens: list[str] = []
    entity_tokens: list[str] = []

    for value in groundtruth.instance:
        if looks_like_node(value):
            node_tokens.append(value.lower())
            entity_tokens.append(value.lower())
        else:
            lowered = value.lower()
            pod_tokens.append(lowered)
            entity_tokens.append(lowered)
            base = strip_pod_suffix(lowered)
            if base != lowered:
                services.append(base)
                entity_tokens.append(base)

    entity_tokens.extend(item.lower() for item in services)
    namespace_tokens = derive_namespaces(services, groundtruth.instance)
    if groundtruth.instance_type.lower() == "node" and groundtruth.instance:
        node_tokens.extend(item.lower() for item in groundtruth.instance)

    return CaseContext(
        input_case=input_case,
        groundtruth=groundtruth,
        start_utc=start_utc,
        end_utc=end_utc,
        start_local=start_utc.astimezone(BEIJING),
        end_local=end_utc.astimezone(BEIJING),
        service_tokens=unique_non_empty(item.lower() for item in services),
        pod_tokens=unique_non_empty(item.lower() for item in pod_tokens),
        node_tokens=unique_non_empty(item.lower() for item in node_tokens),
        entity_tokens=unique_non_empty(item.lower() for item in entity_tokens),
        file_tokens=unique_non_empty(item.lower() for item in services + pod_tokens + node_tokens),
        namespace_tokens=namespace_tokens,
        extraction_profile=extraction_profile,
    )


def parse_utc(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(UTC)


def summarize_case(dataset_root: Path, case: CaseContext, max_metric_signals: int = 0) -> TelemetrySummary:
    observation_end = observation_end_utc(case)
    provenance_profile, evaluation_eligibility, target_selection = provenance_for_profile(case.extraction_profile)
    targets: dict[str, Any] = {}
    if case.extraction_profile == EXTRACTION_PROFILE_GROUNDTRUTH:
        targets = {
            "instance_type": case.groundtruth.instance_type,
            "service": case.groundtruth.service,
            "instance": case.groundtruth.instance,
            "source": case.groundtruth.source,
            "destination": case.groundtruth.destination,
        }
    summary = TelemetrySummary(
        case_id=case.groundtruth.uuid,
        provenance_profile=provenance_profile,
        evaluation_eligibility=evaluation_eligibility,
        target_selection=target_selection,
        time_window_utc=f"{case.groundtruth.start_time} -> {case.groundtruth.end_time}",
        observation_window_utc=f"{iso_z(case.start_utc)} -> {iso_z(observation_end)}",
        targets=targets,
    )

    if max_metric_signals <= 0:
        max_metric_signals = (
            DEFAULT_BLIND_MAX_METRIC_SIGNALS
            if case.extraction_profile == EXTRACTION_PROFILE_BLIND
            else DEFAULT_GUIDED_MAX_METRIC_SIGNALS
        )
    summary.metric_signals = collect_metric_signals(dataset_root, case, max_metric_signals)
    summary.log_signals = collect_log_signals(dataset_root, case)
    summary.trace_signals = collect_trace_signals(dataset_root, case)
    return summary


def collect_metric_signals(dataset_root: Path, case: CaseContext, max_signals: int) -> list[MetricSignal]:
    signals: list[MetricSignal] = []
    for path in candidate_metric_files(dataset_root, case):
        signal_rows = metric_signals_for_file(path, case)
        if not signal_rows:
            continue
        signals.extend(signal_rows[:2])
    return select_metric_signals(signals, max_signals)


def select_metric_signals(signals: list[MetricSignal], limit: int) -> list[MetricSignal]:
    if limit <= 0:
        return []
    ranked = sorted(signals, key=lambda item: (-item.score, item.entity, item.metric, item.source_file))
    selected: list[MetricSignal] = []
    selected_keys: set[tuple[str, str, str]] = set()

    for dimension in (lambda item: item.entity, lambda item: item.metric):
        dimension_values: set[str] = set()
        for signal in ranked:
            key = (signal.entity, signal.metric, signal.source_file)
            value = dimension(signal)
            if key in selected_keys or value in dimension_values:
                continue
            selected.append(signal)
            selected_keys.add(key)
            dimension_values.add(value)
            if len(selected) >= limit:
                break
        if len(selected) >= limit:
            break

    if len(selected) < limit:
        for signal in ranked:
            key = (signal.entity, signal.metric, signal.source_file)
            if key in selected_keys:
                continue
            selected.append(signal)
            selected_keys.add(key)
            if len(selected) >= limit:
                break
    return ensure_metric_source_family_coverage(selected, ranked)


def metric_source_family(source_file: str) -> str:
    name = source_file.lower()
    for family in ("infra_pod", "infra_node", "infra_tidb", "infra_tikv", "infra_pd"):
        if name.startswith(family):
            return family
    if name.startswith(("service_", "pod_")):
        return "apm"
    if name.startswith("infra_"):
        return "infra_other"
    return "other"


def ensure_metric_source_family_coverage(
    selected: list[MetricSignal], ranked: list[MetricSignal]
) -> list[MetricSignal]:
    if not selected:
        return selected
    required_families = ("apm", "infra_pod", "infra_node", "infra_tidb", "infra_tikv", "infra_pd")
    available = {metric_source_family(signal.source_file) for signal in ranked}
    represented = {metric_source_family(signal.source_file) for signal in selected}
    for family in required_families:
        if family not in available or family in represented:
            continue
        candidate = next(signal for signal in ranked if metric_source_family(signal.source_file) == family)
        family_counts = Counter(metric_source_family(signal.source_file) for signal in selected)
        replace_index = next(
            (
                index
                for index in range(len(selected) - 1, -1, -1)
                if family_counts[metric_source_family(selected[index].source_file)] > 1
            ),
            len(selected) - 1,
        )
        selected[replace_index] = candidate
        represented = {metric_source_family(signal.source_file) for signal in selected}
    return selected


def collect_log_signals(dataset_root: Path, case: CaseContext) -> list[LogSignal]:
    counter: dict[tuple[str, str, str], list[str]] = defaultdict(list)
    for path in candidate_hourly_files(dataset_root, case, "log"):
        table = pq.read_table(path, columns=["k8_pod", "k8_node_name", LOG_TIMESTAMP_COL, "message"])
        for row in table.to_pylist():
            timestamp = str(row.get(LOG_TIMESTAMP_COL, ""))
            if not in_time_window(timestamp, case.start_utc, observation_end_utc(case)):
                continue
            pod = str(row.get("k8_pod", "") or "")
            node = str(row.get("k8_node_name", "") or "")
            message = str(row.get("message", "") or "")
            if case.extraction_profile != EXTRACTION_PROFILE_BLIND and not is_relevant_log_row(case, pod, node, message):
                continue
            pattern = normalize_log_message(message)
            key = (pod, node, pattern)
            counter[key].append(timestamp)

    signals: list[LogSignal] = []
    for (pod, node, pattern), timestamps in counter.items():
        timestamps.sort()
        signals.append(
            LogSignal(
                pod=pod,
                node=node,
                pattern=pattern,
                count=len(timestamps),
                first_seen=timestamps[0],
                last_seen=timestamps[-1],
                signal_score=log_pattern_signal_score(pattern),
            )
        )
    informative = [item for item in signals if item.signal_score > 0]
    pool = informative if informative else signals
    pool.sort(key=lambda item: (-item.signal_score, -item.count, item.pattern))
    return pool[:MAX_LOG_SIGNALS]


def collect_trace_signals(dataset_root: Path, case: CaseContext) -> list[TraceSignal]:
    grouped: dict[tuple[str, str, str], list[tuple[float, bool]]] = defaultdict(list)
    for path in candidate_hourly_files(dataset_root, case, "trace"):
        table = pq.read_table(path, columns=["operationName", TRACE_TIMESTAMP_COL, "duration", "tags", "process"])
        for row in table.to_pylist():
            ts_millis = int(row.get(TRACE_TIMESTAMP_COL, 0) or 0)
            if not in_trace_window(ts_millis, case.start_utc, observation_end_utc(case)):
                continue
            process = row.get("process") or {}
            service = str(process.get("serviceName", "") or "")
            tags = flatten_tag_list(row.get("tags") or [])
            peer = str(tags.get("rpc.service") or tags.get("net.peer.ip") or tags.get("net.peer.name") or "")
            operation = str(row.get("operationName", "") or "")
            if case.extraction_profile != EXTRACTION_PROFILE_BLIND and not is_relevant_trace_row(
                case, service, operation, peer, tags
            ):
                continue
            status = str(tags.get("status.code") or tags.get("otel.status_code") or "")
            is_error = status not in {"", "0", "STATUS_CODE_UNSET", "UNSET"}
            grouped[(service, operation, peer)].append((float(row.get("duration", 0) or 0) / 1000.0, is_error))

    signals: list[TraceSignal] = []
    for (service, operation, peer), items in grouped.items():
        durations = sorted(duration for duration, _ in items)
        if not durations:
            continue
        error_count = sum(1 for _, is_error in items if is_error)
        avg_duration = sum(durations) / len(durations)
        p95 = durations[min(len(durations) - 1, max(0, math.ceil(len(durations) * 0.95) - 1))]
        score = (error_count * 1000.0) + avg_duration * math.log(len(durations) + 1)
        signals.append(
            TraceSignal(
                service=service,
                operation=operation,
                peer=peer,
                count=len(durations),
                error_count=error_count,
                avg_duration_ms=avg_duration,
                p95_duration_ms=p95,
                score=score,
            )
        )
    signals.sort(key=lambda item: item.score, reverse=True)
    return signals[:MAX_TRACE_SIGNALS]


def candidate_metric_files(dataset_root: Path, case: CaseContext) -> list[Path]:
    paths: list[Path] = []
    for date_dir in iter_date_dirs(dataset_root, case):
        if case.extraction_profile == EXTRACTION_PROFILE_BLIND:
            paths.extend(sorted((date_dir / "metric-parquet").rglob("*.parquet")))
            continue

        date_label = date_dir.name
        apm_dir = date_dir / "metric-parquet" / "apm"
        if case.namespace_tokens and "hipstershop" in case.namespace_tokens:
            namespace_file = apm_dir / f"pod_ns_hipstershop_{date_label}.parquet"
            if namespace_file.exists():
                paths.append(namespace_file)

        for service in case.service_tokens:
            service_file = apm_dir / "service" / f"service_{service}_{date_label}.parquet"
            if service_file.exists():
                paths.append(service_file)

        pod_dir = apm_dir / "pod"
        if pod_dir.exists():
            for token in case.pod_tokens + case.service_tokens:
                for match in pod_dir.glob(f"pod_{token}*_{date_label}.parquet"):
                    paths.append(match)

        infra_dir = date_dir / "metric-parquet" / "infra"
        if case.node_tokens:
            paths.extend(sorted((infra_dir / "infra_node").glob("*.parquet")))
        else:
            paths.extend(sorted((infra_dir / "infra_pod").glob("*.parquet")))

        if needs_tidb_metrics(case):
            paths.extend(sorted((infra_dir / "infra_tidb").glob("*.parquet")))
            paths.extend(sorted((date_dir / "metric-parquet" / "other").glob("*.parquet")))

    deduped: list[Path] = []
    seen: set[Path] = set()
    for path in paths:
        if path.exists() and path not in seen:
            deduped.append(path)
            seen.add(path)
    return deduped


def candidate_hourly_files(dataset_root: Path, case: CaseContext, kind: str) -> list[Path]:
    out: list[Path] = []
    observation_end_local = observation_end_utc(case).astimezone(BEIJING)
    for hour in iter_local_hours(case.start_local, observation_end_local):
        date_dir = dataset_root / "extracted" / hour.strftime("%Y-%m-%d")
        if kind == "log":
            path = date_dir / "log-parquet" / f"log_filebeat-server_{hour.strftime('%Y-%m-%d_%H')}-00-00.parquet"
        elif kind == "trace":
            path = date_dir / "trace-parquet" / f"trace_jaeger-span_{hour.strftime('%Y-%m-%d_%H')}-00-00.parquet"
        else:
            raise ValueError(f"unsupported hourly kind: {kind}")
        if path.exists():
            out.append(path)
    return out


def metric_signals_for_file(path: Path, case: CaseContext) -> list[MetricSignal]:
    table = pq.read_table(path)
    df = table.to_pandas()
    if "time" not in df.columns:
        return []

    incident_start = metric_incident_start_utc(case)
    observation_end = observation_end_utc(case)
    baseline_start = incident_start - timedelta(minutes=30)
    df["time"] = df["time"].astype(str)
    df = df[(df["time"] >= iso_z(baseline_start)) & (df["time"] <= iso_z(observation_end))]
    if df.empty:
        return []

    signals: list[MetricSignal] = []
    if case.extraction_profile == EXTRACTION_PROFILE_BLIND:
        frames = metric_entity_frames(df, path)
    else:
        filtered = filter_metric_rows(df, case, path)
        if filtered.empty:
            return []
        frames = [(metric_entity_hint(filtered, path), filtered)]

    for entity, frame in frames:
        signals.extend(
            metric_signals_for_frame(
                table.schema,
                frame,
                path,
                entity,
                baseline_start,
                incident_start,
                observation_end,
            )
        )

    signals.sort(key=lambda item: (-item.score, item.entity, item.metric))
    return signals


def metric_entity_frames(df: pd.DataFrame, path: Path) -> list[tuple[str, pd.DataFrame]]:
    for column in ("pod", "object_id", "kubernetes_node", "instance", "namespace", "object_type"):
        if column not in df.columns:
            continue
        values = df[column].fillna("").astype(str)
        valid = ~values.str.strip().str.lower().isin({"", "null", "none", "nan"})
        entities = unique_non_empty(values[valid].tolist())
        if entities:
            return [(entity, df[values == entity]) for entity in entities]
    return [(path.stem, df)]


def metric_signals_for_frame(
    schema: Any,
    df: pd.DataFrame,
    path: Path,
    entity: str,
    baseline_start: datetime,
    incident_start: datetime,
    observation_end: datetime,
) -> list[MetricSignal]:
    baseline_df = df[(df["time"] >= iso_z(baseline_start)) & (df["time"] < iso_z(incident_start))]
    incident_df = df[(df["time"] >= iso_z(incident_start)) & (df["time"] <= iso_z(observation_end))]
    if incident_df.empty:
        return []

    signals: list[MetricSignal] = []
    for column in numeric_metric_columns(schema):
        baseline_values = pd.to_numeric(baseline_df[column], errors="coerce").dropna()
        incident_values = pd.to_numeric(incident_df[column], errors="coerce").dropna()
        if incident_values.empty:
            continue
        baseline_mean = float(baseline_values.mean()) if not baseline_values.empty else 0.0
        incident_mean = float(incident_values.mean())
        incident_max = float(incident_values.max())
        delta = incident_mean - baseline_mean
        if abs(baseline_mean) < 1e-9:
            score = abs(incident_mean)
        else:
            score = abs(delta / baseline_mean)
        if score < 0.1 and abs(delta) < 0.5:
            continue
        signals.append(
            MetricSignal(
                source_file=path.name,
                entity=entity,
                metric=column,
                baseline_mean=baseline_mean,
                incident_mean=incident_mean,
                incident_max=incident_max,
                score=score,
                sample_count=int(incident_values.shape[0]),
            )
        )

    return signals


def filter_metric_rows(df: pd.DataFrame, case: CaseContext, path: Path) -> pd.DataFrame:
    if case.node_tokens:
        mask = series_contains_any(df.get("kubernetes_node"), case.node_tokens, index=df.index) | series_contains_any(
            df.get("instance"), case.node_tokens, index=df.index
        )
        return df[mask] if mask.any() else df

    strong_mask = (
        series_contains_any(df.get("object_id"), case.service_tokens, index=df.index)
        | series_contains_any(df.get("pod"), case.pod_tokens + case.service_tokens, index=df.index)
        | series_contains_any(df.get("instance"), case.service_tokens + case.pod_tokens, index=df.index)
        | series_contains_any(df.get("object_type"), case.service_tokens, index=df.index)
    )
    if strong_mask.any():
        return df[strong_mask]

    if "pod_ns_" in path.name.lower():
        namespace_mask = series_contains_any(df.get("object_id"), case.namespace_tokens, index=df.index) | series_contains_any(
            df.get("namespace"), case.namespace_tokens, index=df.index
        )
        if namespace_mask.any():
            return df[namespace_mask]

    if needs_tidb_metrics(case) and path.parent.name in {"infra_tidb", "other"}:
        namespace_mask = series_contains_any(df.get("namespace"), case.namespace_tokens, index=df.index)
        return df[namespace_mask] if namespace_mask.any() else df

    lowered_name = path.name.lower()
    if any(token in lowered_name for token in case.file_tokens):
        return df
    return df.iloc[0:0]


def metric_entity_hint(df: pd.DataFrame, path: Path) -> str:
    for column in ("object_id", "pod", "kubernetes_node", "namespace", "object_type"):
        if column in df.columns:
            values = unique_non_empty(str(item) for item in df[column].tolist()[:5])
            if values:
                return ",".join(values[:3])
    return path.stem


def numeric_metric_columns(schema: Any) -> list[str]:
    out: list[str] = []
    for field in schema:
        if field.name in SKIP_METRIC_COLUMNS:
            continue
        if str(field.type) in NUMERIC_TYPES:
            out.append(field.name)
    return out


def flatten_tag_list(items: Iterable[dict[str, Any]]) -> dict[str, str]:
    out: dict[str, str] = {}
    for item in items:
        key = str(item.get("key", "") or "")
        if not key:
            continue
        out[key] = str(item.get("value", "") or "")
    return out


def is_relevant_log_row(case: CaseContext, pod: str, node: str, message: str) -> bool:
    haystack = " ".join([pod.lower(), node.lower(), message.lower()])
    return any(token in haystack for token in case.entity_tokens)


def is_relevant_trace_row(
    case: CaseContext,
    service: str,
    operation: str,
    peer: str,
    tags: dict[str, str],
) -> bool:
    values = [service.lower(), operation.lower(), peer.lower()]
    values.extend(str(value).lower() for value in tags.values())
    haystack = " ".join(values)
    return any(token in haystack for token in case.entity_tokens)


def normalize_log_message(message: str) -> str:
    message = message.strip()
    if message.startswith("{") and message.endswith("}"):
        try:
            parsed = json.loads(message)
        except json.JSONDecodeError:
            pass
        else:
            parts = []
            for key in ("message", "severity", "http.resp.status", "error", "exception", "grpc.code"):
                if key in parsed:
                    parts.append(f"{key}={parsed[key]}")
            if parts:
                message = "json " + " ".join(str(item) for item in parts)
    message = ANSI_RE.sub("", message)
    message = UUID_RE.sub("<uuid>", message)
    message = HEX_RE.sub("<hex>", message)
    message = IP_RE.sub("<ip>", message)
    message = NUMBER_RE.sub("<n>", message)
    message = WHITESPACE_RE.sub(" ", message)
    return message[:220]


def log_pattern_signal_score(pattern: str) -> int:
    lowered = pattern.lower()
    positive_terms = [
        "error",
        "exception",
        "timeout",
        "unavailable",
        "canceled",
        "cancelled",
        "refused",
        "failed",
        "failure",
        "panic",
        "deadline",
        "warning",
    ]
    negative_terms = [
        "request starting",
        "request finished",
        "executed endpoint",
        "executing endpoint",
        "hosting.diagnostics",
        "endpointmiddleware",
    ]
    score = sum(2 for term in positive_terms if term in lowered)
    if re.search(r"(?<!\d)500(?!\d)", lowered):
        score += 2
    if re.search(r"(?<!\d)503(?!\d)", lowered):
        score += 2
    score -= sum(1 for term in negative_terms if term in lowered)
    return score


def render_telemetry_doc(case: CaseContext, summary: TelemetrySummary) -> str:
    lines = [
        "# Telemetry Evidence Case",
        "",
        f"- case_id: {case.groundtruth.uuid}",
        f"- provenance_profile: {summary.provenance_profile}",
        f"- evaluation_eligibility: {summary.evaluation_eligibility}",
        f"- target_selection: {summary.target_selection}",
        f"- time_window_utc: {case.groundtruth.start_time} -> {case.groundtruth.end_time}",
        f"- observation_window_utc: {summary.observation_window_utc}",
    ]
    if case.extraction_profile == EXTRACTION_PROFILE_BLIND:
        lines.append("- target_scope: all_entities_in_window")
    else:
        lines.extend(
            [
                f"- instance_type: {case.groundtruth.instance_type}",
                f"- service: {case.groundtruth.service or 'unknown'}",
                f"- instance: {', '.join(case.groundtruth.instance) if case.groundtruth.instance else 'unknown'}",
            ]
        )
        if case.groundtruth.source or case.groundtruth.destination:
            lines.append(f"- path: {case.groundtruth.source or 'unknown'} -> {case.groundtruth.destination or 'unknown'}")

    lines.extend(
        [
            "",
            "## Anomaly Description",
            "",
            case.input_case.anomaly_description or "No anomaly description provided.",
            "",
            "## Metric Signals",
            "",
        ]
    )
    if summary.metric_signals:
        for signal in summary.metric_signals:
            lines.append(
                "- "
                + f"{signal.metric} [{signal.entity}] from {signal.source_file}: "
                + f"baseline_mean={signal.baseline_mean:.2f}, "
                + f"incident_mean={signal.incident_mean:.2f}, "
                + f"incident_max={signal.incident_max:.2f}, "
                + f"score={signal.score:.2f}, samples={signal.sample_count}"
            )
    else:
        lines.append("- no metric signal extracted")

    lines.extend(["", "## Log Signals", ""])
    if summary.log_signals:
        for signal in summary.log_signals:
            lines.append(
                "- "
                + f"{signal.pod or 'unknown-pod'} @ {signal.node or 'unknown-node'}: "
                + f"count={signal.count}, window={signal.first_seen} -> {signal.last_seen}, pattern={signal.pattern}"
            )
    else:
        lines.append("- no log signal extracted")

    lines.extend(["", "## Trace Signals", ""])
    if summary.trace_signals:
        for signal in summary.trace_signals:
            lines.append(
                "- "
                + f"{signal.service or 'unknown-service'} | {signal.operation}: "
                + f"peer={signal.peer or 'unknown'}, count={signal.count}, "
                + f"errors={signal.error_count}, avg_ms={signal.avg_duration_ms:.2f}, "
                + f"p95_ms={signal.p95_duration_ms:.2f}"
            )
    else:
        lines.append("- no trace signal extracted")

    lines.extend(["", "## Retrieval Keywords", ""])
    keywords = build_retrieval_keywords(case, summary)
    lines.append(" ".join(keywords) if keywords else "none")
    lines.append("")
    return "\n".join(lines)


def build_doc_metadata(case: CaseContext, summary: TelemetrySummary, split: str) -> TelemetryDocMetadata:
    return TelemetryDocMetadata(
        case_id=case.groundtruth.uuid,
        doc_id=case.groundtruth.uuid,
        doc_kind="telemetry_evidence",
        split=split,
        provenance_profile=summary.provenance_profile,
        evaluation_eligibility=summary.evaluation_eligibility,
        target_selection=summary.target_selection,
        service=case.groundtruth.service or "unknown",
        instance_type=case.groundtruth.instance_type or "unknown",
        instance=list(case.groundtruth.instance),
        source=case.groundtruth.source,
        destination=case.groundtruth.destination,
        start_time=case.groundtruth.start_time,
        end_time=case.groundtruth.end_time,
        observation_start_time=iso_z(case.start_utc),
        observation_end_time=iso_z(observation_end_utc(case)),
        service_tokens=list(case.service_tokens),
        pod_tokens=list(case.pod_tokens),
        node_tokens=list(case.node_tokens),
        namespace_tokens=list(case.namespace_tokens),
        metric_signal_count=len(summary.metric_signals),
        log_signal_count=len(summary.log_signals),
        trace_signal_count=len(summary.trace_signals),
        metric_names=unique_non_empty(signal.metric for signal in summary.metric_signals[:8]),
        trace_services=unique_non_empty(signal.service for signal in summary.trace_signals[:8] if signal.service),
        trace_operations=unique_non_empty(signal.operation for signal in summary.trace_signals[:8] if signal.operation),
    )


def build_retrieval_keywords(case: CaseContext, summary: TelemetrySummary) -> list[str]:
    keywords = []
    keywords.extend(case.service_tokens)
    keywords.extend(case.node_tokens)
    keywords.extend(case.pod_tokens)
    keywords.extend(signal.entity for signal in summary.metric_signals[:5] if signal.entity)
    keywords.extend(signal.metric for signal in summary.metric_signals[:5])
    keywords.extend(signal.service for signal in summary.trace_signals[:3] if signal.service)
    keywords.extend(signal.operation for signal in summary.trace_signals[:2] if signal.operation)
    for signal in summary.log_signals[:3]:
        if signal.signal_score <= 0:
            continue
        keywords.extend(extract_pattern_keywords(signal.pattern))
    return unique_non_empty(keywords)


def extract_pattern_keywords(pattern: str) -> list[str]:
    tokens = []
    for item in re.split(r"[^a-zA-Z0-9_.:/-]+", pattern):
        item = item.strip().lower()
        if len(item) < 3 or item.startswith("<") or item in KEYWORD_STOPWORDS:
            continue
        tokens.append(item)
    return tokens[:6]


def iter_date_dirs(dataset_root: Path, case: CaseContext) -> list[Path]:
    out = []
    current = case.start_local.date()
    end = observation_end_utc(case).astimezone(BEIJING).date()
    while current <= end:
        path = dataset_root / "extracted" / current.isoformat()
        if path.exists():
            out.append(path)
        current += timedelta(days=1)
    return out


def iter_local_hours(start_local: datetime, end_local: datetime) -> list[datetime]:
    current = start_local.replace(minute=0, second=0, microsecond=0)
    end = end_local.replace(minute=0, second=0, microsecond=0)
    out = []
    while current <= end:
        out.append(current)
        current += timedelta(hours=1)
    return out


def iso_z(value: datetime) -> str:
    return value.astimezone(UTC).isoformat().replace("+00:00", "Z")


def observation_end_utc(case: CaseContext) -> datetime:
    return max(case.end_utc, case.start_utc + MIN_OBSERVATION_WINDOW)


def metric_incident_start_utc(case: CaseContext) -> datetime:
    return case.start_utc.replace(second=0, microsecond=0)


def in_time_window(timestamp: str, start_utc: datetime, end_utc: datetime) -> bool:
    start_iso = iso_z(start_utc)
    end_iso = iso_z(end_utc)
    return bool(timestamp) and start_iso <= timestamp <= end_iso


def in_trace_window(start_time_millis: int, start_utc: datetime, end_utc: datetime) -> bool:
    start_ms = int(start_utc.timestamp() * 1000)
    end_ms = int(end_utc.timestamp() * 1000)
    return start_ms <= start_time_millis <= end_ms


def derive_namespaces(services: Iterable[str], instances: Iterable[str]) -> list[str]:
    tokens = {item.lower() for item in services if item}
    tokens.update(item.lower() for item in instances if item)
    if any("tidb" in item or item == "pd" for item in tokens):
        return ["tidb"]
    if any(item in HIPSTERSHOP_SERVICES for item in tokens):
        return ["hipstershop"]
    return []


def strip_pod_suffix(value: str) -> str:
    match = POD_SUFFIX_RE.match(value)
    if match:
        return match.group("base")
    return value


def looks_like_node(value: str) -> bool:
    return value.lower().startswith("aiops-k8s-")


def needs_tidb_metrics(case: CaseContext) -> bool:
    lowered = " ".join(case.entity_tokens)
    return any(token in lowered for token in TIDB_HINTS)


def series_contains_any(series: Any, tokens: list[str], index: Any | None = None) -> pd.Series:
    if index is None:
        index = pd.RangeIndex(0)
    if series is None or not tokens:
        return pd.Series(False, index=index)
    data = pd.Series(series).fillna("").astype(str).str.lower()
    if data.empty:
        return pd.Series(False, index=index)
    if len(data.index) != len(index):
        data.index = index[: len(data.index)]
    mask = pd.Series(False, index=data.index)
    for token in tokens:
        if not token:
            continue
        mask = mask | data.str.contains(re.escape(token), regex=True)
    return mask


def unique_non_empty(values: Iterable[str]) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        item = str(value).strip()
        if not item or item.lower() == "null":
            continue
        if item in seen:
            continue
        seen.add(item)
        out.append(item)
    return out


def write_text(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8")


def write_json(path: Path, data: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, ensure_ascii=False) + "\n")


if __name__ == "__main__":
    main()
