package sdk

// EstimateCostUSD returns an approximate cost in USD based on model pricing (per 1M tokens).
// Prices are approximate — check provider pricing pages for accuracy.
func EstimateCostUSD(model string, inputTokens, outputTokens int32) float64 {
	// [input $/1M, output $/1M]
	prices := map[string][2]float64{
		"claude-opus-4-6":    {15.0, 75.0},
		"claude-sonnet-4-6":  {3.0, 15.0},
		"claude-haiku-4-5":   {0.80, 4.0},
		"gpt-4o":             {5.0, 15.0},
		"gpt-4o-mini":        {0.15, 0.60},
		"gemini-1.5-pro":     {7.0, 21.0},
		"gemini-1.5-flash":   {0.35, 1.05},
	}
	p, ok := prices[model]
	if !ok {
		return 0
	}
	return (float64(inputTokens)*p[0] + float64(outputTokens)*p[1]) / 1_000_000
}
