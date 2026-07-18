#!/usr/bin/env python3
"""Freeze an input-only AIOps2025 recorded holdout manifest."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
from datetime import datetime
from pathlib import Path
from typing import Any
from zoneinfo import ZoneInfo


SCHEMA_VERSION = "aiops2025-recorded-split-v1"
DEFAULT_SEED = "opscaptain-aiops2025-holdout-v2"
UTC_TIMESTAMP_RE = re.compile(r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z")
BEIJING = ZoneInfo("Asia/Shanghai")


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Freeze a label-free AIOps2025 recorded holdout manifest.")
    parser.add_argument("--input", required=True, help="AIOps2025 input.json")
    parser.add_argument(
        "--prior-manifest",
        action="append",
        required=True,
        help="Manifest whose consumed cases must be excluded; repeat for multiple generations",
    )
    parser.add_argument("--output", required=True)
    parser.add_argument("--archive-date", action="append", required=True, help="Local telemetry date; repeat per date")
    parser.add_argument("--total-cases", type=int, default=18)
    parser.add_argument("--seed", default=DEFAULT_SEED)
    return parser.parse_args()


def freeze_manifest(
    input_path: Path,
    prior_manifest_paths: Path | list[Path],
    archive_dates: list[str],
    total_cases: int,
    seed: str = DEFAULT_SEED,
) -> dict[str, Any]:
    if total_cases <= 0:
        raise ValueError("total_cases must be positive")
    normalized_dates = unique_non_empty(archive_dates)
    if len(normalized_dates) != len(archive_dates):
        raise ValueError("archive dates must be non-empty and unique")
    for value in normalized_dates:
        datetime.strptime(value, "%Y-%m-%d")

    input_bytes = input_path.read_bytes()
    inputs = json.loads(input_bytes)
    if isinstance(prior_manifest_paths, Path):
        prior_manifest_paths = [prior_manifest_paths]
    if not prior_manifest_paths:
        raise ValueError("at least one prior manifest is required")
    development: list[str] = []
    consumed: set[str] = set()
    prior_hashes: list[str] = []
    for prior_manifest_path in prior_manifest_paths:
        prior_bytes = prior_manifest_path.read_bytes()
        prior = json.loads(prior_bytes)
        prior_development = unique_non_empty(prior.get("development_case_ids", []))
        prior_holdout = unique_non_empty(prior.get("holdout_case_ids", []))
        if not prior_development or not prior_holdout or set(prior_development) & set(prior_holdout):
            raise ValueError("each prior manifest must contain non-empty, disjoint development and holdout ids")
        development.extend(prior_development)
        consumed.update(prior_development)
        consumed.update(prior_holdout)
        consumed.update(unique_non_empty(prior.get("consumed_case_ids", [])))
        prior_hashes.append(hashlib.sha256(prior_bytes).hexdigest())
    development = unique_non_empty(development)

    candidates: dict[str, list[str]] = {value: [] for value in normalized_dates}
    seen_input_ids: set[str] = set()
    for item in inputs:
        case_id = str(item.get("uuid", "")).strip()
        if not case_id or case_id in seen_input_ids:
            raise ValueError("input cases must have non-empty, unique uuid values")
        seen_input_ids.add(case_id)
        if case_id in consumed:
            continue
        local_date = input_local_date(case_id, str(item.get("Anomaly Description", "")))
        if local_date in candidates:
            candidates[local_date].append(case_id)

    quotas = balanced_quotas(candidates, normalized_dates, total_cases)
    selected: list[str] = []
    selected_counts: dict[str, int] = {}
    for archive_date in normalized_dates:
        ranked = sorted(
            candidates[archive_date],
            key=lambda case_id: (selection_digest(seed, archive_date, case_id), case_id),
        )
        chosen = ranked[: quotas[archive_date]]
        selected.extend(chosen)
        selected_counts[archive_date] = len(chosen)

    if set(selected) & consumed or len(selected) != len(set(selected)):
        raise ValueError("selected holdout must be unique and disjoint from all consumed cases")

    excluded_ids = sorted(consumed)
    consumed_ids = sorted(consumed | set(selected))
    prior_hashes_digest = hashlib.sha256(("\n".join(prior_hashes) + "\n").encode()).hexdigest()
    return {
        "schema_version": SCHEMA_VERSION,
        "source": "agenticopseval/AIOps2025",
        "archive_dates": normalized_dates,
        "selection_contract": (
            "input-only local-date stratification; exclude every prior development/holdout case; "
            "rank remaining ids by seeded SHA-256; labels are unavailable to selection"
        ),
        "selection_algorithm": (
            "capacity-balanced round-robin quotas in archive_dates order; "
            "within each date sha256(seed\\0archive_date\\0case_id), ascending"
        ),
        "selection_seed": seed,
        "holdout_case_count": total_cases,
        "selected_counts_by_archive_date": selected_counts,
        "input_sha256": hashlib.sha256(input_bytes).hexdigest(),
        "prior_manifest_count": len(prior_hashes),
        "prior_manifest_sha256": prior_hashes_digest,
        "prior_manifest_sha256s": prior_hashes,
        "excluded_case_count": len(excluded_ids),
        "excluded_case_ids_sha256": hashlib.sha256(("\n".join(excluded_ids) + "\n").encode()).hexdigest(),
        "consumed_case_count": len(consumed_ids),
        "consumed_case_ids_sha256": hashlib.sha256(("\n".join(consumed_ids) + "\n").encode()).hexdigest(),
        "consumed_case_ids": consumed_ids,
        "development_case_ids": development,
        "holdout_case_ids": selected,
    }


def input_local_date(case_id: str, description: str) -> str:
    timestamps = UTC_TIMESTAMP_RE.findall(description)
    if len(timestamps) != 2:
        raise ValueError(f"case {case_id} must contain exactly two UTC timestamps")
    start = datetime.fromisoformat(timestamps[0].replace("Z", "+00:00"))
    end = datetime.fromisoformat(timestamps[1].replace("Z", "+00:00"))
    if end < start:
        raise ValueError(f"case {case_id} has an inverted UTC time window")
    return start.astimezone(BEIJING).date().isoformat()


def selection_digest(seed: str, archive_date: str, case_id: str) -> str:
    return hashlib.sha256(f"{seed}\0{archive_date}\0{case_id}".encode()).hexdigest()


def balanced_quotas(candidates: dict[str, list[str]], archive_dates: list[str], total_cases: int) -> dict[str, int]:
    if sum(len(candidates[value]) for value in archive_dates) < total_cases:
        raise ValueError(f"only {sum(len(candidates[value]) for value in archive_dates)} unused input cases; need {total_cases}")
    quotas = {value: 0 for value in archive_dates}
    remaining = total_cases
    while remaining > 0:
        progressed = False
        for value in archive_dates:
            if quotas[value] >= len(candidates[value]):
                continue
            quotas[value] += 1
            remaining -= 1
            progressed = True
            if remaining == 0:
                break
        if not progressed:
            raise ValueError("unable to allocate requested holdout cases")
    return quotas


def unique_non_empty(values: list[Any]) -> list[str]:
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        normalized = str(value).strip()
        if normalized and normalized not in seen:
            seen.add(normalized)
            out.append(normalized)
    return out


def main() -> None:
    args = parse_args()
    try:
        manifest = freeze_manifest(
            Path(args.input).resolve(),
            [Path(path).resolve() for path in args.prior_manifest],
            args.archive_date,
            args.total_cases,
            args.seed,
        )
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        raise SystemExit(str(exc)) from exc
    output = Path(args.output).resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(manifest, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
