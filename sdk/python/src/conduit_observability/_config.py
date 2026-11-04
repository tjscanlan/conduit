"""Resolve the client's gateway address and certificate files from the environment."""
from __future__ import annotations

import os
from pathlib import Path
from typing import NamedTuple

DEFAULT_GATEWAY_ADDR = "localhost:4317"

ENV_GATEWAY_ADDR = "CONDUIT_GATEWAY_ADDR"
ENV_CERT_DIR = "CONDUIT_CERT_DIR"
ENV_AGENT = "CONDUIT_AGENT"
ENV_TLS_CERT = "CONDUIT_TLS_CERT"
ENV_TLS_KEY = "CONDUIT_TLS_KEY"
ENV_TLS_CA = "CONDUIT_TLS_CA"

# Files in a certificate directory that are never an agent's.
_NOT_AGENTS = {"ca", "server"}


class Settings(NamedTuple):
    gateway_addr: str
    tls_cert: str
    tls_key: str
    tls_ca: str


def _env(*keys: str) -> str:
    for k in keys:
        if os.environ.get(k):
            return os.environ[k]
    return ""


def from_env() -> Settings:
    """See Client.from_env for the variables. Raises ConfigError with a hint."""
    from .client import ConfigError

    addr = _env(ENV_GATEWAY_ADDR, "OTEL_GATEWAY_ADDR") or DEFAULT_GATEWAY_ADDR
    # TLS_CERT/TLS_KEY/TLS_CA are the 0.1.0 names, still accepted.
    cert, key, ca = _env(ENV_TLS_CERT, "TLS_CERT"), _env(ENV_TLS_KEY, "TLS_KEY"), _env(ENV_TLS_CA, "TLS_CA")

    cert_dir = os.environ.get(ENV_CERT_DIR)
    if cert_dir:
        d = Path(cert_dir)
        agent = os.environ.get(ENV_AGENT)
        if not agent and not (cert and key):
            agent = _sole_agent(d)
        cert = cert or str(d / f"{agent}.crt")
        key = key or str(d / f"{agent}.key")
        ca = ca or str(d / "ca.crt")

    missing = [name for name, v in ((ENV_TLS_CERT, cert), (ENV_TLS_KEY, key), (ENV_TLS_CA, ca)) if not v]
    if missing:
        raise ConfigError(
            f"no agent certificate configured: set {ENV_CERT_DIR} (and {ENV_AGENT} if it holds several agents), "
            f"or {', '.join(missing)}; for a local stack run `make quickstart` and use its "
            "deployments/quickstart/.conduit/certs")
    return Settings(addr, cert, key, ca)


def _sole_agent(d: Path) -> str:
    from .client import ConfigError

    if not d.is_dir():
        raise ConfigError(f"{ENV_CERT_DIR}={d} is not a directory")
    agents = sorted(p.stem for p in d.glob("*.crt") if p.stem not in _NOT_AGENTS and (d / f"{p.stem}.key").is_file())
    if len(agents) == 1:
        return agents[0]
    if not agents:
        raise ConfigError(f"{ENV_CERT_DIR}={d} has no agent certificates (<name>.crt with <name>.key); "
                          "create one with `scripts/gen-certs.sh agent NAME`")
    raise ConfigError(f"{ENV_CERT_DIR}={d} has several agent certificates ({', '.join(agents)}): "
                      f"set {ENV_AGENT} to pick one")
