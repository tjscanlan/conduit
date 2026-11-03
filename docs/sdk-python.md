# Python SDK

`sdk/python` (the `conduit-sdk` distribution, imported as `conduit_sdk`)
sends spans from a Python agent to the Conduit gateway over mTLS and gRPC.
It requires Python 3.10 or newer.

## 1. Get an agent certificate

The process is the same as for the Go SDK. The certificate's CommonName is
the agent's identity: the gateway rate-limits by it and records it on every
span as `agent_name`. See [sdk-go.md](sdk-go.md#1-get-an-agent-certificate).

```bash
./scripts/gen-certs.sh agent research-bot
```

## 2. Install

The SDK isn't on PyPI yet. Install it from a checkout:

```bash
pip install -e path/to/conduit/sdk/python
```

## 3. Instrument

```python
from conduit_sdk import Client, LLMCall

client = Client(
    "localhost:4317",                 # host must match a SAN on the gateway cert
    tls_cert="certs/research-bot.crt",
    tls_key="certs/research-bot.key",
    tls_ca="certs/ca.crt",            # the only CA trusted for the gateway
)
# or: Client.from_env() reads CONDUIT_GATEWAY_ADDR, TLS_CERT, TLS_KEY, TLS_CA

with client.span("agent.run") as run:
    run.set_attr("task", "summarize")

    with client.span("llm.call") as llm:        # child of agent.run
        resp = call_model(prompt)                # an exception here marks llm as an error
        llm.record_llm_call(LLMCall(
            model=resp.model, prompt=prompt, completion=resp.text,
            input_tokens=resp.input_tokens, output_tokens=resp.output_tokens,
            cost_usd=resp.cost_usd,
        ))
        llm.set_ok()

client.close()                                   # or use `with Client(...) as client:`
```

Spans opened with `client.span(...)` nest automatically. The current span
is tracked with `contextvars`, so nesting is correct per thread and per
asyncio task, and a single `Client` can be shared by the whole process.

### API

| Call | What it does |
|------|--------------|
| `Client(addr, tls_cert, tls_key, tls_ca, timeout=10.0)` | Opens a TLS channel to the gateway. The gateway's certificate is verified against `tls_ca` only. |
| `client.span(name, *, raise_errors=False)` | A context manager. It opens a child of the current span, makes it current, and ends and sends it on exit. If the block raises, the span is marked as an error with `error.message`, and the exception propagates. |
| `client.start_span(name)` | Starts a child of the current span without making it current. You call `end()` yourself. |
| `span.set_attr(k, v)` | Adds a string attribute. The gateway allows up to 128 attributes, keys up to 128 bytes and values up to 4096 bytes. A span over those limits is rejected. |
| `span.set_ok()` / `span.set_error(err)` | Sets the span status. `set_error` stores the message in `error.message`, cut to 4096 bytes. `set_error(None)` does nothing. |
| `span.record_llm_call(LLMCall(...))` | Attaches model, prompt, completion, token counts and cost. Prompts and completions are capped at 128 KiB each. |
| `span.end()` | Sends the span once; calling it again does nothing. Raises `ConduitError` if delivery fails or the gateway rejects the span. |
| `span.trace_id`, `span.span_id` | 32-hex and 16-hex IDs, for log correlation. |

### Delivery errors

- **`span.end()`** raises `ConduitError` when a span isn't accepted, for
  example if the gateway is unreachable, the agent is over its rate limit,
  or validation fails.
- **`with client.span(...)`** logs delivery failures to the `conduit_sdk`
  logger by default, so instrumentation can never crash your agent. Pass
  `raise_errors=True` to get `ConduitError` instead. Even then, an exception
  raised by your own code inside the block always wins and is never masked.

### Things to know

- **Synchronous delivery:** each span is one blocking RPC, sent when the
  span ends. Set `timeout` if the gateway might be slow.
- **No retries:** the SDK doesn't retry failed sends.
- **Identity comes from the certificate:** setting an `agent_name`
  attribute, or passing the legacy `agent_name` argument, doesn't change
  the agent's identity.
- **Price table:** `estimate_cost_usd(model, in, out)` uses a small
  built-in table that will go out of date. Prefer the cost your provider
  reports.

## Development

```bash
cd sdk/python
uv venv && uv pip install -e '.[dev]'    # or python -m venv / pip
.venv/bin/python -m pytest
```

The tests run against an in-process gRPC server that requires client
certificates, using certificates generated on the fly. The generated stubs
in `src/conduit_sdk/_proto` are committed. After editing `proto/*.proto`,
regenerate them with `make proto`, which also regenerates the Go code.
