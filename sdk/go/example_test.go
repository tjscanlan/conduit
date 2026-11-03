package sdk_test

import (
	"context"
	"errors"
	"log"

	sdk "github.com/your-org/conduit-sdk-go"
)

// Mirrors the snippet in docs/sdk-go.md so the docs keep compiling.
func Example() {
	ctx := context.Background()
	client, err := sdk.New(sdk.Config{
		GatewayAddr: "localhost:4317",
		TLSCert:     "certs/research-bot.crt",
		TLSKey:      "certs/research-bot.key",
		TLSCACert:   "certs/ca.crt",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	span, ctx := client.StartSpan(ctx, "agent.run")
	span.SetAttr("task", "summarize")

	llm, _ := client.StartSpan(ctx, "llm.call")
	if err := errors.New("model timeout"); err != nil {
		llm.SetError(err)
	} else {
		llm.SetOK().RecordLLMCall(sdk.LLMCall{Model: "m", InputTokens: 10, OutputTokens: 20})
	}
	if err := llm.End(ctx); err != nil {
		log.Printf("span not delivered: %v", err)
	}

	span.SetOK()
	_ = span.End(ctx)
}
