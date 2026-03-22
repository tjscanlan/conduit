# Span and LLMCall are defined in sdk.py.
# This module re-exports them so users can import from either location:
#   from agent_observability.span import Span, LLMCall
#   from agent_observability import Span, LLMCall
from .sdk import Span, LLMCall

__all__ = ["Span", "LLMCall"]
