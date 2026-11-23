#!/usr/bin/env python3
"""Generate readable release notes from the git history between two tags.

The release workflow feeds the result into GoReleaser's ``TAG_MESSAGE``
environment variable, so the GitHub Release body and the Telegram notification
both show a real changelog instead of an empty section.

Usage (locally, for a preview):

    python .github/release-tools/generate_release_notes.py --tag v0.2.69

Usage (in CI):

    python .github/release-tools/generate_release_notes.py --tag "$RELEASE_TAG" \
        >> "$GITHUB_OUTPUT"

The output is Markdown with one section per conventional-commit type. Upstream
synchronisation merges are collapsed into a single ``上游同步`` entry so a fork
rebase does not dump hundreds of foreign commits into the notes.
"""
from __future__ import annotations

import argparse
import os
import re
import subprocess
import sys

# Sections are rendered in this order; anything unknown falls into OTHER.
SECTIONS = [
    ("feat", "✨ 新功能"),
    ("fix", "🐛 问题修复"),
    ("perf", "⚡ 性能优化"),
    ("refactor", "♻️ 重构"),
    ("revert", "⏪ 回滚"),
    ("docs", "📝 文档"),
]
MAINTENANCE = "🧰 维护"
OTHER = "📦 其他变更"
UPSTREAM = "🔀 上游同步"

# Commits that only exist to cut a release; they never belong in the notes.
NOISE = [
    re.compile(r"^chore(\([^)]*\))?:\s*bump version", re.I),
    re.compile(r"^chore(\([^)]*\))?:\s*sync VERSION", re.I),
    re.compile(r"^ci:\s*release", re.I),
    re.compile(r"^chore(\([^)]*\))?:\s*release", re.I),
]

CONVENTIONAL = re.compile(r"^(?P<type>[a-zA-Z]+)(?:\((?P<scope>[^)]*)\))?(?P<breaking>!)?:\s*(?P<subject>.+)$")
UPSTREAM_MERGE = re.compile(r"^Merge (?:remote-tracking )?(?:branch|pull request).*upstream|^Merge upstream", re.I)


def git(*args: str, check: bool = True) -> str:
    result = subprocess.run(["git", *args], text=True, capture_output=True)
    if check and result.returncode != 0:
        raise SystemExit(f"git {' '.join(args)} failed: {result.stderr.strip()}")
    return result.stdout.strip() if result.returncode == 0 else ""


def previous_tag(tag: str) -> str | None:
    for pattern in ("v[0-9]*", "[0-9]*"):
        found = git("describe", "--tags", "--abbrev=0", "--match", pattern, f"{tag}^", check=False)
        if found:
            return found
    return None


def compare_url(tag: str, base: str | None) -> str | None:
    if not base:
        return None
    remote = os.environ.get("GITHUB_REPOSITORY") or ""
    if not remote:
        url = git("remote", "get-url", "origin", check=False)
        match = re.search(r"github\.com[:/](?P<repo>[^/\s]+/[^/\s]+?)(?:\.git)?$", url)
        remote = match.group("repo") if match else ""
    if not remote:
        return None
    return f"https://github.com/{remote}/compare/{base}...{tag}"


def tag_body(tag: str) -> str:
    """Hand-written notes from an annotated tag (subject + optional body)."""
    contents = git("tag", "-l", "--format=%(contents)", tag, check=False)
    if not contents:
        return ""
    lines = contents.splitlines()
    # The first line is the tag subject, which duplicates the release title.
    body = "\n".join(lines[1:]).strip()
    return body


def classify(subject: str) -> tuple[str, str]:
    match = CONVENTIONAL.match(subject)
    if not match:
        return OTHER, subject
    kind = match.group("type").lower()
    scope = match.group("scope")
    text = match.group("subject").strip()
    if match.group("breaking") or "BREAKING CHANGE" in subject:
        text = f"**BREAKING** {text}"
    if scope:
        text = f"**{scope}**: {text}"
    for known, title in SECTIONS:
        if kind == known:
            return title, text
    if kind in {"build", "ci", "chore", "style", "test"}:
        return MAINTENANCE, text
    return OTHER, text


def is_noise(subject: str) -> bool:
    return any(pattern.search(subject) for pattern in NOISE)


def collect(tag: str, base: str | None, max_items: int) -> tuple[dict[str, list[str]], list[str], int]:
    range_spec = f"{base}..{tag}" if base else tag
    sections: dict[str, list[str]] = {}
    upstream: list[str] = []
    skipped = 0

    merges = git("log", "--first-parent", "--merges", "--format=%H", range_spec, check=False).split()
    upstream_shas = set()
    for sha in merges:
        subject = git("log", "-1", "--format=%s", sha, check=False)
        if not UPSTREAM_MERGE.search(subject):
            continue
        upstream_shas.add(sha)
        count = git("rev-list", "--count", f"{sha}^1..{sha}^2", check=False) or "?"
        upstream.append(f"同步上游 {count} 个提交（`{subject}`）")

    log = git("log", "--first-parent", "--no-merges", "--format=%s", range_spec, check=False)
    for subject in log.splitlines():
        subject = subject.strip()
        if not subject or is_noise(subject):
            skipped += 1
            continue
        title, text = classify(subject)
        sections.setdefault(title, []).append(text)

    if upstream_shas:
        # Merges themselves are never listed individually.
        skipped += len(upstream_shas)

    if max_items > 0:
        for title, items in sections.items():
            if len(items) > max_items:
                hidden = len(items) - max_items
                sections[title] = items[:max_items] + [f"……以及另外 {hidden} 项"]

    return sections, upstream, skipped


def render(tag: str, sections: dict[str, list[str]], upstream: list[str], body: str,
           url: str | None) -> str:
    parts: list[str] = []
    if body:
        parts.append(body.strip())

    parts.append("## 📋 变更内容")
    if not sections and not upstream:
        parts.append("_本版本没有面向用户的变更（仅版本号与流水线维护）。_")

    ordered = [title for _, title in SECTIONS] + [MAINTENANCE, OTHER]
    for title in [*ordered, UPSTREAM]:
        items = upstream if title == UPSTREAM else sections.get(title, [])
        if not items:
            continue
        parts.append(f"### {title}")
        parts.append("\n".join(f"- {item}" for item in items))

    if url:
        parts.append(f"**完整对比**：[{tag} 变更]({url})")

    return "\n\n".join(parts).strip() + "\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="生成 release notes / changelog")
    parser.add_argument("--tag", required=True, help="本次发布的 tag，例如 v0.2.69")
    parser.add_argument("--previous-tag", help="对比基线 tag，默认自动取最近的一个")
    parser.add_argument("--max-items", type=int, default=30, help="每个分区最多列出的条目数，0 表示不限制")
    parser.add_argument("--no-tag-body", action="store_true", help="不把 annotated tag 正文放在最前面")
    args = parser.parse_args(argv)

    base = args.previous_tag if args.previous_tag is not None else previous_tag(args.tag)
    if base:
        # Make sure the base is an ancestor; otherwise the range is nonsense.
        if subprocess.run(["git", "merge-base", "--is-ancestor", base, args.tag],
                          capture_output=True).returncode != 0:
            base = None
    sections, upstream, _ = collect(args.tag, base, args.max_items)
    body = "" if args.no_tag_body else tag_body(args.tag)
    sys.stdout.write(render(args.tag, sections, upstream, body, compare_url(args.tag, base)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
