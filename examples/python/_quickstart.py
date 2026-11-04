"""Connect the examples to a Conduit gateway with as little setup as possible.

By default this finds the certificates that `make quickstart` generated and
sends as agent "my-agent". Environment:

  CONDUIT_GATEWAY_ADDR  gateway address (default localhost:4317)
  CONDUIT_CERT_DIR      directory with ca.crt and <agent>.crt/.key (default:
                        deployments/quickstart/.conduit/certs in this repo)
  CONDUIT_AGENT         which agent certificate to use (default my-agent)
  CONDUIT_UI_URL        where to link traces (default http://localhost:8080)
"""
import os
import sys
from pathlib import Path

from conduit_observability import Client

QUICKSTART_CERTS = Path("deployments", "quickstart", ".conduit", "certs")


def _cert_dir() -> Path:
    if os.environ.get("CONDUIT_CERT_DIR"):
        return Path(os.environ["CONDUIT_CERT_DIR"])
    for base in [Path.cwd(), *Path.cwd().parents, *Path(__file__).resolve().parents]:
        if (base / QUICKSTART_CERTS / "ca.crt").is_file():
            return base / QUICKSTART_CERTS
    sys.exit("error: no quickstart certificates found: run `make quickstart` in the repo root, or set CONDUIT_CERT_DIR")


def client() -> Client:
    certs = _cert_dir()
    agent = os.environ.get("CONDUIT_AGENT", "my-agent")
    return Client(
        os.environ.get("CONDUIT_GATEWAY_ADDR", "localhost:4317"),
        tls_cert=str(certs / f"{agent}.crt"),
        tls_key=str(certs / f"{agent}.key"),
        tls_ca=str(certs / "ca.crt"),
    )


def trace_url(trace_id: str) -> str:
    return f"{os.environ.get('CONDUIT_UI_URL', 'http://localhost:8080')}/traces/{trace_id}"
