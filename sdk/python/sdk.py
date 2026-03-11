"""
Agent Observability SDK for Python agents.

Usage:
    from agent_observability import Client, LLMCall

    client = Client.from_env()

    with client.span("my-tool-call") as span:
        result = call_my_tool()
        span.set_attr("tool.name", "my-tool")

    with client.span("llm-call") as span:
        response = anthropic.messages.create(...)
        span.record_llm_call(LLMCall(
            model="claude-sonnet-4-20250514",
            prompt=prompt,
            completion=response.content[0].text,
            input_tokens=response.usage.input_tokens,
            output_tokens=response.usage.output_tokens,
        ))
"""

import ssl
import time
import uuid
import os
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Optional, Iterator
import grpc


@dataclass
class LLMCall:
    model: str
    prompt: str
    completion: str
    input_tokens: int
    output_tokens: int
    cost_usd: float = 0.0


@dataclass
class Span:
    trace_id: str
    span_id: str
    parent_id: str
    name: str
    start_time: float
    attrs: dict = field(default_factory=dict)
    llm_call: Optional[LLMCall] = None
    _client: object = field(default=None, repr=False)

    def set_attr(self, key: str, value: str) -> "Span":
        self.attrs[key] = value
        return self

    def record_llm_call(self, call: LLMCall) -> "Span":
        self.llm_call = call
        return self

    def end(self) -> None:
        if self._client:
            self._client._flush(self, end_time=time.time())


class Client:
    """Thread-safe SDK client. Create once per process."""

    def __init__(self, gateway_addr: str, agent_name: str,
                 tls_cert: str, tls_key: str, tls_ca: str):
        self.agent_name = agent_name
        self._channel = self._build_channel(gateway_addr, tls_cert, tls_key, tls_ca)
        self._trace_id_var: Optional[str] = None
        self._span_id_var: Optional[str] = None

    @classmethod
    def from_env(cls) -> "Client":
        """Build a client from environment variables.

        Required env vars:
            OTEL_GATEWAY_ADDR  — e.g. gateway:4317
            OTEL_AGENT_NAME    — identifies this agent
            TLS_CERT           — path to client cert
            TLS_KEY            — path to client key
            TLS_CA             — path to CA cert
        """
        return cls(
            gateway_addr=os.environ["OTEL_GATEWAY_ADDR"],
            agent_name=os.environ["OTEL_AGENT_NAME"],
            tls_cert=os.environ["TLS_CERT"],
            tls_key=os.environ["TLS_KEY"],
            tls_ca=os.environ["TLS_CA"],
        )

    @contextmanager
    def span(self, name: str, trace_id: Optional[str] = None) -> Iterator[Span]:
        """Context manager that opens a span and ends it on exit.

        Example:
            with client.span("fetch-context") as s:
                s.set_attr("db.table", "documents")
                result = db.query(...)
        """
        tid = trace_id or self._trace_id_var or str(uuid.uuid4()).replace("-", "")
        parent = self._span_id_var
        sid = str(uuid.uuid4()).replace("-", "")

        s = Span(
            trace_id=tid,
            span_id=sid,
            parent_id=parent or "",
            name=name,
            start_time=time.time(),
            _client=self,
        )

        # Push context
        prev_trace, prev_span = self._trace_id_var, self._span_id_var
        self._trace_id_var = tid
        self._span_id_var = sid

        try:
            yield s
        finally:
            s.end()
            # Pop context
            self._trace_id_var = prev_trace
            self._span_id_var = prev_span

    def _flush(self, span: Span, end_time: float) -> None:
        """Serialize span to protobuf and send over gRPC. Stub."""
        pass

    def _build_channel(self, addr: str, cert: str, key: str, ca: str):
        with open(cert, "rb") as f:
            cert_data = f.read()
        with open(key, "rb") as f:
            key_data = f.read()
        with open(ca, "rb") as f:
            ca_data = f.read()

        creds = grpc.ssl_channel_credentials(
            root_certificates=ca_data,
            private_key=key_data,
            certificate_chain=cert_data,
        )
        return grpc.secure_channel(addr, creds)

    def close(self) -> None:
        self._channel.close()

    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()
