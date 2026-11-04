# Python SDK

`sdk/python` (the `conduit-observability` distribution, imported as `conduit_observability`)
sends spans from a Python agent to the Conduit gateway over mTLS and gRPC.
It requires Python 3.10 or newer.

For runnable Python agents (hello-trace, tool calling, Claude, OpenAI) that
work against `make quickstart` with no setup, see [Examples](examples.md).

## Quick start (2 minutes)

With a local stack from `make quickstart` (needs only Docker):

```bash
export CONDUIT_CERT_DIR=$PWD/deployments/quickstart/.conduit/certs CONDUIT_AGENT=my-agent
pip install conduit-observability     # 0.2.0 or newer
```

```python
import conduit_observability as conduit

conduit.configure()            # reads CONDUIT_CERT_DIR, CONDUIT_AGENT, CONDUIT_GATEWAY_ADDR
conduit.get_client().check()   # optional: fail fast if the gateway won't accept us

@conduit.traced()              # one span per call, named after the function
def search(query):
    ...

with conduit.span("agent.run") as run:
    search("conduit")          # its span is a child of agent.run
    run.set_ok()
conduit.shutdown()
```

Open <http://localhost:8080>: the trace appears under agent `my-agent`.
The [examples](examples.md) build on this: tool calls, asyncio, Claude, and OpenAI.

### Record real LLM usage

`llm_call_from_anthropic` and `llm_call_from_openai` turn a provider
response into an `LLMCall`. Each one captures the model, the text and tool
calls, the real token counts and an estimated cost. They read the response
by attribute, so they never import the provider SDKs. The `anthropic` and
`openai` extras install those SDKs for you if you want them.

```python
with conduit.span("llm.call") as llm:
    msg = anthropic_client.messages.create(model=..., max_tokens=1024, messages=messages)
    llm.record_llm_call(conduit.llm_call_from_anthropic(msg, prompt=user_text))
    llm.set_ok()
# OpenAI: conduit.llm_call_from_openai(resp) for Chat Completions or Responses API results
```

## Configuration

`conduit.configure()` and `Client.from_env()` read:

| Variable | Meaning |
|---|---|
| `CONDUIT_CERT_DIR` | Directory with `ca.crt` and `<agent>.crt`/`<agent>.key` (the layout `make quickstart` and `scripts/gen-certs.sh` produce). |
| `CONDUIT_AGENT` | Which agent's certificate to use from that directory. Optional when it holds exactly one agent (`ca` and `server` files are skipped). |
| `CONDUIT_TLS_CERT`, `CONDUIT_TLS_KEY`, `CONDUIT_TLS_CA` | Explicit file paths; they override `CONDUIT_CERT_DIR`. The 0.1.0 names `TLS_CERT`, `TLS_KEY`, `TLS_CA` still work. |
| `CONDUIT_GATEWAY_ADDR` | Gateway address, default `localhost:4317` (`OTEL_GATEWAY_ADDR` is accepted too). The host must be in the gateway certificate. |

To be explicit, call `conduit.configure(gateway_addr=..., tls_cert=..., tls_key=..., tls_ca=...)`
or pass a `Client`.

**Module API vs `Client`:** the module-level `span()` and `@traced` never
break your agent. If nothing can be configured, they become no-ops and log
one warning. Use a `Client` directly for explicit control, for example
`raise_errors=True`.

## When it doesn't connect

The SDK raises `ConfigError` (a `ConduitError`) with what to do when the
client is created:

- **Nothing configured.**
- **Several agents in the directory:** it lists them.
- **A missing file:** points at `make quickstart` / `scripts/gen-certs.sh`.
- **A key that belongs to another certificate.**
- **A CA file that isn't PEM.**

`check()` and every send add a hint to gRPC errors:

- the gateway isn't running or reachable;
- the host isn't in the gateway's certificate;
- a certificate verification or TLS handshake failure, which usually means
  the agent's certificate and the gateway's come from different CAs;
- rate limiting, timeouts.

## Production: agent certificates

The process is the same as for the Go SDK. The certificate's CommonName is
the agent's identity: the gateway rate-limits by it and records it on every
span as `agent_name`. See [sdk-go.md](sdk-go.md#production-agent-certificates).

```bash
./scripts/gen-certs.sh agent research-bot
```

## Install

```bash
pip install conduit-observability
```

Import it as `conduit_observability`.

## Instrument with a Client

```python
from conduit_observability import Client, LLMCall

client = Client(
    "localhost:4317",                 # host must match a SAN on the gateway cert
    tls_cert="certs/research-bot.crt",
    tls_key="certs/research-bot.key",
    tls_ca="certs/ca.crt",            # the only CA trusted for the gateway
)
# or: Client.from_env(); see Configuration

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
| `conduit.configure(client=None, **kwargs)` | Sets the module-level client (from the environment by default). `get_client()` returns it; `shutdown()` closes it. |
| `conduit.span(name)` / `@conduit.traced(name=None, **attrs)` | A span on the module-level client: a context manager, or a decorator for sync and async functions (OK on return, error if it raises). `current_span()` is the innermost open span. |
| `llm_call_from_anthropic(msg, prompt="")` / `llm_call_from_openai(resp, prompt="")` | An `LLMCall` from a provider response, with real usage and an estimated cost. |
| `Client.from_env(timeout=10.0)` / `client.check()` | Builds a client from the environment; checks the gateway accepts it (raises a diagnosed `ConduitError` if not). |
| `Client(addr, tls_cert, tls_key, tls_ca, timeout=10.0)` | Opens a TLS channel to the gateway. The gateway's certificate is verified against `tls_ca` only. |
| `client.span(name, *, raise_errors=False)` | A context manager. It opens a child of the current span, makes it current, and ends and sends it on exit. If the block raises, the span is marked as an error with `error.message`, and the exception propagates. |
| `client.start_span(name)` | Starts a child of the current span without making it current. You call `end()` yourself. |
| `span.set_attr(k, v)` | Adds a string attribute. The gateway allows up to 128 attributes, keys up to 128 bytes and values up to 4096 bytes. A span over those limits is rejected. |
| `span.set_ok()` / `span.set_error(err)` | Sets the span status. `set_error` stores the message in `error.message`, cut to 4096 bytes. `set_error(None)` does nothing. |
| `span.record_llm_call(LLMCall(...))` | Attaches model, prompt, completion, token counts and cost. Prompts and completions are capped at 128 KiB each. |
| `span.end()` | Sends the span once; calling it again does nothing. Raises `ConduitError` if delivery fails or the gateway rejects the span. |
| `client.log(severity, body, attributes=None)` | Sends one log record (`SEVERITY_DEBUG`, `SEVERITY_INFO`, `SEVERITY_WARN` or `SEVERITY_ERROR`). Inside `with client.span(...)` it's linked to that span and trace. Raises `ConduitError` if it's rejected. Bodies are limited to 32 KiB. |
| `client.gauge(name, value, labels=None)` / `client.counter(...)` | Sends one metric sample. The same naming rules apply as in the Go SDK: Prometheus names, `conduit_` and `__` reserved, and the `agent` label set from your certificate. |
| `span.trace_id`, `span.span_id` | 32-hex and 16-hex IDs, for log correlation. |

### Delivery errors

- **`span.end()`** raises `ConduitError` when a span isn't accepted, for
  example if the gateway is unreachable, the agent is over its rate limit,
  or validation fails.
- **`with client.span(...)`** logs delivery failures to the `conduit_observability`
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
in `src/conduit_observability/_proto` are committed. After editing `proto/*.proto`,
regenerate them with `make proto`, which also regenerates the Go code.
