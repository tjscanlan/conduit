// tool-agent is a tool-calling agent loop with a simulated model, so it runs
// without an API key. It shows the span layout Conduit expects from an agent:
//
//	agent.run
//	├── llm.call        (model asks for tools; tokens and cost recorded)
//	├── tool.get_weather
//	├── tool.get_weather
//	└── llm.call        (model answers)
//
// claude-agent and openai-agent are the same loop against real models.
//
//	go run ./tool-agent              # from examples/go, after `make quickstart`
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tjscanlan/conduit/examples/go/internal/quickstart"
	"github.com/tjscanlan/conduit/examples/go/internal/weather"
	sdk "github.com/tjscanlan/conduit/sdk/go"
)

type toolCall struct {
	Name  string
	Input json.RawMessage
}

// simulatedModel stands in for an LLM: first it asks for the weather in two
// cities, then, given the results, it answers.
func simulatedModel(toolResults []string) (calls []toolCall, answer string, in, out int32) {
	time.Sleep(150 * time.Millisecond) // pretend latency
	if len(toolResults) == 0 {
		return []toolCall{
			{weather.Name, json.RawMessage(`{"city":"Paris"}`)},
			{weather.Name, json.RawMessage(`{"city":"Tokyo"}`)},
		}, "", 180, 42
	}
	return nil, "Tokyo is warmer: 19°C and clear, versus 14°C and light rain in Paris.", 260, 24
}

func main() {
	client, err := quickstart.Client()
	if err != nil {
		quickstart.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()

	run, ctx := client.StartSpan(ctx, "agent.run")
	run.SetAttr("example", "tool-agent").SetAttr("question", weather.Question)

	var results []string
	for turn := 1; ; turn++ {
		// One span per model call, with what was sent and what came back.
		llm, _ := client.StartSpan(ctx, "llm.call")
		calls, answer, in, out := simulatedModel(results)
		completion := answer
		if len(calls) > 0 {
			b, _ := json.Marshal(calls)
			completion = "tool calls: " + string(b)
		}
		llm.SetAttr("turn", fmt.Sprint(turn))
		llm.RecordLLMCall(sdk.LLMCall{Model: "simulated", Prompt: weather.Question, Completion: completion, InputTokens: in, OutputTokens: out})
		llm.SetOK()
		if err := llm.End(ctx); err != nil {
			quickstart.Fatal(err)
		}

		if len(calls) == 0 {
			fmt.Println("Answer:", answer)
			break
		}
		// One span per tool call; a failing tool marks its span as an error.
		for _, call := range calls {
			tool, _ := client.StartSpan(ctx, "tool."+call.Name)
			tool.SetAttr("tool.input", string(call.Input))
			result, err := weather.Call(call.Input)
			if err != nil {
				tool.SetError(err)
				result = "error: " + err.Error()
			} else {
				tool.SetOK()
			}
			if err := tool.End(ctx); err != nil {
				quickstart.Fatal(err)
			}
			results = append(results, result)
		}
	}

	run.SetOK()
	if err := run.End(ctx); err != nil {
		quickstart.Fatal(err)
	}
	fmt.Println("View it:", quickstart.TraceURL(run.TraceID()))
}
