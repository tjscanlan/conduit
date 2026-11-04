"""Point the SDK at the local quickstart unless the environment says otherwise.

The SDK reads CONDUIT_GATEWAY_ADDR, CONDUIT_CERT_DIR and CONDUIT_AGENT
(see Client.from_env). When CONDUIT_CERT_DIR isn't set, this fills in the
certificates `make quickstart` generated, as agent "my-agent".
CONDUIT_UI_URL sets where trace links point (default http://localhost:8080).
"""
import os
import sys
from pathlib import Path

import conduit_observability as conduit

QUICKSTART_CERTS = Path("deployments", "quickstart", ".conduit", "certs")


def configure() -> None:
    """Configure the module-level client from the environment and check the gateway."""
    if not os.environ.get("CONDUIT_CERT_DIR"):
        for base in [Path.cwd(), *Path.cwd().parents, *Path(__file__).resolve().parents]:
            if (base / QUICKSTART_CERTS / "ca.crt").is_file():
                os.environ["CONDUIT_CERT_DIR"] = str(base / QUICKSTART_CERTS)
                os.environ.setdefault("CONDUIT_AGENT", "my-agent")
                break
    try:
        conduit.configure().check()  # fail fast, with a hint, if anything is off
    except conduit.ConduitError as err:
        sys.exit(f"error: {err}")


def trace_url(trace_id: str) -> str:
    return f"{os.environ.get('CONDUIT_UI_URL', 'http://localhost:8080')}/traces/{trace_id}"
