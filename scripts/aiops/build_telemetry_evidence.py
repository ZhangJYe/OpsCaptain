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
MAX_METRIC_SIGNALS = 8
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
    time_window_utc: str
    targets: dict[str, Any]
    metric_signals: list[MetricSignal] = field(default_factory=list)
    log_signals: list[LogSignal] = field(default_factory=list)
    trace_signals: list[TraceSignal] = field(default_factory=list)

    def to_json(self) -> dict[str, Any]:
        return {
            "case_id": self.case_id,
            "time_window_utc": self.time_window_utc,
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
    service: str
    instance_type: str
    instance: list[str]
    source: str
    destination: str
    start_time: str
    end_time: str
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
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    dataset_root = Path(args.dataset_root).resolve()
    output_root = Path(args.output_root).resolve() if args.output_root else (dataset_root / "baseline").resolve()
    split_manifest = Path(args.split_manifest).resolve() if args.split_manifest else (output_root / "eval" / "build_split.json")

    inputs = load_input_cases(dataset_root / "input.json")
    groundtruth = load_groundtruth_cases(dataset_root / "groundtruth.jsonl")
    build_ids = load_build_ids(split_manifest) if split_manifest.exists() else None

    docs_dir = output_root / "docs_evidence_telemetry"
    docs_build_dir = output_root / "docs_evidence_telemetry_build"
    telemetry_dir = output_root / "telemetry"
    docs_dir.mkdir(parents=True, exist_ok=True)
    docs_build_dir.mkdir(parents=True, exist_ok=True)
    telemetry_dir.mkdir(parents=True, exist_ok=True)

    all_ids = sorted(set(inputs) & set(groundtruth))
    if args.limit > 0:
        all_ids = all_ids[: args.limit]

    summaries: list[TelemetrySummary] = []
    build_summaries: list[TelemetrySummary] = []
    metadata_rows: list[TelemetryDocMetadata] = []
    build_metadata_rows: list[TelemetryDocMetadata] = []
    stats = {
        "cases": 0,
        "build_cases": 0,
        "metric_signals": 0,
        "log_signals": 0,
        "trace_signals": 0,
        "empty_cases": 0,
        "output_root": str(output_root),
    }

    for case_id in all_ids:
        context = build_case_context(inputs[case_id], groundtruth[case_id])
        summary = summarize_case(dataset_root, context)
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

    write_jsonl(telemetry_dir / "case_evidence_summary.jsonl", [item.to_json() for item in summaries])
    write_jsonl(telemetry_dir / "case_evidence_summary_build.jsonl", [item.to_json() for item in build_summaries])
    write_jsonl(telemetry_dir / "doc_metadata.jsonl", [item.to_json() for item in metadata_rows])
    write_jsonl(telemetry_dir / "doc_metadata_build.jsonl", [item.to_json() for item in build_metadata_rows])
    write_json(
        telemetry_dir / "telemetry_report.json",
        {
            **stats,
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


def build_case_context(input_case: InputCase, groundtruth: GroundTruthCase) -> CaseContext:
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
    )


def parse_utc(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(UTC)


def summarize_case(dataset_root: Path, case: CaseContext) -> TelemetrySummary:
    summary = TelemetrySummary(
        case_id=case.groundtruth.uuid,
        time_window_utc=f"{case.groundtruth.start_time} -> {case.groundtruth.end_time}",
        targets={
            "instance_type": case.groundtruth.instance_type,
            "service": case.groundtruth.service,
            "instance": case.groundtruth.instance,
            "source": case.groundtruth.source,
            "destination": case.groundtruth.destination,
        },
    )

    summary.metric_signals = collect_metric_signals(dataset_root, case)
    summary.log_signals = collect_log_signals(dataset_root, case)
    summary.trace_signals = collect_trace_signals(dataset_root, case)
    return summary


def collect_metric_signals(dataset_root: Path, case: CaseContext) -> list[MetricSignal]:
    signals: list[MetricSignal] = []
    for path in candidate_metric_files(dataset_root, case):
        signal_rows = metric_signals_for_file(path, case)
        if not signal_rows:
            continue
        signals.extend(signal_rows[:2])
    signals.sort(key=lambda item: item.score, reverse=True)
    return signals[:MAX_METRIC_SIGNALS]


def collect_log_signals(dataset_root: Path, case: CaseContext) -> list[LogSignal]:
    counter: dict[tuple[str, str, str], list[str]] = defaultdict(list)
    for path in candidate_hourly_files(dataset_root, case, "log"):
        table = pq.read_table(path, columns=["k8_pod", "k8_node_name", LOG_TIMESTAMP_COL, "message"])
        for row in table.to_pylist():
            timestamp = str(row.get(LOG_TIMESTAMP_COL, ""))
            if not in_time_window(timestamp, case.start_utc, case.end_utc):
                continue
            pod = str(row.get("k8_pod", "") or "")
            node = str(row.get("k8_node_name", "") or "")
            message = str(row.get("message", "") or "")
            if not is_relevant_log_row(case, pod, node, message):
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
            if not in_trace_window(ts_millis, case.start_utc, case.end_utc):
                continue
            process = row.get("process") or {}
            service = str(process.get("serviceName", "") or "")
            tags = flatten_tag_list(row.get("tags") or [])
            peer = str(tags.get("rpc.service") or tags.get("net.peer.ip") or tags.get("net.peer.name") or "")
            operation = str(row.get("operationName", "") or "")
            if not is_relevant_trace_row(case, service, operation, peer, tags):
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
    for hour in iter_local_hours(case.start_local, case.end_local):
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

    start = case.start_utc
    end = case.end_utc
    baseline_start = start - timedelta(minutes=30)
    df["time"] = df["time"].astype(str)
    df = df[(df["time"] >= iso_z(baseline_start)) & (df["time"] <= iso_z(end))]
    if df.empty:
        return []

    df = filter_metric_rows(df, case, path)
    if df.empty:
        return []

    baseline_df = df[(df["time"] >= iso_z(baseline_start)) & (df["time"] < iso_z(start))]
    incident_df = df[(df["time"] >= iso_z(start)) & (df["time"] <= iso_z(end))]
    if incident_df.empty:
        return []

    entity = metric_entity_hint(df, path)
    signals: list[MetricSignal] = []
    for column in numeric_metric_columns(table.schema):
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

    signals.sort(key=lambda item: item.score, reverse=True)
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
        f"- time_window_utc: {case.groundtruth.start_time} -> {case.groundtruth.end_time}",
        f"- instance_type: {case.groundtruth.instance_type}",
        f"- service: {case.groundtruth.service or 'unknown'}",
        f"- instance: {', '.join(case.groundtruth.instance) if case.groundtruth.instance else 'unknown'}",
    ]
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
        service=case.groundtruth.service or "unknown",
        instance_type=case.groundtruth.instance_type or "unknown",
        instance=list(case.groundtruth.instance),
        source=case.groundtruth.source,
        destination=case.groundtruth.destination,
        start_time=case.groundtruth.start_time,
        end_time=case.groundtruth.end_time,
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
    end = case.end_local.date()
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
