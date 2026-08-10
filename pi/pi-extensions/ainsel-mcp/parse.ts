export interface ServerRef {
	name: string;
	url: string;
	token?: string;
}

/**
 * Parse the MCP_SERVERS env value: comma-separated "name=url" pairs.
 * Whitespace around entries is tolerated. Empty input returns []. Throws
 * on malformed input. Duplicate server names are dropped; the first
 * occurrence wins (the controller assembles MCP_SERVERS from multiple
 * sources and may emit the same server twice).
 */
export function parseServers(env: string | undefined): ServerRef[] {
	const trimmed = (env ?? "").trim();
	if (trimmed === "") return [];
	const parts = trimmed.split(",");
	const out: ServerRef[] = [];
	const seen = new Set<string>();
	for (const raw of parts) {
		const p = raw.trim();
		const eq = p.indexOf("=");
		if (eq < 1 || eq === p.length - 1) {
			throw new Error(
				`malformed MCP_SERVERS entry ${JSON.stringify(p)} (want name=url)`,
			);
		}
		const name = p.slice(0, eq).trim();
		const url = p.slice(eq + 1).trim();
		if (name === "" || url === "") {
			throw new Error(
				`malformed MCP_SERVERS entry ${JSON.stringify(p)} (want name=url)`,
			);
		}
		if (seen.has(name)) continue;
		seen.add(name);
		out.push({ name, url });
	}
	return out;
}

/**
 * Parse the MCP_SERVER_TOKENS env value: comma-separated "name=token" pairs.
 * Returns a map from server name to token. Empty/unset input returns {}.
 */
export function parseTokens(env: string | undefined): Record<string, string> {
	const trimmed = (env ?? "").trim();
	if (trimmed === "") return {};
	const parts = trimmed.split(",");
	const out: Record<string, string> = {};
	for (const raw of parts) {
		const p = raw.trim();
		const eq = p.indexOf("=");
		if (eq < 1 || eq === p.length - 1) continue;
		const name = p.slice(0, eq).trim();
		const token = p.slice(eq + 1).trim();
		if (name !== "" && token !== "") {
			out[name] = token;
		}
	}
	return out;
}
