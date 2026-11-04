// Command smoke-agent sends one known two-span trace through a Conduit
// gateway and prints its trace ID. scripts/smoke-test.sh uses it to check
// the whole pipeline end to end.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"time"

	sdk "github.com/tjscanlan/conduit/sdk/go"
)

func main() {
	addr := flag.String("addr", "localhost:4317", "gateway address")
	cert := flag.String("cert", "certs/agent-example.crt", "client certificate")
	key := flag.String("key", "certs/agent-example.key", "client key")
	ca := flag.String("ca", "certs/ca.crt", "CA that signed the gateway certificate")
	flag.Parse()

	c, err := sdk.New(sdk.Config{GatewayAddr: *addr, TLSCert: *cert, TLSKey: *key, TLSCACert: *ca})
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	root, ctx := c.StartSpan(ctx, "smoke.agent.run")
	// The gateway must overwrite this with the certificate's CommonName.
	root.SetAttr("smoke", "true").SetAttr("agent_name", "spoofed-name")

	child, cctx := c.StartSpan(ctx, "smoke.llm.call")
	if err := c.Log(cctx, sdk.SeverityWarn, "smoke: retrying tool call", map[string]string{"attempt": "2"}); err != nil {
		log.Fatalf("send log: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	child.RecordLLMCall(sdk.LLMCall{Model: "smoke-model", Prompt: "hi", Completion: "hello", InputTokens: 12, OutputTokens: 34, CostUSD: 0.0042})
	child.SetError(errors.New("tool call timed out"))
	if err := child.End(ctx); err != nil {
		log.Fatalf("send child span: %v", err)
	}
	if err := c.Gauge(ctx, "smoke_queue_depth", 7, map[string]string{"sdk": "go"}); err != nil {
		log.Fatalf("send metric: %v", err)
	}
	root.SetOK()
	if err := root.End(ctx); err != nil {
		log.Fatalf("send root span: %v", err)
	}
	fmt.Println(root.TraceID())
}
