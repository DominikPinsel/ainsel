# Changelog

## [0.2.0](https://github.com/DominikPinsel/ainsel/compare/v0.1.5...v0.2.0) (2026-10-10)


### ⚠ BREAKING CHANGES

* Agent.spec.enabledMCPs is removed from the schema. Agents that still carry it must be reconciled by an operator that includes the migration (shipped one release earlier) before this CRD is applied, or their MCP servers have to be re-selected on the agent's Tools tab.
* **release:** Agent.spec.enabledMCPs is removed from the schema. Agents that still carry it must be reconciled by an operator that includes the migration (shipped one release earlier) before this CRD is applied, or their MCP servers have to be re-selected on the agent's Tools tab.
* **api:** Agent.spec.enabledMCPs is removed from the schema. Agents that still carry it must be reconciled by an operator that includes the migration (shipped one release earlier) before this CRD is applied, or their MCP servers have to be re-selected on the agent's Tools tab.

### Features

* add Channels UI with event journey visualization ([#215](https://github.com/DominikPinsel/ainsel/issues/215)) ([77e0a39](https://github.com/DominikPinsel/ainsel/commit/77e0a39c197a1ca9c1e8afe29f8760ade7e2f77b))
* add pi-auto-compact extension to the agent runtime ([#336](https://github.com/DominikPinsel/ainsel/issues/336)) ([03bdc24](https://github.com/DominikPinsel/ainsel/commit/03bdc2450d61ff682057b4baccdf52db38da9822)), closes [#335](https://github.com/DominikPinsel/ainsel/issues/335)
* add spec.llm.vision so pi sends images to vision-capable models ([#180](https://github.com/DominikPinsel/ainsel/issues/180)) ([631dc74](https://github.com/DominikPinsel/ainsel/commit/631dc74e6f76d58fe49842749bd1d16210cf5cfb))
* authentication without mandatory OIDC — no-auth UI + local users + chart auth.mode ([#108](https://github.com/DominikPinsel/ainsel/issues/108)) ([#109](https://github.com/DominikPinsel/ainsel/issues/109)) ([f14078b](https://github.com/DominikPinsel/ainsel/commit/f14078b6b73d3346c06c44949a8e60cb9a181c93))
* **channels:** reflect the revised channel model in the UI ([#231](https://github.com/DominikPinsel/ainsel/issues/231)) ([13707b3](https://github.com/DominikPinsel/ainsel/commit/13707b32e048e908c67d4c09f7fc289cb831fc24))
* **channels:** relation graph with mermaid — one level up, downstream only ([#241](https://github.com/DominikPinsel/ainsel/issues/241)) ([a8b2500](https://github.com/DominikPinsel/ainsel/commit/a8b2500b35166dbd1d7583c930cc9f7e957ad04b))
* **channels:** roleless channels — table view, real connections, no built-ins ([#233](https://github.com/DominikPinsel/ainsel/issues/233)) ([82506e8](https://github.com/DominikPinsel/ainsel/commit/82506e806c9c4af0790e4806bfe89e94f222d8b1))
* **channels:** the hub owns channels — registry, routing, API, MCP, console ([#238](https://github.com/DominikPinsel/ainsel/issues/238)) ([0606578](https://github.com/DominikPinsel/ainsel/commit/0606578c59fa3d7676450e9e1c8f484d9dc3ea77))
* **channels:** three channel kinds + relation tree, custom channels ([#234](https://github.com/DominikPinsel/ainsel/issues/234)) ([adfb373](https://github.com/DominikPinsel/ainsel/commit/adfb373019afd55e46d719718750cb2616fcb01d))
* **ci:** add a product release workflow with generated changelogs ([#190](https://github.com/DominikPinsel/ainsel/issues/190)) ([8102cbf](https://github.com/DominikPinsel/ainsel/commit/8102cbf18174a4257a6a68916d77c94d5c8ef5c4))
* **ci:** Docker Hub tag retention policy and collection pass ([#228](https://github.com/DominikPinsel/ainsel/issues/228)) ([954cf67](https://github.com/DominikPinsel/ainsel/commit/954cf6773fed2456decd54054c38d4b295a92558))
* **ci:** drive releases with release-please, and require conventional PR titles ([#193](https://github.com/DominikPinsel/ainsel/issues/193)) ([9715193](https://github.com/DominikPinsel/ainsel/commit/9715193d9cb7f1f361cce74682bd0353cf2de746))
* default ainsel models to unsloth/gpt-oss-120b-GGUF:F16 ([#337](https://github.com/DominikPinsel/ainsel/issues/337)) ([6025ab6](https://github.com/DominikPinsel/ainsel/commit/6025ab61033fde26745146c6f45bbc88b16e6cc3))
* **frontend:** add outcome filter to the activity view ([#57](https://github.com/DominikPinsel/ainsel/issues/57)) ([d2a31a2](https://github.com/DominikPinsel/ainsel/commit/d2a31a2562e742b8ceb7707ea5adffe45da802e5))
* **frontend:** collapsible sidebar for small mobile displays ([#46](https://github.com/DominikPinsel/ainsel/issues/46)) ([4cfdc6a](https://github.com/DominikPinsel/ainsel/commit/4cfdc6a4ed3ba41c3b6fd7628de97efd4c5787ae)), closes [#41](https://github.com/DominikPinsel/ainsel/issues/41)
* **frontend:** Fjord design overhaul ([#83](https://github.com/DominikPinsel/ainsel/issues/83)) ([af55f3d](https://github.com/DominikPinsel/ainsel/commit/af55f3de8bc518f469ebafd3a0d6a0954354427a))
* live conversation tracking — persist invocations, report per turn, poll event detail ([#157](https://github.com/DominikPinsel/ainsel/issues/157)) ([0f456fe](https://github.com/DominikPinsel/ainsel/commit/0f456fecdb7ac97b2d9602ea3fa9d72c4a9a5c2c))
* **mcp:** rename agents and connectors without recreating them ([#240](https://github.com/DominikPinsel/ainsel/issues/240)) ([2fdbedb](https://github.com/DominikPinsel/ainsel/commit/2fdbedb9b63afdb9047e076c0ade3c19e48db1c8))
* **nav:** list the agents you last opened under Agents ([#235](https://github.com/DominikPinsel/ainsel/issues/235)) ([c79de58](https://github.com/DominikPinsel/ainsel/commit/c79de581a9511d4c629a52b7c154c6f144f6e7d5))
* **observability:** explain empty invocation transcripts via queue state ([#198](https://github.com/DominikPinsel/ainsel/issues/198)) ([c04e5fa](https://github.com/DominikPinsel/ainsel/commit/c04e5fa66b76b2603fcce12fae589b4c82677014)), closes [#195](https://github.com/DominikPinsel/ainsel/issues/195)
* park agents when their queue drains and wake them on the next event ([#247](https://github.com/DominikPinsel/ainsel/issues/247)) ([798306b](https://github.com/DominikPinsel/ainsel/commit/798306b52737bff341fcb368c07ed8b242e3eba3))
* publish docs as GitHub Pages site ([#89](https://github.com/DominikPinsel/ainsel/issues/89)) ([6946d01](https://github.com/DominikPinsel/ainsel/commit/6946d016b5d5c221fb45ae3b153b6c6f53c1144b)), closes [#88](https://github.com/DominikPinsel/ainsel/issues/88)
* **runner:** inline raw event payload into agent prompt ([#84](https://github.com/DominikPinsel/ainsel/issues/84)) ([1464b27](https://github.com/DominikPinsel/ainsel/commit/1464b27932f75f1bc70d8be194b4e599c56b8028))


### Bug Fixes

* allow ingress-controller traffic to hub-backend, drop obsolete N… ([9ae41dd](https://github.com/DominikPinsel/ainsel/commit/9ae41dd52fa7af174ac2d52093b19935bcfbda31))
* allow ingress-controller traffic to hub-backend, drop obsolete NATS policies ([d876029](https://github.com/DominikPinsel/ainsel/commit/d8760297942ffe5b6e8c59097ac742edbbfba69a))
* **channels:** align the Last 24h count columns in the register table ([#246](https://github.com/DominikPinsel/ainsel/issues/246)) ([815ee22](https://github.com/DominikPinsel/ainsel/commit/815ee22f032b27f67497bf8d3a70d573d1931bdd))
* **chart:** allow webhook connector pods to reach hub-backend under netpol ([#12](https://github.com/DominikPinsel/ainsel/issues/12)) ([106fae3](https://github.com/DominikPinsel/ainsel/commit/106fae3bd5dd24e544e537569b6f141e7307ad41))
* **chart:** make fresh standalone installs work out of the box ([#106](https://github.com/DominikPinsel/ainsel/issues/106)) ([31ead59](https://github.com/DominikPinsel/ainsel/commit/31ead593b225d8b1ac1ba22a8bdd40386d84f980))
* **chart:** robust agent pod selectors under netpol; add connectors-webhook-ingress ([#13](https://github.com/DominikPinsel/ainsel/issues/13)) ([d24200c](https://github.com/DominikPinsel/ainsel/commit/d24200c7b2cd42cf06fa2dc5d1145014d323284b))
* **chart:** ship CRDs as templates so upgrades apply schema changes ([#160](https://github.com/DominikPinsel/ainsel/issues/160)) ([e7836a3](https://github.com/DominikPinsel/ainsel/commit/e7836a3d6cdaffe6c9917fc60575d265a816b2aa))
* **ci:** migrate docs-site to ESLint flat config so Pages can deploy again ([#275](https://github.com/DominikPinsel/ainsel/issues/275)) ([4d48424](https://github.com/DominikPinsel/ainsel/commit/4d484242cba6e43752be3eca49f22c5d63ffeea7))
* **ci:** name the setting that has blocked every release since the start ([#230](https://github.com/DominikPinsel/ainsel/issues/230)) ([4dea70c](https://github.com/DominikPinsel/ainsel/commit/4dea70c07744a190b32b59d9b1bd14a1aee8cf6c))
* **ci:** push images on workflow_dispatch runs, not only on push ([#38](https://github.com/DominikPinsel/ainsel/issues/38)) ([6013dc6](https://github.com/DominikPinsel/ainsel/commit/6013dc6ed0b5f47bc112ff03c169c0f6fbae1973))
* **ci:** raise the Node floor to 24 so the frontend toolchain can run ([#189](https://github.com/DominikPinsel/ainsel/issues/189)) ([75e55f9](https://github.com/DominikPinsel/ainsel/commit/75e55f922a885950598e8fe12103c821a20e6f3e))
* **ci:** read Docker Hub username from vars instead of secrets ([#39](https://github.com/DominikPinsel/ainsel/issues/39)) ([9ea905d](https://github.com/DominikPinsel/ainsel/commit/9ea905d75950e868de111e3a7f22fdfa15ea5ea1))
* **ci:** set up envtest binaries before operator tests ([23c11ef](https://github.com/DominikPinsel/ainsel/commit/23c11efbba4a552f8335278d110a3f68f6728e5f))
* **ci:** stop main builds from publishing the :dev image tag ([#182](https://github.com/DominikPinsel/ainsel/issues/182)) ([786bf07](https://github.com/DominikPinsel/ainsel/commit/786bf07679acddfbe9596679c3eb9114e7555292))
* **ci:** stop writing registry tags nothing intends to publish ([#229](https://github.com/DominikPinsel/ainsel/issues/229)) ([94cf4c6](https://github.com/DominikPinsel/ainsel/commit/94cf4c6bc1e77e4bc31d5a3b6587a23217c9b7db))
* dedupe MCP server entries and retry sidecar connections on startup ([#60](https://github.com/DominikPinsel/ainsel/issues/60)) ([3a6f280](https://github.com/DominikPinsel/ainsel/commit/3a6f2808851b040f4a4b5f486d0cca57f44ab8ad))
* **deps:** bump brace-expansion overrides to fix high-severity audit ([10a9407](https://github.com/DominikPinsel/ainsel/commit/10a94076ff06b20c2a7c72ca99d267d6dbc789b7))
* expose chat endpoints under /api/internal for the agent sidecar ([#59](https://github.com/DominikPinsel/ainsel/issues/59)) ([83b8bb8](https://github.com/DominikPinsel/ainsel/commit/83b8bb8a1b662d68afff3977fb2b71e5e53e9e85))
* **frontend:** break infinite refresh loop on expired oidc sessions ([#159](https://github.com/DominikPinsel/ainsel/issues/159)) ([33e78c5](https://github.com/DominikPinsel/ainsel/commit/33e78c5db2bfce42cb120db2ef6a8c4ba3e34d58))
* **frontend:** break infinite refresh loop on expired oidc sessions ([#159](https://github.com/DominikPinsel/ainsel/issues/159)) ([#183](https://github.com/DominikPinsel/ainsel/issues/183)) ([edfa60b](https://github.com/DominikPinsel/ainsel/commit/edfa60b1a7bc78e4c5bc568c2c91841d9a5f93ef))
* **frontend:** keep the payload well dark in every theme ([#248](https://github.com/DominikPinsel/ainsel/issues/248)) ([c9addb2](https://github.com/DominikPinsel/ainsel/commit/c9addb25a9801697e9a80f1a6e09513b98fc90f8))
* **hub:** carry filter `values` across the trigger API — fixes [#251](https://github.com/DominikPinsel/ainsel/issues/251) ([#252](https://github.com/DominikPinsel/ainsel/issues/252)) ([0030eff](https://github.com/DominikPinsel/ainsel/commit/0030eff8522367bdbc10973a6b2c4f16015bd65f))
* **hub:** honor subject filter in queue/recent endpoint ([#81](https://github.com/DominikPinsel/ainsel/issues/81)) ([dc762a8](https://github.com/DominikPinsel/ainsel/commit/dc762a8cd270872584495c8a886e005e43719a98)), closes [#77](https://github.com/DominikPinsel/ainsel/issues/77)
* **hub:** report pre-limit match count as invocations total ([#80](https://github.com/DominikPinsel/ainsel/issues/80)) ([b117511](https://github.com/DominikPinsel/ainsel/commit/b1175117baeb9c9fad49751c475bfffaaa7e02ed)), closes [#76](https://github.com/DominikPinsel/ainsel/issues/76)
* make hub internal token platform-managed so image config cannot break the claim path ([#42](https://github.com/DominikPinsel/ainsel/issues/42)) ([8ecbdc9](https://github.com/DominikPinsel/ainsel/commit/8ecbdc92419187e43d445c1236cb211a24bb53cb))
* make the full activity history queryable (server-side pagination) ([#82](https://github.com/DominikPinsel/ainsel/issues/82)) ([b720605](https://github.com/DominikPinsel/ainsel/commit/b72060549827a1d90bd2c76fbbde8f81918629d8))
* **mcp:** forward groupId on trigger creation for authz-enabled hubs ([#78](https://github.com/DominikPinsel/ainsel/issues/78)) ([c1f9058](https://github.com/DominikPinsel/ainsel/commit/c1f9058b62d6650fbdb800e560d332375d7b1544)), closes [#58](https://github.com/DominikPinsel/ainsel/issues/58)
* **mcp:** return compact summaries from list_recent_events by default ([#79](https://github.com/DominikPinsel/ainsel/issues/79)) ([438d5ef](https://github.com/DominikPinsel/ainsel/commit/438d5eff2bf683f7d7e42f2ca1a0bf352c8bd42b)), closes [#75](https://github.com/DominikPinsel/ainsel/issues/75)
* **operator:** fold the runtime profile's MCP servers into the migration ([#166](https://github.com/DominikPinsel/ainsel/issues/166)) ([aa78f35](https://github.com/DominikPinsel/ainsel/commit/aa78f35dd77b6864d724200994255994e3421d0b))
* **operator:** keep the profile's MCP token when a legacy name collides ([#167](https://github.com/DominikPinsel/ainsel/issues/167)) ([e43a25b](https://github.com/DominikPinsel/ainsel/commit/e43a25b03685d8d46f64514d3a4052add3e89ab0))
* preserve existing agent Deployment selector to unbreak reconciles ([#11](https://github.com/DominikPinsel/ainsel/issues/11)) ([903cafd](https://github.com/DominikPinsel/ainsel/commit/903cafd380ddfaf068fd6e963a85795724512333))
* **profile:** show Expired instead of Active for tokens past their expiry ([#197](https://github.com/DominikPinsel/ainsel/issues/197)) ([0543cba](https://github.com/DominikPinsel/ainsel/commit/0543cbae49df26254ae7ae11de65589590fd8ce6)), closes [#152](https://github.com/DominikPinsel/ainsel/issues/152)
* repair observability dashboard data (tokens 0, errors 0, KPI decimals) ([#85](https://github.com/DominikPinsel/ainsel/issues/85)) ([d0faa91](https://github.com/DominikPinsel/ainsel/commit/d0faa9176ed81d3fccc497cb85df796bc1c5e1c5))
* resolve OIDC endpoints via discovery instead of hardcoded paths ([#86](https://github.com/DominikPinsel/ainsel/issues/86)) ([c5438e9](https://github.com/DominikPinsel/ainsel/commit/c5438e97509434e3f1ac7dda43340c8b542d58cc))
* roll connector deployment when webhook secret is rotated ([#74](https://github.com/DominikPinsel/ainsel/issues/74)) ([a325861](https://github.com/DominikPinsel/ainsel/commit/a325861ca2d05cdb8a6ccc51ce5a0ca169e93c57))


### Miscellaneous Chores

* merge main into develop — reconcile the two Dependabot lanes ([#232](https://github.com/DominikPinsel/ainsel/issues/232)) ([3615e86](https://github.com/DominikPinsel/ainsel/commit/3615e8617f0593279947748098e31f77173cd5ca))
* **release:** promote develop to main — agent-centric rework and release-please pipeline ([#187](https://github.com/DominikPinsel/ainsel/issues/187)) ([0ae751b](https://github.com/DominikPinsel/ainsel/commit/0ae751bc843eb188edb584478444820de415a781))


### Code Refactoring

* **api:** remove Agent.spec.enabledMCPs ([#181](https://github.com/DominikPinsel/ainsel/issues/181)) ([22f1cc3](https://github.com/DominikPinsel/ainsel/commit/22f1cc38dc6def9484b7f998124533687d53abd0))
