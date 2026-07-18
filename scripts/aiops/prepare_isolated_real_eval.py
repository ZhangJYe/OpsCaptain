#!/usr/bin/env python3
"""Create an isolated real-evaluation config from the canonical config."""

import argparse
from pathlib import Path
import re


OVERRIDES = {
    "aiops.gos.enabled": "true",
    "aiops.gos.evaluation.telemetry_profile": '"real"',
    "aiops.gos.evaluation.telemetry_provenance": '"controlled_fault_injection_v1"',
    "aiops.gos.structured_cognition.enabled": "true",
    "aiops.gos.structured_cognition.call_timeout_ms": "30000",
    "aiops.gos.state_conversion.enabled": "true",
    "mcp.log_url": '""',
    "mcp.log_http_url": '"http://127.0.0.1:28088/tools/query_logs"',
    "mcp.log_default_window": '"6h"',
    "mcp.prefer_http_log_tool": "true",
    "prometheus.address": '"http://127.0.0.1:19090"',
}


MAPPING_LINE = re.compile(r"^(?P<indent>\s*)(?P<key>[A-Za-z0-9_]+):(?P<rest>.*)$")


def rewrite_config(source, overrides=None):
    overrides = OVERRIDES if overrides is None else overrides
    stack = []
    found = set()
    output = []
    for raw in source.splitlines(keepends=True):
        match = MAPPING_LINE.match(raw)
        if not match or raw.lstrip().startswith("#"):
            output.append(raw)
            continue
        indent = len(match.group("indent"))
        key = match.group("key")
        while stack and stack[-1][0] >= indent:
            stack.pop()
        path = ".".join([item[1] for item in stack] + [key])
        rest = match.group("rest")
        if path in overrides:
            newline = "\n" if raw.endswith("\n") else ""
            comment = ""
            if "#" in rest:
                comment = "  #" + rest.split("#", 1)[1].rstrip("\n")
            raw = f'{match.group("indent")}{key}: {overrides[path]}{comment}{newline}'
            found.add(path)
        output.append(raw)
        value = rest.split("#", 1)[0].strip()
        if value == "":
            stack.append((indent, key))
    missing = sorted(set(overrides) - found)
    if missing:
        raise ValueError("config overrides not found: " + ", ".join(missing))
    return "".join(output)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--log-http-url", default="http://127.0.0.1:28088/tools/query_logs")
    parser.add_argument("--prometheus-address", default="http://127.0.0.1:19090")
    parser.add_argument("--milvus-address")
    args = parser.parse_args()

    source_path = Path(args.input)
    output_path = Path(args.output)
    overrides = dict(OVERRIDES)
    overrides["mcp.log_http_url"] = f'"{args.log_http_url}"'
    overrides["prometheus.address"] = f'"{args.prometheus_address}"'
    if args.milvus_address:
        overrides["milvus.address"] = f'"{args.milvus_address}"'
    rendered = rewrite_config(source_path.read_text(encoding="utf-8"), overrides)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(rendered, encoding="utf-8")
    print(f"isolated real eval config written: {output_path}")


if __name__ == "__main__":
    main()
