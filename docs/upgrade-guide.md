# Upgrade Guide

This guide covers how to upgrade an existing Ainsel installation to a newer chart version.

## General upgrade procedure

Before upgrading, complete these pre-upgrade steps:

1. **Back up the Postgres database.** All persistent state (agents, personas, triggers, connectors, invocation history) lives in Postgres. Create a backup before proceeding.
2. **Read the changelog.** Review [`CHANGELOG.md`](https://github.com/DominikPinsel/ainsel/blob/main/CHANGELOG.md) or the notes for the target version on the [releases page](https://github.com/DominikPinsel/ainsel/releases). Breaking changes are listed first.
3. **Check for breaking changes.** See [Checking for breaking changes](#checking-for-breaking-changes) below.
4. **Fetch the released chart.** Charts are published to GHCR as OCI artifacts,
   one per release, tagged with the chart version:

   ```bash
   helm pull oci://ghcr.io/dominikpinsel/charts/ainsel --version <X.Y.Z>
   ```

5. **Upgrade:**

   ```bash
   helm upgrade ainsel oci://ghcr.io/dominikpinsel/charts/ainsel \
     --version <X.Y.Z> -n <namespace> -f values.yaml
   ```

   If you are deploying directly from the repository chart directory:

   ```bash
   helm upgrade ainsel ./chart -n <namespace> -f values.yaml
   ```

## Checking for breaking changes

`CHANGELOG.md` at the root of this repository is generated and maintained by [release-please](https://github.com/googleapis/release-please) from conventional commits, and the same content is published on each GitHub Release. Entries are grouped by type (*BREAKING*, *Features*, *Bug Fixes*, *Miscellaneous Chores*) and linked to their commits and PRs.

To check whether the new chart version introduces new or changed values, diff the default values between versions:

```bash
# Export the new chart's default values
helm show values ainsel/ainsel > new-values.yaml

# Compare against your current values.yaml
diff values.yaml new-values.yaml
```

This reveals keys that are new (you may need to add them) or keys that have been renamed or removed (you must update your `values.yaml` accordingly).

## CRD upgrades

Helm does **not** upgrade CRDs automatically on `helm upgrade`. This is intentional: Helm skips CRD updates during upgrades to prevent accidental data loss caused by destructive schema changes.

If a release introduces new or changed CRDs, apply them manually before or after the Helm upgrade:

```bash
kubectl apply -f chart/templates/crds/
```

Check the changelog for the release to find out whether CRDs have changed and what the recommended apply order is.

## Database migrations

The hub backend runs database migrations automatically at startup. When you upgrade the hub image, any pending migrations are applied when the new pod starts.

If a migration fails, the hub pod will crash-loop. Diagnose with:

```bash
kubectl logs -n <namespace> deployment/ainsel-hub
```

Look for migration error messages near the top of the log output.

**Always back up Postgres before upgrading.** If a migration produces unexpected results, the only safe recovery path is to restore from the backup taken before the upgrade.

## Rolling back

List available revisions:

```bash
helm history ainsel -n <namespace>
```

Roll back to a previous revision:

```bash
helm rollback ainsel <revision> -n <namespace>
```

> **Important:** Rolling back the Helm release restores the previous Kubernetes resources (Deployments, ConfigMaps, etc.) and the previous image versions. It does **not** reverse any database migrations that have already run. If the migration must be reversed, restore the Postgres database from the backup taken before the upgrade.

## Version-specific notes

### Upgrading to v0.1

No breaking changes.
