# Examples

Runnable agents in [`examples/`](https://github.com/tjscanlan/conduit/tree/main/examples),
in Go and Python. Each one sends a trace to a local Conduit and prints a link
to it in the UI. CI runs every example on every push, so they stay working.

## Run one in two commands

```bash
make quickstart                        # once: Conduit on localhost, needs only Docker
cd examples/go && go run ./tool-agent  # or: python examples/python/tool_agent.py
```

The examples find the certificates `make quickstart` generated and send as
agent `my-agent`. To point them somewhere else, set `CONDUIT_GATEWAY_ADDR`,
`CONDUIT_CERT_DIR` (a directory with `ca.crt` and `<agent>.crt`/`.key`), and
`CONDUIT_AGENT`.

For Python, install the SDK first: `pip install conduit-observability`, or
`pip install "conduit-observability[anthropic,openai]"` to include the Claude
and OpenAI SDKs. To try SDK changes from a checkout, use `pip install -e sdk/python`.

## The examples

| Example | Go (`examples/go`) | Python (`examples/python`) | Needs |
|---|---|---|---|
| **Hello trace.** The smallest useful trace: a run with one LLM call. Start here. | `go run ./hello-trace` | `hello_trace.py` | nothing |
| **Tool-calling agent.** A model/tool loop with a simulated model. | `go run ./tool-agent` | `tool_agent.py` | nothing |
| **Async agent.** Concurrent tool calls with `asyncio.gather`; spans still nest correctly. | | `async_agent.py` | nothing |
| **Claude agent.** The same loop with the official Anthropic SDK and real token usage. | `go run ./claude-agent` | `claude_agent.py` | `ANTHROPIC_API_KEY` |
| **OpenAI agent.** The same loop with the official OpenAI SDK (Chat Completions). | `go run ./openai-agent` | `openai_agent.py` | `OPENAI_API_KEY` |

The Claude examples default to `claude-sonnet-5` (override with
`ANTHROPIC_MODEL`). The OpenAI examples default to `gpt-5-mini` (override with
`OPENAI_MODEL`). Without a key, they say so and exit.

## What a tool-calling trace looks like

Every tool-calling example produces the same span tree. It's a good shape
to copy into your own agent:

```text
agent.run                 one per task; attributes: question, example
├── llm.call              one per model call: model, prompt, completion, tokens, cost
├── tool.get_weather      one per tool call: tool.input; error status if the tool failed
├── tool.get_weather
└── llm.call              the final answer
```

- **Tools run outside the `llm.call` span.** Each `llm.call` span ends before
  its tools run, so its duration is the model's latency alone.
- **Tool spans are children of the run, not of the model call.** On the
  waterfall, concurrent tool calls then show up side by side (see the async
  example).
- **Record the provider's real usage.** Take the token counts and model name
  from the API response (`usage.input_tokens` for Anthropic,
  `usage.prompt_tokens` for OpenAI). `EstimateCostUSD` / `estimate_cost_usd`
  turns them into an approximate cost for models it knows.

## How CI checks them without API keys

`scripts/examples-check.sh` runs every example against a running quickstart
and confirms each trace appears in the UI with the expected spans. The
Claude and OpenAI examples run against `scripts/fake-llm-server.py`, a
stdlib fake of the Messages and Chat Completions APIs. Both official SDKs
honor `ANTHROPIC_BASE_URL` and `OPENAI_BASE_URL`, so the examples' real
request building and response parsing are exercised. The fake rejects a
second turn that lacks correct tool results.
