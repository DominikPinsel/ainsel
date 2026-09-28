#!/usr/bin/env python3
"""Fail if a dev-image workflow can publish a tag it should not.

The publish decision in `.github/workflows/dev-image-*.yml` is the only thing
standing between a build and a permanent registry tag, and it has already leaked
twice: `:dev` was written by main builds (PR #182), and any `workflow_dispatch` -
including one from an unreviewed feature branch - published because the steps only
asked "is this a pull request?".

Those are shell conditions inside a YAML `run:` block, so nothing checks them
until a build runs. This does, on every change to a workflow: it executes the real
step body the way the runner would (expressions substituted, `$GITHUB_OUTPUT`
parsed) across every event/branch combination, and asserts the decision.

Run from the repository root: python3 scripts/test-dev-image-workflows.py
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

import yaml

SHA = "0ae751bc1234567890"
WORKFLOWS = Path(".github/workflows")

# event name, GITHUB_REF, the `publish` dispatch input, expected decision
CASES = [
    ("push", "refs/heads/develop", "", "true"),
    ("push", "refs/heads/main", "", "true"),
    ("pull_request", "refs/pull/99/merge", "", "false"),
    # A dispatch with no answer means "do not publish".
    ("workflow_dispatch", "refs/heads/feat-x", "", "false"),
    ("workflow_dispatch", "refs/heads/feat-x", "false", "false"),
    # An explicit opt-in publishes the base image, and nothing else.
    ("workflow_dispatch", "refs/heads/feat-x", "true", "true"),
    ("workflow_dispatch", "refs/heads/develop", "true", "true"),
]


def parse_github_output(text: str) -> dict[str, str]:
    """Read `$GITHUB_OUTPUT`, including the `key<<DELIM` multiline form."""
    out: dict[str, str] = {}
    lines = text.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i]
        match = re.match(r"^([A-Za-z_][A-Za-z0-9_]*)<<(\w+)$", line)
        if match:
            key, delim, buf = match.group(1), match.group(2), []
            i += 1
            while i < len(lines) and lines[i] != delim:
                buf.append(lines[i])
                i += 1
            out[key] = "\n".join(buf)
        elif "=" in line:
            key, value = line.split("=", 1)
            out[key] = value
        i += 1
    return out


def run_meta_step(step: dict, event: str, ref: str, publish_input: str) -> dict[str, str]:
    """Execute a `Compute image metadata` body as the runner would."""
    script = step["run"].replace("${{ inputs.publish }}", publish_input)
    with tempfile.NamedTemporaryFile("w+", suffix=".out") as handle:
        env = dict(
            os.environ,
            GITHUB_EVENT_NAME=event,
            GITHUB_SHA=SHA,
            GITHUB_REF=ref,
            GITHUB_OUTPUT=handle.name,
        )
        subprocess.run(["bash", "-ef", "-c", script], env=env, check=True)
        return parse_github_output(handle.read())


def check_workflow(path: Path) -> list[str]:
    doc = yaml.safe_load(path.read_text())
    steps = doc["jobs"]["build"]["steps"]
    problems: list[str] = []

    meta = next((s for s in steps if s.get("name") == "Compute image metadata"), None)
    if meta is None:
        return [f"{path.name}: no `Compute image metadata` step to gate on"]
    login = next((s for s in steps if s.get("name") == "Login to Docker Hub"), None)
    if login is None:
        return [f"{path.name}: no `Login to Docker Hub` step?"]
    if login.get("if") != "steps.meta.outputs.publish == 'true'":
        problems.append(f"login is guarded by {login.get('if')!r}, not by the publish output")

    builds = [s for s in steps if "build-push-action" in str(s.get("uses", ""))]
    if not builds:
        return [f"{path.name}: no build-push-action step found"]
    for step in builds:
        with_ = step.get("with") or {}
        push, load = str(with_.get("push")), str(with_.get("load"))
        if "github.event_name" in push + load:
            problems.append(
                f"{step.get('name')}: derives push/load from the event name, "
                "which publishes on any non-PR event including a stray dispatch"
            )
        if "steps.meta.outputs.publish" not in push:
            problems.append(f"{step.get('name')}: push={push!r} ignores the publish decision")

    for event, ref, publish_input, want in CASES:
        got = run_meta_step(meta, event, ref, publish_input)
        if got.get("publish") != want:
            problems.append(
                f"{event} on {ref} (publish input {publish_input or '(unset)'}) "
                f"decided publish={got.get('publish')!r}, want {want!r}"
            )
        # :dev is what the dev cluster runs; only develop may name it.
        has_dev = ":dev" in got.get("tags", "")
        if has_dev != (ref == "refs/heads/develop"):
            problems.append(f"{event} on {ref}: :dev in tags = {has_dev}")
        if path.name == "dev-image-pi.yml":
            problems += check_pi_variants(got, ref, problems)
            problems += check_pi_base(got, event, ref)
    return problems


def check_pi_base(got: dict[str, str], event: str, ref: str) -> list[str]:
    """A variant must not FROM a tag that does not exist when it builds.

    buildx resolves FROM from the registry, not the runner's image store, so a
    build-only run has no :<sha> of its own to build on. Getting this wrong fails
    the build (learned from run 35830189618); the alternative - falling back to a
    floating tag - builds something whose base is not the one just compiled, which
    is only acceptable when nothing is being published from it.
    """
    published = got.get("publish") == "true"
    want = SHA[:7] if published else "dev"
    got_base = got.get("base_tag")
    if got_base != want:
        return [
            f"{event} on {ref}: variants build on base_tag={got_base!r}, want {want!r} "
            f"({'this run pushed :<sha>' if published else 'nothing was pushed, so :<sha> does not exist'})"
        ]
    return []


def check_pi_variants(got: dict[str, str], ref: str, seen: list[str]) -> list[str]:
    """The pi variants must never publish a per-build tag, and must build on the
    base produced by this same run."""
    problems: list[str] = []
    variants = got.get("go_tags", "") + "\n" + got.get("maui_tags", "")
    if re.search(r"\d+\.\d+-[0-9a-f]{7}", variants):
        problems.append(
            f"{ref}: variants publish a <float>-<sha> tag ({variants!r}) - "
            "~3 GB of provenance nothing pulls, and 88 GB of the registry"
        )
    published = got.get("publish_variants") == "true"
    floats = {"dpinsel/ainsel-pi-go:1.24", "dpinsel/ainsel-pi-maui:8.0"}
    if published and set(variants.splitlines()) != floats:
        problems.append(f"{ref}: publish_variants=true but tags are {variants!r}")
    if not published and "local-check" not in variants:
        problems.append(f"{ref}: variants are not being published but tags are {variants!r}")
    if published != (ref == "refs/heads/develop" and got.get("publish") == "true"):
        problems.append(f"{ref}: publish_variants={got.get('publish_variants')!r}")
    return problems


def check_pi_workflow() -> list[str]:
    path = WORKFLOWS / "dev-image-pi.yml"
    if not path.exists():
        return ["dev-image-pi.yml is gone"]
    doc = yaml.safe_load(path.read_text())
    steps = doc["jobs"]["build"]["steps"]
    problems: list[str] = []
    for name in ("Build and push Go variant", "Build and push .NET MAUI variant"):
        step = next((s for s in steps if s.get("name") == name), None)
        if step is None:
            problems.append(f"{name} step is gone")
            continue
        if step.get("if") != "github.ref != 'refs/heads/main'":
            problems.append(f"{name}: if={step.get('if')!r} - main must not build variants")
        args = str((step.get("with") or {}).get("build-args"))
        if "BASE_TAG=${{ steps.meta.outputs.base_tag }}" not in args:
            problems.append(
                f"{name}: build-args={args!r} - the base must come from the same step "
                "that decides whether it was published, not a hardcoded tag"
            )
    return problems


def main() -> int:
    if not WORKFLOWS.is_dir():
        print("Run from the repository root.", file=sys.stderr)
        return 2
    failed = False
    for path in sorted(WORKFLOWS.glob("dev-image-*.yml")):
        problems = check_workflow(path) + (
            check_pi_workflow() if path.name == "dev-image-pi.yml" else []
        )
        if problems:
            failed = True
            print(f"FAIL {path.name}")
            for problem in problems:
                print(f"      {problem}")
        else:
            print(f"ok   {path.name}: publish gate holds for {len(CASES)} event/branch cases")
    if failed:
        print(
            "\nA dev-image workflow can write a permanent registry tag; see "
            "CONTRIBUTING.md for the tag policy.",
            file=sys.stderr,
        )
        return 1
    print("\nAll dev-image workflows gate publishing on an explicit decision.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
