#!/usr/bin/env python3
import argparse
import datetime as dt
import json
import pathlib
import re
import sys
import urllib.error
import urllib.request


def load_sources(path):
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
    if not isinstance(data, list):
        raise ValueError("sources file must contain a JSON array")
    return data


def fetch_text(url, timeout):
    req = urllib.request.Request(url, headers={"User-Agent": "OpsCaptainKnowledgeFetcher/1.0"})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        charset = resp.headers.get_content_charset() or "utf-8"
        return resp.read().decode(charset, errors="replace")


def strip_frontmatter(text):
    lines = text.splitlines()
    if not lines or lines[0].strip() not in {"---", "+++"}:
        return text
    marker = lines[0].strip()
    for idx in range(1, len(lines)):
        if lines[idx].strip() == marker:
            return "\n".join(lines[idx + 1 :]).strip() + "\n"
    return text


def preprocess(text):
    text = strip_frontmatter(text)
    text = re.sub(r"{{[%<][\s\S]*?[%>]}}", "", text)
    text = re.sub(r"(?m)^import\s+.*$", "", text)
    text = re.sub(r"(?m)^export\s+.*$", "", text)
    text = re.sub(r"<!--([\s\S]*?)-->", "", text)
    text = re.sub(r"(?m)^#{1,6}\s*$", "", text)
    text = text.replace("\r\n", "\n").replace("\r", "\n")
    text = re.sub(r"\n{3,}", "\n\n", text)
    return text.strip()


def safe_slug(value):
    value = re.sub(r"[^a-zA-Z0-9._-]+", "-", value.strip().lower())
    return value.strip("-") or "document"


def write_doc(source, raw, output_dir, fetched_at):
    provider = safe_slug(source["provider"])
    slug = safe_slug(source["slug"])
    target_dir = output_dir / provider
    target_dir.mkdir(parents=True, exist_ok=True)
    md_path = target_dir / f"{slug}.md"
    metadata_path = target_dir / f"{slug}.metadata.json"
    cleaned = preprocess(raw)
    title = source.get("title") or slug
    tags = source.get("tags") or []
    body = [
        f"# {title}",
        "",
        "## Upstream Metadata",
        "",
        f"- Source: {source['source_url']}",
        f"- License: {source['license']}",
        f"- Provider: {source['provider']}",
        f"- Tags: {', '.join(tags)}",
        "",
        "## Content",
        "",
        cleaned,
        "",
    ]
    md_path.write_text("\n".join(body), encoding="utf-8")
    metadata = {
        "_source": f"upstream://{provider}/{slug}",
        "source_url": source["source_url"],
        "fetch_url": source["url"],
        "license": source["license"],
        "provider": source["provider"],
        "title": title,
        "tags": tags,
        "fetched_at": fetched_at,
        "instance_type": "upstream_ops_doc",
    }
    metadata_path.write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return md_path


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--sources", default="scripts/knowledge/sources.json")
    parser.add_argument("--output", default="docs/knowledge/upstream")
    parser.add_argument("--source", action="append", default=[])
    parser.add_argument("--timeout", type=float, default=20)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    sources_path = pathlib.Path(args.sources)
    output_dir = pathlib.Path(args.output)
    selected = set(args.source)
    sources = load_sources(sources_path)
    if selected:
        sources = [s for s in sources if s.get("provider") in selected or s.get("slug") in selected]
    if not sources:
        raise SystemExit("no sources selected")

    fetched_at = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()
    failures = []
    written = []
    for source in sources:
        label = f"{source.get('provider')}/{source.get('slug')}"
        if args.dry_run:
            print(f"dry_run {label} {source['url']}")
            continue
        try:
            raw = fetch_text(source["url"], args.timeout)
            path = write_doc(source, raw, output_dir, fetched_at)
            written.append(str(path))
            print(f"fetched {label} -> {path}")
        except (urllib.error.URLError, TimeoutError, ValueError, OSError) as exc:
            failures.append(f"{label}: {exc}")
            print(f"failed {label}: {exc}", file=sys.stderr)

    print(f"summary written={len(written)} failed={len(failures)}")
    if failures:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
