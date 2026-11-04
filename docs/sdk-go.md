# Go SDK

`sdk/go` sends spans from a Go agent to the Conduit gateway over mTLS and
gRPC. It's a separate Go module, `github.com/tjscanlan/conduit/sdk/go`, and
it requires Go 1.26 or newer.

For runnable Go agents (hello-trace, tool calling, Claude, OpenAI) that
work against `make quickstart` with no setup, see [Examples](examples.md).

## 1. Get an agent certificate

Every agent authenticates with its own client certificate. The
certificate's CommonName is the agent's identity: the gateway rate-limits
by it and records it on every span as `agent_name`. Setting an attribute
can't change it.

For local development:

```bash
./scripts/gen-certs.sh                    # once: CA, gateway cert, example agent cert
./scripts/gen-certs.sh agent research-bot # one per agent
```

This writes `certs/research-bot.crt` and `certs/research-bot.key`. In
production, issue client certificates from your own PKI, signed by the CA
the gateway trusts (`TLS_CA`).

## 2. Add the module

```bash
go get github.com/tjscanlan/conduit/sdk/go@v0.1.0
```

The SDK lives in a subdirectory of the repo, so its tags are prefixed:
`sdk/go/v0.1.0`. It is released together with the main module, which provides
the generated proto package. To develop against a checkout instead:

```bash
go work init ./your-agent ./path/to/conduit/sdk/go ./path/to/conduit
```

## 3. Instrument

```go
import sdk "github.com/tjscanlan/conduit/sdk/go"

client, err := sdk.New(sdk.Config{
	GatewayAddr: "localhost:4317",          // host must match a SAN on the gateway cert
	TLSCert:     "certs/research-bot.crt",
	TLSKey:      "certs/research-bot.key",
	TLSCACert:   "certs/ca.crt",            // the only CA trusted for the gateway
})
if err != nil {
	log.Fatal(err)
}
defer client.Close()

span, ctx := client.StartSpan(ctx, "agent.run")
span.SetAttr("task", "summarize")

llm, _ := client.StartSpan(ctx, "llm.call") // child of agent.run: same trace, parent set
resp, err := callModel(ctx, prompt)
if err != nil {
	llm.SetError(err)
} else {
	llm.SetOK().RecordLLMCall(sdk.LLMCall{
		Model:        resp.Model,
		Prompt:       prompt,
		Completion:   resp.Text,
		InputTokens:  resp.InputTokens,
		OutputTokens: resp.OutputTokens,
		CostUSD:      resp.CostUSD,
	})
}
if err := llm.End(ctx); err != nil {
	log.Printf("span not delivered: %v", err)
}

span.SetOK()
_ = span.End(ctx)
```

### API

| Call | What it does |
|------|--------------|
| `sdk.New(cfg)` | Connects to the gateway over TLS 1.3. The gateway's certificate is verified against `TLSCACert` only, never the system roots. Returns an error if any certificate file is missing or invalid. |
| `client.StartSpan(ctx, name)` | Starts a span. If `ctx` carries a span from an earlier `StartSpan`, the new span joins its trace as a child; otherwise it starts a new trace. Returns the span and a context carrying it. |
| `span.SetAttr(k, v)` | Adds a string attribute. The gateway allows up to 128 attributes, keys up to 128 bytes and values up to 4096 bytes. A span over those limits is rejected. |
| `span.SetOK()` / `span.SetError(err)` | Sets the span status. `SetError` also stores `err.Error()` in `error.message`, cut to 4096 bytes. `SetError(nil)` does nothing. Spans you don't mark have status `unset`. |
| `span.RecordLLMCall(call)` | Attaches model, prompt, completion, token counts and cost. Prompts and completions are capped at 128 KiB each. |
| `span.End(ctx)` | Sends the span synchronously with one RPC. Returns an error if delivery fails or the gateway rejects the span, with the gateway's reason. |
| `client.Log(ctx, severity, body, attrs)` | Sends one log record (`sdk.SeverityDebug`, `Info`, `Warn` or `Error`). If `ctx` carries a span from `StartSpan`, the record is linked to that span and its trace. Returns an error if the gateway rejects it. Bodies are limited to 32 KiB. |
| `client.Gauge(ctx, name, value, labels)` / `client.Counter(...)` | Sends one metric sample to VictoriaMetrics. Names follow Prometheus rules, and names starting with `conduit_` or `__` are reserved. The `agent` label is always set from your certificate. Counters must be ≥ 0, and values must be finite. |
| `client.Close()` | Closes the connection. |

`sdk.EstimateCostUSD(model, in, out)` uses a small built-in price table
that will go out of date. Prefer the cost your provider reports.

### Rate limits

Each agent identity may send `RATE_LIMIT_RPS` spans per second (default
100), with bursts up to that number. Over the limit, `End` returns a
`ResourceExhausted` error. See [security.md](security.md#ingestion-rate-limiting).

### Things to know

- **Synchronous delivery:** `End` makes one RPC per span and blocks until
  the gateway answers. Call it off your hot path, or pass a context with a
  deadline.
- **No retries:** the SDK doesn't retry failed sends, so handle the error
  from `End` if delivery matters.
- **Agent name comes from the certificate:** `Config.AgentName` isn't sent.
  Identity always comes from the client certificate.
