"""Conduit SDK: emit agent spans to the Conduit gateway over mTLS/gRPC.

Quickest start (after ``make quickstart``; see Client.from_env)::

    import conduit_observability as conduit

    conduit.configure()   # CONDUIT_CERT_DIR, CONDUIT_AGENT, CONDUIT_GATEWAY_ADDR

    @conduit.traced()
    def run_agent(task): ...
"""

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
    ConfigError,
    LLMCall,
    Span,
)
from .llm import estimate_cost_usd
from .providers import llm_call_from_anthropic, llm_call_from_openai
from .tracing import configure, current_span, get_client, shutdown, span, traced

__all__ = [
    "Client",
    "ConduitError",
    "ConfigError",
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
    "configure",
    "current_span",
    "estimate_cost_usd",
    "get_client",
    "llm_call_from_anthropic",
    "llm_call_from_openai",
    "shutdown",
    "span",
    "traced",
]
