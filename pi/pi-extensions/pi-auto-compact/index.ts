/**
 * Ainsel Pi Auto Compact extension.
 *
 * Thin adapter that loads the upstream `pi-auto-compact` npm package
 * (https://github.com/Chasen-Liao/pi-auto-compact, MIT) as a pi extension in
 * the ainsel-pi runtime. The heavy lifting lives upstream; this file only
 * re-exports the default factory so the entrypoint's per-extension index.ts
 * glob picks the extension up like every other runtime extension.
 *
 * What the extension does (see upstream and ./README.md for detail):
 *   - Before each idle prompt, when the projected context crosses the
 *     configured threshold (default 78% of the model's context window),
 *     it compacts once and only then lets the prompt through.
 *   - Pi's own mid-run compaction stays enabled as the safety net.
 *   - Threshold/config is read per prompt from ~/.pi/agent/pi-auto-compact.json.
 *
 * RPC-mode notes for this runtime: the image runs pi in `--mode rpc`, so
 * `ctx.hasUI` is false — status-bar and notify output are no-ops upstream,
 * and the `/compact-threshold` slash command never receives inputs here;
 * configuration is done via the JSON config file instead.
 *
 * Upstream package version is bumped in ./package.json ("pi-auto-compact").
 */

import autoCompact from "pi-auto-compact/extensions/auto-compact.ts";

export default autoCompact;