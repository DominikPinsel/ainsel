#!/usr/bin/env python3
"""Compute the next release version from conventional commits.

Mirrors the bump rules documented in CONTRIBUTING.md:

    feat                                     -> minor
    fix / perf / refactor / docs / test      -> patch
    breaking change (`!` or BREAKING CHANGE) -> major
    chore / ci / build / style / anything else -> ignored

On a 0.x base a breaking change bumps the *minor* instead of the major, which
is the usual reading of semver §4: below 1.0.0 nothing is stable, so there is
no stable contract to break. `refactor(api)!: remove Agent.spec.enabledMCPs`
on a 0.1.5 base therefore yields 0.2.0, not 1.0.0.

The base version is the newest `vX.Y.Z` tag reachable from HEAD. If the repo
has never been tagged there is no previous release to bump from, so the version
in chart/Chart.yaml *is* the release - it is the only version this project has
ever carried, and the release workflow requires the two to agree anyway. The
commit analysis still runs and is reported, because "these commits imply a
minor bump" is useful to know before the first tag, it just has nothing to
apply to.

Usage:
    scripts/next-version.py                 # next version, e.g. v0.2.0
    scripts/next-version.py --explain       # ... and why, on stderr
    scripts/next-version.py --base v0.1.0   # bump from an explicit tag
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

# type(scope)!: subject
SUBJECT = re.compile(r"^(?P<type>[a-zA-Z]+)(?:\((?P<scope>[^)]*)\))?(?P<bang>!?)\s*:\s*(?P<subject>.*)$")
VERSION_TAG = re.compile(r"^v(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(?:-(?P<pre>[0-9A-Za-z.-]+))?$")

MINOR_TYPES = {"feat"}
PATCH_TYPES = {"fix", "perf", "refactor", "docs", "test"}
IGNORED_TYPES = {"chore", "ci", "build", "style", "revert"}


def git(*args: str, root: Path) -> str:
    try:
        return subprocess.run(
            ["git", "-C", str(root), *args],
            check=True,
            capture_output=True,
            text=True,
        ).stdout
    except subprocess.CalledProcessError as exc:
        detail = (exc.stderr or "").strip().splitlines()
        raise SystemExit(
            "error: git %s failed\n       %s" % (" ".join(args[:2]), detail[-1] if detail else "no output")
        ) from None


def tag_exists(tag: str, root: Path) -> bool:
    probe = subprocess.run(
        ["git", "-C", str(root), "rev-parse", "-q", "--verify", f"refs/tags/{tag}^{{}}"],
        capture_output=True,
        text=True,
    )
    return probe.returncode == 0


def repo_root() -> Path:
    return Path(git("rev-parse", "--show-toplevel", root=Path(".")).strip())


def latest_tag(root: Path) -> str | None:
    """Newest semver tag reachable from HEAD, by version order not name order."""
    out = git("tag", "--merged", "HEAD", root=root)
    best: tuple[int, int, int] | None = None
    best_tag: str | None = None
    for tag in out.split():
        m = VERSION_TAG.match(tag)
        if not m or m.group("pre"):  # prereleases are not a base to bump from
            continue
        key = (int(m.group("major")), int(m.group("minor")), int(m.group("patch")))
        if best is None or key > best:
            best, best_tag = key, tag
    return best_tag


def chart_version(root: Path) -> str | None:
    chart = root / "chart" / "Chart.yaml"
    if not chart.is_file():
        return None
    for line in chart.read_text(encoding="utf-8").splitlines():
        m = re.match(r"^version:\s*([0-9]+\.[0-9]+\.[0-9]+)\s*$", line)
        if m:
            return m.group(1)
    return None


def commits(root: Path, base_tag: str | None) -> list[tuple[str, str, str]]:
    """(sha, subject, body) for every non-merge commit in the release range."""
    rng = f"{base_tag}..HEAD" if base_tag else "HEAD"
    # %x1f separates fields, %x1e separates records; survives newlines in bodies.
    out = git("log", "--no-merges", f"--pretty=%H%x1f%s%x1f%b%x1e", rng, root=root)
    result = []
    for record in out.split("\x1e"):
        record = record.strip("\n")
        if not record.strip():
            continue
        parts = record.split("\x1f")
        if len(parts) < 2:
            continue
        sha, subject = parts[0], parts[1]
        body = parts[2] if len(parts) > 2 else ""
        result.append((sha, subject, body))
    return result


def classify(subject: str, body: str) -> tuple[str, str] | None:
    """Return (bump, type) where bump is 'major'|'minor'|'patch'|''."""
    m = SUBJECT.match(subject)
    if not m:
        return None
    ctype = m.group("type").lower()
    breaking = bool(m.group("bang")) or "BREAKING CHANGE:" in body or "BREAKING-CHANGE:" in body
    if breaking:
        return "major", ctype
    if ctype in MINOR_TYPES:
        return "minor", ctype
    if ctype in PATCH_TYPES:
        return "patch", ctype
    return "", ctype


RANK = {"": 0, "patch": 1, "minor": 2, "major": 3}


def bump(base: tuple[int, int, int], level: str, zero_based_breaking: bool) -> tuple[int, int, int]:
    major, minor, patch = base
    if level == "major":
        if major == 0 and zero_based_breaking:
            return major, minor + 1, 0
        return major + 1, 0, 0
    if level == "minor":
        return major, minor + 1, 0
    if level == "patch":
        return major, minor, patch + 1
    return major, minor, patch


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--base", help="base version tag (default: newest vX.Y.Z tag, else chart version)")
    ap.add_argument("--explain", action="store_true", help="print the reasoning to stderr")
    ap.add_argument(
        "--strict-breaking",
        action="store_true",
        help="a breaking change bumps major even on a 0.x base",
    )
    args = ap.parse_args()

    root = repo_root()

    base_tag = args.base
    base_source = "tag"
    if base_tag:
        m = VERSION_TAG.match(base_tag)
        if not m:
            print(f"error: --base {base_tag!r} is not vX.Y.Z", file=sys.stderr)
            return 2
        if not tag_exists(base_tag, root):
            print(
                f"error: tag {base_tag} does not exist in this repository\n"
                f"       existing version tags: {', '.join(git('tag', '--merged', 'HEAD', root=root).split()) or '(none)'}",
                file=sys.stderr,
            )
            return 2
        base_ver = (int(m.group("major")), int(m.group("minor")), int(m.group("patch")))
    else:
        base_tag = latest_tag(root)
        if base_tag:
            m = VERSION_TAG.match(base_tag)
            assert m is not None
            base_ver = (int(m.group("major")), int(m.group("minor")), int(m.group("patch")))
        else:
            cv = chart_version(root)
            if not cv:
                print(
                    "error: no version tag exists and chart/Chart.yaml has no parsable version;\n"
                    "       pass --base vX.Y.Z for the first release",
                    file=sys.stderr,
                )
                return 2
            base_source = "chart/Chart.yaml (repo has never been tagged)"
            base_ver = tuple(int(p) for p in cv.split("."))  # type: ignore[assignment]

    log = commits(root, base_tag)
    level = ""
    reasons: list[str] = []
    for sha, subject, body in log:
        c = classify(subject, body)
        if not c:
            continue
        lvl, ctype = c
        if RANK[lvl] > RANK[level]:
            level = lvl
        if lvl:
            reasons.append(f"    {lvl:5} {sha[:8]} {subject[:88]}")

    if base_tag:
        nxt = bump(base_ver, level, zero_based_breaking=not args.strict_breaking)
        first_release = False
    else:
        # Never tagged: the chart version is the release. Bumping it would
        # treat the version we are about to publish as if it were the previous
        # one, which is how a first release ends up skipping a version.
        nxt = base_ver
        first_release = True
    version = "v%d.%d.%d" % nxt

    if args.explain:
        base_str = ".".join(str(p) for p in base_ver)
        print(f"base:    v{base_str}  (from {base_source})", file=sys.stderr)
        print(f"range:   {base_tag + '..HEAD' if base_tag else 'all history'}", file=sys.stderr)
        print(f"commits: {len(log)} scanned", file=sys.stderr)
        print(f"highest: {level or 'none (nothing bump-worthy)'}", file=sys.stderr)
        for line in reasons[:25]:
            print(line, file=sys.stderr)
        if len(reasons) > 25:
            print(f"    ... and {len(reasons) - 25} more", file=sys.stderr)
        if first_release:
            print(
                "note:    no tag exists yet, so the chart version is the release;\n"
                "         the commit analysis above has no previous version to bump from",
                file=sys.stderr,
            )
        elif level == "major" and base_ver[0] == 0 and not args.strict_breaking:
            print("note:    breaking change on a 0.x base bumps minor, not major", file=sys.stderr)
        print(f"result:  {version}", file=sys.stderr)

    print(version)
    return 0


if __name__ == "__main__":
    sys.exit(main())
