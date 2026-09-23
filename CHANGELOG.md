# Changelog

## [0.2.0](https://github.com/DominikPinsel/ainsel/compare/v0.1.5...v0.2.0) (2026-09-23)


### ⚠ BREAKING CHANGES

* **release:** Agent.spec.enabledMCPs is removed from the schema. Agents that still carry it must be reconciled by an operator that includes the migration (shipped one release earlier) before this CRD is applied, or their MCP servers have to be re-selected on the agent's Tools tab.

### Features

* add spec.llm.vision so pi sends images to vision-capable models ([#180](https://github.com/DominikPinsel/ainsel/issues/180)) ([631dc74](https://github.com/DominikPinsel/ainsel/commit/631dc74e6f76d58fe49842749bd1d16210cf5cfb))
* authentication without mandatory OIDC — no-auth UI + local users + chart auth.mode ([#108](https://github.com/DominikPinsel/ainsel/issues/108)) ([#109](https://github.com/DominikPinsel/ainsel/issues/109)) ([f14078b](https://github.com/DominikPinsel/ainsel/commit/f14078b6b73d3346c06c44949a8e60cb9a181c93))
* **frontend:** add outcome filter to the activity view ([#57](https://github.com/DominikPinsel/ainsel/issues/57)) ([d2a31a2](https://github.com/DominikPinsel/ainsel/commit/d2a31a2562e742b8ceb7707ea5adffe45da802e5))
* **frontend:** collapsible sidebar for small mobile displays ([#46](https://github.com/DominikPinsel/ainsel/issues/46)) ([4cfdc6a](https://github.com/DominikPinsel/ainsel/commit/4cfdc6a4ed3ba41c3b6fd7628de97efd4c5787ae)), closes [#41](https://github.com/DominikPinsel/ainsel/issues/41)
* **frontend:** Fjord design overhaul ([#83](https://github.com/DominikPinsel/ainsel/issues/83)) ([af55f3d](https://github.com/DominikPinsel/ainsel/commit/af55f3de8bc518f469ebafd3a0d6a0954354427a))
* **observability:** explain empty invocation transcripts via queue state ([#198](https://github.com/DominikPinsel/ainsel/issues/198)) ([c04e5fa](https://github.com/DominikPinsel/ainsel/commit/c04e5fa66b76b2603fcce12fae589b4c82677014)), closes [#195](https://github.com/DominikPinsel/ainsel/issues/195)
* publish docs as GitHub Pages site ([#89](https://github.com/DominikPinsel/ainsel/issues/89)) ([6946d01](https://github.com/DominikPinsel/ainsel/commit/6946d016b5d5c221fb45ae3b153b6c6f53c1144b)), closes [#88](https://github.com/DominikPinsel/ainsel/issues/88)
* **runner:** inline raw event payload into agent prompt ([#84](https://github.com/DominikPinsel/ainsel/issues/84)) ([1464b27](https://github.com/DominikPinsel/ainsel/commit/1464b27932f75f1bc70d8be194b4e599c56b8028))


### Bug Fixes

* allow ingress-controller traffic to hub-backend, drop obsolete N… ([9ae41dd](https://github.com/DominikPinsel/ainsel/commit/9ae41dd52fa7af174ac2d52093b19935bcfbda31))
* allow ingress-controller traffic to hub-backend, drop obsolete NATS policies ([d876029](https://github.com/DominikPinsel/ainsel/commit/d8760297942ffe5b6e8c59097ac742edbbfba69a))
* **chart:** allow webhook connector pods to reach hub-backend under netpol ([#12](https://github.com/DominikPinsel/ainsel/issues/12)) ([106fae3](https://github.com/DominikPinsel/ainsel/commit/106fae3bd5dd24e544e537569b6f141e7307ad41))
* **chart:** make fresh standalone installs work out of the box ([#106](https://github.com/DominikPinsel/ainsel/issues/106)) ([31ead59](https://github.com/DominikPinsel/ainsel/commit/31ead593b225d8b1ac1ba22a8bdd40386d84f980))
* **chart:** robust agent pod selectors under netpol; add connectors-webhook-ingress ([#13](https://github.com/DominikPinsel/ainsel/issues/13)) ([d24200c](https://github.com/DominikPinsel/ainsel/commit/d24200c7b2cd42cf06fa2dc5d1145014d323284b))
* **ci:** push images on workflow_dispatch runs, not only on push ([#38](https://github.com/DominikPinsel/ainsel/issues/38)) ([6013dc6](https://github.com/DominikPinsel/ainsel/commit/6013dc6ed0b5f47bc112ff03c169c0f6fbae1973))
* **ci:** raise the Node floor to 24 so the frontend toolchain can run ([#189](https://github.com/DominikPinsel/ainsel/issues/189)) ([75e55f9](https://github.com/DominikPinsel/ainsel/commit/75e55f922a885950598e8fe12103c821a20e6f3e))
* **ci:** read Docker Hub username from vars instead of secrets ([#39](https://github.com/DominikPinsel/ainsel/issues/39)) ([9ea905d](https://github.com/DominikPinsel/ainsel/commit/9ea905d75950e868de111e3a7f22fdfa15ea5ea1))
* **ci:** set up envtest binaries before operator tests ([23c11ef](https://github.com/DominikPinsel/ainsel/commit/23c11efbba4a552f8335278d110a3f68f6728e5f))
* **ci:** stop main builds from publishing the :dev image tag ([#182](https://github.com/DominikPinsel/ainsel/issues/182)) ([786bf07](https://github.com/DominikPinsel/ainsel/commit/786bf07679acddfbe9596679c3eb9114e7555292))
* dedupe MCP server entries and retry sidecar connections on startup ([#60](https://github.com/DominikPinsel/ainsel/issues/60)) ([3a6f280](https://github.com/DominikPinsel/ainsel/commit/3a6f2808851b040f4a4b5f486d0cca57f44ab8ad))
* **deps:** bump brace-expansion overrides to fix high-severity audit ([10a9407](https://github.com/DominikPinsel/ainsel/commit/10a94076ff06b20c2a7c72ca99d267d6dbc789b7))
* expose chat endpoints under /api/internal for the agent sidecar ([#59](https://github.com/DominikPinsel/ainsel/issues/59)) ([83b8bb8](https://github.com/DominikPinsel/ainsel/commit/83b8bb8a1b662d68afff3977fb2b71e5e53e9e85))
* **frontend:** break infinite refresh loop on expired oidc sessions ([#159](https://github.com/DominikPinsel/ainsel/issues/159)) ([33e78c5](https://github.com/DominikPinsel/ainsel/commit/33e78c5db2bfce42cb120db2ef6a8c4ba3e34d58))
* **hub:** honor subject filter in queue/recent endpoint ([#81](https://github.com/DominikPinsel/ainsel/issues/81)) ([dc762a8](https://github.com/DominikPinsel/ainsel/commit/dc762a8cd270872584495c8a886e005e43719a98)), closes [#77](https://github.com/DominikPinsel/ainsel/issues/77)
* **hub:** report pre-limit match count as invocations total ([#80](https://github.com/DominikPinsel/ainsel/issues/80)) ([b117511](https://github.com/DominikPinsel/ainsel/commit/b1175117baeb9c9fad49751c475bfffaaa7e02ed)), closes [#76](https://github.com/DominikPinsel/ainsel/issues/76)
* make hub internal token platform-managed so image config cannot break the claim path ([#42](https://github.com/DominikPinsel/ainsel/issues/42)) ([8ecbdc9](https://github.com/DominikPinsel/ainsel/commit/8ecbdc92419187e43d445c1236cb211a24bb53cb))
* make the full activity history queryable (server-side pagination) ([#82](https://github.com/DominikPinsel/ainsel/issues/82)) ([b720605](https://github.com/DominikPinsel/ainsel/commit/b72060549827a1d90bd2c76fbbde8f81918629d8))
* **mcp:** forward groupId on trigger creation for authz-enabled hubs ([#78](https://github.com/DominikPinsel/ainsel/issues/78)) ([c1f9058](https://github.com/DominikPinsel/ainsel/commit/c1f9058b62d6650fbdb800e560d332375d7b1544)), closes [#58](https://github.com/DominikPinsel/ainsel/issues/58)
* **mcp:** return compact summaries from list_recent_events by default ([#79](https://github.com/DominikPinsel/ainsel/issues/79)) ([438d5ef](https://github.com/DominikPinsel/ainsel/commit/438d5eff2bf683f7d7e42f2ca1a0bf352c8bd42b)), closes [#75](https://github.com/DominikPinsel/ainsel/issues/75)
* preserve existing agent Deployment selector to unbreak reconciles ([#11](https://github.com/DominikPinsel/ainsel/issues/11)) ([903cafd](https://github.com/DominikPinsel/ainsel/commit/903cafd380ddfaf068fd6e963a85795724512333))
* **profile:** show Expired instead of Active for tokens past their expiry ([#197](https://github.com/DominikPinsel/ainsel/issues/197)) ([0543cba](https://github.com/DominikPinsel/ainsel/commit/0543cbae49df26254ae7ae11de65589590fd8ce6)), closes [#152](https://github.com/DominikPinsel/ainsel/issues/152)
* repair observability dashboard data (tokens 0, errors 0, KPI decimals) ([#85](https://github.com/DominikPinsel/ainsel/issues/85)) ([d0faa91](https://github.com/DominikPinsel/ainsel/commit/d0faa9176ed81d3fccc497cb85df796bc1c5e1c5))
* resolve OIDC endpoints via discovery instead of hardcoded paths ([#86](https://github.com/DominikPinsel/ainsel/issues/86)) ([c5438e9](https://github.com/DominikPinsel/ainsel/commit/c5438e97509434e3f1ac7dda43340c8b542d58cc))
* roll connector deployment when webhook secret is rotated ([#74](https://github.com/DominikPinsel/ainsel/issues/74)) ([a325861](https://github.com/DominikPinsel/ainsel/commit/a325861ca2d05cdb8a6ccc51ce5a0ca169e93c57))


### Miscellaneous Chores

* **release:** promote develop to main — agent-centric rework and release-please pipeline ([#187](https://github.com/DominikPinsel/ainsel/issues/187)) ([0ae751b](https://github.com/DominikPinsel/ainsel/commit/0ae751bc843eb188edb584478444820de415a781))
