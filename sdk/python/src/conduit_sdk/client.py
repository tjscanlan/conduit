"""Conduit SDK for Python agents: send spans to the gateway over mTLS/gRPC.

Usage::

    from conduit_sdk import Client, LLMCall

    with Client.from_env() as client:
        with client.span("agent.run") as run:
            run.set_attr("task", "summarize")
            with client.span("llm.call") as llm:      # child of agent.run
                resp = call_model(prompt)
                llm.record_llm_call(LLMCall(model=resp.model, prompt=prompt,
                                            completion=resp.text,
                                            input_tokens=resp.in_tokens,
                                            output_tokens=resp.out_tokens))

An exception raised inside ``with client.span(...)`` marks the span as an
error (recording the message in ``error.message``) and is re-raised.
"""

from __future__ import annotations

import contextvars
import logging
import os
import secrets
import time
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Iterator, Optional

import grpc

from ._proto import ingestion_pb2, ingestion_pb2_grpc, span_pb2

log = logging.getLogger("conduit_sdk")

# Mirrors the gateway's schema.MaxAttributeValLen; longer values get the
# whole span rejected.
MAX_ATTR_VALUE_BYTES = 4096

# The span that new spans become children of, per thread and per asyncio task.
_current: contextvars.ContextVar[Optional["Span"]] = contextvars.ContextVar("conduit_current_span", default=None)


class ConduitError(Exception):
    """A span could not be delivered or was rejected by the gateway."""


@dataclass
class LLMCall:
    model: str
    prompt: str
    completion: str
    input_tokens: int
    output_tokens: int
    cost_usd: float = 0.0


STATUS_UNSET = span_pb2.SPAN_STATUS_UNSET
STATUS_OK = span_pb2.SPAN_STATUS_OK
STATUS_ERROR = span_pb2.SPAN_STATUS_ERROR


@dataclass
class Span:
    trace_id: str
    span_id: str
    parent_id: str
    name: str
    start_ns: int
    attrs: dict = field(default_factory=dict)
    status: int = STATUS_UNSET
    llm_call: Optional[LLMCall] = None
    _client: Optional["Client"] = field(default=None, repr=False)
    _ended: bool = field(default=False, repr=False)

    def set_attr(self, key: str, value: str) -> "Span":
        self.attrs[key] = str(value)
        return self

    def set_ok(self) -> "Span":
        self.status = STATUS_OK
        return self

    def set_error(self, err: Optional[BaseException | str]) -> "Span":
        """Mark the span failed. ``None`` is a no-op, so callers needn't branch."""
        if err is None:
            return self
        self.status = STATUS_ERROR
        self.attrs["error.message"] = _truncate_utf8(str(err), MAX_ATTR_VALUE_BYTES)
        return self

    def record_llm_call(self, call: LLMCall) -> "Span":
        self.llm_call = call
        return self

    def end(self) -> None:
        """Send the span (once). Raises ConduitError if it isn't accepted."""
        if self._ended:
            return
        self._ended = True
        if self._client is not None:
            self._client._send(self, time.time_ns())


class Client:
    """SDK client. Create one per process and share it; safe across threads and asyncio tasks."""

    def __init__(self, gateway_addr: str, tls_cert: str, tls_key: str, tls_ca: str,
                 timeout: float = 10.0, agent_name: Optional[str] = None):
        # agent_name is accepted for compatibility but not sent: the gateway takes
        # the agent's identity from its client certificate's CommonName.
        del agent_name
        self._timeout = timeout
        self._channel = _secure_channel(gateway_addr, tls_cert, tls_key, tls_ca)
        self._stub = ingestion_pb2_grpc.IngestionServiceStub(self._channel)

    @classmethod
    def from_env(cls) -> "Client":
        """Build a client from CONDUIT_GATEWAY_ADDR, TLS_CERT, TLS_KEY, and TLS_CA.

        OTEL_GATEWAY_ADDR is accepted as a fallback for CONDUIT_GATEWAY_ADDR.
        """
        addr = os.environ.get("CONDUIT_GATEWAY_ADDR") or os.environ["OTEL_GATEWAY_ADDR"]
        return cls(addr, os.environ["TLS_CERT"], os.environ["TLS_KEY"], os.environ["TLS_CA"])

    def start_span(self, name: str) -> Span:
        """Start a span that is a child of the current span (if any). Call ``end()`` yourself."""
        parent = _current.get()
        return Span(
            # W3C/OTLP sizes: 16-byte trace ID, 8-byte span ID, lowercase hex.
            trace_id=parent.trace_id if parent else secrets.token_hex(16),
            span_id=secrets.token_hex(8),
            parent_id=parent.span_id if parent else "",
            name=name,
            start_ns=time.time_ns(),
            _client=self,
        )

    @contextmanager
    def span(self, name: str, *, raise_errors: bool = False) -> Iterator[Span]:
        """Open a span as the current span; end and send it on exit.

        By default, delivery failures are logged and not raised, so
        instrumentation can't break the agent. With ``raise_errors=True`` they
        raise ConduitError, unless the block is already raising its own
        exception, which is never masked.
        """
        s = self.start_span(name)
        token = _current.set(s)
        failed = False
        try:
            yield s
        except BaseException as exc:
            failed = True
            s.set_error(exc)
            raise
        finally:
            _current.reset(token)
            try:
                s.end()
            except ConduitError as err:
                if raise_errors and not failed:
                    raise
                log.warning("conduit: span %r not delivered: %s", s.name, err)

    def _send(self, s: Span, end_ns: int) -> None:
        req = ingestion_pb2.IngestSpanRequest(span=_to_proto(s, end_ns))
        try:
            resp = self._stub.IngestSpan(req, timeout=self._timeout)
        except grpc.RpcError as err:
            raise ConduitError(f"ingest span: {err.code().name}: {err.details()}") from err
        if not resp.accepted:
            raise ConduitError(f"gateway rejected span: {resp.rejection_reason}")

    def close(self) -> None:
        self._channel.close()

    def __enter__(self) -> "Client":
        return self

    def __exit__(self, *_exc) -> None:
        self.close()


def _to_proto(s: Span, end_ns: int) -> span_pb2.Span:
    out = span_pb2.Span(
        trace_id=s.trace_id,
        span_id=s.span_id,
        parent_id=s.parent_id,
        name=s.name,
        status=s.status,
        attributes=s.attrs,
    )
    out.start_time.FromNanoseconds(s.start_ns)
    out.end_time.FromNanoseconds(max(end_ns, s.start_ns))
    if s.llm_call is not None:
        c = s.llm_call
        out.llm_call.CopyFrom(span_pb2.LLMCallData(
            model=c.model, prompt=c.prompt, completion=c.completion,
            input_tokens=c.input_tokens, output_tokens=c.output_tokens, cost_usd=c.cost_usd,
        ))
    return out


def _secure_channel(addr: str, cert: str, key: str, ca: str) -> grpc.Channel:
    """mTLS channel that trusts only ``ca`` for the gateway's certificate."""
    def read(path: str) -> bytes:
        with open(path, "rb") as f:
            return f.read()

    creds = grpc.ssl_channel_credentials(
        root_certificates=read(ca),
        private_key=read(key),
        certificate_chain=read(cert),
    )
    return grpc.secure_channel(addr, creds)


def _truncate_utf8(s: str, limit: int) -> str:
    b = s.encode("utf-8")
    if len(b) <= limit:
        return s
    return b[:limit].decode("utf-8", errors="ignore")
