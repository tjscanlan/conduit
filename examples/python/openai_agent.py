"""The tool_agent.py loop against OpenAI, using the official OpenAI SDK (Chat
Completions). Each model call becomes an llm.call span with the real token
usage, via llm_call_from_openai() (which also reads Responses API results).

    pip install -e "sdk/python[openai]"        # the SDK from this checkout, plus the openai SDK
    export OPENAI_API_KEY=...                # required
    export OPENAI_MODEL=...                  # optional, default gpt-5-mini
    python examples/python/openai_agent.py   # after `make quickstart`
"""
import json
import os
import sys

import conduit_observability as conduit

import _quickstart
import _weather

if not os.environ.get("OPENAI_API_KEY"):
    sys.exit("Set OPENAI_API_KEY to run this example (or try tool_agent.py, which needs no key).")

import openai  # noqa: E402  (after the key check, so the hint shows even if it isn't installed)

MODEL = os.environ.get("OPENAI_MODEL", "gpt-5-mini")
TOOLS = [{"type": "function",
          "function": {"name": _weather.NAME, "description": _weather.DESCRIPTION, "parameters": _weather.SCHEMA}}]

oai = openai.OpenAI()  # reads OPENAI_API_KEY (and OPENAI_BASE_URL)


@conduit.traced("tool.get_weather")
def run_tool(tc) -> dict:
    conduit.current_span().set_attr("tool.input", tc.function.arguments)
    try:
        content = _weather.call(tc.function.name, json.loads(tc.function.arguments))
    except (_weather.ToolError, json.JSONDecodeError) as err:
        conduit.current_span().set_error(err)
        content = f"error: {err}"
    return {"role": "tool", "tool_call_id": tc.id, "content": content}


_quickstart.configure()
with conduit.span("agent.run") as run:
    run.set_attr("example", "openai-agent").set_attr("question", _weather.QUESTION)
    messages = [{"role": "user", "content": _weather.QUESTION}]
    for turn in range(1, 6):
        with conduit.span("llm.call") as llm:
            llm.set_attr("turn", turn)
            resp = oai.chat.completions.create(model=MODEL, messages=messages, tools=TOOLS)
            # Model, completion (text and tool calls), real token usage, estimated cost.
            llm.record_llm_call(conduit.llm_call_from_openai(resp, prompt=json.dumps(messages[-1], default=str)))
            llm.set_attr("finish_reason", resp.choices[0].finish_reason).set_ok()
        msg = resp.choices[0].message
        if not msg.tool_calls:
            print("Answer:", msg.content)
            run.set_ok()
            break
        # Tools run after the llm.call span closes, each in its own span.
        messages.append(msg.model_dump(exclude_none=True))
        messages += [run_tool(tc) for tc in msg.tool_calls]
    else:
        run.set_error("no answer after 5 turns")
conduit.shutdown()

print("View it:", _quickstart.trace_url(run.trace_id))
