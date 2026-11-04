"""from_env, check(), first-run diagnostics, the module-level API, and provider helpers."""
import asyncio
import logging
import shutil
from types import SimpleNamespace

import pytest

import conduit_observability as conduit
from conduit_observability import Client, ConduitError, ConfigError, STATUS_ERROR, STATUS_OK
from conduit_observability import tracing
from test_client import client_for, gateway, make_pki  # noqa: F401  (gateway is a fixture)

ENV = ["CONDUIT_GATEWAY_ADDR", "OTEL_GATEWAY_ADDR", "CONDUIT_CERT_DIR", "CONDUIT_AGENT",
       "CONDUIT_TLS_CERT", "CONDUIT_TLS_KEY", "CONDUIT_TLS_CA", "TLS_CERT", "TLS_KEY", "TLS_CA"]


@pytest.fixture(autouse=True)
def clean(monkeypatch):
    for k in ENV:
        monkeypatch.delenv(k, raising=False)
    tracing.shutdown()
    tracing._tried_env = False
    yield
    tracing.shutdown()
    tracing._tried_env = False


def cert_dir(tmp_path, pki, *agents):
    """Lay pki out like `make quickstart`: ca.crt, server.crt, <agent>.crt/.key."""
    d = tmp_path / "certs"
    d.mkdir(exist_ok=True)
    shutil.copy(pki["ca.crt"], d / "ca.crt")
    (d / "server.crt").write_bytes(pki["server_pair"][1])
    (d / "server.key").write_bytes(pki["server_pair"][0])
    for a in agents:
        shutil.copy(pki["client.crt"], d / f"{a}.crt")
        shutil.copy(pki["client.key"], d / f"{a}.key")
    return d


def test_from_env_with_cert_dir_sends_spans(gateway, monkeypatch):
    fake, addr, pki, tmp_path = gateway
    monkeypatch.setenv("CONDUIT_CERT_DIR", str(cert_dir(tmp_path, pki, "my-agent")))
    monkeypatch.setenv("CONDUIT_GATEWAY_ADDR", addr)
    with Client.from_env(timeout=5) as c:
        c.check()
        with c.span("from-env", raise_errors=True):
            pass
    assert [s.name for s in fake.spans] == ["from-env"]


def test_from_env_with_several_agents_asks_for_one(tmp_path, monkeypatch):
    pki = make_pki(tmp_path, "conduit")
    monkeypatch.setenv("CONDUIT_CERT_DIR", str(cert_dir(tmp_path, pki, "demo-agent", "my-agent")))
    with pytest.raises(ConfigError, match=r"several agent certificates \(demo-agent, my-agent\).*CONDUIT_AGENT"):
        Client.from_env()
    monkeypatch.setenv("CONDUIT_AGENT", "my-agent")
    Client.from_env().close()


def test_from_env_legacy_names_and_nothing_set(tmp_path, monkeypatch):
    with pytest.raises(ConfigError, match="CONDUIT_CERT_DIR.*make quickstart"):
        Client.from_env()
    pki = make_pki(tmp_path, "conduit")
    monkeypatch.setenv("TLS_CERT", pki["client.crt"])
    monkeypatch.setenv("TLS_KEY", pki["client.key"])
    monkeypatch.setenv("TLS_CA", pki["ca.crt"])
    Client.from_env().close()  # 0.1.0's variable names still work


def test_config_mistakes_fail_at_construction(tmp_path):
    pki = make_pki(tmp_path, "conduit")
    other = make_pki(tmp_path, "other")
    with pytest.raises(ConfigError, match="make quickstart"):
        Client("localhost:1", str(tmp_path / "nope.crt"), pki["client.key"], pki["ca.crt"])
    with pytest.raises(ConfigError, match="belongs to a different certificate"):
        Client("localhost:1", pki["client.crt"], other["client.key"], pki["ca.crt"])
    bad = tmp_path / "bad.crt"
    bad.write_text("not a pem")
    with pytest.raises(ConfigError, match="not a PEM certificate"):
        Client("localhost:1", pki["client.crt"], pki["client.key"], str(bad))


@pytest.mark.parametrize("case,want", [
    ("not running", "can't reach the gateway at localhost:1"),
    ("wrong CA", "not signed by the CA in"),
    ("agent from another CA", "signed by the CA the gateway trusts"),
])
def test_check_explains_connection_failures(gateway, case, want):
    fake, addr, pki, tmp_path = gateway
    other = make_pki(tmp_path, "other")
    if case == "not running":
        c = client_for("localhost:1", pki)
    elif case == "wrong CA":
        c = client_for(addr, pki, ca=other["ca.crt"])
    else:
        c = Client(addr, other["client.crt"], other["client.key"], pki["ca.crt"], timeout=5)
    with c, pytest.raises(ConduitError, match=want) as exc:
        c.check()
    print(exc.value)


def test_traced_and_module_span(gateway):
    fake, addr, pki, _ = gateway
    conduit.configure(client_for(addr, pki))

    @conduit.traced(kind="tool")
    def add(a, b):
        return a + b

    @conduit.traced("custom.name")
    async def fetch():
        await asyncio.sleep(0)
        return conduit.current_span().name

    @conduit.traced()
    def boom():
        raise ValueError("bad input")

    with conduit.span("agent.run") as run:
        assert add(2, 3) == 5
        assert asyncio.run(fetch()) == "custom.name"
        with pytest.raises(ValueError):
            boom()
    by = {s.name: s for s in fake.spans}
    assert set(by) == {"agent.run", "test_traced_and_module_span.<locals>.add", "custom.name",
                       "test_traced_and_module_span.<locals>.boom"}
    for name, s in by.items():
        if name != "agent.run":
            assert s.parent_id == run.span_id and s.trace_id == run.trace_id, name
    assert by["test_traced_and_module_span.<locals>.add"].status == STATUS_OK
    assert by["test_traced_and_module_span.<locals>.add"].attributes["kind"] == "tool"
    boom_span = by["test_traced_and_module_span.<locals>.boom"]
    assert boom_span.status == STATUS_ERROR and boom_span.attributes["error.message"] == "bad input"


def test_unconfigured_tracing_is_a_noop_with_one_warning(caplog):
    @conduit.traced()
    def work():
        return 42

    with caplog.at_level(logging.WARNING, logger="conduit_observability"):
        assert work() == 42
        with conduit.span("x"):
            pass
    warnings = [r for r in caplog.records if "tracing disabled" in r.message]
    assert len(warnings) == 1 and "CONDUIT_CERT_DIR" in warnings[0].message


def test_llm_call_from_anthropic():
    msg = SimpleNamespace(
        model="claude-haiku-4-5",
        content=[SimpleNamespace(type="text", text="Checking."),
                 SimpleNamespace(type="tool_use", id="t1", name="get_weather", input={"city": "Paris"})],
        usage=SimpleNamespace(input_tokens=1000, output_tokens=200))
    call = conduit.llm_call_from_anthropic(msg, prompt="weather?")
    assert (call.model, call.prompt, call.input_tokens, call.output_tokens) == ("claude-haiku-4-5", "weather?", 1000, 200)
    assert call.completion == 'Checking.\ntool calls: get_weather({"city": "Paris"})'
    assert call.cost_usd == pytest.approx((1000 * 0.80 + 200 * 4.0) / 1e6)


def test_llm_call_from_openai_chat_and_responses():
    chat = {"model": "gpt-4o-mini", "usage": {"prompt_tokens": 50, "completion_tokens": 7},
            "choices": [{"message": {"content": None, "tool_calls": [
                {"function": {"name": "get_weather", "arguments": '{"city":"Tokyo"}'}}]}}]}
    call = conduit.llm_call_from_openai(chat)
    assert call.completion == 'tool calls: get_weather({"city":"Tokyo"})'
    assert (call.input_tokens, call.output_tokens) == (50, 7) and call.cost_usd > 0

    responses = SimpleNamespace(model="gpt-5-mini", usage=SimpleNamespace(input_tokens=30, output_tokens=5), output=[
        SimpleNamespace(type="message", content=[SimpleNamespace(type="output_text", text="Tokyo is warmer.")])])
    call = conduit.llm_call_from_openai(responses, prompt="q")
    assert (call.completion, call.input_tokens, call.output_tokens, call.cost_usd) == ("Tokyo is warmer.", 30, 5, 0)
