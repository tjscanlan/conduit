#!/usr/bin/env python3
"""Build the agent-readable docs into the Pages output (stdlib only).

Writes into OUT_DIR (the built Jekyll site, e.g. _site/):
  llms.txt         llmstxt.org-style index: title, summary, and one link per
                   doc to its raw Markdown, with a one-line description
  llms-full.txt    every doc's Markdown concatenated, with source URLs
  <page>.md        raw Markdown for every page (front matter and Liquid
                   stripped), next to its HTML, e.g. /architecture.md

docs/_data/docs.json is the single manifest; it also drives the site nav.

Usage:
  scripts/build-agent-docs.py OUT_DIR   build
  scripts/build-agent-docs.py --check   verify the manifest matches docs/*.md
"""
import json
import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs"
MANIFEST = json.loads((DOCS / "_data/docs.json").read_text())
BASE = MANIFEST["site"]["base_url"].rstrip("/")

FRONT_MATTER = re.compile(r"\A---\n.*?\n---\n+", re.S)
LIQUID = re.compile(r"{%.*?%}|{{.*?}}", re.S)
GENERATED_NOTE = re.compile(r"^<!-- Generated .*?-->\n+", re.M)


def pages():
    for section in MANIFEST["sections"]:
        for page in section["pages"]:
            yield section["name"], page


def overview_markdown() -> str:
    """index.md is an HTML landing page; agents get the README's Markdown instead."""
    text = (ROOT / "README.md").read_text()
    text = re.sub(r"\A(<p.*?</p>\s*|<h1.*?</h1>\s*|---\s*)+", "", text, flags=re.S)  # HTML header block
    text = text.replace("](docs/", "](")  # links are relative to the docs root
    title = MANIFEST["site"]["title"]
    return f"# {title}\n\n> {MANIFEST['site']['summary']}\n\n{text.strip()}\n"


def page_markdown(page) -> str:
    if page["file"] == "index.md":
        return overview_markdown()
    text = (DOCS / page["file"]).read_text()
    text = FRONT_MATTER.sub("", text)
    text = GENERATED_NOTE.sub("", text)
    text = LIQUID.sub("", text)
    if not text.lstrip().startswith("# "):
        text = f"# {page['title']}\n\n{text}"
    return text.strip() + "\n"


def md_url(page) -> str:
    return f"{BASE}/{page['file']}"


def llms_txt() -> str:
    site = MANIFEST["site"]
    out = [f"# {site['title']}", "", f"> {site['summary']}", "",
           "Each link below is the page's raw Markdown. The same pages are browsable as HTML "
           f"at {BASE}/. Source of truth: the docs/ directory of https://github.com/tjscanlan/conduit.", ""]
    current = None
    for section, page in pages():
        if section != current:
            out += [f"## {section}", ""] if current is None else ["", f"## {section}", ""]
            current = section
        out.append(f"- [{page['title']}]({md_url(page)}): {page['description']}")
    out += ["", "## Optional", "",
            f"- [Full documentation]({BASE}/llms-full.txt): every page above concatenated into one file",
            "- [AGENTS.md](https://github.com/tjscanlan/conduit/blob/main/AGENTS.md): how to build, test, and change the codebase (for coding agents)",
            ""]
    return "\n".join(out)


def llms_full() -> str:
    site = MANIFEST["site"]
    out = [f"# {site['title']} documentation (full)", "", f"> {site['summary']}", ""]
    for _, page in pages():
        out += ["---", "", f"<!-- Source: {md_url(page)} -->", "", page_markdown(page).rstrip(), ""]
    return "\n".join(out) + "\n"


def check() -> list[str]:
    errors = []
    listed = {p["file"] for _, p in pages()}
    on_disk = {p.name for p in DOCS.glob("*.md")}
    for f in sorted(on_disk - listed):
        errors.append(f"docs/{f} is not in docs/_data/docs.json (add it so agents and the nav can find it)")
    for f in sorted(listed - on_disk):
        errors.append(f"docs/_data/docs.json lists {f}, which doesn't exist")
    for _, p in pages():
        if len(p.get("description", "")) < 20:
            errors.append(f"{p['file']}: description is missing or too short")
    return errors


def main(argv):
    errors = check()
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    if argv[1:] == ["--check"]:
        print("docs manifest OK")
        return 0
    if len(argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    out = pathlib.Path(argv[1])
    out.mkdir(parents=True, exist_ok=True)
    for _, page in pages():
        (out / page["file"]).write_text(page_markdown(page))
    (out / "llms.txt").write_text(llms_txt())
    (out / "llms-full.txt").write_text(llms_full())
    print(f"wrote llms.txt, llms-full.txt and {sum(1 for _ in pages())} Markdown pages to {out}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
