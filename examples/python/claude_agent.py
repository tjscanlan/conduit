"""The tool_agent.py loop against Claude, using the official Anthropic SDK.
Each Messages API call becomes an llm.call span with the model's real token
usage, via llm_call_from_anthropic().

    pip install "conduit-observability[anthropic]"
    export ANTHROPIC_API_KEY=...             # required
    export ANTHROPIC_MODEL=...               # optional, default claude-sonnet-5
    python examples/python/claude_agent.py   # after `make quickstart`
"""
import json
import os
import sys

import conduit_observability as conduit

import _quickstart
import _weather

if not os.environ.get("ANTHROPIC_API_KEY"):
    sys.exit("Set ANTHROPIC_API_KEY to run this example (or try tool_agent.py, which needs no key).")

import anthropic  # noqa: E402  (after the key check, so the hint shows even if it isn't installed)

MODEL = os.environ.get("ANTHROPIC_MODEL", "claude-sonnet-5")
TOOLS = [{"name": _weather.NAME, "description": _weather.DESCRIPTION, "input_schema": _weather.SCHEMA}]

claude = anthropic.Anthropic()  # reads ANTHROPIC_API_KEY (and ANTHROPIC_BASE_URL)


@conduit.traced("tool.get_weather")
def run_tool(use) -> dict:
    conduit.current_span().set_attr("tool.input", json.dumps(use.input))
    try:
        return {"type": "tool_result", "tool_use_id": use.id, "content": _weather.call(use.name, use.input)}
    except _weather.ToolError as err:
        conduit.current_span().set_error(err)
        return {"type": "tool_result", "tool_use_id": use.id, "content": str(err), "is_error": True}


_quickstart.configure()
with conduit.span("agent.run") as run:
    run.set_attr("example", "claude-agent").set_attr("question", _weather.QUESTION)
    messages = [{"role": "user", "content": _weather.QUESTION}]
    for turn in range(1, 6):
        with conduit.span("llm.call") as llm:
            llm.set_attr("turn", turn)
            msg = claude.messages.create(model=MODEL, max_tokens=1024, tools=TOOLS, messages=messages)
            # Model, completion (text and tool calls), real token usage, estimated cost.
            llm.record_llm_call(conduit.llm_call_from_anthropic(msg, prompt=json.dumps(messages[-1]["content"], default=str)))
            llm.set_attr("stop_reason", msg.stop_reason).set_ok()
        if msg.stop_reason != "tool_use":
            print("Answer:", "".join(b.text for b in msg.content if b.type == "text"))
            run.set_ok()
            break
        # Tools run after the llm.call span closes, each in its own span.
        results = [run_tool(b) for b in msg.content if b.type == "tool_use"]
        messages += [{"role": "assistant", "content": msg.content}, {"role": "user", "content": results}]
    else:
        run.set_error("no answer after 5 turns")
conduit.shutdown()

print("View it:", _quickstart.trace_url(run.trace_id))
