"""A tool-calling agent loop with a simulated model, so it runs without an API
key. It shows the span layout Conduit expects from an agent:

    agent.run
    ├── llm.call          (model asks for tools; tokens recorded)
    ├── tool.get_weather
    ├── tool.get_weather
    └── llm.call          (model answers)

claude_agent.py and openai_agent.py are the same loop against real models.

    python examples/python/tool_agent.py     # after `make quickstart`
"""
import json
import time

import conduit_observability as conduit
from conduit_observability import LLMCall

import _quickstart
import _weather


def simulated_model(tool_results):
    """Stands in for an LLM: first asks for two cities' weather, then answers."""
    time.sleep(0.15)  # pretend latency
    if not tool_results:
        return [("get_weather", {"city": "Paris"}), ("get_weather", {"city": "Tokyo"})], "", 180, 42
    return [], "Tokyo is warmer: 19°C and clear, versus 14°C and light rain in Paris.", 260, 24


# @traced wraps each call in a span: OK on return, an error if it raises.
@conduit.traced("tool.get_weather")
def get_weather(args: dict) -> str:
    conduit.current_span().set_attr("tool.input", json.dumps(args))
    return _weather.call("get_weather", args)


_quickstart.configure()
with conduit.span("agent.run") as run:
    run.set_attr("example", "tool-agent").set_attr("question", _weather.QUESTION)
    results = []
    for turn in range(1, 6):
        # One span per model call, with what was sent and what came back.
        with conduit.span("llm.call") as llm:
            calls, answer, tokens_in, tokens_out = simulated_model(results)
            llm.set_attr("turn", turn)
            llm.record_llm_call(LLMCall(model="simulated", prompt=_weather.QUESTION,
                                        completion=answer or f"tool calls: {json.dumps(calls)}",
                                        input_tokens=tokens_in, output_tokens=tokens_out))
            llm.set_ok()
        if not calls:
            print("Answer:", answer)
            break
        for _name, args in calls:
            try:
                results.append(get_weather(args))
            except _weather.ToolError as err:  # the tool's span is already marked as an error
                results.append(f"error: {err}")
    run.set_ok()
conduit.shutdown()

print("View it:", _quickstart.trace_url(run.trace_id))
