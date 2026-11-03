"""Send one known two-span trace through a Conduit gateway and print its trace ID.

scripts/smoke-test.sh uses this to check the Python SDK end to end.
"""

import argparse

from conduit_sdk import Client, LLMCall


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--addr", default="localhost:4317")
    p.add_argument("--cert", default="certs/agent-example.crt")
    p.add_argument("--key", default="certs/agent-example.key")
    p.add_argument("--ca", default="certs/ca.crt")
    a = p.parse_args()

    with Client(a.addr, a.cert, a.key, a.ca) as client:
        with client.span("pysmoke.agent.run", raise_errors=True) as root:
            root.set_attr("sdk", "python")
            try:
                # An exception inside a span marks it as an error, then propagates.
                with client.span("pysmoke.tool.call", raise_errors=True) as tool:
                    tool.record_llm_call(LLMCall(model="py-model", prompt="hi", completion="",
                                                 input_tokens=7, output_tokens=0))
                    raise TimeoutError("tool call timed out")
            except TimeoutError:
                pass
            root.set_ok()
        print(root.trace_id)


if __name__ == "__main__":
    main()
