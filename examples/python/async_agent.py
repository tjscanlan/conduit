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

import conduit_observability as conduit
from conduit_observability import LLMCall

import _quickstart
import _weather


@conduit.traced("tool.get_weather")  # works on async functions too
async def get_weather(city: str) -> str:
    conduit.current_span().set_attr("tool.input", json.dumps({"city": city}))
    await asyncio.sleep(random.uniform(0.2, 0.4))  # pretend network call
    return _weather.call("get_weather", {"city": city})


async def fake_llm(prompt: str, completion: str, tokens_in: int, tokens_out: int) -> None:
    with conduit.span("llm.call") as llm:
        await asyncio.sleep(0.15)
        llm.record_llm_call(LLMCall(model="simulated", prompt=prompt, completion=completion,
                                    input_tokens=tokens_in, output_tokens=tokens_out))
        llm.set_ok()


async def main() -> None:
    with conduit.span("agent.run") as run:
        run.set_attr("example", "async-agent").set_attr("question", _weather.QUESTION)
        await fake_llm(_weather.QUESTION, "tool calls: get_weather(Paris), get_weather(Tokyo)", 180, 42)
        # Both tool calls at once; each gets its own span under agent.run.
        paris, tokyo = await asyncio.gather(get_weather("Paris"), get_weather("Tokyo"))
        warmer = "Tokyo" if json.loads(tokyo)["temp_c"] > json.loads(paris)["temp_c"] else "Paris"
        await fake_llm(f"Results: {paris} {tokyo}", f"{warmer} is warmer.", 260, 8)
        run.set_ok()
    print(f"Answer: {warmer} is warmer.")
    print("View it:", _quickstart.trace_url(run.trace_id))


_quickstart.configure()
asyncio.run(main())
conduit.shutdown()
