#!/usr/bin/env python3
"""Tests for scripts/hub-retention.py - the tag retention decision logic.

Run: python3 -m unittest scripts.test_hub_retention  (or python3 scripts/test_hub_retention.py)

These pin the boundaries the policy is named for: which tags are considered at
all, the newest-N floor, the max-age cutoff, and the guards that stop a purge
from deleting the only stable tags.
"""

import datetime as dt
import importlib.machinery
import importlib.util
import sys
import unittest
from pathlib import Path

_MODULE_PATH = Path(__file__).with_name("hub-retention.py")
_spec = importlib.util.spec_from_loader(
    "hub_retention",
    loader=importlib.machinery.SourceFileLoader("hub_retention", str(_MODULE_PATH)),
)
hr = importlib.util.module_from_spec(_spec)
# The hyphen in the filename means this is not a normal import; register the
# module before executing it so @dataclass can resolve its own __module__.
sys.modules["hub_retention"] = hr
_spec.loader.exec_module(hr)

NOW = dt.datetime(2026, 9, 23, 12, 0, 0, tzinfo=dt.timezone.utc)


def tag(name, age_days=0.0):
    return hr.Tag(name=name, pushed=NOW - dt.timedelta(days=age_days), full_size=1_000_000)


def policy(**kw):
    kw.setdefault("now", NOW)
    return hr.Policy(**kw)


def plan(tags, **kw):
    return hr.plan_for_repo("ainsel-test", tags, policy(**kw))


def dispositions(tags, **kw):
    return {d.tag.name: d.disposition for d in plan(tags, **kw)}


def names(decisions, *wanted):
    return sorted(
        d.tag.name for d in decisions if d.disposition in (wanted or (hr.Disposition.DELETE,))
    )


class TestClassify(unittest.TestCase):
    def test_build_tags_are_disposable(self):
        self.assertEqual(hr.classify("0ae751b"), "short-sha")
        self.assertEqual(hr.classify("1.24-9df41e5"), "variant-sha")
        self.assertEqual(hr.classify("8.0-0ae751b"), "variant-sha")

    def test_live_tags_are_protected(self):
        for name in ("latest", "dev", "main", "buildcache", "buildcache-v2"):
            self.assertEqual(hr.classify(name), "protected", name)
        # pi variant floating tags are what agents actually pull.
        for name in ("1.24", "8.0"):
            self.assertEqual(hr.classify(name), "protected", name)

    def test_release_tags_classify_as_release(self):
        for name in ("0.1.0", "0.2.1", "v0.3.0", "1.10.2", "0.4.0-rc.1"):
            self.assertEqual(hr.classify(name), "release", name)

    def test_variant_release_tags_classify_as_release(self):
        # `1.24-0.1.0` / `8.0-v0.2.0` are what the release pipeline leaves on the
        # pi variants; they must never look like build provenance.
        for name in ("1.24-0.1.0", "8.0-0.1.0", "1.24-v0.2.0", "8.0-0.3.0-rc.1"):
            self.assertEqual(hr.classify(name), "release", name)

    def test_unexpected_names_are_reported_not_collected(self):
        for name in ("sha-0ae751b", "0ae751b-dev", "develop", "1.2.3.4", "G0A0F00", "latest-dev"):
            self.assertEqual(hr.classify(name), "unknown", name)


class TestProtection(unittest.TestCase):
    def test_protected_tags_survive_any_window(self):
        old = [tag("dev", 900), tag("latest", 900), tag("main", 900), tag("8.0", 900)]
        self.assertEqual(names(plan(old)), [])
        for d in plan(old):
            self.assertEqual(d.disposition, hr.Disposition.KEEP_PROTECTED)

    def test_unknown_tags_are_never_deleted_even_when_ancient(self):
        self.assertEqual(names(plan([tag("nonsense-tag", 900)])), [])
        self.assertEqual(dispositions([tag("nonsense-tag", 900)])["nonsense-tag"],
                         hr.Disposition.KEEP_UNKNOWN)

    def test_release_tags_are_protected_by_default(self):
        self.assertEqual(names(plan([tag("0.1.0", 900)])), [])
        self.assertEqual(dispositions([tag("0.1.0", 900)])["0.1.0"],
                         hr.Disposition.KEEP_PROTECTED)


class TestNewestFloor(unittest.TestCase):
    def test_newest_n_are_kept_regardless_of_age(self):
        builds = [tag(f"{i:07d}", 900) for i in range(30)]
        kept = names(plan(builds, keep_newest=20, max_age_days=30), hr.Disposition.KEEP_NEWEST)
        self.assertEqual(len(kept), 20)
        # The kept set is the newest 20 (lowest index == newest pushed here).
        self.assertEqual(kept, [f"{i:07d}" for i in range(20)])

    def test_rank_boundary_is_exact(self):
        builds = [tag(f"{i:07d}", 900) for i in range(3)]
        got = names(plan(builds, keep_newest=2, max_age_days=30))
        self.assertEqual(got, ["0000002"])

    def test_zero_keep_newest_collects_everything_old(self):
        builds = [tag("aaaaaaa", 900), tag("bbbbbbb", 900)]
        self.assertEqual(len(names(plan(builds, keep_newest=0, max_age_days=30))), 2)


class TestPerClassFloors(unittest.TestCase):
    """The two disposable classes are floored differently: MB vs GB per tag."""

    def build(self, n, fmt):
        return [tag(fmt.format(i), 900) for i in range(n)]

    def test_short_sha_floor_is_twenty_by_default(self):
        got = names(plan(self.build(26, "{:07d}")))
        self.assertEqual(len(got), 6)

    def test_variant_sha_floor_is_five_by_default(self):
        got = names(plan(self.build(9, "1.24-{:07d}")))
        self.assertEqual(len(got), 4)

    def test_floors_do_not_leak_across_classes(self):
        mixed = self.build(6, "{:07d}") + self.build(9, "8.0-{:07d}")
        got = names(plan(mixed))
        self.assertEqual(len([g for g in got if "-" in g]), 4)
        self.assertEqual(len([g for g in got if "-" not in g]), 0)

    def test_variant_floor_can_be_overridden(self):
        got = names(plan(self.build(9, "1.24-{:07d}"), keep_newest_variant=0))
        self.assertEqual(len(got), 9)


class TestAgeWindow(unittest.TestCase):
    def test_boundary_at_exactly_max_age_is_kept(self):
        got = dispositions([tag("abcdef1", 30.0)], keep_newest=0, max_age_days=30)["abcdef1"]
        self.assertEqual(got, hr.Disposition.KEEP_RECENT)

    def test_one_second_past_the_window_is_collected(self):
        got = dispositions(
            [tag("abcdef1", 30.0 + 1 / 86400.0)], keep_newest=0, max_age_days=30
        )["abcdef1"]
        self.assertEqual(got, hr.Disposition.DELETE)

    def test_young_builds_are_kept_beyond_the_newest_floor(self):
        builds = [tag(f"{i:07d}", 1) for i in range(40)]
        self.assertEqual(names(plan(builds, keep_newest=20)), [])


class TestRanking(unittest.TestCase):
    def test_classes_are_ranked_independently(self):
        # A repo whose sha builds are all ancient must not let its protected
        # tags consume slots in the variant-sha keep-newest window.
        mixed = [
            tag("dev", 5),
            tag("buildcache", 5),
            tag("1.24-0000001", 900),
            tag("0000001", 900),
        ]
        got = names(plan(mixed, keep_newest=1, max_age_days=30))
        self.assertEqual(got, [])  # each class keeps its newest member

    def test_oldest_are_collected_first(self):
        builds = [tag("aaaaaaa", 10), tag("bbbbbbb", 100), tag("ccccccc", 1000)]
        got = names(plan(builds, keep_newest=0, max_age_days=30))
        self.assertEqual(got, ["bbbbbbb", "ccccccc"])


class TestReleasePurge(unittest.TestCase):
    def test_release_purge_needs_a_reference_version(self):
        p = policy(include_release_tags=True)
        self.assertIsNotNone(hr.release_purge_blocked(p))
        got = dispositions([tag("0.1.0", 900)], include_release_tags=True)["0.1.0"]
        self.assertEqual(got, hr.Disposition.KEEP_PROTECTED)

    def test_release_purge_rejects_a_malformed_version(self):
        p = policy(include_release_tags=True, latest_release="dev")
        self.assertIsNotNone(hr.release_purge_blocked(p))

    def test_current_release_survives_its_own_purge(self):
        got = dispositions(
            [tag("0.2.0", 1)], include_release_tags=True, latest_release="0.2.0"
        )["0.2.0"]
        self.assertEqual(got, hr.Disposition.KEEP_PROTECTED)

    def test_superseded_release_tags_are_collectable(self):
        got = dispositions(
            [tag("0.1.0", 900), tag("0.2.0", 1)],
            include_release_tags=True,
            latest_release="0.2.0",
        )
        self.assertEqual(got["0.1.0"], hr.Disposition.DELETE_RELEASE)
        self.assertEqual(got["0.2.0"], hr.Disposition.KEEP_PROTECTED)

    def test_version_compare_is_numeric_not_lexical(self):
        # "0.10.0" < "0.9.0" as strings; the purge must not rely on that.
        self.assertTrue(hr.parse_semver("0.10.0") > hr.parse_semver("0.9.0"))
        got = dispositions(
            [tag("0.9.0", 900)], include_release_tags=True, latest_release="0.10.0"
        )["0.9.0"]
        self.assertEqual(got, hr.Disposition.DELETE_RELEASE)

    def test_prerelease_reference_compares_on_core_version(self):
        got = dispositions(
            [tag("0.2.0", 900)], include_release_tags=True, latest_release="v0.2.0-rc.1"
        )["0.2.0"]
        self.assertEqual(got, hr.Disposition.KEEP_PROTECTED)


    def test_variant_release_tags_compare_on_their_version_not_their_prefix(self):
        # `1.24-0.1.0` must be judged as release 0.1.0, never as "1.24".
        got = dispositions(
            [tag("1.24-0.1.0", 900), tag("8.0-0.1.0", 900)],
            include_release_tags=True,
            latest_release="0.4.0",
        )
        self.assertEqual(got["1.24-0.1.0"], hr.Disposition.DELETE_RELEASE)
        self.assertEqual(got["8.0-0.1.0"], hr.Disposition.DELETE_RELEASE)
        still_current = dispositions(
            [tag("1.24-0.4.0", 1)], include_release_tags=True, latest_release="0.4.0"
        )["1.24-0.4.0"]
        self.assertEqual(still_current, hr.Disposition.KEEP_PROTECTED)

    def test_release_core_extracts_the_version(self):
        for name, want in [
            ("0.2.1", "0.2.1"),
            ("v0.2.1", "v0.2.1"),
            ("1.24-0.1.0", "0.1.0"),
            ("8.0-v0.3.0", "v0.3.0"),
            ("0.4.0-rc.1", "0.4.0-rc.1"),
        ]:
            self.assertEqual(hr.release_core(name), want, name)


class TestPolicyGuard(unittest.TestCase):
    """The apply-path guard against a fat-fingered threshold."""

    def test_sane_policy_has_no_problems(self):
        self.assertEqual(hr.policy_problems(policy(), hr.REPOS), [])

    def test_zero_age_is_refused(self):
        got = hr.policy_problems(policy(max_age_days=0), ["ainsel-mcp"])
        self.assertEqual(len(got), 1)
        self.assertIn("max-age-days", got[0])

    def test_zero_keep_newest_is_refused(self):
        got = hr.policy_problems(policy(keep_newest=0), ["ainsel-mcp"])
        self.assertTrue(any("keep-newest" in p for p in got))

    def test_negative_variant_floor_is_refused(self):
        got = hr.policy_problems(policy(keep_newest_variant=-1), ["ainsel-mcp"])
        self.assertTrue(any("variant" in p for p in got))

    def test_malformed_repo_name_is_refused(self):
        got = hr.policy_problems(policy(), ["ainsel-mcp; rm -rf /"])
        self.assertEqual(len(got), 1)

    def test_a_degenerate_policy_still_plans_for_dry_runs(self):
        # The guard lives in main(), not in decide(): exploring what a threshold
        # would collect must stay possible without credentials.
        decisions = plan([tag("abcdef1", 900)], max_age_days=0, keep_newest=0)
        self.assertEqual(decisions[0].disposition, hr.Disposition.DELETE)


class TestSummarise(unittest.TestCase):
    def test_counts_split_across_repos(self):
        decisions = plan([tag("dev", 900), tag("abcdef1", 900), tag("1234567", 900)], keep_newest=0)
        s = hr.summarise(decisions)["ainsel-test"]
        self.assertEqual(s["total"], 3)
        self.assertEqual(s["delete"], 2)
        self.assertEqual(s["bytes"], 2_000_000)

    def test_markdown_reports_dry_run_and_counts(self):
        decisions = {"ainsel-test": plan([tag("abcdef1", 900), tag("1234567", 1)], keep_newest=0)}
        md = hr.write_markdown(decisions, applied=False)
        self.assertIn("dry-run (nothing deleted)", md)
        self.assertIn("**1 tags collected.**", md)
        self.assertIn("`ainsel-test`", md)

    def test_markdown_flags_unmatched_tags(self):
        md = hr.write_markdown({"ainsel-test": plan([tag("weird-name", 900)])}, applied=False)
        self.assertIn("Tags no rule matched", md)
        self.assertIn("`ainsel-test:weird-name`", md)

    def test_markdown_says_applied_when_it_did(self):
        md = hr.write_markdown({"ainsel-test": plan([tag("abcdef1", 900)])}, applied=True)
        self.assertIn("apply (tags were deleted)", md)


if __name__ == "__main__":
    unittest.main()
