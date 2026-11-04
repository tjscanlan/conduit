"""Conduit SDK: emit agent spans to the Conduit gateway over mTLS/gRPC."""

from .client import (
    SEVERITY_DEBUG,
    SEVERITY_ERROR,
    SEVERITY_INFO,
    SEVERITY_UNSPECIFIED,
    SEVERITY_WARN,
    STATUS_ERROR,
    STATUS_OK,
    STATUS_UNSET,
    Client,
    ConduitError,
    LLMCall,
    Span,
)
from .llm import estimate_cost_usd

__all__ = [
    "Client",
    "ConduitError",
    "LLMCall",
    "Span",
    "SEVERITY_DEBUG",
    "SEVERITY_ERROR",
    "SEVERITY_INFO",
    "SEVERITY_UNSPECIFIED",
    "SEVERITY_WARN",
    "STATUS_ERROR",
    "STATUS_OK",
    "STATUS_UNSET",
    "estimate_cost_usd",
]
