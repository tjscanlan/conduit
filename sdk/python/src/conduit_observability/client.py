"""Conduit SDK for Python agents: send spans to the gateway over mTLS/gRPC.

Usage::

    from conduit_observability import Client, LLMCall

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
import secrets
import ssl
import time
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Iterator, Optional

import grpc

from . import _config
from ._proto import ingestion_pb2, ingestion_pb2_grpc, log_pb2, metric_pb2, span_pb2

log = logging.getLogger("conduit_observability")

# Mirrors the gateway's schema.MaxAttributeValLen; longer values get the
# whole span rejected.
MAX_ATTR_VALUE_BYTES = 4096

# The span that new spans become children of, per thread and per asyncio task.
_current: contextvars.ContextVar[Optional["Span"]] = contextvars.ContextVar("conduit_current_span", default=None)


class ConduitError(Exception):
    """A span could not be delivered or was rejected by the gateway."""


class ConfigError(ConduitError):
    """The client is misconfigured (missing or mismatched certificate files, no settings)."""


@dataclass
class LLMCall:
    model: str
    prompt: str
    completion: str
    input_tokens: int
    output_tokens: int
    cost_usd: float = 0.0


SEVERITY_UNSPECIFIED = log_pb2.LOG_SEVERITY_UNSPECIFIED
SEVERITY_DEBUG = log_pb2.LOG_SEVERITY_DEBUG
SEVERITY_INFO = log_pb2.LOG_SEVERITY_INFO
SEVERITY_WARN = log_pb2.LOG_SEVERITY_WARN
SEVERITY_ERROR = log_pb2.LOG_SEVERITY_ERROR

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
        if not gateway_addr:
            raise ConfigError(f"gateway_addr is empty (the local quickstart's is {_config.DEFAULT_GATEWAY_ADDR})")
        _check_files(tls_cert, tls_key, tls_ca)
        self._addr, self._tls_cert, self._tls_ca = gateway_addr, tls_cert, tls_ca
        self._timeout = timeout
        self._channel = _secure_channel(gateway_addr, tls_cert, tls_key, tls_ca)
        self._stub = ingestion_pb2_grpc.IngestionServiceStub(self._channel)

    @classmethod
    def from_env(cls, timeout: float = 10.0) -> "Client":
        """Build a client from the environment. The shortest setup is a
        certificate directory, such as the one ``make quickstart`` writes::

            CONDUIT_CERT_DIR=deployments/quickstart/.conduit/certs CONDUIT_AGENT=my-agent

        It uses ``<dir>/ca.crt`` and ``<dir>/<agent>.crt``/``.key``;
        CONDUIT_AGENT may be omitted when the directory holds one agent.
        CONDUIT_TLS_CERT, CONDUIT_TLS_KEY and CONDUIT_TLS_CA set the files
        directly (TLS_CERT, TLS_KEY and TLS_CA are accepted too), and
        CONDUIT_GATEWAY_ADDR the gateway (default ``localhost:4317``).
        Raises ConfigError, with what to set, if nothing is configured.
        """
        s = _config.from_env()
        return cls(s.gateway_addr, s.tls_cert, s.tls_key, s.tls_ca, timeout=timeout)

    def check(self) -> None:
        """Verify the gateway is reachable and accepts this agent's certificate.

        Sends an empty batch. Call it at startup to fail fast with an
        actionable ConduitError instead of on the first span.
        """
        try:
            self._stub.IngestBatch(ingestion_pb2.IngestBatchRequest(), timeout=self._timeout)
        except grpc.RpcError as err:
            raise self._rpc_error("check gateway", err) from err

    def _rpc_error(self, op: str, err: grpc.RpcError) -> ConduitError:
        detail = f"{err.code().name}: {err.details()}"
        hint = _diagnose(detail, self._addr, self._tls_cert, self._tls_ca)
        return ConduitError(f"{op}: {hint}: {detail}" if hint else f"{op}: {detail}")

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

    def log(self, severity: int, body: str, attributes: Optional[dict] = None) -> None:
        """Send one log record. Inside a span (``with client.span(...)``) it is
        linked to that span and its trace. Raises ConduitError if not accepted."""
        rec = log_pb2.LogRecord(
            severity=severity,
            body=body,
            attributes={k: str(v) for k, v in (attributes or {}).items()},
        )
        rec.timestamp.FromNanoseconds(time.time_ns())
        current = _current.get()
        if current is not None:
            rec.trace_id, rec.span_id = current.trace_id, current.span_id
        try:
            resp = self._stub.IngestBatch(ingestion_pb2.IngestBatchRequest(logs=[rec]), timeout=self._timeout)
        except grpc.RpcError as err:
            raise self._rpc_error("ingest log", err) from err
        if resp.logs_accepted != 1:
            reason = resp.errors[0] if resp.errors else "unknown reason"
            raise ConduitError(f"gateway rejected log: {reason}")

    def gauge(self, name: str, value: float, labels: Optional[dict] = None) -> None:
        """Record the current value of ``name`` (e.g. a queue depth)."""
        self._metric(name, value, labels, metric_pb2.METRIC_TYPE_GAUGE)

    def counter(self, name: str, value: float, labels: Optional[dict] = None) -> None:
        """Record a cumulative, non-negative value of ``name`` (e.g. requests_total)."""
        self._metric(name, value, labels, metric_pb2.METRIC_TYPE_COUNTER)

    def _metric(self, name: str, value: float, labels: Optional[dict], typ: int) -> None:
        m = metric_pb2.Metric(name=name, value=float(value), type=typ,
                              labels={k: str(v) for k, v in (labels or {}).items()})
        m.timestamp.FromNanoseconds(time.time_ns())
        try:
            resp = self._stub.IngestBatch(ingestion_pb2.IngestBatchRequest(metrics=[m]), timeout=self._timeout)
        except grpc.RpcError as err:
            raise self._rpc_error("ingest metric", err) from err
        if resp.metrics_accepted != 1:
            reason = resp.errors[0] if resp.errors else "unknown reason"
            raise ConduitError(f"gateway rejected metric: {reason}")

    def _send(self, s: Span, end_ns: int) -> None:
        req = ingestion_pb2.IngestSpanRequest(span=_to_proto(s, end_ns))
        try:
            resp = self._stub.IngestSpan(req, timeout=self._timeout)
        except grpc.RpcError as err:
            raise self._rpc_error("ingest span", err) from err
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


def _check_files(cert: str, key: str, ca: str) -> None:
    """Catch file mistakes now, rather than as a TLS failure on the first span."""
    for label, path in (("client certificate", cert), ("client key", key), ("CA certificate", ca)):
        try:
            with open(path, "rb"):
                pass
        except OSError as err:
            raise ConfigError(f"{label} {path}: {err.strerror}; create agent certificates with "
                              "`make quickstart` or `scripts/gen-certs.sh agent NAME`") from err
    try:
        ssl.create_default_context().load_cert_chain(cert, key)
    except ssl.SSLError as err:
        if "KEY_VALUES_MISMATCH" in str(err) or "key values mismatch" in str(err):
            raise ConfigError(f"{key} is not the key for {cert} (the key belongs to a different certificate)") from err
        raise ConfigError(f"can't load {cert} with {key}: {err}") from err
    try:
        ssl.create_default_context(cafile=ca)
    except ssl.SSLError as err:
        raise ConfigError(f"CA certificate {ca} is not a PEM certificate: {err}") from err


def _diagnose(detail: str, addr: str, cert: str, ca: str) -> str:
    """A hint for a failed RPC, from gRPC's error text."""
    d = detail.lower()
    if "connection refused" in d or "dns resolution failed" in d or "name resolution" in d:
        return f"can't reach the gateway at {addr}; is it running? (for a local stack: make quickstart)"
    if "not in peer certificate" in d or "hostname mismatch" in d:
        return (f"the gateway's certificate does not cover the host in {addr!r}; "
                "connect using a name in its certificate (e.g. localhost)")
    if "certificate_verify_failed" in d or "certificate verify failed" in d:
        return (f"the gateway's certificate is not signed by the CA in {ca}, or the gateway rejected this "
                f"agent's certificate {cert}; both must come from the CA set the gateway uses")
    if "handshake" in d or "ssl" in d or "socket closed" in d or "tls" in d:
        return (f"the TLS handshake with {addr} failed; check that {cert} is signed by the CA the gateway "
                f"trusts and that {ca} signed the gateway's certificate")
    if d.startswith("resource_exhausted"):
        return "the gateway is rate limiting this agent (RATE_LIMIT_RPS items/s per agent); send less often or batch"
    if d.startswith(("unauthenticated", "permission_denied")):
        return "the gateway did not accept this agent's identity; check the client certificate"
    if d.startswith("deadline_exceeded"):
        return f"the gateway at {addr} did not answer in time"
    if d.startswith("unavailable"):
        return f"the gateway at {addr} is unavailable; is it running and is the address right?"
    return ""


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
