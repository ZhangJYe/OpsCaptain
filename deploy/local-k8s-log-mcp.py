#!/usr/bin/env python3
import argparse
import json
import queue
import subprocess
import threading
import time
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

NAMESPACE = "freeexchanged"
TOOL_NAME = "query_freeexchanged_k8s_logs"
SESSIONS = {}
SESSIONS_LOCK = threading.Lock()


def jsonrpc_result(request_id, result):
    return {"jsonrpc": "2.0", "id": request_id, "result": result}


def jsonrpc_error(request_id, code, message):
    return {"jsonrpc": "2.0", "id": request_id, "error": {"code": code, "message": message}}


def run_kubectl(args, timeout=8):
    cmd = ["/usr/local/bin/k3s", "kubectl"] + args
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


def list_pods():
    raw = run_kubectl(["get", "pods", "-n", NAMESPACE, "-o", "json"], timeout=8)
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


def query_logs(arguments):
    query = str(arguments.get("query") or "")
    focus = str(arguments.get("focus") or "")
    limit = int(arguments.get("limit") or 5)
    limit = max(1, min(limit, 20))
    terms = log_terms(query + "\n" + focus)

    records = []
    for pod in list_pods():
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
                timeout=10,
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

    return {"success": True, "logs": records, "message": f"found {len(records)} log records"}


def infer_level(line):
    if "panic" in line or "fatal" in line:
        return "fatal"
    if "error" in line or "failed" in line or "exception" in line:
        return "error"
    if "warn" in line:
        return "warn"
    return "info"


def tool_schema():
    return {
        "name": TOOL_NAME,
        "description": "Query recent FreeExchanged k3s pod logs on this server.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {"type": "string", "description": "Natural language query or error keyword."},
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
                "serverInfo": {"name": "freeexchanged-local-k8s-log-mcp", "version": "0.1.0"},
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

    def write_event(self, event, data):
        self.wfile.write(f"event: {event}\n".encode())
        for line in str(data).splitlines() or [""]:
            self.wfile.write(f"data: {line}\n".encode())
        self.wfile.write(b"\n")
        self.wfile.flush()

    def log_message(self, fmt, *args):
        return


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=18088)
    args = parser.parse_args()
    server = ThreadingHTTPServer((args.host, args.port), Handler)
    print(f"freeexchanged local k8s log MCP listening on {args.host}:{args.port}", flush=True)
    server.serve_forever()


if __name__ == "__main__":
    main()
