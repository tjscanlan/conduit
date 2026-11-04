"""The tool_agent.py loop against OpenAI, using the official OpenAI SDK (Chat
Completions). Each model call becomes an llm.call span with the real token
usage.

    pip install conduit-observability openai
    export OPENAI_API_KEY=...                # required
    export OPENAI_MODEL=...                  # optional, default gpt-5-mini
    python examples/python/openai_agent.py   # after `make quickstart`
"""
import json
import os
import sys

from conduit_observability import LLMCall, estimate_cost_usd

import _quickstart
import _weather

if not os.environ.get("OPENAI_API_KEY"):
    sys.exit("Set OPENAI_API_KEY to run this example (or try tool_agent.py, which needs no key).")

import openai  # noqa: E402  (after the key check, so the hint shows even if it isn't installed)

MODEL = os.environ.get("OPENAI_MODEL", "gpt-5-mini")
TOOLS = [{"type": "function",
          "function": {"name": _weather.NAME, "description": _weather.DESCRIPTION, "parameters": _weather.SCHEMA}}]

oai = openai.OpenAI()  # reads OPENAI_API_KEY (and OPENAI_BASE_URL)

with _quickstart.client() as client:
    with client.span("agent.run", raise_errors=True) as run:
        run.set_attr("example", "openai-agent").set_attr("question", _weather.QUESTION)
        messages = [{"role": "user", "content": _weather.QUESTION}]
        for turn in range(1, 6):
            with client.span("llm.call") as llm:
                llm.set_attr("turn", turn)
                resp = oai.chat.completions.create(model=MODEL, messages=messages, tools=TOOLS)
                choice = resp.choices[0]
                msg = choice.message
                calls = ", ".join(f"{tc.function.name}({tc.function.arguments})" for tc in msg.tool_calls or [])
                llm.record_llm_call(LLMCall(
                    model=resp.model,
                    prompt=json.dumps(messages[-1], default=str),
                    completion=f"{msg.content or ''}\ntool calls: {calls}".strip() if calls else (msg.content or ""),
                    input_tokens=resp.usage.prompt_tokens,
                    output_tokens=resp.usage.completion_tokens,
                    cost_usd=estimate_cost_usd(resp.model, resp.usage.prompt_tokens, resp.usage.completion_tokens),
                ))
                llm.set_attr("finish_reason", choice.finish_reason).set_ok()
            if not msg.tool_calls:
                print("Answer:", msg.content)
                run.set_ok()
                break
            # Tools run after the llm.call span closes, each in its own span.
            messages.append(msg.model_dump(exclude_none=True))
            for tc in msg.tool_calls:
                with client.span(f"tool.{tc.function.name}") as tool:
                    tool.set_attr("tool.input", tc.function.arguments)
                    try:
                        content = _weather.call(tc.function.name, json.loads(tc.function.arguments))
                        tool.set_ok()
                    except (_weather.ToolError, json.JSONDecodeError) as err:
                        content = f"error: {err}"
                        tool.set_error(err)
                messages.append({"role": "tool", "tool_call_id": tc.id, "content": content})
        else:
            run.set_error("no answer after 5 turns")

print("View it:", _quickstart.trace_url(run.trace_id))
