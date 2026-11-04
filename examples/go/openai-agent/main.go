// openai-agent is the tool-calling loop from tool-agent against OpenAI, using
// the official OpenAI Go SDK (Chat Completions). Each model call becomes an
// llm.call span carrying the real token usage.
//
//	export OPENAI_API_KEY=...        # required
//	export OPENAI_MODEL=...          # optional, default gpt-5-mini
//	go run ./openai-agent            # from examples/go, after `make quickstart`
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"

	"github.com/tjscanlan/conduit/examples/go/internal/quickstart"
	"github.com/tjscanlan/conduit/examples/go/internal/weather"
	sdk "github.com/tjscanlan/conduit/sdk/go"
)

const maxTurns = 5

func main() {
	if os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Fprintln(os.Stderr, "Set OPENAI_API_KEY to run this example (or try `go run ./tool-agent`, which needs no key).")
		os.Exit(2)
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-5-mini"
	}

	client, err := quickstart.Client()
	if err != nil {
		quickstart.Fatal(err)
	}
	defer client.Close()
	oai := openai.NewClient() // reads OPENAI_API_KEY (and OPENAI_BASE_URL)
	ctx := context.Background()

	run, ctx := client.StartSpan(ctx, "agent.run")
	run.SetAttr("example", "openai-agent").SetAttr("question", weather.Question)

	params := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage(weather.Question)},
		Tools: []openai.ChatCompletionToolUnionParam{openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
			Name:        weather.Name,
			Description: openai.String(weather.Description),
			Parameters:  openai.FunctionParameters(weather.Schema),
		})},
	}

	answer, err := loop(ctx, client, &oai, params)
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

func loop(ctx context.Context, client *sdk.Client, oai *openai.Client, params openai.ChatCompletionNewParams) (string, error) {
	for turn := 1; turn <= maxTurns; turn++ {
		llm, _ := client.StartSpan(ctx, "llm.call")
		llm.SetAttr("turn", fmt.Sprint(turn))
		resp, err := oai.Chat.Completions.New(ctx, params)
		if err == nil && len(resp.Choices) == 0 {
			err = fmt.Errorf("model returned no choices")
		}
		if err != nil {
			llm.SetError(err)
			_ = llm.End(ctx)
			return "", err
		}
		msg := resp.Choices[0].Message

		var calls []string
		for _, tc := range msg.ToolCalls {
			calls = append(calls, fmt.Sprintf("%s(%s)", tc.Function.Name, tc.Function.Arguments))
		}
		completion := msg.Content
		if len(calls) > 0 {
			completion = strings.TrimSpace(completion + "\ntool calls: " + strings.Join(calls, ", "))
		}
		in, out := int32(resp.Usage.PromptTokens), int32(resp.Usage.CompletionTokens)
		llm.RecordLLMCall(sdk.LLMCall{
			Model: resp.Model, Prompt: lastMessage(params.Messages), Completion: completion,
			InputTokens: in, OutputTokens: out, CostUSD: sdk.EstimateCostUSD(resp.Model, in, out),
		})
		llm.SetAttr("finish_reason", resp.Choices[0].FinishReason).SetOK()
		if err := llm.End(ctx); err != nil {
			return "", err
		}

		if len(msg.ToolCalls) == 0 {
			return msg.Content, nil
		}
		params.Messages = append(params.Messages, msg.ToParam())
		for _, tc := range msg.ToolCalls {
			result := runTool(ctx, client, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			params.Messages = append(params.Messages, openai.ToolMessage(result, tc.ID))
		}
	}
	return "", fmt.Errorf("no answer after %d turns", maxTurns)
}

// runTool executes one tool call in its own span and returns the result to
// send back to the model.
func runTool(ctx context.Context, client *sdk.Client, name string, input json.RawMessage) string {
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
		result = "error: " + err.Error()
	} else {
		tool.SetOK()
	}
	if endErr := tool.End(ctx); endErr != nil {
		fmt.Fprintln(os.Stderr, "warning: tool span not sent:", endErr)
	}
	return result
}

func lastMessage(messages []openai.ChatCompletionMessageParamUnion) string {
	b, _ := json.Marshal(messages[len(messages)-1])
	return string(b)
}
