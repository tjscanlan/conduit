#!/usr/bin/env python3
"""Generate docs/roadmap.md from docs/_data/roadmap.json (the source of truth).

The Markdown is what readers see on GitHub. The docs site renders the same
data as a visual page (docs/_layouts/roadmap.html). CI fails if roadmap.md
is out of date: run `make roadmap` after editing the JSON.
"""
import json
import pathlib

ROOT = pathlib.Path(__file__).resolve().parent.parent
data = json.loads((ROOT / "docs/_data/roadmap.json").read_text())

out = [
    "---",
    "layout: roadmap",
    "title: Roadmap",
    "hide_title: true",
    "body_class: roadmap-page",
    "---",
    "",
    "<!-- Generated from docs/_data/roadmap.json by scripts/gen-roadmap.py. Do not edit. -->",
    "",
    "# Roadmap",
    "",
    data["intro"],
    "",
    data["note"],
    "",
    "## Shipped",
    "",
]
for group in data["shipped"]:
    out.append(f"**{group['group']}**")
    out += [f"- {item}" for item in group["items"]]
    out.append("")
for stage in data["stages"]:
    out += [f"## {stage['title']}: {stage['theme'].lower()}", "", "| Item | Area | Why |", "|------|------|-----|"]
    out += [f"| **{i['title']}** | {i['area']} | {i['why']} |" for i in stage["items"]]
    out.append("")
out += ["## Open decisions", "", "These need a maintainer's call before the related work can start.", "",
        "| Decision | Options | Raised in |", "|----------|---------|-----------|"]
for d in data["decisions"]:
    opts = "; ".join(d["options"]) + (f". {d['lean']}" if d.get("lean") else "")
    out.append(f"| **{d['title']}** | {opts} | [{d['raised']}]({d['url']}) |")
out.append("")

(ROOT / "docs/roadmap.md").write_text("\n".join(out))
print("wrote docs/roadmap.md")
