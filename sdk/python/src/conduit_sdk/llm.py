"""Cost estimation utilities for LLM calls."""

# Prices per 1M tokens: (input $/1M, output $/1M)
_PRICES: dict[str, tuple[float, float]] = {
    "claude-opus-4-6":   (15.0, 75.0),
    "claude-sonnet-4-6": (3.0, 15.0),
    "claude-haiku-4-5":  (0.80, 4.0),
    "gpt-4o":            (5.0, 15.0),
    "gpt-4o-mini":       (0.15, 0.60),
    "gemini-1.5-pro":    (7.0, 21.0),
    "gemini-1.5-flash":  (0.35, 1.05),
}


def estimate_cost_usd(model: str, input_tokens: int, output_tokens: int) -> float:
    """Estimate cost in USD from token counts and model pricing.

    Returns 0.0 for unknown models — add the model to _PRICES to fix.
    """
    prices = _PRICES.get(model)
    if prices is None:
        return 0.0
    input_price, output_price = prices
    return (input_tokens * input_price + output_tokens * output_price) / 1_000_000
