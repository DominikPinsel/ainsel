/**
 * Ainsel MCP Extension for Pi.
 *
 * Reads MCP_SERVERS from env (format: comma-separated name=url pairs).
 * Connects to each MCP server over streamable HTTP, lists its tools, and
 * registers them with `pi.registerTool` as mcp__<server>__<tool>.
 *
 * If MCP_SERVERS is unset/empty, the extension is a no-op.
 *
 * Auto-injects user_id from AGENT_NAME env into tool calls whose schema
 * declares a user_id property and the LLM didn't supply one.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { parseServers, parseTokens } from "./parse.js";
import { Catalog } from "./catalog.js";

interface McpToolDetails {
	status: "ok" | "error";
	summary: string;
	data?: unknown;
	error?: string;
}

export default async function ainselMcpExtension(
	pi: ExtensionAPI,
): Promise<void> {
	const envValue = process.env.MCP_SERVERS;
	let refs;
	try {
		refs = parseServers(envValue);
	} catch (err) {
		console.error(
			"pi-ainsel-mcp: invalid MCP_SERVERS env; starting without MCP tools",
			{ err: err instanceof Error ? err.message : String(err), raw: envValue },
		);
		return;
	}
	if (refs.length === 0) {
		console.error(
			"pi-ainsel-mcp: MCP_SERVERS empty or unset; no MCP tools registered",
		);
		return;
	}

	// Merge tokens from MCP_SERVER_TOKENS into server refs.
	const tokens = parseTokens(process.env.MCP_SERVER_TOKENS);
	refs = refs.map((ref) => ({
		...ref,
		token: tokens[ref.name] || ref.token,
	}));

	const catalog = new Catalog({ agentName: process.env.AGENT_NAME });
	await catalog.connect(refs);

	for (const tool of catalog.tools) {
		pi.registerTool({
			name: tool.name,
			label: tool.name,
			description: tool.description,
			parameters: tool.inputSchema as never,
			async execute(_toolCallId, params, signal) {
				try {
					const result = await catalog.call(
						tool.name,
						(params ?? {}) as Record<string, unknown>,
						signal,
					);
					// MCP CallToolResult shape: { content: [{type, text}...], isError? }
					const content =
						(
							result as {
								content?: Array<{ type: string; text?: string }>;
							}
						).content ?? [];
					const text = content
						.filter(
							(c) => c.type === "text" && typeof c.text === "string",
						)
						.map((c) => c.text as string)
						.join("\n");
					const isError =
						(result as { isError?: boolean }).isError === true;
					const summary =
						text || (isError ? "MCP tool returned error" : "MCP tool ok");
					const details: McpToolDetails = {
						status: isError ? "error" : "ok",
						summary,
						data: result,
					};
					return {
						content: [{ type: "text" as const, text: summary }],
						details,
						isError,
					};
				} catch (err) {
					const message =
						err instanceof Error ? err.message : String(err);
					const details: McpToolDetails = {
						status: "error",
						summary: message,
						error: message,
					};
					return {
						content: [{ type: "text" as const, text: message }],
						details,
						isError: true,
					};
				}
			},
		});
	}

	console.error(
		`pi-ainsel-mcp: registered ${catalog.tools.length} MCP tools`,
	);
}
