package schema

// NewLLMCall builds an LLMCallData from raw field values.
func NewLLMCall(model, prompt, completion string, inputTokens, outputTokens int32, costUSD float64) *LLMCallData {
	return &LLMCallData{
		Model:        model,
		Prompt:       prompt,
		Completion:   completion,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
	}
}
