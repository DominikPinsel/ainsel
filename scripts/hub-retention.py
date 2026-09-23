#!/usr/bin/env python3
"""Report or enforce a tag retention policy on the Docker Hub images.

Every push to `main` or `develop` publishes a `:<short-sha>` image, and
`dev-image-pi.yml` publishes a `1.24-<sha>` / `8.0-<sha>` variant tag per
build. Nothing consumes those per-commit tags - they exist only so a build is
traceable - and Docker Hub has no server-side retention for a personal
namespace, so they accumulate forever. As of this script's writing the account
carried 456 tags / 125 GB, 91% of it in the three `pi*` repos.

This script is the enforcement half of the policy described in
docs/maintenance.md. It is **dry-run by default**: it prints what it would
delete and reclaims nothing unless `--apply` is passed with credentials.

Safety stance, in order of importance:
  * A tag is only ever deleted if it matches a known *disposable* pattern.
    Anything unrecognised is reported as `unknown` and left alone, so a new
    naming scheme surfaces in the report instead of being silently collected.
  * `latest`, `dev`, `main`, `buildcache*` and every release tag
    (`X.Y.Z` / `vX.Y.Z`) are protected. Release tags are only reconsidered by
    an explicit `--include-release-tags`, which additionally demands `--yes`
    and a published release newer than the tag in question.
  * Deletion is one API call per tag with no cascading: sibling tags that
    share a manifest are untouched, so removing `:<sha>` never moves `:dev`.

Run from the repository root:

    python3 scripts/hub-retention.py                      # report all repos
    python3 scripts/hub-retention.py --repo ainsel-pi-maui
    DOCKERHUB_USERNAME=... DOCKERHUB_TOKEN=... \
      python3 scripts/hub-retention.py --apply            # enforce
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass, field

API = "https://hub.docker.com/v2"
USER_AGENT = "ainsel-hub-retention"

#: Repositories the CI workflows publish. Keep in sync with the `image:`
#: entries in .github/workflows/dev-image-*.yml and release.yml.
REPOS = [
    "ainsel-chat-mcp",
    "ainsel-hub-backend",
    "ainsel-hub-frontend",
    "ainsel-k8s-ai-agent-operator",
    "ainsel-k8s-event-source-gateway-operator",
    "ainsel-mcp",
    "ainsel-pi",
    "ainsel-pi-go",
    "ainsel-pi-maui",
    "ainsel-webhook-receiver",
]

# Tags whose deletion would break something.
PROTECTED_EXACT = {"latest", "dev", "main"}
PROTECTED_PREFIX = ("buildcache",)
RE_SEMVER = re.compile(r"^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$")
# Floating major.minor tags that the pi variants move in place (`1.24`, `8.0`).
RE_FLOAT = re.compile(r"^\d+\.\d+$")
# A release built on top of a floating variant tag: `1.24-0.1.0`, `8.0-v0.2.0`.
RE_VARIANT_RELEASE = re.compile(r"^\d+\.\d+-v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$")

# Tags this script is allowed to delete.
RE_SHORT_SHA = re.compile(r"^[0-9a-f]{7}$")
RE_VARIANT_SHA = re.compile(r"^\d+\.\d+-[0-9a-f]{7}$")


class Disposition:
    KEEP_PROTECTED = "protected"
    KEEP_RECENT = "keep-recent"
    KEEP_NEWEST = "keep-newest"
    KEEP_UNKNOWN = "unknown"
    DELETE = "delete"
    DELETE_RELEASE = "delete-release"


@dataclass
class Tag:
    name: str
    pushed: dt.datetime
    full_size: int = 0
    digest: str = ""


@dataclass
class Decision:
    repo: str
    tag: Tag
    disposition: str
    reason: str
    rank: int | None = None


@dataclass
class Policy:
    """Retention thresholds. Defaults are the values agreed in docs/maintenance.md.

    The newest-N floor is per class, because the two disposable classes cost very
    different amounts of registry: a `:<short-sha>` app image is a few hundred MB,
    while a `pi-maui` `8.0-<sha>` is ~3 GB and no consumer has ever pulled it by
    that tag. Keeping 20 of the latter would pin ~60 GB of pure provenance.
    """

    max_age_days: int = 30
    keep_newest: int = 20
    keep_newest_variant: int = 5
    include_release_tags: bool = False
    #: Explicit human confirmation, required alongside --include-release-tags.
    confirmed: bool = False
    #: Newest published release version, e.g. "0.2.0". Required before release
    #: tags become collectable, so a purge can't delete the only stable tags.
    latest_release: str | None = None
    now: dt.datetime = field(default_factory=lambda: dt.datetime.now(dt.timezone.utc))


def classify(name: str) -> str:
    """Which retention class a tag belongs to: protected, disposable or unknown."""
    if name in PROTECTED_EXACT or name.startswith(PROTECTED_PREFIX):
        return "protected"
    if RE_FLOAT.match(name):
        return "protected"
    if RE_VARIANT_RELEASE.match(name):
        return "release"
    if RE_SEMVER.match(name):
        return "release"
    if RE_SHORT_SHA.match(name):
        return "short-sha"
    if RE_VARIANT_SHA.match(name):
        return "variant-sha"
    return "unknown"


def parse_semver(value: str) -> tuple[int, ...]:
    core = re.sub(r"^v", "", value).split("-", 1)[0]
    return tuple(int(p) for p in core.split("."))


def release_core(name: str) -> str:
    """The version part of a release tag, without any variant prefix.

    `0.2.1` -> `0.2.1`, `v0.2.1` -> `0.2.1`, `1.24-0.1.0` -> `0.1.0`. Without
    this, a pi variant release tag would be compared on its `1.24` toolchain
    prefix and look newer than every real release.
    """
    match = re.search(r"v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$", name)
    return match.group(0) if match else name


def decide_one(repo: str, tag: Tag, rank: int, policy: Policy) -> Decision:
    """Decide a single tag, given its newest-first rank among its repo's disposables."""
    kind = classify(tag.name)
    age_days = (policy.now - tag.pushed).total_seconds() / 86400.0

    if kind in ("protected", "unknown"):
        label = Disposition.KEEP_PROTECTED if kind == "protected" else Disposition.KEEP_UNKNOWN
        return Decision(repo, tag, label, f"{kind} tag is never collected")

    if kind == "release":
        if not policy.include_release_tags:
            return Decision(repo, tag, Disposition.KEEP_PROTECTED, "release tag (protected)")
        problem = release_purge_blocked(policy)
        if problem:
            return Decision(repo, tag, Disposition.KEEP_PROTECTED, problem)
        if parse_semver(release_core(tag.name)) >= parse_semver(release_core(policy.latest_release)):
            return Decision(
                repo, tag, Disposition.KEEP_PROTECTED, "newest release, still current"
            )
        return Decision(
            repo,
            tag,
            Disposition.DELETE_RELEASE,
            f"pre-pipeline release tag, superseded by {policy.latest_release}",
        )

    # Disposable: keep the newest N unconditionally, then anything young enough.
    floor = policy.keep_newest_variant if kind == "variant-sha" else policy.keep_newest
    if rank < floor:
        return Decision(
            repo, tag, Disposition.KEEP_NEWEST, f"within newest {floor} of its class"
        )
    if age_days <= policy.max_age_days:
        return Decision(
            repo, tag, Disposition.KEEP_RECENT, f"{int(age_days)}d old, within {policy.max_age_days}d"
        )
    return Decision(
        repo,
        tag,
        Disposition.DELETE,
        f"{int(age_days)}d old and rank {rank + 1}",
    )


def release_purge_blocked(policy: Policy) -> str | None:
    """Why release-tag purging is not permitted right now, or None if it is."""
    if not policy.latest_release:
        return "release tags held: no --latest-release given to compare against"
    if not re.match(r"^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.]+)?$", release_core(policy.latest_release)):
        return f"release tags held: --latest-release {policy.latest_release!r} is not a version"
    return None


RE_REPO = re.compile(r"^[A-Za-z0-9._-]+$")


def policy_problems(policy: Policy, repos: list[str]) -> list[str]:
    """Policies that look like a fat-fingered flag rather than an intent.

    Enforced only for --apply: a dry run should accept anything so a human can
    explore what a threshold would do.
    """
    problems: list[str] = []
    if policy.max_age_days < 1:
        problems.append(f"--max-age-days {policy.max_age_days} would collect every build tag by age")
    if policy.keep_newest < 1:
        problems.append(f"--keep-newest {policy.keep_newest} leaves no recent build tags")
    if policy.keep_newest_variant < 0:
        problems.append(f"--keep-newest-variant {policy.keep_newest_variant} is not a count")
    for repo in repos:
        if not RE_REPO.match(repo):
            problems.append(f"{repo!r} is not a repository name")
    return problems


def plan_for_repo(repo: str, tags: list[Tag], policy: Policy) -> list[Decision]:
    """Rank a repo's tags newest-first and decide each one."""
    ordered = sorted(tags, key=lambda t: t.pushed, reverse=True)
    # Ranks are assigned per disposable class so a repo's protected and unknown
    # tags cannot push a build's own tag out of the keep-newest window.
    ranks: dict[str, int] = {}
    decisions: list[Decision] = []
    for tag in ordered:
        kind = classify(tag.name)
        rank = None
        if kind in ("short-sha", "variant-sha"):
            rank = ranks.get(kind, 0)
            ranks[kind] = rank + 1
        decisions.append(decide_one(repo, tag, rank if rank is not None else 1 << 30, policy))
    return decisions


# --------------------------------------------------------------------------
# Docker Hub HTTP. Listing is anonymous (the repos are public); deleting is not.
# --------------------------------------------------------------------------


def _request(url: str, auth: str | None = None, method: str = "GET") -> tuple[int, bytes]:
    headers = {"User-Agent": USER_AGENT}
    if auth:
        headers["Authorization"] = auth
    req = urllib.request.Request(url, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return resp.status, resp.read()
    except urllib.error.HTTPError as err:
        return err.code, err.read()


def basic_auth(username: str, password: str) -> str:
    """Hub API 2.0 takes HTTP basic auth with a Docker Hub access token."""
    import base64

    return "Basic " + base64.b64encode(f"{username}:{password}".encode()).decode()


def verify_credentials(auth: str) -> str:
    """Fail early with a clear message if the token cannot reach the account."""
    status, body = _request(f"{API}/user/", auth=auth)
    if status != 200:
        raise RuntimeError(
            f"Docker Hub authentication failed (HTTP {status}): {body[:200]!r} - "
            "check DOCKERHUB_USERNAME/DOCKERHUB_TOKEN and that the token can delete images"
        )
    return json.loads(body).get("username") or "(unknown)"


def fetch_tags(repo: str) -> list[Tag]:
    """Every tag of a public repo, newest data we can get from the Hub API."""
    tags: list[Tag] = []
    url = f"{API}/repositories/dpinsel/{repo}/tags/?page_size=100"
    while url:
        status, body = _request(url)
        if status == 404:
            return tags
        if status != 200:
            raise RuntimeError(f"listing {repo} failed (HTTP {status}): {body[:200]!r}")
        page = json.loads(body)
        for item in page.get("results", []):
            pushed_raw = item.get("tag_last_pushed") or item.get("last_updated")
            if not pushed_raw:
                continue
            tags.append(
                Tag(
                    name=item["name"],
                    pushed=dt.datetime.fromisoformat(pushed_raw.replace("Z", "+00:00")),
                    full_size=item.get("full_size") or 0,
                    digest=item.get("digest") or "",
                )
            )
        url = page.get("next")
    return tags


def delete_tag(repo: str, name: str, auth: str) -> tuple[bool, str]:
    """DELETE one tag. Returns (deleted, detail)."""
    quoted = urllib.parse.quote(name, safe="")
    url = f"{API}/repositories/dpinsel/{repo}/tags/{quoted}"
    status, body = _request(url, auth=auth, method="DELETE")
    if status in (200, 202, 204):
        return True, "deleted"
    if status == 404:
        return False, "already gone"
    return False, f"HTTP {status}: {body[:200]!r}"


# --------------------------------------------------------------------------
# Reporting
# --------------------------------------------------------------------------


def summarise(decisions: list[Decision]) -> dict:
    out: dict[str, dict] = {}
    for d in decisions:
        bucket = out.setdefault(d.repo, {"total": 0, "delete": 0, "bytes": 0, "kept": 0})
        bucket["total"] += 1
        if d.disposition in (Disposition.DELETE, Disposition.DELETE_RELEASE):
            bucket["delete"] += 1
            bucket["bytes"] += d.tag.full_size
        else:
            bucket["kept"] += 1
    return out


def print_report(plans: dict[str, list[Decision]], applied: bool) -> int:
    summary = summarise([d for ds in plans.values() for d in ds])
    total_del = sum(s["delete"] for s in summary.values())
    total_bytes = sum(s["bytes"] for s in summary.values())

    for repo, decisions in sorted(plans.items()):
        s = summary[repo]
        if s["delete"] == 0 and s["total"] == 0:
            print(f"{repo}: no tags")
            continue
        print(
            f"\n{repo}: {s['total']} tags, {s['delete']} to delete "
            f"({s['bytes'] / 1e9:.2f} GB logical), {s['kept']} kept"
            + ("" if s["delete"] else " - nothing collected")
        )
        for d in sorted(decisions, key=lambda x: x.tag.pushed):
            if d.disposition in (Disposition.DELETE, Disposition.DELETE_RELEASE):
                verb = "PURGE" if applied else "would delete"
                print(f"    {verb:12s} {d.tag.name:16s} {d.tag.pushed:%Y-%m-%d}  {d.reason}")
        unknown = [d for d in decisions if d.disposition == Disposition.KEEP_UNKNOWN]
        for d in unknown:
            print(f"    UNKNOWN      {d.tag.name:16s} {d.tag.pushed:%Y-%m-%d}  not matched by any rule - left alone")

    print(
        f"\n{'APPLIED' if applied else 'DRY RUN'}: {total_del} of "
        f"{sum(s['total'] for s in summary.values())} tags, "
        f"{total_bytes / 1e9:.2f} GB logical (upper bound; deleted layers are shared with kept tags)"
    )
    return total_del


def write_markdown(plans: dict[str, list[Decision]], applied: bool) -> str:
    summary = summarise([d for ds in plans.values() for d in ds])
    lines = [
        "## Docker Hub tag retention",
        "",
        f"Mode: `{'apply (tags were deleted)' if applied else 'dry-run (nothing deleted)'}`",
        "",
        "| repo | tags | delete | kept | logical GB |",
        "|---|---|---|---|---|",
    ]
    for repo in sorted(plans):
        s = summary.get(repo, {"total": 0, "delete": 0, "kept": 0, "bytes": 0})
        lines.append(
            f"| `{repo}` | {s['total']} | {s['delete']} | {s['kept']} | {s['bytes'] / 1e9:.2f} |"
        )
    total = sum(s["delete"] for s in summary.values())
    unknown = [d for ds in plans.values() for d in ds if d.disposition == Disposition.KEEP_UNKNOWN]
    lines += [
        "",
        f"**{total} tags collected.**",
        "",
    ]
    if unknown:
        lines += [
            "### Tags no rule matched",
            "",
            "These were left alone but need a rule before they pile up:",
            "",
        ]
        lines += [f"- `{d.repo}:{d.tag.name}`" for d in unknown[:40]]
    return "\n".join(lines) + "\n"


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    p.add_argument("--repo", action="append", dest="repos", help="only this repo (repeatable)")
    p.add_argument("--max-age-days", type=int, default=30)
    p.add_argument("--keep-newest", type=int, default=20, help="floor for :<short-sha> tags")
    p.add_argument(
        "--keep-newest-variant",
        type=int,
        default=5,
        help="floor for the multi-GB pi variant <float>-<sha> tags",
    )
    p.add_argument("--apply", action="store_true", help="actually delete (needs credentials)")
    p.add_argument(
        "--include-release-tags",
        action="store_true",
        help="also consider pre-pipeline release tags; requires --latest-release and --yes",
    )
    p.add_argument("--latest-release", help="newest published release version, e.g. 0.2.0")
    p.add_argument("--yes", action="store_true", help="confirm a release-tag purge")
    p.add_argument("--json", action="store_true", dest="as_json", help="machine-readable output")
    p.add_argument("--markdown", help="also write a markdown summary to this file")
    p.add_argument(
        "--now", help="override 'now' (ISO 8601), for reproducible runs and tests"
    )
    args = p.parse_args(argv)

    now = dt.datetime.now(dt.timezone.utc)
    if args.now:
        now = dt.datetime.fromisoformat(args.now.replace("Z", "+00:00"))

    policy = Policy(
        max_age_days=args.max_age_days,
        keep_newest=args.keep_newest,
        keep_newest_variant=args.keep_newest_variant,
        include_release_tags=args.include_release_tags,
        confirmed=args.yes,
        latest_release=args.latest_release,
        now=now,
    )
    if policy.include_release_tags:
        if not policy.confirmed:
            print("error: --include-release-tags also needs --yes", file=sys.stderr)
            return 2
        problem = release_purge_blocked(policy)
        if problem:
            print(f"error: {problem}", file=sys.stderr)
            return 2
    repos = args.repos or REPOS

    # Cheapest checks first: a malformed policy should be reported before we
    # spend a round trip authenticating.
    if args.apply:
        problems = policy_problems(policy, repos)
        if problems:
            for problem in problems:
                print(f"error: refusing --apply: {problem}", file=sys.stderr)
            return 2

    auth = None
    if args.apply:
        username = os.environ.get("DOCKERHUB_USERNAME")
        password = os.environ.get("DOCKERHUB_TOKEN")
        if not username or not password:
            print(
                "error: --apply needs DOCKERHUB_USERNAME and DOCKERHUB_TOKEN "
                "(a personal access token with delete:delete-image scope)",
                file=sys.stderr,
            )
            return 2
        auth = basic_auth(username, password)
        try:
            verify_credentials(auth)
        except Exception as err:  # noqa: BLE001 - report and exit, no traceback
            print(f"error: {err}", file=sys.stderr)
            return 2

    plans: dict[str, list[Decision]] = {}
    for repo in repos:
        try:
            tags = fetch_tags(repo)
        except Exception as err:  # noqa: BLE001
            print(f"error: {repo}: {err}", file=sys.stderr)
            return 1
        plans[repo] = plan_for_repo(repo, tags, policy)

    if args.as_json:
        payload = {
            repo: [
                {
                    "tag": d.tag.name,
                    "pushed": d.tag.pushed.isoformat(),
                    "bytes": d.tag.full_size,
                    "disposition": d.disposition,
                    "reason": d.reason,
                }
                for d in decisions
            ]
            for repo, decisions in plans.items()
        }
        print(json.dumps(payload, indent=2, sort_keys=True))
    else:
        print_report(plans, args.apply)

    if args.markdown:
        with open(args.markdown, "w", encoding="utf-8") as fh:
            fh.write(write_markdown(plans, args.apply))

    if args.apply:
        failures = 0
        for repo, decisions in sorted(plans.items()):
            for d in decisions:
                if d.disposition not in (Disposition.DELETE, Disposition.DELETE_RELEASE):
                    continue
                ok, detail = delete_tag(repo, d.tag.name, auth)
                if not ok:
                    failures += 1
                    print(f"FAILED {repo}:{d.tag.name} - {detail}", file=sys.stderr)
        if failures:
            print(f"\n{failures} deletions failed", file=sys.stderr)
            return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
