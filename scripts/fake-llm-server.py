#!/usr/bin/env python3
"""A fake Anthropic + OpenAI API for running the provider examples in CI.

Serves just enough of the Messages API (POST /v1/messages) and Chat
Completions (POST /v1/chat/completions) to drive the examples' weather tool
loop: the first call asks for get_weather in Paris and Tokyo; once the
request carries both tool results (each must contain "temp_c"), it answers.
Malformed requests get a 400, so a broken example fails loudly.

  scripts/fake-llm-server.py PORT
  ANTHROPIC_BASE_URL=http://127.0.0.1:PORT ANTHROPIC_API_KEY=fake ...
  OPENAI_BASE_URL=http://127.0.0.1:PORT/v1 OPENAI_API_KEY=fake ...
"""
import json
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

CITIES = ["Paris", "Tokyo"]
ANSWER = "Tokyo is warmer (19°C, clear) than Paris (14°C, light rain)."


def anthropic(req):
    msgs = req["messages"]
    if not req.get("tools") or not req.get("max_tokens"):
        raise ValueError("expected tools and max_tokens")
    results = [b for b in msgs[-1]["content"] if isinstance(b, dict) and b.get("type") == "tool_result"] \
        if isinstance(msgs[-1]["content"], list) else []
    base = {"id": f"msg_{len(msgs)}", "type": "message", "role": "assistant", "model": req["model"],
            "stop_sequence": None}
    if not results:
        return base | {"stop_reason": "tool_use", "usage": {"input_tokens": 412, "output_tokens": 61},
                       "content": [{"type": "text", "text": "Let me check both cities."}] +
                                  [{"type": "tool_use", "id": f"toolu_{i}", "name": "get_weather", "input": {"city": c}}
                                   for i, c in enumerate(CITIES)]}
    if len(results) != len(CITIES) or not all("temp_c" in json.dumps(r.get("content")) for r in results):
        raise ValueError(f"bad tool results: {results}")
    if msgs[-2]["role"] != "assistant":
        raise ValueError("assistant tool_use turn missing before tool results")
    return base | {"stop_reason": "end_turn", "usage": {"input_tokens": 530, "output_tokens": 22},
                   "content": [{"type": "text", "text": ANSWER}]}


def openai(req):
    msgs = req["messages"]
    if not req.get("tools"):
        raise ValueError("expected tools")
    tool_msgs = [m for m in msgs if m.get("role") == "tool"]
    base = {"id": f"chatcmpl-{len(msgs)}", "object": "chat.completion", "created": int(time.time()),
            "model": req["model"]}
    if not tool_msgs:
        calls = [{"id": f"call_{i}", "type": "function",
                  "function": {"name": "get_weather", "arguments": json.dumps({"city": c})}}
                 for i, c in enumerate(CITIES)]
        return base | {"choices": [{"index": 0, "finish_reason": "tool_calls",
                                    "message": {"role": "assistant", "content": None, "tool_calls": calls}}],
                       "usage": {"prompt_tokens": 98, "completion_tokens": 44, "total_tokens": 142}}
    if len(tool_msgs) != len(CITIES) or not all("temp_c" in str(m.get("content")) for m in tool_msgs):
        raise ValueError(f"bad tool messages: {tool_msgs}")
    if {m.get("tool_call_id") for m in tool_msgs} != {"call_0", "call_1"}:
        raise ValueError("tool messages must answer call_0 and call_1")
    return base | {"choices": [{"index": 0, "finish_reason": "stop",
                                "message": {"role": "assistant", "content": ANSWER}}],
                   "usage": {"prompt_tokens": 180, "completion_tokens": 20, "total_tokens": 200}}


ROUTES = {"/v1/messages": anthropic, "/v1/chat/completions": openai}


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        route = ROUTES.get(self.path.split("?")[0])
        if route is None:
            return self.reply(404, {"error": {"type": "not_found", "message": self.path}})
        try:
            req = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))))
            return self.reply(200, route(req))
        except (ValueError, KeyError, TypeError, IndexError) as e:
            return self.reply(400, {"type": "error", "error": {"type": "invalid_request_error", "message": str(e)}})

    def reply(self, code, body):
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):
        sys.stderr.write("fake-llm: " + fmt % args + "\n")


if __name__ == "__main__":
    ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
