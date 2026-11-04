// hello-trace sends the smallest useful trace: one agent run with one
// (pretend) LLM call inside it. Start here.
//
//	make quickstart                  # once, in the repo root
//	go run ./hello-trace             # from examples/go
package main

import (
	"context"
	"fmt"

	"github.com/tjscanlan/conduit/examples/go/internal/quickstart"
	sdk "github.com/tjscanlan/conduit/sdk/go"
)

func main() {
	client, err := quickstart.Client()
	if err != nil {
		quickstart.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()

	// A span is one unit of work. Spans started from ctx become its children.
	run, ctx := client.StartSpan(ctx, "hello.run")
	run.SetAttr("example", "hello-trace")

	llm, _ := client.StartSpan(ctx, "hello.llm")
	llm.RecordLLMCall(sdk.LLMCall{
		Model:        "example-model",
		Prompt:       "Say hello",
		Completion:   "Hello from Conduit!",
		InputTokens:  3,
		OutputTokens: 4,
	})
	llm.SetOK()
	if err := llm.End(ctx); err != nil {
		quickstart.Fatal(err)
	}

	run.SetOK()
	if err := run.End(ctx); err != nil { // spans are sent when they end
		quickstart.Fatal(err)
	}
	fmt.Println("Sent trace", run.TraceID())
	fmt.Println("View it:  ", quickstart.TraceURL(run.TraceID()))
}
