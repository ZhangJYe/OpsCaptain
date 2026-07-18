#!/usr/bin/env python3
"""Generate measured telemetry from small, isolated controlled faults."""

import argparse
from datetime import datetime, timezone
import json
import math
from pathlib import Path
import queue
import socket
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import URLError
from urllib.request import urlopen


NAMESPACE = "opscaptain-eval"
PROVENANCE = "controlled_fault_injection_v1"


def utc_now():
    return datetime.now(timezone.utc).isoformat().replace("+00:00", "Z")


def record(case_id, service, level, message, measurements):
    return {
        "timestamp": utc_now(),
        "namespace": NAMESPACE,
        "environment": "isolated-real-eval",
        "case_id": case_id,
        "service": service,
        "level": level,
        "message": message,
        "measurements": measurements,
        "provenance": {
            "source": "controlled_fault_injection",
            "runner_version": "v1",
            "measured": True,
        },
    }


def cpu_saturation():
    wall_start = time.perf_counter()
    cpu_start = time.process_time()
    value = 0.0
    while time.perf_counter() - wall_start < 1.5:
        value += math.sqrt(12345.6789)
    wall_seconds = time.perf_counter() - wall_start
    cpu_seconds = time.process_time() - cpu_start
    ratio = min(1.0, cpu_seconds / max(wall_seconds, 0.001))
    return record(
        "real-controlled-001",
        "eval-cpu-checkout",
        "warn",
        "single busy-loop worker consumed a CPU core while memory remained stable",
        {"cpu_ratio": round(ratio, 4), "wall_seconds": round(wall_seconds, 4), "checksum": round(value, 2)},
    ), {"opscaptain_eval_cpu_ratio": ratio}


def lock_contention():
    lock = threading.Lock()
    barrier = threading.Barrier(8)
    waits = []
    waits_lock = threading.Lock()

    def contender():
        barrier.wait()
        for _ in range(8):
            started = time.perf_counter()
            with lock:
                waited = time.perf_counter() - started
                time.sleep(0.008)
            with waits_lock:
                waits.append(waited)

    threads = [threading.Thread(target=contender) for _ in range(8)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    waits.sort()
    p95 = waits[min(len(waits) - 1, int(len(waits) * 0.95))]
    return record(
        "real-controlled-002",
        "eval-lock-payment",
        "warn",
        "request workers queued behind one shared mutex; lock wait dominated processing time",
        {"lock_wait_p95_seconds": round(p95, 6), "samples": len(waits)},
    ), {"opscaptain_eval_lock_wait_seconds": p95}


class SlowHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        time.sleep(0.2)
        self.send_response(200)
        self.send_header("Content-Length", "2")
        self.end_headers()
        try:
            self.wfile.write(b"ok")
        except BrokenPipeError:
            pass

    def log_message(self, _format, *_args):
        return


def dependency_timeout():
    server = ThreadingHTTPServer(("127.0.0.1", 0), SlowHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    timeouts = 0
    url = f"http://127.0.0.1:{server.server_port}/risk"
    try:
        for _ in range(5):
            try:
                with urlopen(url, timeout=0.05) as response:
                    response.read(16)
            except (TimeoutError, URLError):
                timeouts += 1
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=1)
    return record(
        "real-controlled-003",
        "eval-risk-client",
        "error",
        "external risk dependency exceeded the 50ms client deadline on every measured request",
        {"requests": 5, "timeouts": timeouts, "client_timeout_ms": 50},
    ), {"opscaptain_eval_dependency_timeouts_total": float(timeouts)}


def refused_endpoint():
    holder = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    holder.bind(("127.0.0.1", 0))
    port = holder.getsockname()[1]
    holder.close()
    refused = 0
    error_text = ""
    for _ in range(3):
        client = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        client.settimeout(0.2)
        try:
            client.connect(("127.0.0.1", port))
        except OSError as exc:
            refused += 1
            error_text = exc.__class__.__name__
        finally:
            client.close()
    return record(
        "real-controlled-004",
        "eval-config-client",
        "error",
        "configured dependency endpoint pointed to a closed port and every connection was refused",
        {"attempts": 3, "connection_refused": refused, "error_type": error_text},
    ), {"opscaptain_eval_connection_refused_total": float(refused)}


def cache_stampede():
    expires_at = time.monotonic() + 0.05
    cache = {f"hot-{index}": expires_at for index in range(32)}
    time.sleep(0.06)
    misses = 0
    origin_requests = 0
    counter_lock = threading.Lock()

    def read_hot_key(index):
        nonlocal misses, origin_requests
        key = f"hot-{index % len(cache)}"
        if cache[key] <= time.monotonic():
            with counter_lock:
                misses += 1
                origin_requests += 1

    threads = [threading.Thread(target=read_hot_key, args=(index,)) for index in range(64)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    return record(
        "real-controlled-005",
        "eval-cache-catalog",
        "warn",
        "hot cache keys shared one expiry boundary and concurrent misses amplified origin requests",
        {"expired_keys": len(cache), "cache_misses": misses, "origin_requests": origin_requests},
    ), {
        "opscaptain_eval_cache_misses_total": float(misses),
        "opscaptain_eval_origin_requests_total": float(origin_requests),
    }


def slow_consumer_backlog():
    work = queue.Queue()
    consumed = 0
    consumed_lock = threading.Lock()
    started = time.perf_counter()

    def consumer():
        nonlocal consumed
        while True:
            item = work.get()
            try:
                if item is None:
                    return
                time.sleep(0.015)
                with consumed_lock:
                    consumed += 1
            finally:
                work.task_done()

    thread = threading.Thread(target=consumer)
    thread.start()
    peak_depth = 0
    for item in range(24):
        work.put(item)
        peak_depth = max(peak_depth, work.qsize())
    work.put(None)
    peak_depth = max(peak_depth, work.qsize() - 1)
    work.join()
    thread.join(timeout=1)
    drain_seconds = time.perf_counter() - started
    return record(
        "real-development-001",
        "eval-queue-orders",
        "warn",
        "a producer burst outpaced the single slow consumer and built a measured work-queue backlog",
        {"produced": 24, "consumed": consumed, "peak_queue_depth": peak_depth, "drain_seconds": round(drain_seconds, 4)},
    ), {
        "opscaptain_eval_queue_peak_depth": float(peak_depth),
        "opscaptain_eval_queue_drain_seconds": drain_seconds,
    }


class RetryFailureHandler(BaseHTTPRequestHandler):
    requests = 0
    requests_lock = threading.Lock()

    def do_GET(self):
        with self.requests_lock:
            type(self).requests += 1
        self.send_response(503)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def log_message(self, _format, *_args):
        return


def retry_storm():
    RetryFailureHandler.requests = 0
    server = ThreadingHTTPServer(("127.0.0.1", 0), RetryFailureHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    logical_requests = 4
    attempts_per_request = 3
    try:
        for _ in range(logical_requests):
            for _ in range(attempts_per_request):
                try:
                    urlopen(f"http://127.0.0.1:{server.server_port}/inventory", timeout=0.2)
                except URLError as exc:
                    close = getattr(exc, "close", None)
                    if close is not None:
                        close()
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=1)
    attempts = RetryFailureHandler.requests
    return record(
        "real-development-002",
        "eval-retry-inventory",
        "error",
        "upstream HTTP 503 responses were retried immediately without backoff and amplified each logical request",
        {"logical_requests": logical_requests, "attempts": attempts, "attempts_per_request": attempts_per_request, "backoff_ms": 0},
    ), {
        "opscaptain_eval_retry_attempts_total": float(attempts),
        "opscaptain_eval_logical_requests_total": float(logical_requests),
    }


def connection_pool_exhaustion():
    pool = threading.Semaphore(2)
    barrier = threading.Barrier(8)
    waits = []
    waits_lock = threading.Lock()

    def borrower():
        barrier.wait()
        started = time.perf_counter()
        with pool:
            waited = time.perf_counter() - started
            time.sleep(0.04)
        with waits_lock:
            waits.append(waited)

    threads = [threading.Thread(target=borrower) for _ in range(8)]
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    waits.sort()
    p95 = waits[min(len(waits) - 1, int(len(waits) * 0.95))]
    return record(
        "real-development-003",
        "eval-db-profile",
        "warn",
        "eight request workers contended for a two-slot database connection pool and waited for a free connection",
        {"workers": len(waits), "pool_size": 2, "pool_wait_p95_seconds": round(p95, 6)},
    ), {
        "opscaptain_eval_db_pool_wait_seconds": p95,
        "opscaptain_eval_db_pool_size": 2.0,
    }


SUITES = {
    "holdout_v1": [cpu_saturation, lock_contention, dependency_timeout, refused_endpoint, cache_stampede],
    "development_v1": [slow_consumer_backlog, retry_storm, connection_pool_exhaustion],
}


def build_fixture(suite="holdout_v1"):
    cases = SUITES[suite]
    records = []
    samples = []
    for fault in cases:
        measured_record, metrics = fault()
        records.append(measured_record)
        for metric, value in metrics.items():
            samples.append((metric, measured_record["case_id"], measured_record["service"], value))
        samples.append(("opscaptain_eval_fault_active", measured_record["case_id"], measured_record["service"], 1.0))
    return records, samples


def prometheus_text(samples):
    lines = []
    seen = set()
    for metric, case_id, service, value in samples:
        if metric not in seen:
            lines.append(f"# TYPE {metric} gauge")
            seen.add(metric)
        lines.append(f'{metric}{{case_id="{case_id}",service="{service}",provenance="{PROVENANCE}"}} {value:.6f}')
    return "\n".join(lines) + "\n"


def write_records(path, records):
    path.parent.mkdir(parents=True, exist_ok=True)
    payload = "".join(json.dumps(item, ensure_ascii=False) + "\n" for item in records)
    path.write_text(payload, encoding="utf-8")


class FixtureHandler(BaseHTTPRequestHandler):
    metrics = b""

    def do_GET(self):
        if self.path == "/healthz":
            payload = b'{"ok":true,"provenance":"controlled_fault_injection_v1"}\n'
            content_type = "application/json"
        elif self.path == "/metrics":
            payload = self.metrics
            content_type = "text/plain; version=0.0.4"
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, _format, *_args):
        return


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=19100)
    parser.add_argument("--log-file", required=True)
    parser.add_argument("--suite", choices=sorted(SUITES), default="holdout_v1")
    args = parser.parse_args()

    records, samples = build_fixture(args.suite)
    write_records(Path(args.log_file), records)
    FixtureHandler.metrics = prometheus_text(samples).encode()
    server = ThreadingHTTPServer((args.host, args.port), FixtureHandler)
    print(f"isolated real fixture ready suite={args.suite} cases={len(records)} host={args.host} port={args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
