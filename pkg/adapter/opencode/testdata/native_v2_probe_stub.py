"""Deterministic loopback Chat Completions fixture; never calls a provider."""
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Stub:
    def __init__(self, workdir):
        self.workdir = str(workdir)
        self.requests = []
        self.errors = []
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_GET(self):
                self.reply_json({"object": "list", "data": [{"id": "hook-fixture", "object": "model"}]})

            def reply_json(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_POST(self):
                length = int(self.headers.get("Content-Length", "0"))
                if length > 2 * 1024 * 1024:
                    self.send_error(413)
                    return
                body = json.loads(self.rfile.read(length))
                tools = [t.get("function", {}).get("name") for t in body.get("tools", [])]
                messages = body.get("messages", [])
                has_result = any(m.get("role") == "tool" for m in messages)
                owner.requests.append({"path": self.path, "tools": tools, "has_result": has_result})
                if len(owner.requests) > 12:
                    owner.errors.append("provider stub request bound exceeded")
                    self.send_error(429)
                    return
                if tools and not has_result:
                    if "shell" not in tools:
                        owner.errors.append("native shell tool absent: " + ",".join(str(t) for t in tools))
                        self.send_error(400)
                        return
                    args = {"command": "printf '%s\n' tool >> lifecycle.log; printf done > shell-ran", "workdir": owner.workdir, "timeout": 3000}
                    delta = {"role": "assistant", "tool_calls": [{"index": 0, "id": "call_native_hook", "type": "function", "function": {"name": "shell", "arguments": json.dumps(args)}}]}
                    finish = "tool_calls"
                else:
                    delta, finish = {"role": "assistant", "content": "NATIVE_FIXTURE_DONE"}, "stop"
                if not body.get("stream"):
                    message = dict(delta)
                    for call in message.get("tool_calls", []):
                        call.pop("index", None)
                    self.reply_json({"id": "fixture", "object": "chat.completion", "model": "hook-fixture", "choices": [{"index": 0, "message": message, "finish_reason": finish}], "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
                    return
                chunks = [
                    {"id": "fixture", "object": "chat.completion.chunk", "model": "hook-fixture", "choices": [{"index": 0, "delta": delta, "finish_reason": None}]},
                    {"id": "fixture", "object": "chat.completion.chunk", "model": "hook-fixture", "choices": [{"index": 0, "delta": {}, "finish_reason": finish}], "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}},
                ]
                data = ("".join("data: " + json.dumps(c) + "\n\n" for c in chunks) + "data: [DONE]\n\n").encode()
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.url = "http://127.0.0.1:" + str(self.server.server_address[1]) + "/v1"

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=3)
