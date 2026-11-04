# conduit-observability

The Python SDK for [Conduit](https://tjscanlan.github.io/conduit/), secure, self-hosted observability for AI agents. It sends **traces, logs and metrics** from your agent to a Conduit gateway over gRPC with mutual TLS.

```bash
pip install conduit-observability
```

Requires Python 3.10 or newer, and a running Conduit gateway with a client certificate for your agent. See [Deployment](https://tjscanlan.github.io/conduit/deployment.html).

## Quick start

```python
from conduit_observability import Client, LLMCall, SEVERITY_WARN

with Client(
    "gateway.example.com:4317",
    tls_cert="certs/research-bot.crt",   # your agent's client certificate
    tls_key="certs/research-bot.key",
    tls_ca="certs/ca.crt",               # the only CA trusted for the gateway
) as client:
    with client.span("agent.run") as run:
        run.set_attr("task", "summarize")

        with client.span("llm.call") as llm:          # nested: same trace
            resp = call_model(prompt)                  # an exception marks the span as an error
            llm.record_llm_call(LLMCall(model=resp.model, prompt=prompt, completion=resp.text,
                                        input_tokens=resp.input_tokens, output_tokens=resp.output_tokens))
            llm.set_ok()

        client.log(SEVERITY_WARN, "retrying tool call", {"attempt": 2})   # linked to agent.run
        client.gauge("queue_depth", 7, {"queue": "tasks"})
```

## What you get

- **Spans** that nest automatically. The current span is tracked with `contextvars`, so nesting is correct across threads and asyncio tasks.
- **Errors recorded for you:** an exception inside `with client.span(...)` marks the span as an error, records the message, and propagates.
- **Logs** (`client.log`) linked to the current span, and **metrics** (`client.gauge`, `client.counter`).
- **Strict TLS:** the gateway is verified against your CA only, never the system roots.
- **Identity from your certificate:** the gateway takes your agent's identity from the certificate's CommonName, so an attribute can't spoof it.

## Documentation

- [Python SDK guide](https://tjscanlan.github.io/conduit/sdk-python.html): the full API, limits and delivery semantics.
- [API reference](https://tjscanlan.github.io/conduit/api.html): the wire protocol and validation limits.
- [Source](https://github.com/tjscanlan/conduit/tree/main/sdk/python).

Licensed under Apache-2.0.
