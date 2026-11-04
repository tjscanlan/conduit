"""Module-level tracing: one shared client, ``@traced``, and ``span()``.

    import conduit_observability as conduit

    conduit.configure()                 # from the environment; see Client.from_env

    @conduit.traced()                   # a span per call, named after the function
    def plan(task): ...

    with conduit.span("agent.run") as run:
        run.set_attr("task", "summarize")
        plan("summarize")               # its span is a child of agent.run

Instrumentation never breaks the agent: if no client can be configured,
spans are no-ops and a warning is logged once.
"""
from __future__ import annotations

import functools
import inspect
import logging
import threading
from contextlib import contextmanager
from typing import Any, Callable, Iterator, Optional, TypeVar

from .client import Client, ConduitError, Span, _current

log = logging.getLogger("conduit_observability")

F = TypeVar("F", bound=Callable[..., Any])

_lock = threading.Lock()
_client: Optional[Client] = None
_tried_env = False


def configure(client: Optional[Client] = None, **kwargs: Any) -> Client:
    """Set the client used by ``span()`` and ``@traced``.

    With no arguments it is built from the environment (``Client.from_env``)
    and raises ConfigError, with what to set, if that fails. Pass a Client,
    or Client's arguments (gateway_addr=..., tls_cert=..., ...), to be explicit.
    """
    global _client, _tried_env
    new = client or (Client(**kwargs) if kwargs else Client.from_env())
    with _lock:
        old, _client, _tried_env = _client, new, True
    if old is not None and old is not new:
        old.close()
    return new


def get_client() -> Optional[Client]:
    """The configured client. On first use without ``configure()``, tries the
    environment once; returns None (and warns once) if that fails."""
    global _client, _tried_env
    with _lock:
        if _client is not None or _tried_env:
            return _client
        _tried_env = True
        try:
            _client = Client.from_env()
        except ConduitError as err:
            log.warning("conduit: tracing disabled: %s", err)
        return _client


def shutdown() -> None:
    """Close the configured client (spans after this are no-ops until configure())."""
    global _client
    with _lock:
        old, _client = _client, None
    if old is not None:
        old.close()


def current_span() -> Optional[Span]:
    """The span opened by the innermost ``span()`` / ``@traced`` in this thread or task."""
    return _current.get()


@contextmanager
def span(name: str) -> Iterator[Span]:
    """Open a span on the configured client; a no-op span if there is none.

    Like ``Client.span``: an exception marks the span as an error and is
    re-raised; delivery failures are logged, not raised.
    """
    client = get_client()
    if client is None:
        s = Span(trace_id="", span_id="", parent_id="", name=name, start_ns=0)
        yield s  # unsent: no client
        return
    with client.span(name) as s:
        yield s


def traced(name: Optional[str] = None, **attrs: str) -> Callable[[F], F]:
    """Decorate a function (sync or async) to run inside a span.

    The span is named ``name`` or the function's qualified name, gets
    ``attrs``, is marked OK on return and as an error if the function raises.
    """
    def decorate(fn: F) -> F:
        span_name = name or fn.__qualname__

        if inspect.iscoroutinefunction(fn):
            @functools.wraps(fn)
            async def async_wrapper(*args, **kwargs):
                with span(span_name) as s:
                    for k, v in attrs.items():
                        s.set_attr(k, v)
                    result = await fn(*args, **kwargs)
                    s.set_ok()
                    return result
            return async_wrapper  # type: ignore[return-value]

        @functools.wraps(fn)
        def wrapper(*args, **kwargs):
            with span(span_name) as s:
                for k, v in attrs.items():
                    s.set_attr(k, v)
                result = fn(*args, **kwargs)
                s.set_ok()
                return result
        return wrapper  # type: ignore[return-value]

    return decorate
