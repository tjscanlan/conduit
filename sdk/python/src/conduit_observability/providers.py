"""Build an LLMCall from an Anthropic or OpenAI response, with its real usage.

These read the response objects by attribute, so they don't import (or
require) the provider SDKs. Install them with the extras if you like:
``pip install "conduit-observability[anthropic]"`` or ``[openai]``.

    msg = anthropic_client.messages.create(...)
    span.record_llm_call(llm_call_from_anthropic(msg, prompt=user_text))

    resp = openai_client.chat.completions.create(...)   # or .responses.create(...)
    span.record_llm_call(llm_call_from_openai(resp, prompt=user_text))
"""
from __future__ import annotations

import json
from typing import Any

from .client import LLMCall
from .llm import estimate_cost_usd


def _get(obj: Any, name: str, default: Any = None) -> Any:
    if isinstance(obj, dict):
        return obj.get(name, default)
    return getattr(obj, name, default)


def llm_call_from_anthropic(message: Any, prompt: str = "") -> LLMCall:
    """From a Messages API response (``anthropic.types.Message``).

    The completion is the text blocks, followed by any tool calls as
    ``tool calls: name({...})``.
    """
    texts, calls = [], []
    for block in _get(message, "content") or []:
        kind = _get(block, "type")
        if kind == "text":
            texts.append(_get(block, "text", ""))
        elif kind == "tool_use":
            calls.append(f"{_get(block, 'name')}({json.dumps(_get(block, 'input'), default=str)})")
    usage = _get(message, "usage")
    model = _get(message, "model", "") or ""
    tokens_in, tokens_out = int(_get(usage, "input_tokens", 0) or 0), int(_get(usage, "output_tokens", 0) or 0)
    return LLMCall(model=model, prompt=prompt, completion=_completion(texts, calls),
                   input_tokens=tokens_in, output_tokens=tokens_out,
                   cost_usd=estimate_cost_usd(model, tokens_in, tokens_out))


def llm_call_from_openai(response: Any, prompt: str = "") -> LLMCall:
    """From a Chat Completions response, or a Responses API response."""
    texts, calls = [], []
    usage = _get(response, "usage")
    choices = _get(response, "choices")
    if choices:  # Chat Completions
        msg = _get(choices[0], "message")
        if _get(msg, "content"):
            texts.append(_get(msg, "content"))
        for tc in _get(msg, "tool_calls") or []:
            fn = _get(tc, "function")
            calls.append(f"{_get(fn, 'name')}({_get(fn, 'arguments')})")
        tokens_in, tokens_out = _get(usage, "prompt_tokens", 0), _get(usage, "completion_tokens", 0)
    else:  # Responses API
        for item in _get(response, "output") or []:
            if _get(item, "type") == "message":
                texts += [_get(c, "text", "") for c in _get(item, "content") or [] if _get(c, "type") == "output_text"]
            elif _get(item, "type") == "function_call":
                calls.append(f"{_get(item, 'name')}({_get(item, 'arguments')})")
        tokens_in, tokens_out = _get(usage, "input_tokens", 0), _get(usage, "output_tokens", 0)
    model = _get(response, "model", "") or ""
    tokens_in, tokens_out = int(tokens_in or 0), int(tokens_out or 0)
    return LLMCall(model=model, prompt=prompt, completion=_completion(texts, calls),
                   input_tokens=tokens_in, output_tokens=tokens_out,
                   cost_usd=estimate_cost_usd(model, tokens_in, tokens_out))


def _completion(texts: list, calls: list) -> str:
    text = "".join(texts)
    if calls:
        return f"{text}\ntool calls: {', '.join(calls)}".strip()
    return text
