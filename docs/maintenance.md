# Registry maintenance

Docker Hub is the artifact store for every image this project builds, and it has
no server-side retention for a personal namespace. Nothing in the pipeline ever
deleted a tag, so the registry only ever grew: **456 tags / 125.71 GB across 11
repositories** when this document was written, 91% of the bytes in the three `pi*
images. This page is the policy that stops that, plus the runbook for applying it.

Related: [`CONTRIBUTING.md`](../CONTRIBUTING.md) (CI overview, release flow),
[`docs/deployment.md`](deployment.md) (installing the chart),
[`pi/README.md`](../pi/README.md) (pi image variants).

## Tag taxonomy

Every tag in `dpinsel/ainsel-*` belongs to exactly one class. If you add a
workflow that writes a new shape, add it here and to the patterns in
`scripts/hub-retention.py` — an unrecognised tag is *never* deleted, but it is
reported as `UNKNOWN`, and the goal is to keep that list empty.

| class | shape | written by | collected? |
|---|---|---|---|
| build | `:<short-sha>` (7 hex) | `dev-image-*.yml` on push to `main`/`develop` | yes |
| variant build | `<major.minor>-<short-sha>`, e.g. `1.24-0ae751b` | `dev-image-pi.yml` | yes |
| mutable | `:dev` | `dev-image-*.yml`, `develop` only | **never** |
| float | `:1.24`, `:8.0` | `dev-image-pi.yml` | **never** |
| cache | `buildcache`, `buildcache-v2` | no workflow — dead since `b66f4adc` (2026-07-20) | yes, see below |
| release | `:X.Y.Z`, `:vX.Y.Z`, `<float>-X.Y.Z` | `release.yml` | only by an explicit purge |
| `:latest` | `:latest` | `release.yml` | **never** |

`:dev` is what the `ainsel-dev` namespace runs; `:latest` and the semver tags are
what users pull. Everything else exists for provenance — the record of which
commit produced which image — and no automated consumer resolves them.

## Policy

Enforced by `scripts/hub-retention.py`, scheduled by
`.github/workflows/maintenance-untag.yml`:

- a build or variant-build tag becomes collectable after **30 days**
- the **newest 20** build tags per repo are always kept, regardless of age, so a
  recent image can always be pulled back without rebuilding
- the **newest 5** variant-build tags per repo are kept, not 20: `pi-maui` is
  ~3 GB per tag and nothing has ever pulled it by that tag, so 20 would pin about
  60 GB of provenance
- every other class is protected. Protected means *not deletable by this tool*,
  including on a purge

The two floors are per class on purpose. Ranking is also per class, so a repo's
protected tags cannot push a build's own tag out of its window.

**Cache tags are judged by evidence, not age.** A `buildcache*` tag is only
worth keeping while something writes it, so the tool reads `.github/workflows/`
for a `cache-to: type=registry:` line and protects the tags only if it finds one.
No workflow configures a registry cache today — every build is cold, and the four
`buildcache` / `buildcache-v2` tags left on the frontend and agent-operator repos
are leftovers of the pre-migration pipeline (~2.4 GB). A checkout whose workflows
cannot be read is treated as live (fail-protective), and `--keep-cache-tags`
forces protection. If caching is ever added back, the tags protect themselves
again with no change here.

At the time of writing, the default policy collects **223 of 456 tags, 86.38 GB**
(logical, an upper bound — layers shared with kept tags are not actually freed
until the registry garbage-collects them).

## Running it

From the repository root. Listing is anonymous (the repos are public), so a
report needs no credentials:

```bash
python3 scripts/hub-retention.py                       # whole namespace, dry run
python3 scripts/hub-retention.py --repo ainsel-pi-maui # one repo
python3 scripts/hub-retention.py --keep-cache-tags     # protect buildcache* anyway
python3 scripts/hub-retention.py --json | jq 'to_entries[].value | map(select(.disposition == "delete")) | length'
```

Deletion is opt-in and needs a token that can delete images:

```bash
export DOCKERHUB_USERNAME=dpinsel
export DOCKERHUB_TOKEN=...            # personal access token, delete:delete-image
python3 scripts/hub-retention.py --apply
```

`--apply` refuses to run on a policy that looks like a fat-fingered flag
(`--max-age-days 0`, `--keep-newest 0`, a malformed repo name). A dry run accepts
anything, so you can explore what a threshold would collect before authenticating.

Run the tests before changing any of the patterns:

```bash
python3 -m unittest scripts.test_hub_retention -v
```

## In CI

`.github/workflows/maintenance-untag.yml`

| trigger | what happens |
|---|---|
| `schedule` (Mon 05:17 UTC) | unit tests, then a **report-only** pass; the tag list lands in the run summary and a warning is raised if anything is collectable |
| `workflow_dispatch` | same, plus every threshold as an input; deletes only if `apply` is checked |
| `pull_request` touching the script or policy docs | unit tests only; the registry is untouched |

The cron never deletes. That is deliberate: an unattended job must not be able to
take an image out from under a running deployment, and a deletion should always
have a human who looked at a list first.

## Purging pre-pipeline release tags

The `release-please` pipeline has produced exactly one run, and it failed before
publishing anything, so there is **no release on GitHub** (`git tag -l` is empty)
while Docker Hub carries `0.1.0` on eight repos plus `0.2.0`, `0.2.1`, `0.3.0` on
the frontend. Those tags came from the Forgejo-era pipeline: they are two months
stale, mutually inconsistent per component, and `:latest` on several repos points
at them.

They are also what `chart/values.yaml` pins, which is why the purge is a separate,
deliberate step rather than part of the weekly pass:

```bash
python3 scripts/hub-retention.py --include-release-tags --yes \
  --latest-release <version of a published release> --apply
```

Three guards have to pass before a single tag is deleted:

1. `--include-release-tags` also requires `--yes`.
2. `--latest-release` is required and must be a version — the tool will not
   "purge" release tags when it cannot prove a newer release exists.
3. A tag is only collected if the supplied release is strictly newer, so the
   current release always survives its own purge.

Do not run it until a real release is published *and* `chart/values.yaml` no
longer pins the old tags (see "Known rough edge: chart image tags" in
[`CONTRIBUTING.md`](../CONTRIBUTING.md)).

## Rotation notes

`secrets.DOCKERHUB_TOKEN` is used by the build workflows to log in and push.
Deleting a tag through the Hub API is a distinct scope (`delete:delete-image` on a
granular personal access token), so the existing token may 403 on a first apply.
Create the token at <https://hub.docker.com/settings/security>, and keep it read
plus write plus delete.

`verify_credentials()` calls `GET /v2/user/` before touching anything, so a wrong
or under-scoped token fails with a message instead of half-applying a policy.

## Pull rate limits

Docker Hub's [usage and limits](https://docs.docker.com/docker-hub/download-rate-limit/)
table gives a Personal account **200 pulls per six hours when authenticated** and
**100 per six hours per IPv4 address (or IPv6 /64) when not**, "subject to fair
use".

An earlier draft of this page estimated that `deploy-dev.yml`'s digest check alone
spent "about 96 of the 100" anonymous budget every six hours. **That estimate is
wrong, and wrong in the useful direction** - measured against the live registry on
2026-09-23 from a single egress IP:

| what was sent | requests | `ratelimit-remaining` |
| --- | --- | --- |
| `HEAD .../manifests/dev` — exactly what the digest check does | 12 | `100` → `100`, unmoved |
| `GET` a config blob + a layer blob | 2 | `100` → `99` |
| `GET` manifest + one layer, three different repos | 6 | `99` → `97` |

A `HEAD` manifest probe is **not billed at all**, and a billed pull costs roughly
one unit per image rather than one per request: an image with 13 layers did not
turn into 13 units. The `docker-ratelimit-source: <ip>` header confirms attribution
is per address while anonymous.

That leaves the budget real headroom, so **the cluster deliberately keeps pulling
anonymously**: no `imagePullSecrets`, no credential to rotate, nothing in the chart
to keep honest.

Revisit it if either of these shows up, since they are the only ways this becomes a
problem:

- `kubectl describe pod` events on `ainsel-dev` show `429` / `toomanyrequests` -
  the shared-address case, where several agents' pods pull fresh layers at once
  through one node's egress IP
- `deploy-dev.yml` logs `::warning::digest lookup failed`. It fails open, so a
  throttled registry silently stops pinning new digests instead of breaking loudly

The fix in both cases has the same shape: a `kubernetes.io/dockerconfigjson`
secret in the namespace, referenced by `imagePullSecrets`, which the chart already
accepts per component (`chart/values.yaml`, default `[]`). Authenticated pulls
bill the account's 200 rather than the address's 100. Do it on that evidence, not
on arithmetic like the estimate above.
