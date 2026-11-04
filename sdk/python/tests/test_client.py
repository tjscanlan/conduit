import asyncio
import datetime
import threading
from concurrent import futures

import grpc
import pytest
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import ec
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

from conduit_observability import (SEVERITY_ERROR, SEVERITY_INFO, SEVERITY_WARN, STATUS_ERROR, STATUS_OK,
                         STATUS_UNSET, Client, ConduitError, LLMCall)
from conduit_observability._proto import ingestion_pb2, ingestion_pb2_grpc
from conduit_observability.client import MAX_ATTR_VALUE_BYTES, _truncate_utf8


def _pem(cert):
    return cert.public_bytes(serialization.Encoding.PEM)


def _key_pem(key):
    return key.private_bytes(serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8,
                             serialization.NoEncryption())


def make_pki(tmp_path, name):
    """A throwaway CA plus a server cert (for localhost) and a client cert, on disk."""
    now = datetime.datetime.now(datetime.timezone.utc)

    def build(subject, issuer, pub, signer, *, ca=False, eku=None, san=None):
        b = (x509.CertificateBuilder().subject_name(subject).issuer_name(issuer).public_key(pub)
             .serial_number(x509.random_serial_number())
             .not_valid_before(now - datetime.timedelta(hours=1))
             .not_valid_after(now + datetime.timedelta(hours=1))
             .add_extension(x509.BasicConstraints(ca=ca, path_length=None), critical=True))
        if eku:
            b = b.add_extension(x509.ExtendedKeyUsage([eku]), critical=False)
        if san:
            b = b.add_extension(x509.SubjectAlternativeName([x509.DNSName(san)]), critical=False)
        return b.sign(signer, hashes.SHA256())

    ca_key = ec.generate_private_key(ec.SECP256R1())
    ca_name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, f"{name} CA")])
    ca = build(ca_name, ca_name, ca_key.public_key(), ca_key, ca=True)

    srv_key = ec.generate_private_key(ec.SECP256R1())
    srv = build(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "gateway")]), ca_name,
                srv_key.public_key(), ca_key, eku=ExtendedKeyUsageOID.SERVER_AUTH, san="localhost")

    cli_key = ec.generate_private_key(ec.SECP256R1())
    cli = build(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "py-agent")]), ca_name,
                cli_key.public_key(), ca_key, eku=ExtendedKeyUsageOID.CLIENT_AUTH)

    paths = {}
    for label, data in {"ca.crt": _pem(ca), "client.crt": _pem(cli), "client.key": _key_pem(cli_key)}.items():
        p = tmp_path / f"{name}-{label}"
        p.write_bytes(data)
        paths[label] = str(p)
    paths["server_pair"] = (_key_pem(srv_key), _pem(srv))
    paths["ca_pem"] = _pem(ca)
    return paths


class FakeGateway(ingestion_pb2_grpc.IngestionServiceServicer):
    """Records spans and rejects any named 'reject-me'."""

    def __init__(self):
        self.spans = []
        self.logs = []
        self.metrics = []
        self.lock = threading.Lock()

    def IngestBatch(self, request, context):
        resp = ingestion_pb2.IngestBatchResponse()
        for i, m in enumerate(request.metrics):
            if m.name.startswith("conduit_"):
                resp.errors.append(f"metric[{i}]: validation: metric names starting with conduit_ or __ are reserved")
                continue
            with self.lock:
                self.metrics.append(m)
            resp.metrics_accepted += 1
        for i, rec in enumerate(request.logs):
            if not rec.body:
                resp.errors.append(f"log[{i}]: validation: log body is required")
                continue
            with self.lock:
                self.logs.append(rec)
            resp.logs_accepted += 1
        return resp

    def IngestSpan(self, request, context):
        if request.span.name == "reject-me":
            return ingestion_pb2.IngestSpanResponse(accepted=False, rejection_reason="name not allowed")
        with self.lock:
            self.spans.append(request.span)
        return ingestion_pb2.IngestSpanResponse(accepted=True)


@pytest.fixture
def gateway(tmp_path):
    """An in-process gRPC server that requires client certs from the 'conduit' CA."""
    pki = make_pki(tmp_path, "conduit")
    fake = FakeGateway()
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    ingestion_pb2_grpc.add_IngestionServiceServicer_to_server(fake, server)
    creds = grpc.ssl_server_credentials([pki["server_pair"]], root_certificates=pki["ca_pem"],
                                        require_client_auth=True)
    port = server.add_secure_port("localhost:0", creds)
    server.start()
    yield fake, f"localhost:{port}", pki, tmp_path
    server.stop(None)


def client_for(addr, pki, ca=None, **kw):
    return Client(addr, pki["client.crt"], pki["client.key"], ca or pki["ca.crt"], timeout=5, **kw)


def test_spans_reach_gateway_over_mtls(gateway):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        with c.span("agent.run") as run:
            run.set_attr("task", "summarize").set_ok()
            with c.span("llm.call") as llm:
                llm.record_llm_call(LLMCall(model="m", prompt="p", completion="c",
                                            input_tokens=3, output_tokens=5, cost_usd=0.25))

    assert len(fake.spans) == 2
    child, root = fake.spans  # the child ends (and is sent) first
    assert root.name == "agent.run" and root.parent_id == "" and root.attributes["task"] == "summarize"
    assert root.status == STATUS_OK
    assert child.trace_id == root.trace_id and child.parent_id == root.span_id
    assert len(root.trace_id) == 32 and len(root.span_id) == 16  # W3C/OTLP sizes
    assert child.llm_call.model == "m" and child.llm_call.input_tokens == 3 and child.llm_call.cost_usd == 0.25
    assert child.status == STATUS_UNSET
    assert child.end_time.ToNanoseconds() >= child.start_time.ToNanoseconds() > 0


def test_exception_marks_span_error_and_reraises(gateway):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        with pytest.raises(ValueError):
            with c.span("tool.call"):
                raise ValueError("tool timed out")
    (s,) = fake.spans
    assert s.status == STATUS_ERROR
    assert s.attributes["error.message"] == "tool timed out"


def test_set_error_none_is_noop_and_truncation():
    from conduit_observability.client import Span
    s = Span("t", "s", "", "n", 0)
    assert s.set_error(None).status == STATUS_UNSET
    s.set_error("é" * 3000)  # 6000 bytes
    msg = s.attrs["error.message"]
    assert len(msg.encode()) <= MAX_ATTR_VALUE_BYTES and msg == "é" * (MAX_ATTR_VALUE_BYTES // 2)
    assert _truncate_utf8("aé", 2) == "a"


def test_explicit_end_raises_on_rejection_but_context_manager_only_logs(gateway, caplog):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        s = c.start_span("reject-me")
        with pytest.raises(ConduitError, match="name not allowed"):
            s.end()
        s.end()  # idempotent: a second end() doesn't resend
        with c.span("reject-me"):
            pass  # must not raise
    assert "not delivered" in caplog.text
    assert fake.spans == []


def test_raise_errors_option(gateway, caplog):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        with pytest.raises(ConduitError, match="name not allowed"):
            with c.span("reject-me", raise_errors=True):
                pass
        # The block's own exception wins over the delivery error; it is never masked.
        with pytest.raises(KeyError):
            with c.span("reject-me", raise_errors=True):
                raise KeyError("boom")
    assert "not delivered" in caplog.text


def test_log_links_to_current_span(gateway):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        c.log(SEVERITY_INFO, "agent started")
        with c.span("tool.call") as s:
            c.log(SEVERITY_WARN, "retrying", {"attempt": 2})
            with pytest.raises(ConduitError, match="log body is required"):
                c.log(SEVERITY_ERROR, "")
    outside, inside = fake.logs
    assert outside.trace_id == "" and outside.span_id == "" and outside.severity == SEVERITY_INFO
    assert inside.trace_id == s.trace_id and inside.span_id == s.span_id
    assert inside.severity == SEVERITY_WARN and inside.attributes["attempt"] == "2"
    assert inside.timestamp.ToNanoseconds() > 0


def test_gauge_and_counter(gateway):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        c.gauge("queue_depth", 7, {"queue": "tasks"})
        c.counter("tool_calls_total", 12)
        with pytest.raises(ConduitError, match="reserved"):
            c.gauge("conduit_fake", 1)
    gauge, counter = fake.metrics
    assert gauge.name == "queue_depth" and gauge.value == 7 and gauge.labels["queue"] == "tasks"
    assert gauge.type == 1 and counter.type == 2 and counter.value == 12  # GAUGE, COUNTER
    assert gauge.timestamp.ToNanoseconds() > 0


def test_rejects_gateway_from_untrusted_ca(gateway):
    fake, addr, pki, tmp_path = gateway
    other = make_pki(tmp_path, "other")
    with client_for(addr, pki, ca=other["ca.crt"]) as c:
        s = c.start_span("x")
        with pytest.raises(ConduitError):
            s.end()
    assert fake.spans == []


def test_parent_tracking_is_isolated_per_thread(gateway):
    fake, addr, pki, _ = gateway
    with client_for(addr, pki) as c:
        barrier = threading.Barrier(4)

        def worker(i):
            with c.span(f"root-{i}"):
                barrier.wait()  # all roots open at once
                with c.span(f"child-{i}"):
                    pass

        threads = [threading.Thread(target=worker, args=(i,)) for i in range(4)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

    by_name = {s.name: s for s in fake.spans}
    for i in range(4):
        root, child = by_name[f"root-{i}"], by_name[f"child-{i}"]
        assert child.parent_id == root.span_id and child.trace_id == root.trace_id, f"thread {i} cross-linked"
    assert len({by_name[f"root-{i}"].trace_id for i in range(4)}) == 4


def test_parent_tracking_is_isolated_per_asyncio_task(gateway):
    fake, addr, pki, _ = gateway

    async def main(c):
        async def task(i):
            with c.span(f"aroot-{i}"):
                await asyncio.sleep(0.01)  # interleave tasks while roots are open
                with c.span(f"achild-{i}"):
                    await asyncio.sleep(0)

        await asyncio.gather(*(task(i) for i in range(4)))

    with client_for(addr, pki) as c:
        asyncio.run(main(c))
    by_name = {s.name: s for s in fake.spans}
    for i in range(4):
        assert by_name[f"achild-{i}"].parent_id == by_name[f"aroot-{i}"].span_id, f"task {i} cross-linked"
