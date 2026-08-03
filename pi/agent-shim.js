// agent-shim.js — implements the `agent --list-tools` contract.
//
// hub-backend's AgentImage tool-sync Job invokes the agent image with
// `Command: ["agent", "--list-tools"]`. In the legacy ainsel-ai-agent
// image that hits a Go binary; in the pi-native image it hits this script
// (via entrypoint.sh dispatch, and via /usr/local/bin/agent which execs
// us with the same arg).
//
// Output is the documented `listToolsManifest` shape:
//   { "schemaVersion": "v1",
//     "tools": [
//       { "name": "forgejo", "description": "Read and write Forgejo..." },
//       ...
//     ] }
//
// Each tool is discovered by listing AINSEL_TOOLS_DIR (default
// /usr/local/bin/tools) for executables, then introspecting each one via
// `<binary> --schema` to harvest its description. Tools that don't
// support --schema yet emit just {name}; hub-backend tolerates missing
// descriptions.

import { execFileSync } from "node:child_process";
import { readdirSync, statSync } from "node:fs";
import path from "node:path";

const TOOLS_DIR = process.env.AINSEL_TOOLS_DIR ?? "/usr/local/bin/tools";

function listToolBinaries() {
	let entries;
	try {
		entries = readdirSync(TOOLS_DIR);
	} catch (err) {
		console.error(`agent-shim: cannot read ${TOOLS_DIR}: ${err.message}`);
		return [];
	}
	const bins = [];
	for (const name of entries.sort()) {
		const full = path.join(TOOLS_DIR, name);
		let st;
		try {
			st = statSync(full);
		} catch {
			continue;
		}
		// Skip non-files and non-executables.
		if (!st.isFile() || (st.mode & 0o111) === 0) continue;
		bins.push({ name, full });
	}
	return bins;
}

function describeTool(bin) {
	// Try --schema first (rich metadata). If absent / errors / not JSON, fall
	// back to a bare name entry — hub-backend handles missing descriptions.
	const fallback = { name: bin.name };
	try {
		const out = execFileSync(bin.full, ["--schema"], {
			encoding: "utf-8",
			timeout: 5000,
			stdio: ["ignore", "pipe", "pipe"],
		});
		const schema = JSON.parse(out);
		const entry = { name: schema.name || bin.name };
		if (typeof schema.description === "string" && schema.description !== "") {
			entry.description = schema.description;
		}
		return entry;
	} catch {
		return fallback;
	}
}

function main() {
	if (process.argv[2] !== "--list-tools") {
		console.error("agent-shim: only --list-tools is supported");
		process.exit(2);
	}

	const tools = listToolBinaries().map(describeTool);
	process.stdout.write(
		`${JSON.stringify({ schemaVersion: "v1", tools }, null, 2)}\n`,
	);
}

main();
