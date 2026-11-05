"""The smallest useful trace: one agent run with one (pretend) LLM call
inside it. Start here.

    make quickstart                          # once, in the repo root
    pip install conduit-observability
    python examples/python/hello_trace.py
"""
import conduit_observability as conduit
from conduit_observability import LLMCall

import _quickstart

_quickstart.configure()  # your own code: conduit.configure(), reading CONDUIT_* variables

# A span is one unit of work; spans opened inside it become its children.
# Each span is sent when its block exits.
with conduit.span("hello.run") as run:
    run.set_attr("example", "hello-trace")
    with conduit.span("hello.llm") as llm:
        llm.record_llm_call(LLMCall(model="example-model", prompt="Say hello",
                                    completion="Hello from Conduit!", input_tokens=3, output_tokens=4))
        llm.set_ok()
    run.set_ok()
conduit.shutdown()

print("Sent trace", run.trace_id)
print("View it:  ", _quickstart.trace_url(run.trace_id))
