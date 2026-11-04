#!/usr/bin/env python3
"""Generate docs/api.md, the API reference, from the sources of truth (stdlib only).

- Wire API: services, RPCs, messages, and enums parsed from proto/*.proto,
  including their comments.
- Ingestion limits: constants read from internal/schema/*.go and the gateway,
  so the reference states exactly what the gateway enforces.

CI fails if docs/api.md is out of date: run `make api-docs` after changing
a .proto file or a limit.
"""
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
PROTO_ORDER = ["ingestion.proto", "span.proto", "log.proto", "metric.proto"]

FIELD = re.compile(r"^(repeated\s+)?(map<[^>]+>|[\w.]+)\s+(\w+)\s*=\s*(\d+)\s*;\s*(?://\s*(.*))?$")
ENUM_VALUE = re.compile(r"^(\w+)\s*=\s*(\d+)\s*;\s*(?://\s*(.*))?$")
RPC = re.compile(r"^rpc\s+(\w+)\s*\(\s*(\w+)\s*\)\s*returns\s*\(\s*(\w+)\s*\)\s*;\s*(?://\s*(.*))?$")
BLOCK = re.compile(r"^(message|enum|service)\s+(\w+)\s*\{$")


def parse_proto(path):
    """Return a list of blocks: {kind, name, doc, items:[{...}]} in file order."""
    blocks, current, pending = [], None, []
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if line.startswith("//"):
            pending.append(line[2:].strip())
            continue
        if not line:
            if current is None:
                pending = []
            continue
        m = BLOCK.match(line)
        if m and current is None:
            current = {"kind": m.group(1), "name": m.group(2), "doc": " ".join(pending), "items": []}
            pending = []
            continue
        if line == "}" and current is not None:
            blocks.append(current)
            current, pending = None, []
            continue
        if current is None:
            pending = []
            continue
        doc = " ".join(pending)
        pending = []
        if current["kind"] == "message" and (f := FIELD.match(line)):
            typ = (f.group(1) or "").strip()
            typ = f"repeated {f.group(2)}" if typ else f.group(2)
            current["items"].append({"name": f.group(3), "type": typ, "number": f.group(4),
                                     "doc": " ".join(x for x in [doc, f.group(5) or ""] if x)})
        elif current["kind"] == "enum" and (e := ENUM_VALUE.match(line)):
            current["items"].append({"name": e.group(1), "number": e.group(2),
                                     "doc": " ".join(x for x in [doc, e.group(3) or ""] if x)})
        elif current["kind"] == "service" and (r := RPC.match(line)):
            current["items"].append({"name": r.group(1), "req": r.group(2), "resp": r.group(3),
                                     "doc": " ".join(x for x in [doc, r.group(4) or ""] if x)})
        else:
            sys.exit(f"gen-api-docs: can't parse {path.name}: {line!r}")
    return blocks


CONST = re.compile(r"^\s*(?:const\s+)?(\w+)\s*=\s*(.+?)\s*(?://.*)?$")


def go_value(expr):
    """Evaluate the simple constant forms used for limits; fail loudly otherwise."""
    expr = expr.strip()
    if m := re.fullmatch(r'"([^"]*)"', expr):
        return m.group(1)
    if m := re.fullmatch(r"(\d+)\s*<<\s*(\d+)", expr):
        return int(m.group(1)) << int(m.group(2))
    if m := re.fullmatch(r"(\d+)", expr):
        return int(m.group(1))
    if m := re.fullmatch(r"(?:(\d+)\s*\*\s*)?time\.(Hour|Minute|Second)", expr):
        return f"{int(m.group(1) or 1)} {m.group(2).lower()}{'s' if int(m.group(1) or 1) != 1 else ''}"
    sys.exit(f"gen-api-docs: unsupported constant expression {expr!r}")


def go_consts(path, names):
    vals = {}
    for line in path.read_text().splitlines():
        if (m := CONST.match(line)) and m.group(1) in names:
            vals[m.group(1)] = go_value(m.group(2))
    missing = set(names) - set(vals)
    if missing:
        sys.exit(f"gen-api-docs: constants {sorted(missing)} not found in {path}")
    return vals


def human_bytes(n):
    return f"{n // 1024} KiB" if n >= 1024 and n % 1024 == 0 else f"{n} bytes"


def limits():
    schema = ROOT / "internal/schema"
    v = {}
    v.update(go_consts(schema / "validator.go", ["MaxSpanNameLen", "MaxAttributeCount", "MaxAttributeKeyLen",
                                                 "MaxAttributeValLen", "MaxPromptBytes", "MaxCompletionBytes",
                                                 "MaxSpanDuration", "MaxIDLen", "MaxModelNameLen"]))
    v.update(go_consts(schema / "log.go", ["MaxLogBodyBytes", "MaxLogFutureSkew"]))
    v.update(go_consts(schema / "metric.go", ["MaxMetricNameLen", "MaxMetricLabels", "MaxMetricLabelKeyLen",
                                              "MaxMetricLabelValueLen", "ReservedMetricPrefix", "AgentLabel"]))
    v.update(go_consts(ROOT / "cmd/gateway/handler.go", ["maxBatchItems"]))
    return [
        ("Span", "`trace_id`, `span_id`, `parent_id`", f"1–{v['MaxIDLen']} characters of `[A-Za-z0-9_-]` (`parent_id` may be empty). SDKs send W3C sizes: 32 and 16 hex characters."),
        ("Span", "`name`", f"Required, at most {v['MaxSpanNameLen']} bytes"),
        ("Span", "duration", f"`end_time` ≥ `start_time`, at most {v['MaxSpanDuration']}"),
        ("Span, log", "`attributes`", f"At most {v['MaxAttributeCount']} entries; keys 1–{v['MaxAttributeKeyLen']} bytes; values at most {human_bytes(v['MaxAttributeValLen'])}"),
        ("Span", "`llm_call.prompt` / `completion`", f"At most {human_bytes(v['MaxPromptBytes'])} each (dropped at the gateway when `STORE_LLM_PAYLOADS=false`)"),
        ("Span", "`llm_call.model`", f"At most {v['MaxModelNameLen']} bytes"),
        ("Span", "`llm_call` tokens and cost", "Token counts ≥ 0; `cost_usd` finite and ≥ 0"),
        ("Log", "`body`", f"Required, at most {human_bytes(v['MaxLogBodyBytes'])}"),
        ("Log, metric", "`timestamp`", f"No more than {v['MaxLogFutureSkew']} in the future; the receive time if unset"),
        ("Log", "`trace_id` / `span_id`", "Optional, same format as spans; `span_id` requires `trace_id`"),
        ("Metric", "`name`", f"Prometheus name `[a-zA-Z_:][a-zA-Z0-9_:]*`, at most {v['MaxMetricNameLen']} characters; `{v['ReservedMetricPrefix']}*` and `__*` are reserved"),
        ("Metric", "`labels`", f"At most {v['MaxMetricLabels']}; keys `[a-zA-Z_][a-zA-Z0-9_]*` (at most {v['MaxMetricLabelKeyLen']}, not `__*`); values at most {human_bytes(v['MaxMetricLabelValueLen'])}. The `{v['AgentLabel']}` label is always set by the gateway."),
        ("Metric", "`value`", "Finite; counters ≥ 0"),
        ("Batch", "items per `IngestBatch`", f"At most {v['maxBatchItems']} (spans + metrics + logs), and no more than the per-agent rate limit (`RATE_LIMIT_RPS`)"),
    ]


def render():
    blocks = {}
    for name in PROTO_ORDER:
        blocks[name] = parse_proto(ROOT / "proto" / name)
    out = ["<!-- Generated from proto/*.proto and internal/schema by scripts/gen-api-docs.py. Do not edit. -->", "",
           "# API reference", "",
           "Agents talk to the gateway over gRPC with mutual TLS on port 4317, using the `IngestionService` "
           "in package `observability.v1`. The SDKs wrap this API. Use it directly only if you're writing "
           "a client in another language. This page is generated from the `.proto` files and the "
           "gateway's validation constants, so it always matches the code.", "",
           "## Authentication and identity", "",
           "Every RPC requires a TLS 1.3 client certificate signed by the CA the gateway trusts. The "
           "certificate's CommonName is the agent's identity. The gateway stamps it on every span "
           "(`agent_name`), log (`agent_name`) and metric (`agent` label), overwriting any value the "
           "client supplies.", "",
           "## Errors and partial acceptance", "",
           "- **`IngestSpan`** returns `accepted: false` with a `rejection_reason` when validation fails.",
           "- **`IngestBatch`** validates each item independently. It returns per-kind accepted counts, "
           "plus an `errors` list with entries like `log[2]: validation: log body is required`, where "
           "the index is the item's position in its kind's list.",
           "- **gRPC status `RESOURCE_EXHAUSTED`:** the agent is over its rate limit (charged per item). Retry later.",
           "- **gRPC status `INVALID_ARGUMENT`:** the batch is larger than 1000 items or the per-agent limit; split it.",
           "- **gRPC status `UNAUTHENTICATED`:** no valid client certificate.", "",
           "## Ingestion limits", "",
           "| Applies to | Field | Rule |", "|---|---|---|"]
    out += [f"| {a} | {f} | {r} |" for a, f, r in limits()]
    out.append("")
    for name in PROTO_ORDER:
        out += [f"## `{name}`", ""]
        for b in blocks[name]:
            out += [f"### {b['kind']} `{b['name']}`", ""]
            if b["doc"]:
                out += [b["doc"], ""]
            if b["kind"] == "service":
                out += ["| RPC | Request | Response | Description |", "|---|---|---|---|"]
                out += [f"| `{i['name']}` | `{i['req']}` | `{i['resp']}` | {i['doc']} |" for i in b["items"]]
            elif b["kind"] == "message":
                out += ["| # | Field | Type | Description |", "|---|---|---|---|"]
                out += [f"| {i['number']} | `{i['name']}` | `{i['type']}` | {i['doc']} |" for i in b["items"]]
            else:
                out += ["| Value | Number | Description |", "|---|---|---|"]
                out += [f"| `{i['name']}` | {i['number']} | {i['doc']} |" for i in b["items"]]
            out.append("")
    return "\n".join(out)


if __name__ == "__main__":
    (ROOT / "docs/api.md").write_text(render())
    print("wrote docs/api.md")
