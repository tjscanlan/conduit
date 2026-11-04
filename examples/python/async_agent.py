"""An asyncio agent that runs its tool calls concurrently.

Conduit's current span is tracked per asyncio task, so spans opened in
tasks started with asyncio.gather() still nest under the span that was
current when the tasks were created: both tool spans are children of
agent.run and overlap in time on the trace waterfall.

    python examples/python/async_agent.py    # after `make quickstart`
"""
import asyncio
import json
import random

from conduit_observability import LLMCall

import _quickstart
import _weather


async def get_weather(client, city: str) -> str:
    with client.span("tool.get_weather") as tool:
        tool.set_attr("tool.input", json.dumps({"city": city}))
        await asyncio.sleep(random.uniform(0.2, 0.4))  # pretend network call
        result = _weather.call("get_weather", {"city": city})
        tool.set_ok()
        return result


async def fake_llm(client, prompt: str, completion: str, tokens_in: int, tokens_out: int) -> None:
    with client.span("llm.call") as llm:
        await asyncio.sleep(0.15)
        llm.record_llm_call(LLMCall(model="simulated", prompt=prompt, completion=completion,
                                    input_tokens=tokens_in, output_tokens=tokens_out))
        llm.set_ok()


async def main() -> None:
    with _quickstart.client() as client:
        with client.span("agent.run", raise_errors=True) as run:
            run.set_attr("example", "async-agent").set_attr("question", _weather.QUESTION)
            await fake_llm(client, _weather.QUESTION, "tool calls: get_weather(Paris), get_weather(Tokyo)", 180, 42)
            # Both tool calls at once; each gets its own span under agent.run.
            paris, tokyo = await asyncio.gather(get_weather(client, "Paris"), get_weather(client, "Tokyo"))
            answer = "Tokyo is warmer." if json.loads(tokyo)["temp_c"] > json.loads(paris)["temp_c"] else "Paris is warmer."
            await fake_llm(client, f"Results: {paris} {tokyo}", answer, 260, 8)
            run.set_ok()
        print("Answer:", answer)
        print("View it:", _quickstart.trace_url(run.trace_id))


asyncio.run(main())
