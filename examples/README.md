# Conduit examples

Runnable Go and Python agents that send traces to a local Conduit.

```bash
make quickstart                        # once, from the repo root (needs only Docker)
cd examples/go && go run ./tool-agent  # or: python examples/python/tool_agent.py
```

| | Go | Python | Needs |
|---|---|---|---|
| Hello trace | `go run ./hello-trace` | `hello_trace.py` | nothing |
| Tool-calling agent (simulated model) | `go run ./tool-agent` | `tool_agent.py` | nothing |
| Async agent, concurrent tools | | `async_agent.py` | nothing |
| Claude agent (Anthropic SDK) | `go run ./claude-agent` | `claude_agent.py` | `ANTHROPIC_API_KEY` |
| OpenAI agent (OpenAI SDK) | `go run ./openai-agent` | `openai_agent.py` | `OPENAI_API_KEY` |

Full guide, including the recommended span layout:
[docs/examples.md](../docs/examples.md) ·
[online](https://tjscanlan.github.io/conduit/examples.html).
