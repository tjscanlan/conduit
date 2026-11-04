# conduit-observability

The Python SDK for [Conduit](https://tjscanlan.github.io/conduit/), secure, self-hosted observability for AI agents. It sends **traces, logs and metrics** from your agent to a Conduit gateway over gRPC with mutual TLS.

```bash
pip install conduit-observability
```

Requires Python 3.10 or newer, and a running Conduit gateway with a client certificate for your agent. See [Deployment](https://tjscanlan.github.io/conduit/deployment.html).

## Quick start

Point it at your agent's certificates. A local stack from Conduit's
`make quickstart` writes them to `deployments/quickstart/.conduit/certs`:

```bash
export CONDUIT_CERT_DIR=/path/to/certs CONDUIT_AGENT=my-agent CONDUIT_GATEWAY_ADDR=localhost:4317
```

```python
import conduit_observability as conduit

conduit.configure()                       # from CONDUIT_* variables; errors say what to fix

@conduit.traced()                         # a span per call; errors are recorded
def search(query): ...

with conduit.span("agent.run") as run:
    search("conduit")                     # nested under agent.run
    with conduit.span("llm.call") as llm:
        msg = anthropic_client.messages.create(...)
        llm.record_llm_call(conduit.llm_call_from_anthropic(msg))   # real token usage
    run.set_ok()
```

For explicit control, use a `Client`: `Client(addr, tls_cert=..., tls_key=..., tls_ca=...)`
with `client.span(...)`, `client.log(...)`, `client.gauge(...)` and `client.check()`.

## What you get

- **Two-line setup:** `configure()` reads a certificate directory from the environment and `@traced` instruments functions, sync or async. Misconfiguration fails with an error that says what to fix.
- **Real LLM usage:** `llm_call_from_anthropic` / `llm_call_from_openai` record the model, tokens and cost from a provider response (install the `[anthropic]` or `[openai]` extra for the SDKs).
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
