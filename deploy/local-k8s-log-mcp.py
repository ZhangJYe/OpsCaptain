#!/usr/bin/env python3
import argparse
from datetime import datetime, timezone
import json
import os
import queue
import re
import subprocess
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlencode, urlparse
from urllib.request import Request, urlopen

NAMESPACE = "freeexchanged"
TOOL_NAME = "query_freeexchanged_k8s_logs"
LOG_BACKEND = "k8s"
KUBECTL_PATH = "/usr/local/bin/k3s"
LOKI_URL = "http://127.0.0.1:3100"
LOG_FILE = ""
SESSIONS = {}
SESSIONS_LOCK = threading.Lock()
QUERY_TIMEOUT_SECONDS = 4.0
POD_LOG_TIMEOUT_SECONDS = 2.0
MAX_PODS = 8
MAX_WINDOW_SECONDS = 6 * 60 * 60
MAX_RESPONSE_BYTES = 2 * 1024 * 1024
SYNTHETIC_LOG_INTERVAL_SECONDS = 0.0
SYNTHETIC_LOGS = (
    {"service": "checkout", "level": "error", "incident": "payment-timeout", "message": "payment upstream timeout after 3000ms", "trace_id": "demo-timeout"},
    {"service": "payment", "level": "error", "incident": "database-refused", "message": "connection refused to payment database", "trace_id": "demo-db-refused"},
    {"service": "gateway", "level": "warn", "incident": "latency-spike", "message": "request latency exceeded SLO p99=2.8s", "trace_id": "demo-latency"},
    {"service": "catalog", "level": "error", "incident": "dependency-unavailable", "message": "dependency unavailable circuit breaker opened", "trace_id": "demo-circuit"},
    {"service": "checkout", "level": "info", "incident": "healthy", "message": "request completed status=200 duration_ms=84", "trace_id": "demo-healthy"},
)


def jsonrpc_result(request_id, result):
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def jsonrpc_error(request_id, code, message):
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}


def run_kubectl(args, timeout=8):
    cmd = [KUBECTL_PATH, "kubectl"] + args
    completed = subprocess.run(
        cmd,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=timeout,
    )
    if completed.returncode != 0:
        raise RuntimeError(completed.stderr.strip() or "kubectl command failed")
    return completed.stdout


def list_pods(timeout=8):
    raw = run_kubectl(["get", "pods", "-n", NAMESPACE, "-o", "json"], timeout=timeout)
    data = json.loads(raw)
    pods = []
    for item in data.get("items", []):
        phase = item.get("status", {}).get("phase", "")
        name = item.get("metadata", {}).get("name", "")
        if name and phase in {"Running", "Succeeded", "Failed"}:
            pods.append(name)
    return pods


def log_terms(query):
    query = (query or "").lower()
    if any(token in query for token in ["错误", "异常", "失败", "超时", "报错"]):
        return ["error", "failed", "fail", "panic", "exception", "timeout", "refused", "fatal", "warn"]
    words = []
    current = []
    for ch in query:
        if ch.isalnum() or ch in "-_":
            current.append(ch)
        elif current:
            words.append("".join(current).strip("-_"))
            current = []
    if current:
        words.append("".join(current).strip("-_"))

    stop = {
        "freeexchanged",
        "log",
        "logs",
        "k8s",
        "pod",
        "recent",
        "hour",
        "hours",
        "query",
        "check",
        "analyze",
        "analysis",
        "current",
    }
    terms = []
    for word in words:
        if len(word) >= 3 and word not in stop and word not in terms:
            terms.append(word)
    return terms or ["error", "failed", "fail", "panic", "exception", "timeout", "refused", "fatal", "warn"]


def query_k8s_logs(arguments):
    deadline = time.monotonic() + QUERY_TIMEOUT_SECONDS
    query = str(arguments.get("query") or "")
    focus = str(arguments.get("focus") or "")
    limit = int(arguments.get("limit") or 5)
    limit = max(1, min(limit, 20))
    terms = log_terms(query + "\n" + focus)

    records = []
    try:
        pods = list_pods(timeout=remaining_timeout(deadline, 2.0))
    except Exception as exc:
        return {
            "success": False,
            "degraded": True,
            "logs": records,
            "error": f"failed to list pods: {exc}",
            "message": "log backend degraded before pod discovery",
        }

    inspected = 0
    for pod in pods[:MAX_PODS]:
        if time.monotonic() >= deadline:
            return {
                "success": True,
                "degraded": True,
                "logs": records,
                "message": f"partial result: inspected {inspected}/{len(pods)} pods before timeout",
            }
        inspected += 1
        try:
            output = run_kubectl(
                [
                    "logs",
                    "-n",
                    NAMESPACE,
                    pod,
                    "--all-containers=true",
                    "--prefix=true",
                    "--since=2h",
                    "--tail=160",
                ],
                timeout=remaining_timeout(deadline, POD_LOG_TIMEOUT_SECONDS),
            )
        except Exception as exc:
            line = f"{pod}: failed to read pod logs: {exc}"
            if any(term in line.lower() for term in terms):
                records.append({"service": pod, "message": line, "level": "error"})
            continue

        lines = [line.strip() for line in output.splitlines() if line.strip()]
        for line in reversed(lines):
            lower = line.lower()
            if not any(term in lower for term in terms):
                continue
            records.append(
                {
                    "namespace": NAMESPACE,
                    "service": pod,
                    "message": line[:500],
                    "level": infer_level(lower),
                }
            )
            if len(records) >= limit:
                return {"success": True, "logs": records, "message": f"found {len(records)} log records"}

    message = f"found {len(records)} log records"
    if len(pods) > MAX_PODS:
        message += f"; inspected first {MAX_PODS}/{len(pods)} pods"
    return {"success": True, "logs": records, "message": message}


def parse_window_seconds(raw):
    text = str(raw or "").strip().lower()
    if not text:
        return min(30 * 60, MAX_WINDOW_SECONDS)

    match = re.search(r"(\d+(?:\.\d+)?)\s*(秒|分钟|小时|天|s|m|h|d)", text)
    if not match:
        return min(30 * 60, MAX_WINDOW_SECONDS)

    value = float(match.group(1))
    unit = match.group(2)
    multiplier = {
        "秒": 1,
        "s": 1,
        "分钟": 60,
        "m": 60,
        "小时": 60 * 60,
        "h": 60 * 60,
        "天": 24 * 60 * 60,
        "d": 24 * 60 * 60,
    }[unit]
    return max(60, min(int(value * multiplier), MAX_WINDOW_SECONDS))


def logql_string(value):
    return json.dumps(str(value), ensure_ascii=False)


def build_loki_query(arguments):
    service = str(arguments.get("service") or "").strip()
    query = str(arguments.get("query") or "").strip()
    focus = str(arguments.get("focus") or "").strip()

    if service:
        service_pattern = "(?i)" + re.escape(service)
        selector = f"{{service_name=~{logql_string(service_pattern)}}}"
    else:
        selector = '{service_name=~".+"}'

    terms = log_terms(query + "\n" + focus)
    pattern = "(?i)(" + "|".join(re.escape(term) for term in terms[:12]) + ")"
    return f"{selector} |~ {logql_string(pattern)}"


def loki_timestamp(raw):
    try:
        seconds = int(raw) / 1_000_000_000
    except (TypeError, ValueError):
        return str(raw or "")
    return datetime.fromtimestamp(seconds, tz=timezone.utc).isoformat().replace("+00:00", "Z")


def query_loki_logs(arguments):
    deadline = time.monotonic() + QUERY_TIMEOUT_SECONDS
    limit = int(arguments.get("limit") or 5)
    limit = max(1, min(limit, 20))
    window_seconds = parse_window_seconds(arguments.get("window"))
    end_ns = time.time_ns()
    start_ns = end_ns - window_seconds * 1_000_000_000
    logql = build_loki_query(arguments)

    params = urlencode(
        {
            "query": logql,
            "start": str(start_ns),
            "end": str(end_ns),
            "limit": str(limit),
            "direction": "backward",
        }
    )
    endpoint = f"{LOKI_URL.rstrip('/')}/loki/api/v1/query_range?{params}"
    request = Request(
        endpoint,
        headers={"Accept": "application/json", "User-Agent": "OpsCaptain-log-adapter/1.0"},
    )

    try:
        with urlopen(request, timeout=remaining_timeout(deadline, QUERY_TIMEOUT_SECONDS)) as response:
            raw = response.read(MAX_RESPONSE_BYTES + 1)
    except Exception as exc:
        return {
            "success": False,
            "degraded": True,
            "logs": [],
            "error": f"failed to query Loki: {exc}",
            "message": "log backend degraded while querying Loki",
        }

    if len(raw) > MAX_RESPONSE_BYTES:
        return {
            "success": False,
            "degraded": True,
            "logs": [],
            "error": "Loki response exceeded size limit",
            "message": "log backend returned an oversized response",
        }

    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        return {
            "success": False,
            "degraded": True,
            "logs": [],
            "error": f"failed to decode Loki response: {exc}",
            "message": "log backend returned an invalid response",
        }

    if payload.get("status") != "success":
        return {
            "success": False,
            "degraded": True,
            "logs": [],
            "error": str(payload.get("error") or "Loki query failed"),
            "message": "log backend rejected the query",
        }

    records = []
    for stream_result in payload.get("data", {}).get("result", []):
        labels = stream_result.get("stream") or {}
        for value in stream_result.get("values") or []:
            if not isinstance(value, list) or len(value) < 2:
                continue
            line = str(value[1])
            parsed_line = {}
            try:
                candidate = json.loads(line)
                if isinstance(candidate, dict):
                    parsed_line = candidate
            except json.JSONDecodeError:
                pass
            service = labels.get("service_name") or parsed_line.get("service") or labels.get("container") or "unknown"
            level = labels.get("level") or parsed_line.get("level") or infer_level(line.lower())
            records.append(
                {
                    "timestamp": loki_timestamp(value[0]),
                    "service": str(service),
                    "level": str(level),
                    "message": str(parsed_line.get("message") or line)[:500],
                    "labels": labels,
                }
            )

    records.sort(key=lambda item: item.get("timestamp", ""), reverse=True)
    records = records[:limit]
    return {
        "success": True,
        "logs": records,
        "message": f"found {len(records)} log records from Loki",
        "backend": "loki",
        "query": logql,
    }


def parse_rfc3339_timestamp(raw):
    text = str(raw or "").strip()
    if not text:
        return None
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed.astimezone(timezone.utc)


def file_query_terms(arguments):
    text = "\n".join(
        str(arguments.get(key) or "").lower()
        for key in ("query", "focus", "service")
    )
    stop = {
        "analyze", "check", "current", "error", "failed", "failure", "hour", "hours",
        "incident", "log", "logs", "query", "recent", "service", "timeout", "warn",
    }
    terms = []
    for token in re.findall(r"[a-z0-9][a-z0-9_-]{2,}", text):
        if token not in stop and token not in terms:
            terms.append(token)
    return terms


def query_file_logs(arguments):
    if not LOG_FILE:
        raise RuntimeError("file log backend is not configured")
    try:
        size = os.path.getsize(LOG_FILE)
    except OSError as exc:
        raise RuntimeError(f"file log backend is unavailable: {exc}") from exc
    if size > MAX_RESPONSE_BYTES:
        raise RuntimeError("file log backend exceeded size limit")

    limit = max(1, min(int(arguments.get("limit") or 5), 20))
    service = str(arguments.get("service") or "").strip().lower()
    terms = file_query_terms(arguments)
    cutoff = datetime.now(timezone.utc).timestamp() - parse_window_seconds(arguments.get("window"))
    candidates = []
    with open(LOG_FILE, "r", encoding="utf-8") as handle:
        for line_number, raw in enumerate(handle, start=1):
            if not raw.strip():
                continue
            try:
                record = json.loads(raw)
            except json.JSONDecodeError:
                continue
            if not isinstance(record, dict):
                continue
            timestamp = parse_rfc3339_timestamp(record.get("timestamp"))
            if timestamp is None or timestamp.timestamp() < cutoff:
                continue
            record_service = str(record.get("service") or "").lower()
            if service and service not in record_service:
                continue
            searchable = json.dumps(record, ensure_ascii=False).lower()
            score = sum(1 for term in terms if term in searchable)
            if terms and score == 0:
                continue
            candidates.append((score, timestamp.timestamp(), line_number, record))

    candidates.sort(key=lambda item: (item[0], item[1], item[2]), reverse=True)
    records = [item[3] for item in candidates[:limit]]
    return {
        "success": True,
        "degraded": False,
        "logs": records,
        "message": f"found {len(records)} measured log records",
        "backend": "file",
        "provenance": "controlled_fault_injection_v1",
    }


def query_logs(arguments):
    if LOG_BACKEND == "loki":
        return query_loki_logs(arguments)
    if LOG_BACKEND == "file":
        return query_file_logs(arguments)
    return query_k8s_logs(arguments)


def build_loki_push_payload(record, timestamp_ns=None):
    timestamp_ns = time.time_ns() if timestamp_ns is None else timestamp_ns
    labels = {
        "service_name": str(record["service"]),
        "level": str(record["level"]),
        "incident": str(record["incident"]),
        "environment": "local",
        "source": "opscaptain-simulator",
    }
    return {
        "streams": [
            {
                "stream": labels,
                "values": [[str(timestamp_ns), json.dumps(record, ensure_ascii=False)]],
            }
        ]
    }


def push_synthetic_log(record):
    endpoint = f"{LOKI_URL.rstrip('/')}/loki/api/v1/push"
    payload = json.dumps(build_loki_push_payload(record), ensure_ascii=False).encode()
    request = Request(
        endpoint,
        data=payload,
        method="POST",
        headers={"Content-Type": "application/json", "User-Agent": "OpsCaptain-log-adapter/1.0"},
    )
    with urlopen(request, timeout=QUERY_TIMEOUT_SECONDS) as response:
        response.read(1024)


def synthetic_log_loop():
    index = 0
    while True:
        try:
            push_synthetic_log(SYNTHETIC_LOGS[index % len(SYNTHETIC_LOGS)])
            index += 1
        except Exception:
            pass
        time.sleep(SYNTHETIC_LOG_INTERVAL_SECONDS)


def remaining_timeout(deadline, cap):
    remaining = deadline - time.monotonic()
    if remaining <= 0:
        return 0.1
    return max(0.1, min(cap, remaining))


def infer_level(line):
    if "panic" in line or "fatal" in line:
        return "fatal"
    if "error" in line or "failed" in line or "exception" in line:
        return "error"
    if "warn" in line:
        return "warn"
    return "info"


def tool_schema():
    descriptions = {
        "file": "Query measured logs from an isolated controlled-fault workload.",
        "k8s": "Query recent k3s pod logs on this server.",
        "loki": "Query recent logs from Loki.",
    }
    description = descriptions[LOG_BACKEND]
    return {
        "name": TOOL_NAME,
        "description": description,
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "Natural language query or error keyword."},
                "service": {"type": "string", "description": "Optional service name."},
                "window": {"type": "string", "description": "Optional time window, for example 30m or 2h."},
                "limit": {"type": "integer", "description": "Maximum log records to return."},
                "focus": {"type": "string", "description": "Optional investigation focus."},
                "skill_mode": {"type": "string", "description": "Optional OpsCaption log skill mode."},
            },
        },
    }


def handle_rpc(payload):
    request_id = payload.get("id")
    method = payload.get("method")
    params = payload.get("params") or {}

    if method == "initialize":
        return jsonrpc_result(
            request_id,
            {
                "protocolVersion": params.get("protocolVersion") or "2024-11-05",
                "capabilities": {"tools": {}},
                "serverInfo": {"name": f"opscaptain-{LOG_BACKEND}-log-adapter", "version": "0.2.0"},
            },
        )
    if method == "tools/list":
        return jsonrpc_result(request_id, {"tools": [tool_schema()]})
    if method == "tools/call":
        name = params.get("name")
        if name != TOOL_NAME:
            return jsonrpc_error(request_id, -32601, f"unknown tool: {name}")
        try:
            result = query_logs(params.get("arguments") or {})
            text = json.dumps(result, ensure_ascii=False)
            return jsonrpc_result(request_id, {"content": [{"type": "text", "text": text}]})
        except Exception as exc:
            return jsonrpc_result(
                request_id,
                {"content": [{"type": "text", "text": json.dumps({"success": False, "error": str(exc)})}]},
            )
    if method and method.startswith("notifications/"):
        return None
    return jsonrpc_error(request_id, -32601, f"method not found: {method}")


class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/healthz":
            self.write_json({"ok": True, "service": "opscaptain-log-adapter", "backend": LOG_BACKEND})
            return
        if parsed.path != "/sse":
            self.send_error(404)
            return

        session_id = uuid.uuid4().hex
        q = queue.Queue()
        with SESSIONS_LOCK:
            SESSIONS[session_id] = q

        host = self.headers.get("Host") or f"127.0.0.1:{self.server.server_port}"
        endpoint = f"http://{host}/message?sessionId={session_id}"
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Cache-Control", "no-cache")
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        self.write_event("endpoint", endpoint)

        try:
            while True:
                try:
                    message = q.get(timeout=20)
                    self.write_event("message", json.dumps(message, separators=(",", ":")))
                except queue.Empty:
                    self.wfile.write(b": keepalive\n\n")
                    self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass
        finally:
            with SESSIONS_LOCK:
                SESSIONS.pop(session_id, None)

    def do_POST(self):
        parsed = urlparse(self.path)
        if parsed.path == "/tools/query_logs":
            length = int(self.headers.get("Content-Length") or "0")
            raw = self.rfile.read(length)
            try:
                payload = json.loads(raw or b"{}")
                result = query_logs(payload if isinstance(payload, dict) else {})
                self.write_json(result)
            except Exception as exc:
                self.write_json({"success": False, "degraded": True, "error": str(exc)})
            return

        if parsed.path != "/message":
            self.send_error(404)
            return
        session_id = parse_qs(parsed.query).get("sessionId", [""])[0]
        with SESSIONS_LOCK:
            q = SESSIONS.get(session_id)
        if q is None:
            self.send_error(404, "unknown session")
            return

        length = int(self.headers.get("Content-Length") or "0")
        raw = self.rfile.read(length)
        try:
            payload = json.loads(raw)
            response = handle_rpc(payload)
            if response is not None:
                q.put(response)
        except Exception as exc:
            q.put(jsonrpc_error(None, -32603, str(exc)))

        self.send_response(202)
        self.send_header("Content-Length", "0")
        self.end_headers()

    def write_json(self, payload):
        data = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def write_event(self, event, data):
        self.wfile.write(f"event: {event}\n".encode())
        for line in str(data).splitlines() or [""]:
            self.wfile.write(f"data: {line}\n".encode())
        self.wfile.write(b"\n")
        self.wfile.flush()

    def log_message(self, fmt, *args):
        return


def main():
    global LOG_BACKEND, LOKI_URL, LOG_FILE, NAMESPACE, KUBECTL_PATH, TOOL_NAME
    global QUERY_TIMEOUT_SECONDS, POD_LOG_TIMEOUT_SECONDS, MAX_PODS, MAX_WINDOW_SECONDS
    global SYNTHETIC_LOG_INTERVAL_SECONDS
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18088)
    parser.add_argument("--backend", choices=("file", "k8s", "loki"), default=os.getenv("LOG_MCP_BACKEND", LOG_BACKEND))
    parser.add_argument("--loki-url", default=os.getenv("LOKI_URL", LOKI_URL))
    parser.add_argument("--log-file", default=os.getenv("LOG_MCP_LOG_FILE", LOG_FILE))
    parser.add_argument("--namespace", default=os.getenv("LOG_MCP_K8S_NAMESPACE", NAMESPACE))
    parser.add_argument("--kubectl-path", default=os.getenv("LOG_MCP_KUBECTL_PATH", KUBECTL_PATH))
    parser.add_argument("--tool-name", default=os.getenv("LOG_MCP_TOOL_NAME", TOOL_NAME))
    parser.add_argument("--query-timeout", type=float, default=float(os.getenv("LOG_MCP_QUERY_TIMEOUT_SECONDS", QUERY_TIMEOUT_SECONDS)))
    parser.add_argument("--pod-log-timeout", type=float, default=float(os.getenv("LOG_MCP_POD_LOG_TIMEOUT_SECONDS", POD_LOG_TIMEOUT_SECONDS)))
    parser.add_argument("--max-pods", type=int, default=int(os.getenv("LOG_MCP_MAX_PODS", MAX_PODS)))
    parser.add_argument("--max-window", type=int, default=int(os.getenv("LOG_MCP_MAX_WINDOW_SECONDS", MAX_WINDOW_SECONDS)))
    parser.add_argument(
        "--synthetic-log-interval",
        type=float,
        default=float(os.getenv("LOG_MCP_SYNTHETIC_LOG_INTERVAL_SECONDS", SYNTHETIC_LOG_INTERVAL_SECONDS)),
    )
    args = parser.parse_args()
    LOG_BACKEND = args.backend
    LOKI_URL = args.loki_url.strip()
    LOG_FILE = args.log_file.strip()
    NAMESPACE = args.namespace.strip()
    KUBECTL_PATH = args.kubectl_path.strip()
    TOOL_NAME = args.tool_name.strip() or "query_logs"
    QUERY_TIMEOUT_SECONDS = max(0.5, args.query_timeout)
    POD_LOG_TIMEOUT_SECONDS = max(0.1, args.pod_log_timeout)
    MAX_PODS = max(1, args.max_pods)
    MAX_WINDOW_SECONDS = max(60, args.max_window)
    SYNTHETIC_LOG_INTERVAL_SECONDS = max(0.0, args.synthetic_log_interval)
    if LOG_BACKEND == "file" and not LOG_FILE:
        parser.error("--log-file is required for file backend")
    if LOG_BACKEND == "loki" and SYNTHETIC_LOG_INTERVAL_SECONDS > 0:
        threading.Thread(target=synthetic_log_loop, daemon=True).start()
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"OpsCaptain {LOG_BACKEND} log adapter listening on {args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
