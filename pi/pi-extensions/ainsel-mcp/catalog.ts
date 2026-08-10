import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StreamableHTTPClientTransport } from "@modelcontextprotocol/sdk/client/streamableHttp.js";
import type { ServerRef } from "./parse.js";

const PREFIX = "mcp__";

const sleep = (ms: number): Promise<void> =>
	new Promise((resolve) => setTimeout(resolve, ms));

export interface PrefixedTool {
	name: string; // mcp__<server>__<tool>
	description: string;
	inputSchema: Record<string, unknown>;
	server: string;
	upstreamName: string;
	schemaHasUserId: boolean;
}

export interface CatalogOptions {
	agentName?: string;
	startupTimeoutMs?: number; // default 10000
	connectRetries?: number; // default 4 retries per server
	retryBaseDelayMs?: number; // default 250, doubled after each attempt
	logger?: (msg: string, meta?: Record<string, unknown>) => void;
}

interface ServerEntry {
	name: string;
	client: Client;
}

export class Catalog {
	private servers = new Map<string, ServerEntry>();
	public tools: PrefixedTool[] = [];
	private opts: Required<CatalogOptions>;

	constructor(opts: CatalogOptions) {
		this.opts = {
			agentName: opts.agentName ?? "",
			startupTimeoutMs: opts.startupTimeoutMs ?? 10_000,
			connectRetries: opts.connectRetries ?? 4,
			retryBaseDelayMs: opts.retryBaseDelayMs ?? 250,
			logger:
				opts.logger ??
				((msg, meta) =>
					console.error(`pi-ainsel-mcp: ${msg}`, meta ?? "")),
		};
	}

	/**
	 * Connect to each server, fetch tools, build the prefixed catalog.
	 * Each server is retried with exponential backoff: in an agent pod the
	 * sidecar containers may not be listening yet when the runner starts.
	 * Servers that still fail within the startup budget are logged and
	 * skipped -- never throws.
	 */
	async connect(refs: ServerRef[]): Promise<void> {
		const deadline = AbortSignal.timeout(this.opts.startupTimeoutMs);
		const { connectRetries, retryBaseDelayMs } = this.opts;
		await Promise.allSettled(
			refs.map(async (ref) => {
				for (let attempt = 0; ; attempt++) {
					try {
						await this.connectServer(ref, deadline);
						return;
					} catch (err) {
						const errMsg =
							err instanceof Error ? err.message : String(err);
						if (attempt >= connectRetries || deadline.aborted) {
							this.opts.logger("server unreachable; skipping", {
								server: ref.name,
								attempts: attempt + 1,
								err: errMsg,
							});
							return;
						}
						const delayMs = retryBaseDelayMs * 2 ** attempt;
						this.opts.logger("connect failed; retrying", {
							server: ref.name,
							attempt: attempt + 1,
							delayMs,
							err: errMsg,
						});
						await sleep(delayMs);
					}
				}
			}),
		);
	}

	private async connectServer(
		ref: ServerRef,
		deadline: AbortSignal,
	): Promise<void> {
		const client = new Client(
			{ name: "pi-ainsel-mcp", version: "0.1.0" },
			{ capabilities: {} },
		);
		const transport = new StreamableHTTPClientTransport(
			new URL(ref.url),
			ref.token
				? { requestInit: { headers: { Authorization: `Bearer ${ref.token}` } } }
				: undefined,
		);
		await client.connect(transport);
		const { tools } = await client.listTools(undefined, {
			signal: deadline,
		});
		this.servers.set(ref.name, { name: ref.name, client });
		for (const t of tools) {
			const schema = (t.inputSchema ?? {
				type: "object",
			}) as Record<string, unknown>;
			this.tools.push({
				name: `${PREFIX}${ref.name}__${t.name}`,
				description: t.description ?? "",
				inputSchema: schema,
				server: ref.name,
				upstreamName: t.name,
				schemaHasUserId: schemaHasUserId(schema),
			});
		}
		this.opts.logger("connected", {
			server: ref.name,
			tools: tools.length,
		});
	}

	async call(
		name: string,
		args: Record<string, unknown>,
		signal?: AbortSignal,
	): Promise<unknown> {
		if (!name.startsWith(PREFIX)) throw new Error(`not an MCP tool: ${name}`);
		const rest = name.slice(PREFIX.length);
		const sep = rest.indexOf("__");
		if (sep < 0) throw new Error(`malformed MCP tool name: ${name}`);
		const server = rest.slice(0, sep);
		const toolName = rest.slice(sep + 2);

		const entry = this.servers.get(server);
		if (!entry) throw new Error(`unknown MCP server: ${server}`);

		// Inject user_id from AGENT_NAME if applicable.
		const tool = this.tools.find((t) => t.name === name);
		let finalArgs = args;
		if (
			this.opts.agentName &&
			tool?.schemaHasUserId &&
			!("user_id" in args)
		) {
			finalArgs = { ...args, user_id: this.opts.agentName };
		}

		const result = await entry.client.callTool(
			{ name: toolName, arguments: finalArgs },
			undefined,
			{ signal },
		);
		return result;
	}
}

function schemaHasUserId(schema: Record<string, unknown>): boolean {
	const props =
		(schema.properties as Record<string, unknown> | undefined) ?? {};
	return "user_id" in props;
}
