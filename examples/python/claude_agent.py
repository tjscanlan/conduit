"""The tool_agent.py loop against Claude, using the official Anthropic SDK.
Each Messages API call becomes an llm.call span with the model's real token
usage.

    pip install conduit-observability anthropic
    export ANTHROPIC_API_KEY=...             # required
    export ANTHROPIC_MODEL=...               # optional, default claude-sonnet-5
    python examples/python/claude_agent.py   # after `make quickstart`
"""
import json
import os
import sys

from conduit_observability import LLMCall, estimate_cost_usd

import _quickstart
import _weather

if not os.environ.get("ANTHROPIC_API_KEY"):
    sys.exit("Set ANTHROPIC_API_KEY to run this example (or try tool_agent.py, which needs no key).")

import anthropic  # noqa: E402  (after the key check, so the hint shows even if it isn't installed)

MODEL = os.environ.get("ANTHROPIC_MODEL", "claude-sonnet-5")
TOOLS = [{"name": _weather.NAME, "description": _weather.DESCRIPTION, "input_schema": _weather.SCHEMA}]

claude = anthropic.Anthropic()  # reads ANTHROPIC_API_KEY (and ANTHROPIC_BASE_URL)

with _quickstart.client() as client:
    with client.span("agent.run", raise_errors=True) as run:
        run.set_attr("example", "claude-agent").set_attr("question", _weather.QUESTION)
        messages = [{"role": "user", "content": _weather.QUESTION}]
        for turn in range(1, 6):
            with client.span("llm.call") as llm:
                llm.set_attr("turn", turn)
                msg = claude.messages.create(model=MODEL, max_tokens=1024, tools=TOOLS, messages=messages)
                text = "".join(b.text for b in msg.content if b.type == "text")
                uses = [b for b in msg.content if b.type == "tool_use"]
                calls = ", ".join(f"{b.name}({json.dumps(b.input)})" for b in uses)
                llm.record_llm_call(LLMCall(
                    model=msg.model,
                    prompt=json.dumps(messages[-1]["content"], default=str),
                    completion=f"{text}\ntool calls: {calls}".strip() if uses else text,
                    input_tokens=msg.usage.input_tokens,
                    output_tokens=msg.usage.output_tokens,
                    cost_usd=estimate_cost_usd(msg.model, msg.usage.input_tokens, msg.usage.output_tokens),
                ))
                llm.set_attr("stop_reason", msg.stop_reason).set_ok()
            if msg.stop_reason != "tool_use":
                print("Answer:", text)
                run.set_ok()
                break
            # Tools run after the llm.call span closes, each in its own span.
            results = []
            for use in uses:
                with client.span(f"tool.{use.name}") as tool:
                    tool.set_attr("tool.input", json.dumps(use.input))
                    try:
                        content, is_error = _weather.call(use.name, use.input), False
                        tool.set_ok()
                    except _weather.ToolError as err:
                        content, is_error = str(err), True
                        tool.set_error(err)
                results.append({"type": "tool_result", "tool_use_id": use.id, "content": content, "is_error": is_error})
            messages += [{"role": "assistant", "content": msg.content}, {"role": "user", "content": results}]
        else:
            run.set_error("no answer after 5 turns")

print("View it:", _quickstart.trace_url(run.trace_id))
