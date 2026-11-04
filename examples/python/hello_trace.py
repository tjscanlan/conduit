"""The smallest useful trace: one agent run with one (pretend) LLM call
inside it. Start here.

    make quickstart                          # once, in the repo root
    pip install conduit-observability
    python examples/python/hello_trace.py
"""
from conduit_observability import LLMCall

import _quickstart

with _quickstart.client() as client:
    # A span is one unit of work; spans opened inside it become its children.
    # Each span is sent when its block exits.
    with client.span("hello.run", raise_errors=True) as run:
        run.set_attr("example", "hello-trace")
        with client.span("hello.llm", raise_errors=True) as llm:
            llm.record_llm_call(LLMCall(model="example-model", prompt="Say hello",
                                        completion="Hello from Conduit!", input_tokens=3, output_tokens=4))
            llm.set_ok()
        run.set_ok()

print("Sent trace", run.trace_id)
print("View it:  ", _quickstart.trace_url(run.trace_id))
