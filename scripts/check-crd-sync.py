#!/usr/bin/env python3
"""Fail if a chart CRD copy drifts from its operator base.

The chart ships CRDs as templates (chart/templates/crds/) so helm upgrades
apply schema changes. The manifests are copied verbatim from the operator
bases (plus a helm.sh/resource-policy annotation); this check compares the
`spec` sections so a base change that forgets the chart copy fails CI
instead of silently leaving clusters on a stale schema.

Run from the repository root: python3 scripts/check-crd-sync.py
"""

import sys
import json
from pathlib import Path

import yaml

PAIRS = [
    ("chart/templates/crds/agent.yaml",
     "operators/agent/config/crd/bases/ainsel.dev_agents.yaml"),
    ("chart/templates/crds/agentimage.yaml",
     "operators/agent/config/crd/bases/ainsel.dev_agentimages.yaml"),
    ("chart/templates/crds/webhookconnector.yaml",
     "operators/event-gateway/config/crd/bases/ainsel.dev_webhookconnectors.yaml"),
]


def main() -> int:
    root = Path(__file__).resolve().parent.parent
    failed = False
    for chart_rel, base_rel in PAIRS:
        chart = yaml.safe_load((root / chart_rel).read_text())
        base = yaml.safe_load((root / base_rel).read_text())
        chart_spec = json.dumps(chart.get("spec"), sort_keys=True)
        base_spec = json.dumps(base.get("spec"), sort_keys=True)
        if chart_spec != base_spec:
            failed = True
            print(f"CRD out of sync: {chart_rel}")
            print(f"  operator base changed without updating the chart copy: {base_rel}")
            print("  fix: copy the base file into chart/templates/crds/ (keep the")
            print("  helm.sh/resource-policy: keep annotation)")
    if failed:
        return 1
    print("chart CRD copies match their operator bases")
    return 0


if __name__ == "__main__":
    sys.exit(main())