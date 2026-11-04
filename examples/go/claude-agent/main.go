// claude-agent is the tool-calling loop from tool-agent against Claude, using
// the official Anthropic Go SDK. Each Messages API call becomes an llm.call
// span carrying the model's real token usage.
//
//	export ANTHROPIC_API_KEY=...     # required
//	export ANTHROPIC_MODEL=...       # optional, default claude-sonnet-5
//	go run ./claude-agent            # from examples/go, after `make quickstart`
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tjscanlan/conduit/examples/go/internal/quickstart"
	"github.com/tjscanlan/conduit/examples/go/internal/weather"
	sdk "github.com/tjscanlan/conduit/sdk/go"
)

const maxTurns = 5

func main() {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "Set ANTHROPIC_API_KEY to run this example (or try `go run ./tool-agent`, which needs no key).")
		os.Exit(2)
	}
	model := os.Getenv("ANTHROPIC_MODEL")
	if model == "" {
		model = "claude-sonnet-5"
	}

	client, err := quickstart.Client()
	if err != nil {
		quickstart.Fatal(err)
	}
	defer client.Close()
	claude := anthropic.NewClient() // reads ANTHROPIC_API_KEY (and ANTHROPIC_BASE_URL)
	ctx := context.Background()

	run, ctx := client.StartSpan(ctx, "agent.run")
	run.SetAttr("example", "claude-agent").SetAttr("question", weather.Question)

	tools := []anthropic.ToolUnionParam{{OfTool: &anthropic.ToolParam{
		Name:        weather.Name,
		Description: anthropic.String(weather.Description),
		InputSchema: anthropic.ToolInputSchemaParam{Properties: weather.Schema["properties"], Required: []string{"city"}},
	}}}
	messages := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(weather.Question))}

	answer, err := loop(ctx, client, &claude, model, tools, messages)
	if err != nil {
		run.SetError(err)
	} else {
		run.SetOK()
		fmt.Println("Answer:", answer)
	}
	if endErr := run.End(ctx); endErr != nil {
		quickstart.Fatal(endErr)
	}
	fmt.Println("View it:", quickstart.TraceURL(run.TraceID()))
	if err != nil {
		quickstart.Fatal(err)
	}
}

func loop(ctx context.Context, client *sdk.Client, claude *anthropic.Client, model string,
	tools []anthropic.ToolUnionParam, messages []anthropic.MessageParam) (string, error) {
	for turn := 1; turn <= maxTurns; turn++ {
		llm, _ := client.StartSpan(ctx, "llm.call")
		llm.SetAttr("turn", fmt.Sprint(turn))
		msg, err := claude.Messages.New(ctx, anthropic.MessageNewParams{
			Model: anthropic.Model(model), MaxTokens: 1024, Messages: messages, Tools: tools,
		})
		if err != nil {
			llm.SetError(err)
			_ = llm.End(ctx)
			return "", err
		}

		var text strings.Builder
		var uses []anthropic.ToolUseBlock
		var calls []string
		for _, block := range msg.Content {
			switch b := block.AsAny().(type) {
			case anthropic.TextBlock:
				text.WriteString(b.Text)
			case anthropic.ToolUseBlock:
				uses = append(uses, b)
				calls = append(calls, fmt.Sprintf("%s(%s)", b.Name, b.Input))
			}
		}
		completion := text.String()
		if len(calls) > 0 {
			completion = strings.TrimSpace(completion + "\ntool calls: " + strings.Join(calls, ", "))
		}
		in, out := int32(msg.Usage.InputTokens), int32(msg.Usage.OutputTokens)
		llm.RecordLLMCall(sdk.LLMCall{
			Model: string(msg.Model), Prompt: lastUserText(messages), Completion: completion,
			InputTokens: in, OutputTokens: out, CostUSD: sdk.EstimateCostUSD(string(msg.Model), in, out),
		})
		llm.SetAttr("stop_reason", string(msg.StopReason)).SetOK()
		if err := llm.End(ctx); err != nil {
			return "", err
		}

		if msg.StopReason != anthropic.StopReasonToolUse {
			return text.String(), nil
		}
		// Run tools after the llm.call span ends, so its duration is the model's alone.
		var results []anthropic.ContentBlockParamUnion
		for _, b := range uses {
			results = append(results, runTool(ctx, client, b.ID, b.Name, b.Input))
		}
		messages = append(messages, msg.ToParam(), anthropic.NewUserMessage(results...))
	}
	return "", fmt.Errorf("no answer after %d turns", maxTurns)
}

// runTool executes one tool call in its own span and returns the result
// block to send back to Claude.
func runTool(ctx context.Context, client *sdk.Client, id, name string, input json.RawMessage) anthropic.ContentBlockParamUnion {
	tool, _ := client.StartSpan(ctx, "tool."+name)
	tool.SetAttr("tool.input", string(input))
	var result string
	var err error
	if name == weather.Name {
		result, err = weather.Call(input)
	} else {
		err = fmt.Errorf("unknown tool %q", name)
	}
	if err != nil {
		tool.SetError(err)
		result = err.Error()
	} else {
		tool.SetOK()
	}
	if endErr := tool.End(ctx); endErr != nil {
		fmt.Fprintln(os.Stderr, "warning: tool span not sent:", endErr)
	}
	return anthropic.NewToolResultBlock(id, result, err != nil)
}

func lastUserText(messages []anthropic.MessageParam) string {
	b, _ := json.Marshal(messages[len(messages)-1].Content)
	return string(b)
}
