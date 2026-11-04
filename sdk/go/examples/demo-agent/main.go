// Command demo-agent is the quickstart's built-in sample agent. It sends a
// steady stream of realistic, entirely simulated agent runs (planning LLM
// calls, tool calls, retries, the occasional failure) so a fresh Conduit
// install has data to explore before any real agent is instrumented.
//
// Nothing here calls a model: token counts, costs, and text are made up, and
// every span carries demo=true. The agent's identity is "demo-agent", taken
// by the gateway from its client certificate.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"os/signal"
	"syscall"
	"time"

	sdk "github.com/tjscanlan/conduit/sdk/go"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := flag.String("addr", env("CONDUIT_GATEWAY_ADDR", "localhost:4317"), "gateway address")
	certDir := flag.String("cert-dir", env("CONDUIT_CERT_DIR", "certs"), "directory with ca.crt and demo-agent.{crt,key}")
	interval := flag.Duration("interval", 8*time.Second, "time between simulated runs")
	runs := flag.Int("runs", 0, "stop after this many runs (0 = run until stopped)")
	flag.Parse()

	c, err := sdk.New(sdk.Config{
		GatewayAddr: *addr,
		TLSCert:     *certDir + "/demo-agent.crt",
		TLSKey:      *certDir + "/demo-agent.key",
		TLSCACert:   *certDir + "/ca.crt",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("demo-agent: sending a simulated run to %s every %s", *addr, *interval)
	for n := 1; *runs == 0 || n <= *runs; n++ {
		traceID, err := simulateRun(ctx, c, n)
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			// The gateway may still be starting; keep going and retry next tick.
			log.Printf("demo-agent: run %d: %v", n, err)
		default:
			log.Printf("demo-agent: run %d sent (trace %s)", n, traceID)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*interval):
		}
	}
}

type task struct {
	question string
	tools    []string
}

var tasks = []task{
	{"Summarize yesterday's failed deployments", []string{"search_logs", "fetch_runbook"}},
	{"Which customers hit the rate limit this week?", []string{"sql_query"}},
	{"Draft a reply to ticket #4821", []string{"fetch_ticket", "search_docs"}},
	{"Compare p95 latency before and after the cache change", []string{"query_metrics", "query_metrics"}},
	{"Find the owner of the billing-export job", []string{"search_docs", "lookup_owner"}},
}

var models = []string{"claude-sonnet-4-6", "claude-haiku-4-5", "gpt-4o-mini"}

// simulateRun emits one agent run: plan -> tools -> answer, as a span tree.
func simulateRun(ctx context.Context, c *sdk.Client, n int) (string, error) {
	t := tasks[rand.IntN(len(tasks))]
	model := models[rand.IntN(len(models))]

	root, rctx := c.StartSpan(ctx, "agent.run")
	root.SetAttr("demo", "true").SetAttr("task", t.question).SetAttr("run", fmt.Sprint(n))

	if err := llmCall(rctx, c, "llm.plan", model, "Plan the steps to answer: "+t.question,
		"1. Gather data with tools. 2. Summarize."); err != nil {
		return "", err
	}

	var failed error
	for _, tool := range t.tools {
		if err := toolCall(rctx, c, tool); err != nil {
			failed = err
			break
		}
	}

	if failed == nil {
		if err := llmCall(rctx, c, "llm.answer", model, "Answer using the tool results.",
			"Here is a short summary of what I found (simulated)."); err != nil {
			return "", err
		}
		root.SetOK()
	} else {
		root.SetError(failed)
	}
	if err := c.Gauge(rctx, "demo_agent_queue_depth", float64(rand.IntN(12)), map[string]string{"demo": "true"}); err != nil {
		return "", err
	}
	if err := c.Counter(rctx, "demo_agent_runs_total", 1, map[string]string{"demo": "true"}); err != nil {
		return "", err
	}
	return root.TraceID(), root.End(rctx)
}

func llmCall(ctx context.Context, c *sdk.Client, name, model, prompt, completion string) error {
	span, sctx := c.StartSpan(ctx, name)
	span.SetAttr("demo", "true")
	in, out := int32(400+rand.IntN(1600)), int32(60+rand.IntN(400))
	sleep(sctx, 300, 1400)
	span.RecordLLMCall(sdk.LLMCall{
		Model: model, Prompt: prompt, Completion: completion,
		InputTokens: in, OutputTokens: out, CostUSD: sdk.EstimateCostUSD(model, in, out),
	})
	span.SetOK()
	return span.End(ctx)
}

// toolCall fails about 1 run in 12, after a logged retry, so the UI has
// errors to show.
func toolCall(ctx context.Context, c *sdk.Client, tool string) error {
	span, sctx := c.StartSpan(ctx, "tool."+tool)
	span.SetAttr("demo", "true").SetAttr("tool.name", tool)
	sleep(sctx, 80, 600)
	var err error
	if rand.IntN(12) == 0 {
		if lerr := c.Log(sctx, sdk.SeverityWarn, tool+": upstream timed out, retrying", map[string]string{"demo": "true", "attempt": "2"}); lerr != nil {
			return lerr
		}
		sleep(sctx, 200, 500)
		err = errors.New(tool + ": upstream timed out after retry")
		span.SetError(err)
	} else {
		span.SetOK()
	}
	if endErr := span.End(ctx); endErr != nil {
		return endErr
	}
	return err
}

func sleep(ctx context.Context, minMS, maxMS int) {
	select {
	case <-ctx.Done():
	case <-time.After(time.Duration(minMS+rand.IntN(maxMS-minMS)) * time.Millisecond):
	}
}
