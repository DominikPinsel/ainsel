#!/usr/bin/env python3
"""Generate release notes from conventional commits.

Because this repository squash-merges every PR, one commit equals one PR and
the commit subject is already the PR title - so the changelog writes itself,
as long as commits follow docs/conventions.md.

Sections, in order, empty ones omitted:

    Breaking changes      `!` after the type, or a BREAKING CHANGE: footer
    Features              feat
    Fixes                 fix
    Performance           perf
    Refactoring           refactor
    Documentation         docs
    Tests                 test
    Dependency updates    chore(deps) - split out so bumps don't bury real work
    Other changes         everything else, including unparseable subjects

Nothing is dropped silently: a subject that does not parse still lands in
"Other changes".

Usage:
    scripts/release-notes.py                          # last tag..HEAD
    scripts/release-notes.py --from v0.1.0 --to v0.2.0
    scripts/release-notes.py --from -                 # no prior tag: all history
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

SUBJECT = re.compile(r"^(?P<type>[a-zA-Z]+)(?:\((?P<scope>[^)]*)\))?(?P<bang>!?)\s*:\s*(?P<rest>.*)$")
PR_REF = re.compile(r"\s*\(#(?P<num>\d+)\)\s*$")
VERSION_TAG = re.compile(r"^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$")

SECTIONS = [
    ("feat", "Features"),
    ("fix", "Fixes"),
    ("perf", "Performance"),
    ("refactor", "Refactoring"),
    ("docs", "Documentation"),
    ("test", "Tests"),
]


def git(*args: str) -> str:
    try:
        return subprocess.run(["git", *args], check=True, capture_output=True, text=True).stdout
    except subprocess.CalledProcessError as exc:
        detail = (exc.stderr or "").strip().splitlines()
        raise SystemExit(
            "error: git %s failed\n       %s" % (" ".join(args[:2]), detail[-1] if detail else "no output")
        ) from None


def latest_tag() -> str | None:
    best: tuple[int, int, int] | None = None
    best_tag: str | None = None
    for tag in git("tag", "--merged", "HEAD").split():
        if not VERSION_TAG.match(tag) or "-" in tag[1:]:
            continue
        parts = tuple(int(p) for p in tag[1:].split("."))
        if best is None or parts > best:  # type: ignore[comparison-overlap]
            best, best_tag = parts, tag  # type: ignore[assignment]
    return best_tag


def repo_url() -> str:
    url = git("remote", "get-url", "origin").strip()
    url = re.sub(r"\.git$", "", url)
    if url.startswith("git@"):
        url = "https://" + url[4:].replace(":", "/", 1)
    return url.rstrip("/")


def commits(rng: str) -> list[tuple[str, str, str]]:
    out = git("log", "--no-merges", "--pretty=%H%x1f%s%x1f%b%x1e", rng)
    records = []
    for record in out.split("\x1e"):
        record = record.strip("\n")
        if not record.strip():
            continue
        parts = record.split("\x1f")
        if len(parts) < 2:
            continue
        records.append((parts[0], parts[1], parts[2] if len(parts) > 2 else ""))
    return records


def render_entry(sha: str, subject: str, ctype: str, scope: str | None, base: str) -> str:
    """One bullet: description, scope in bold, PR linked, sha as fallback.

    The conventional-commit prefix is stripped - the section heading already
    says "Features" and the bold scope already says which component, so
    repeating `feat(frontend):` in the bullet is just noise.
    """
    m = SUBJECT.match(subject)
    text = (m.group("rest") if m else subject).strip()
    pr = PR_REF.search(text)
    text = PR_REF.sub("", text).strip()
    link = f"([#{pr.group('num')}]({base}/pull/{pr.group('num')}))" if pr else f"(`{sha[:8]}`)"
    prefix = f"**{scope}:** " if scope else ""
    return f"- {prefix}{text} {link}"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--from", dest="from_ref", help="previous tag, or '-' for all history (default: newest tag)")
    ap.add_argument("--to", default=None, help="version being released (used in the heading only)")
    ap.add_argument("--repo-url", default=None, help="repository URL for PR links (default: origin remote)")
    args = ap.parse_args()

    base = (args.repo_url or repo_url()).rstrip("/")

    from_ref = args.from_ref
    if from_ref is None:
        from_ref = latest_tag() or "-"
    if from_ref != "-":
        probe = subprocess.run(
            ["git", "rev-parse", "-q", "--verify", f"refs/tags/{from_ref}^{{}}"], capture_output=True
        )
        if probe.returncode != 0:
            print(f"error: --from {from_ref!r} is not a tag in this repository", file=sys.stderr)
            return 2
        rng = f"{from_ref}..HEAD"
        range_note = f"since {from_ref}"
    else:
        rng = "HEAD"
        range_note = "all history — this is the first tagged release"

    log = commits(rng)

    breaking: list[str] = []
    grouped: dict[str, list[str]] = {key: [] for key, _ in SECTIONS}
    deps: list[str] = []
    other: list[str] = []

    for sha, subject, body in log:
        m = SUBJECT.match(subject)
        ctype = m.group("type").lower() if m else ""
        scope = m.group("scope") if m and m.group("scope") else None
        is_breaking = bool(m and m.group("bang")) or "BREAKING CHANGE:" in body or "BREAKING-CHANGE:" in body
        entry = render_entry(sha, subject, ctype, scope, base)

        if is_breaking:
            breaking.append(entry)
        elif ctype == "chore" and scope == "deps":
            deps.append(entry)
        elif ctype in grouped:
            grouped[ctype].append(entry)
        else:
            other.append(entry)

    title = args.to or "Unreleased"
    out = [f"## {title}", "", f"_{range_note} — {len(log)} commits._", ""]

    def section(heading: str, entries: list[str]) -> None:
        if not entries:
            return
        out.append(f"### {heading}")
        out.append("")
        out.extend(entries)
        out.append("")

    section(":warning: Breaking changes", breaking)
    for key, heading in SECTIONS:
        section(heading, grouped[key])
    section("Dependency updates", deps)
    section("Other changes", other)

    if from_ref != "-":
        out.append(f"**Full compare:** [{from_ref}...{title}]({base}/compare/{from_ref}...{title})")
        out.append("")

    sys.stdout.write("\n".join(out).rstrip() + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
