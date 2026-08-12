/**
 * Ainsel Runner Extension for Pi
 *
 * This extension is the entire event loop of the pi-native ainsel agent
 * runtime. It runs inside a long-lived `pi --mode rpc` process and:
 *
 *   1. On `session_start`, starts an HTTP long-poll loop against the hub.
 *   2. For every task delivered by the hub:
 *        a. Builds an `<event>…</event>` system block and injects it
 *           as the user message body.
 *        b. Calls `pi.sendUserMessage("Handle …")` to start a turn.
 *        c. Awaits turn completion.
 *        d. ACKs or NACKs the task via the hub REST API.
 *
 * Required env (operator already sets all of these):
 *   HUB_URL, AGENT_NAME, HUB_INTERNAL_VALIDATE_SECRET, OLLAMA_CLOUD_MODEL
 *   (all required). HUB_INTERNAL_VALIDATE_SECRET is the platform-managed
 *   token for the hub internal endpoints — see resolveInternalToken.
 *
 * Optional env:
 *   NAK_DELAY_MS           default 60000 — NACK backoff (ms) before retry
 *   TOOL_RESULT_MAX_CHARS  default 16000 — per-text-block cap for toolResult
 *                          transcript content (chars, UTF-16 code units)
 *   EVENT_DATA_MAX_CHARS   default 16000 — cap for the raw event payload
 *                          inlined into the prompt as <event-data>
 */

import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import * as http from "http";
import * as prom from "prom-client";

// ---------------------------------------------------------------------------
// Secret redaction — defense-in-depth control that replaces every env value
// that could be a secret with "***" before transcript content is sent to the
// hub. Operates on known env values opaquely (no parsing/guessing token
// formats), so it fails closed: a new secret var is automatically covered.
// ---------------------------------------------------------------------------

/** Env var names that are known non-sensitive and must never be redacted. */
const REDACTION_DENYLIST = new Set([
	"AGENT_NAME",
	"PI_PROVIDER",
	"OLLAMA_CLOUD_MODEL",
	"AGENT_PERSONA_PATH",
	"HUB_URL",
	"HUB_ENABLED",
	"OLLAMA_CLOUD_MAX_TURNS",
	"AGENT_TOOLS",
	"MCP_SERVERS",
	"PATH",
	"HOME",
	"HOSTNAME",
	"PWD",
	"NAK_DELAY_MS",
	"TOOL_RESULT_MAX_CHARS",
	"EVENT_DATA_MAX_CHARS",
	"POST_TIMEOUT_MS",
	"TURN_TIMEOUT_MS",
	"TURN_SETTLE_TIMEOUT_MS",
	"NODE_ENV",
	"SHLVL",
	"TERM",
	"LANG",
	"LC_ALL",
]);

/** Env var name prefixes that are known non-sensitive (set per-task). */
const REDACTION_DENYLIST_PREFIXES = ["AINSEL_EVENT_"];

/** Minimum length for a value to be considered a potential secret. */
const MIN_SECRET_LENGTH = 8;

/** The memoized set of secret values to redact. Null until first use. */
let cachedSecrets: string[] | null = null;

/** Build the list of secret values from process.env (called once, memoized). */
function buildSecretList(env: Record<string, string | undefined>): string[] {
	const secrets: string[] = [];
	for (const [key, value] of Object.entries(env)) {
		if (value === undefined || value.length < MIN_SECRET_LENGTH) continue;
		if (REDACTION_DENYLIST.has(key)) continue;
		if (REDACTION_DENYLIST_PREFIXES.some((p) => key.startsWith(p))) continue;
		secrets.push(value);
	}
	// Deduplicate (multiple vars may share the same value).
	return [...new Set(secrets)];
}

/**
 * Replace all occurrences of known env secret values in `input` with `***`.
 * Handles both the raw value and its JSON-escaped form (e.g. a secret
 * containing `"` appears as `\"` inside a JSON string).
 *
 * Exported for unit testing. The secret set is memoized on first call;
 * use `resetRedactionCache()` in tests to re-derive after mutating env.
 */
export function redactSecrets(input: string): string {
	if (cachedSecrets === null) {
		cachedSecrets = buildSecretList(process.env as Record<string, string | undefined>);
	}
	let out = input;
	for (const secret of cachedSecrets) {
		out = out.replaceAll(secret, "***");
		// Also redact the JSON-escaped form (covers secrets with ", \, /, etc.)
		const escaped = JSON.stringify(secret).slice(1, -1);
		if (escaped !== secret) {
			out = out.replaceAll(escaped, "***");
		}
	}
	return out;
}

/** Reset the memoized secret cache. For tests only. */
export function resetRedactionCache(): void {
	cachedSecrets = null;
}

// ---------------------------------------------------------------------------
// Tool-result content size capping — prevents oversized toolResult content
// (large file reads, command output) from producing huge task_conversations
// rows and transcript payloads. Applied per-text-block with head+tail
// retention so both the beginning and the error tail of output survive.
// ---------------------------------------------------------------------------

/** Default per-text-block cap for toolResult content (chars). */
const TOOL_RESULT_MAX_CHARS_DEFAULT = 16_000;

const EVENT_DATA_MAX_CHARS_DEFAULT = 16_000;

/**
 * Resolve the raw-event-payload cap from the `EVENT_DATA_MAX_CHARS` env var,
 * falling back to EVENT_DATA_MAX_CHARS_DEFAULT for unset/invalid values.
 */
export function resolveEventDataMaxChars(): number {
	const env = process.env.EVENT_DATA_MAX_CHARS;
	if (env) {
		const n = parseInt(env, 10);
		if (!Number.isNaN(n) && n > 0) return n;
	}
	return EVENT_DATA_MAX_CHARS_DEFAULT;
}

/**
 * Resolve the per-text-block cap from the `TOOL_RESULT_MAX_CHARS` env var,
 * read lazily at call time so tests can override it without module reload.
 * Falls back to 16 000 when unset or invalid.
 */
export function resolveToolResultMaxChars(): number {
	const env = process.env.TOOL_RESULT_MAX_CHARS;
	if (env) {
		const n = parseInt(env, 10);
		if (!Number.isNaN(n) && n > 0) return n;
	}
	return TOOL_RESULT_MAX_CHARS_DEFAULT;
}

/**
 * Truncate a string to `maxChars` using head+tail retention (75% head,
 * 25% tail) with an ASCII marker showing the omitted char count.
 * Returns the input unchanged when it fits within the cap.
 */
export function truncateWithMarker(text: string, maxChars: number): string {
	if (text.length <= maxChars) return text;
	const headLen = Math.floor(maxChars * 0.75);
	const tailLen = maxChars - headLen;
	const omitted = text.length - headLen - tailLen;
	return (
		text.slice(0, headLen) +
		`\n... [truncated ${omitted} chars] ...\n` +
		text.slice(text.length - tailLen)
	);
}

/**
 * Cap toolResult content for transcript serialization.
 *
 * - **string** → redact then truncate as a single text value.
 * - **array** → map blocks: text blocks are redacted then truncated;
 *   non-text blocks (images, etc.) are reduced to `{ type }` (parity
 *   with user/assistant stripping).
 * - **other** (arbitrary objects) → returned unchanged; the caller's
 *   outer `redactSecrets(JSON.stringify(…))` and the per-message
 *   safety-net cap handle those.
 *
 * Redaction runs *before* truncation so a secret can never be split
 * across the head/tail window boundary.
 *
 * @param content  The raw `m.content` from a toolResult message.
 * @param maxChars Per-text-block cap (defaults to `resolveToolResultMaxChars()`).
 */
export function capToolResultContent(content: unknown, maxChars?: number): unknown {
	const cap = maxChars ?? resolveToolResultMaxChars();

	if (typeof content === "string") {
		return truncateWithMarker(redactSecrets(content), cap);
	}

	if (Array.isArray(content)) {
		return content.map((block: any) => {
			if (block && typeof block === "object" && block.type === "text" && typeof block.text === "string") {
				return { type: "text", text: truncateWithMarker(redactSecrets(block.text), cap) };
			}
			// Non-text blocks (images, etc.) → reduce to { type }.
			if (block && typeof block === "object" && typeof block.type === "string") {
				return { type: block.type };
			}
			return block;
		});
	}

	// Arbitrary objects: pass through unchanged.
	return content;
}

// Prometheus metrics — exported by the agent runtime on :9090/metrics.
const tokenCounter = new prom.Counter({
	name: "agent_tokens_used_total",
	help: "Total tokens consumed by the agent",
	labelNames: ["agent", "repo", "org", "event_type", "token_type", "model"],
});

// EventContext is the normalized view of an event.
export interface EventContext {
	type?: string;
	action?: string;
	actor?: string;
	owner?: string;
	repo?: string;
	number?: number;
	kind?: string;
	id?: string;
	prompt?: string;
	cronTrigger?: string;
	chatSessionId?: string;
	chatMessage?: string;
}

export interface HubEvent {
	type?: string;
	id?: string;
	subject?: {
		kind?: string;
		owner?: string;
		repo?: string;
		number?: number;
	};
	actor?: { login?: string };
	action?: string;
	headers?: Record<string, string>;
	data?: Record<string, unknown>;
	[k: string]: unknown;
}

// Task is the shape returned by GET /api/internal/agents/{name}/next-task.
interface Task {
	id: number;
	event_id: string;
	agent_name: string;
	trigger_name: string;
	invocation_id: string;
	headers: Record<string, string>;
	payload: unknown;
	attempts: number;
}

export function extractEventContext(event: HubEvent): EventContext {
	const data = event.data ?? {};
	const headers = event.headers ?? {};

	const eventType =
		event.type ??
		headers["type"] ??
		(typeof data.type === "string" ? data.type : undefined);
	if (eventType === "chat.message") {
		const sessionId =
			typeof data.session_id === "string" ? data.session_id : undefined;
		const message =
			typeof data.message === "string" ? data.message : "";
		return {
			type: "chat.message",
			id: event.id,
			chatSessionId: sessionId,
			chatMessage: message,
		};
	}

	if (headers["type"] === "cron") {
		const prompt = typeof data.prompt === "string" ? data.prompt : "";
		return {
			type: "cron",
			id: event.id,
			prompt,
			cronTrigger: typeof data.cronTrigger === "string" ? data.cronTrigger : undefined,
		};
	}

	if (event.type || event.subject || event.actor || event.action) {
		return {
			type: event.type,
			action: event.action,
			actor: event.actor?.login,
			owner: event.subject?.owner,
			repo: event.subject?.repo,
			number: event.subject?.number,
			kind: event.subject?.kind,
			id: event.id,
		};
	}

	const action = typeof data.action === "string" ? data.action : undefined;
	const sender = data.sender as Record<string, unknown> | undefined;
	const actor =
		typeof sender?.login === "string" ? sender.login : undefined;

	const repository = data.repository as Record<string, unknown> | undefined;
	const owner =
		typeof (repository?.owner as Record<string, unknown>)?.login === "string"
			? ((repository.owner as Record<string, unknown>).login as string)
			: undefined;
	const repoName =
		typeof repository?.name === "string" ? repository.name : undefined;
	const fullName =
		typeof repository?.full_name === "string"
			? (repository.full_name as string)
			: undefined;
	const repo = repoName ?? fullName;

	let number: number | undefined;
	let kind: string | undefined;
	const issue = data.issue as Record<string, unknown> | undefined;
	const pr = data.pull_request as Record<string, unknown> | undefined;
	if (typeof issue?.number === "number") {
		number = issue.number;
		kind = "issue";
	} else if (typeof pr?.number === "number") {
		number = pr.number;
		kind = "pull_request";
	}

	// The hub sets a canonical "type" header (e.g. "issues", "push")
	// derived from the webhook event-type header in a platform-independent
	// way. Combine it with the action to produce a compound type like
	// "issues.opened".
	const headerType = headers["type"];
	const type =
		action && headerType
			? `${headerType}.${action}`
			: (headerType ?? action);

	return {
		type,
		action,
		actor,
		owner,
		repo,
		number,
		kind,
		id: event.id,
	};
}

// ConversationPayload is the camelCase JSON shape the hub ingest endpoint
// decodes into. `content` is a JSON-serialized string of normalized blocks.
export interface ConversationPayload {
	agentName: string;
	invocationId: string;
	correlationId: string;
	role: string;
	content: string; // JSON-serialized content blocks
	model: string;
	inputTokens: number;
	outputTokens: number;
	stopReason: string;
}

interface SerializeMeta {
	agentName: string;
	invocationId: string;
	correlationId: string;
}

// toBlockArray normalizes message content to a block array. UserMessage.content
// can be a plain string or a (TextContent|ImageContent)[] array; this mirrors
// the guard used in the `context` event handler.
function toBlockArray(content: any): any[] {
	if (typeof content === "string") return [{ type: "text", text: content }];
	return content ?? [];
}

/** Converts pi AgentMessage[] into hub ConversationMessage payloads.
 *  Pure and exported for unit testing. Unknown roles are skipped. */
export function toConversationPayloads(messages: any[], meta: SerializeMeta): ConversationPayload[] {
	const out: ConversationPayload[] = [];
	const base = {
		agentName: meta.agentName,
		invocationId: meta.invocationId,
		correlationId: meta.correlationId,
		model: "",
		inputTokens: 0,
		outputTokens: 0,
		stopReason: "",
	};
	for (const m of messages ?? []) {
		try {
			if (m.role === "user") {
				const blocks = toBlockArray(m.content).map((c: any) =>
					c.type === "text" ? { type: "text", text: c.text } : { type: c.type },
				);
				out.push({ ...base, role: "user", content: redactSecrets(JSON.stringify(blocks)) });
			} else if (m.role === "assistant") {
				const blocks = toBlockArray(m.content).map((c: any) => {
					if (c.type === "text") return { type: "text", text: c.text };
					if (c.type === "thinking") return { type: "thinking", text: c.thinking ?? c.text ?? "" };
					if (c.type === "toolCall") return { type: "toolCall", id: c.id, name: c.name, arguments: c.arguments };
					return { type: c.type };
				});
				out.push({
					...base,
					role: "assistant",
					content: redactSecrets(JSON.stringify(blocks)),
					model: m.model ?? "",
					inputTokens: m.usage?.input ?? 0,
					outputTokens: m.usage?.output ?? 0,
					stopReason: m.stopReason ?? "",
				});
			} else if (m.role === "toolResult") {
				const capped = capToolResultContent(m.content);
				let serialized = redactSecrets(JSON.stringify({ toolCallId: m.toolCallId, isError: !!m.isError, content: capped }));
				// Per-message safety net: cap the final serialized string so
				// pathological many-block or non-text-object content cannot
				// produce an unbounded row.
				const blockCap = resolveToolResultMaxChars();
				const messageCap = Math.max(128_000, 8 * blockCap);
				if (serialized.length > messageCap) {
					serialized = truncateWithMarker(serialized, messageCap);
				}
				out.push({
					...base,
					role: "toolResult",
					content: serialized,
				});
			}
		} catch (err) {
			// Skip messages that fail serialization (e.g. circular toolResult content)
			// rather than discarding the entire transcript.
			logError("skipping unserializable conversation message", { role: m?.role, err: String(err) });
		}
	}
	return out;
}

function logInfo(msg: string, extra: Record<string, unknown> = {}) {
	console.log(JSON.stringify({ level: "info", msg, ...extra }));
}
function logError(msg: string, extra: Record<string, unknown> = {}) {
	console.error(
		JSON.stringify({
			log_type: "error_event",
			severity: "error",
			source: "agent",
			error_message: msg,
			...extra,
		}),
	);
}

function required(name: string): string {
	const v = process.env[name];
	if (!v) {
		logError(`${name} is required; ainsel-runner cannot start`);
		throw new Error(`${name} is required`);
	}
	return v;
}

/**
 * resolveInternalToken returns the token sent as X-Internal-Token to the
 * hub's internal endpoints (poll, ack, nack). It is the platform-managed
 * HUB_INTERNAL_VALIDATE_SECRET, injected by the agent-operator from the
 * chart's shared auth.internalValidateSecret. The legacy AGENT_TOKEN name
 * was removed: it carried the same shared secret under a misleading
 * per-agent-sounding name and a mis-set value broke the claim path
 * (incident 2026-08-07).
 */
export function resolveInternalToken(env: NodeJS.ProcessEnv = process.env): string {
	const token = env.HUB_INTERNAL_VALIDATE_SECRET;
	if (!token) {
		throw new Error("HUB_INTERNAL_VALIDATE_SECRET is not set");
	}
	return token;
}

function repoFullName(ctx: EventContext): string {
	if (ctx.owner && ctx.repo) return `${ctx.owner}/${ctx.repo}`;
	return ctx.repo ?? "";
}

function trackingID(ctx: EventContext): string {
	if (ctx.type === "chat.message" && ctx.chatSessionId) {
		return `chat/${ctx.chatSessionId}`;
	}
	const repo = repoFullName(ctx);
	if (!repo || !ctx.number) return repo;
	const kind = ctx.kind === "pull_request" ? "pulls" : "issues";
	return `${repo}/${kind}/${ctx.number}`;
}

function titleFor(ctx: EventContext): string {
	if (ctx.type === "chat.message") {
		return ctx.chatSessionId
			? `chat ${ctx.chatSessionId} · chat.message`
			: "chat · chat.message";
	}
	if (ctx.type === "cron") {
		const name = ctx.cronTrigger ?? "cron";
		return `${name} · cron`;
	}
	const repo = repoFullName(ctx);
	const ref = ctx.number
		? ctx.kind === "pull_request"
			? `${repo}!${ctx.number}`
			: `${repo}#${ctx.number}`
		: repo;
	const type = ctx.type ?? "event";
	return ref ? `${ref} · ${type}` : type;
}

function startMetricsServer() {
	const server = http.createServer(async (req, res) => {
		if (req.url === "/metrics") {
			res.writeHead(200, { "Content-Type": "text/plain" });
			res.end(await prom.register.metrics());
		} else {
			res.writeHead(404);
			res.end("Not found");
		}
	});
	server.listen(9090, () => {
		logInfo("metrics server listening", { port: 9090 });
	});
	server.on("error", (err) => {
		logError("metrics server error", { err: err.message });
	});
}

export function buildUserMessage(ctx: EventContext, payload?: unknown): string {
	if (ctx.type === "chat.message") {
		const lines = [
			"<event>",
			"  type: chat.message",
			`  session_id: ${ctx.chatSessionId ?? ""}`,
			"</event>",
			"",
			"A user sent you a chat message:",
			"",
			ctx.chatMessage ?? "",
			"",
			"Respond to the user by calling the mcp__chat__send_reply tool with",
			"the session_id above and your reply as the content. Do not post a",
			"Forgejo comment — chat replies go through the chat MCP tool.",
		];
		return lines.join("\n");
	}

	if (ctx.type === "cron") {
		const lines = [
			"<event>",
			"  type: cron",
			`  cron_trigger: ${ctx.cronTrigger ?? ""}`,
			"</event>",
			"",
			ctx.prompt ?? "",
		];
		return lines.join("\n");
	}

	const repo = repoFullName(ctx);
	const ref = ctx.number
		? ctx.kind === "pull_request"
			? `${repo}!${ctx.number}`
			: `${repo}#${ctx.number}`
		: (repo || "(unknown target)");
	const lines = [
		"<event>",
		`  type: ${ctx.type ?? ""}`,
		`  org: ${ctx.owner ?? ""}`,
		`  repo: ${repo}`,
	];
	if (ctx.number) {
		if (ctx.kind === "pull_request") {
			lines.push(`  pr: ${ctx.number}`);
		} else {
			lines.push(`  issue: ${ctx.number}`);
		}
	}
	if (ctx.actor) lines.push(`  actor: ${ctx.actor}`);
	if (ctx.id) lines.push(`  event_id: ${ctx.id}`);
	lines.push("</event>");
	lines.push("");
	lines.push(`Handle the ${ctx.type ?? "event"} on ${ref}.`);
	if (payload !== undefined && payload !== null) {
		lines.push("");
		lines.push("The full raw event payload:");
		lines.push("");
		lines.push("<event-data>");
		lines.push(truncateWithMarker(JSON.stringify(payload, null, 2), resolveEventDataMaxChars()));
		lines.push("</event-data>");
	}
	return lines.join("\n");
}

function setAinselEventEnv(ctx: EventContext, agentName: string) {
	if (ctx.type === "chat.message") {
		process.env.AINSEL_EVENT_ORG = "";
		process.env.AINSEL_EVENT_REPO = "";
		process.env.AINSEL_EVENT_AGENT = agentName;
		process.env.AINSEL_EVENT_TYPE = "chat.message";
		process.env.AINSEL_EVENT_ID = ctx.id ?? "";
		process.env.AINSEL_EVENT_TRACKING_ID = ctx.chatSessionId ?? "";
		return;
	}
	process.env.AINSEL_EVENT_ORG = ctx.owner ?? "";
	process.env.AINSEL_EVENT_REPO = repoFullName(ctx);
	process.env.AINSEL_EVENT_AGENT = agentName;
	process.env.AINSEL_EVENT_TYPE = ctx.type ?? "";
	process.env.AINSEL_EVENT_ID = ctx.id ?? "";
	process.env.AINSEL_EVENT_TRACKING_ID = trackingID(ctx);
}

/**
 * TurnTracker correlates agent_settled / message_end events with the specific
 * task run that started them. A monotonically increasing generation counter
 * prevents stale events from a previous run from resolving the current
 * run's promise or setting its state.
 *
 * Key pi lifecycle distinction:
 *   - `turn_end` fires after EACH LLM response (+ its tool calls). A single
 *     user message can produce many turns (tool-use loop).
 *   - `agent_settled` fires once, when the entire agent run is complete and
 *     pi will not continue automatically (no more retries, compaction, or
 *     follow-ups).
 *
 * The runner must wait for `agent_settled` — not `turn_end` — to know the
 * LLM has finished processing a task. Resolving on `turn_end` causes the
 * runner to ACK the task after the first LLM response while pi is still
 * executing the tool-use loop, leading to the next task's sendUserMessage
 * being injected into the still-running previous agent run.
 */
export interface TurnTracker {
	/** Incremented each time a new task turn begins. */
	generation: number;
	/** The generation of the currently active turn (0 = no active turn). */
	activeGeneration: number;
	/** Resolves the active turn's promise; null when no turn is active. */
	resolve: (() => void) | null;
	/** Per-turn error state, reset when a new turn starts. */
	errorMessage: string | null;
	assistantMessageCompleted: boolean;
	/** Messages captured during the active turn, reset on beginTurn. Each
	 *  entry is tagged with the generation active at capture time so a turn
	 *  only serializes the messages it actually captured. */
	capturedMessages: { gen: number; message: any }[];
	/** Whether the agent has settled (no more events will fire) since the
	 *  last beginTurn. Starts true (agent is idle at startup), set false on
	 *  beginTurn, set true again by markSettled (agent_settled event). */
	settled: boolean;
	/** Resolves a pending waitForSettle promise; null when nobody is waiting. */
	settleResolve: (() => void) | null;
}

export function createTurnTracker(): TurnTracker {
	return {
		generation: 0,
		activeGeneration: 0,
		resolve: null,
		errorMessage: null,
		assistantMessageCompleted: false,
		capturedMessages: [],
		settled: true,
		settleResolve: null,
	};
}

/** Start a new turn: bump generation, reset per-turn state, return a promise
 *  that resolves when agent_settled fires (entire agent run complete). */
export function beginTurn(tracker: TurnTracker): Promise<void> {
	tracker.generation++;
	tracker.activeGeneration = tracker.generation;
	tracker.errorMessage = null;
	tracker.assistantMessageCompleted = false;
	tracker.capturedMessages = [];
	tracker.settled = false;
	return new Promise<void>((resolve) => {
		tracker.resolve = resolve;
	});
}

/** Called by the agent_settled event handler. Only resolves if a turn is
 *  currently active (ignores stale/duplicate events). */
export function endTurn(tracker: TurnTracker) {
	if (tracker.activeGeneration === 0) return; // no active turn
	const r = tracker.resolve;
	tracker.resolve = null;
	tracker.activeGeneration = 0;
	r?.();
}

/** Called by the message_end event handler. Only records state if the
 *  generation matches the active turn. */
export function recordAssistantEnd(tracker: TurnTracker, errorMessage: string | null) {
	if (tracker.activeGeneration === 0) return; // stale event
	tracker.assistantMessageCompleted = true;
	if (errorMessage) {
		tracker.errorMessage = errorMessage;
	}
}

/** Called by the agent_settled event handler. Marks the agent as settled
 *  and resolves any pending waitForSettle promise. */
export function markSettled(tracker: TurnTracker) {
	tracker.settled = true;
	const r = tracker.settleResolve;
	tracker.settleResolve = null;
	r?.();
}

/**
 * Wait for the abandoned turn to fully settle before allowing the next task
 * to begin. Checks ctx.isIdle() first (covers the race where the turn
 * finished between the timeout firing and the abort call). If not idle,
 * awaits the settle promise with a bounded timeout so the poll loop can
 * never hang forever.
 *
 * Returns true if the agent settled, false if the backstop timeout expired.
 */
export async function waitForSettle(
	tracker: TurnTracker,
	ctx: { isIdle(): boolean },
	timeoutMs: number,
): Promise<boolean> {
	// Already settled — nothing to wait for.
	if (tracker.settled || ctx.isIdle()) return true;

	return new Promise<boolean>((resolve) => {
		// Re-check inside the executor to close the window where markSettled
		// fires between the fast-path check above and promise creation.
		if (tracker.settled || ctx.isIdle()) {
			resolve(true);
			return;
		}
		let timer: ReturnType<typeof setTimeout> | undefined;
		tracker.settleResolve = () => {
			clearTimeout(timer);
			resolve(true);
		};
		timer = setTimeout(() => {
			tracker.settleResolve = null;
			resolve(false);
		}, timeoutMs);
	});
}

/**
 * Capture a message into the active turn's buffer (ignores events when no
 * turn is active). Each entry is tagged with the generation that was active
 * at capture time so processTask can serialize only the messages that belong
 * to its own turn.
 *
 * Guarantee (normal path): the generation tag prevents attributing messages
 * to a turn that did not capture them — processTask filters on
 * `gen === thisGeneration`, so messages captured under a different generation
 * are never serialized into the wrong task's transcript.
 *
 * Timeout path (cancel + drain): when a turn times out, processTask calls
 * ctx.abort() to cancel the abandoned turn, then awaits waitForSettle (with
 * a bounded TURN_SETTLE_TIMEOUT_MS backstop) before returning. While the
 * drain is in progress, activeGeneration stays 0, so orphaned message_end
 * events are dropped here and the agent_settled event no-ops in endTurn.
 * This prevents misattribution of the abandoned turn's tail into the next
 * task's transcript. A vanishingly small residual window remains only if
 * abort() fails to terminate the run and the backstop timeout expires;
 * that case is logged loudly for observability.
 */
export function captureMessage(tracker: TurnTracker, message: any): void {
	if (tracker.activeGeneration === 0) return; // no active turn
	tracker.capturedMessages.push({ gen: tracker.activeGeneration, message });
}

const NAK_DELAY_MS = (() => {
	const env = process.env.NAK_DELAY_MS;
	if (env) {
		const ms = parseInt(env, 10);
		if (!Number.isNaN(ms) && ms > 0) return ms;
	}
	return 60 * 1000;
})();

// sleep returns a promise that resolves after ms milliseconds.
function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

async function ackTask(hubUrl: string, token: string, agentName: string, taskId: number) {
	try {
		await fetch(`${hubUrl}/api/internal/agents/${agentName}/tasks/${taskId}/ack`, {
			method: "POST",
			headers: { "X-Internal-Token": token },
		});
	} catch (err) {
		logError("ack failed", { task_id: taskId, err: String(err) });
	}
}

async function nackTask(hubUrl: string, token: string, agentName: string, taskId: number, error: string) {
	try {
		await fetch(`${hubUrl}/api/internal/agents/${agentName}/tasks/${taskId}/nack`, {
			method: "POST",
			headers: { "X-Internal-Token": token, "Content-Type": "application/json" },
			body: JSON.stringify({ error, delay_ms: NAK_DELAY_MS }),
		});
	} catch (err) {
		logError("nack failed", { task_id: taskId, err: String(err) });
	}
}

async function postTaskLog(hubUrl: string, token: string, agentName: string, level: string, message: string, correlationId: string, invocationId: string, fields: Record<string, unknown> = {}) {
	try {
		await fetch(`${hubUrl}/api/internal/task-logs`, {
			method: "POST",
			headers: { "X-Internal-Token": token, "Content-Type": "application/json" },
			body: JSON.stringify({ correlationId, invocationId, agentName, level, message, fields }),
		});
	} catch { /* best-effort */ }
}

/** Per-request timeout (ms) for task-messages POSTs. Node fetch has no default
 *  timeout; without this a stalled hub connection could delay ack indefinitely. */
const POST_TIMEOUT_MS = (() => {
	const env = process.env.POST_TIMEOUT_MS;
	if (env) {
		const ms = parseInt(env, 10);
		if (!Number.isNaN(ms) && ms > 0) return ms;
	}
	return 10_000; // 10 seconds default
})();

/** Post conversation payloads to the hub ingest endpoint. Best-effort:
 *  per-request failures are logged and swallowed so reporting never throws. */
export async function postTaskMessages(hubUrl: string, token: string, payloads: ConversationPayload[]): Promise<void> {
	for (const p of payloads) {
		try {
			await fetch(`${hubUrl}/api/internal/task-messages`, {
				method: "POST",
				headers: { "X-Internal-Token": token, "Content-Type": "application/json" },
				body: JSON.stringify(p),
				signal: AbortSignal.timeout(POST_TIMEOUT_MS),
			});
		} catch (err) {
			logError("task-messages post failed", { role: p.role, err: String(err) });
		}
	}
}

/**
 * Serialize captured messages into conversation payloads and post them to the
 * hub. Best-effort and guaranteed not to throw: a serialization failure (e.g.
 * JSON.stringify on circular tool-result content) or a network error is logged
 * and swallowed so it can never block ack/nack or crash the poll loop.
 */
export async function reportConversation(
	hubUrl: string,
	token: string,
	messages: any[],
	meta: SerializeMeta,
	taskId: number,
): Promise<void> {
	try {
		const payloads = toConversationPayloads(messages, meta);
		if (payloads.length > 0) {
			await postTaskMessages(hubUrl, token, payloads);
		}
	} catch (err) {
		logError("conversation report failed", { task_id: taskId, err: String(err) });
	}
}

/** Maximum time (ms) to wait for a single LLM turn before giving up.
 *  Prevents tasks from hanging forever if the LLM stalls. */
const TURN_TIMEOUT_MS = (() => {
	const env = process.env.TURN_TIMEOUT_MS;
	if (env) {
		const ms = parseInt(env, 10);
		if (!Number.isNaN(ms) && ms > 0) return ms;
	}
	return 10 * 60 * 1000; // 10 minutes default
})();

/** Maximum time (ms) to wait for an aborted turn to settle before proceeding.
 *  Backstop only — after ctx.abort() the run should settle in milliseconds. */
const TURN_SETTLE_TIMEOUT_MS = (() => {
	const env = process.env.TURN_SETTLE_TIMEOUT_MS;
	if (env) {
		const ms = parseInt(env, 10);
		if (!Number.isNaN(ms) && ms > 0) return ms;
	}
	return 10 * 1000; // 10 seconds default
})();

async function processTask(
	pi: ExtensionAPI,
	ctx: ExtensionContext,
	task: Task,
	agentName: string,
	model: string,
	hubUrl: string,
	token: string,
	tracker: TurnTracker,
): Promise<void> {
	// The task payload is the raw event data (e.g. the Forgejo webhook
	// body). The hub sends it as-is — not wrapped in a HubEvent envelope.
	// Construct a HubEvent so extractEventContext can parse it via the
	// fallback path (data + headers).
	const event: HubEvent = {
		id: task.event_id,
		headers: task.headers ?? {},
		data: task.payload as Record<string, unknown>,
	};
	const evCtx = extractEventContext(event);
	const invocationId = task.invocation_id ?? "";
	const start = Date.now();

	logInfo("event received", {
		event_type: evCtx.type,
		tracking_id: trackingID(evCtx),
		task_id: task.id,
	});

	// Post a task.log entry so the hub can track execution.
	const correlationId = `task-${task.id}`;
	await postTaskLog(hubUrl, token, agentName, "info", `Processing ${evCtx.type ?? "event"}`, correlationId, invocationId, { event_type: evCtx.type, task_id: task.id });

	setAinselEventEnv(evCtx, agentName);

	let succeeded = false;
	let errMsg = "";

	// Begin a new turn — bumps the generation counter so stale agent_settled /
	// message_end events from a previous task cannot resolve this turn.
	const turnDone = beginTurn(tracker);
	const thisGeneration = tracker.activeGeneration;

	try {
		await pi.sendUserMessage(buildUserMessage(evCtx, task.payload));

		// Wait for the turn to end, with a timeout to prevent hanging forever.
		let turnTimer: ReturnType<typeof setTimeout> | undefined;
		const timeout = new Promise<"timeout">((resolve) => {
			turnTimer = setTimeout(() => resolve("timeout"), TURN_TIMEOUT_MS);
		});
		const result = await Promise.race([turnDone, timeout]);
		clearTimeout(turnTimer);

		if (result === "timeout") {
			// Clean up tracker so a late agent_settled doesn't resolve an
			// already-abandoned promise. Keep activeGeneration at 0
			// throughout the drain so orphaned events are dropped.
			tracker.resolve = null;
			tracker.activeGeneration = 0;
			succeeded = false;
			errMsg = `Turn timed out after ${TURN_TIMEOUT_MS}ms`;
			logError("turn timed out, nacking for retry", {
				event_type: evCtx.type,
				agent: agentName,
				timeout_ms: TURN_TIMEOUT_MS,
			});

			// Cancel the abandoned turn and wait for it to settle so its
			// tail events cannot leak into the next task's turn.
			ctx.abort();
			const didSettle = await waitForSettle(tracker, ctx, TURN_SETTLE_TIMEOUT_MS);
			if (!didSettle) {
				logError("abandoned turn did not settle within backstop timeout", {
					event_type: evCtx.type,
					agent: agentName,
					settle_timeout_ms: TURN_SETTLE_TIMEOUT_MS,
				});
			}
		} else {
			succeeded = true;
		}
	} catch (err) {
		errMsg = err instanceof Error ? err.message : String(err);
		logError("pi turn failed", {
			error_message: errMsg,
			agent: agentName,
			event_type: evCtx.type,
			tracking_id: trackingID(evCtx),
			invocation_id: invocationId,
		});
	}

	// After a normal endTurn() (via agent_settled), activeGeneration is 0 —
	// that is the expected "run completed" state. A genuine mismatch is when a
	// *different non-zero* generation is active (i.e. a newer run started).
	if (succeeded && (tracker.activeGeneration === 0 || tracker.activeGeneration === thisGeneration)) {
		if (tracker.errorMessage !== null) {
			succeeded = false;
			errMsg = tracker.errorMessage;
			logError("LLM returned error, nacking for retry", {
				event_type: evCtx.type,
				error_message: errMsg,
				agent: agentName,
			});
		}

		if (succeeded && !tracker.assistantMessageCompleted) {
			succeeded = false;
			errMsg = "Turn ended without producing an assistant message";
			logError("turn ended without assistant message, nacking for retry", {
				event_type: evCtx.type,
				agent: agentName,
			});
		}
	} else if (succeeded) {
		// Generation mismatch — the turn state is stale/unreliable.
		succeeded = false;
		errMsg = "Turn state corrupted (generation mismatch); nacking for retry";
		logError("turn generation mismatch, nacking for retry", {
			event_type: evCtx.type,
			agent: agentName,
			expected_generation: thisGeneration,
			actual_generation: tracker.activeGeneration,
		});
	}

	const durationMs = Date.now() - start;

	// Report the conversation transcript. Only messages tagged with this turn's
	// generation are serialized (see captureMessage). reportConversation is
	// guaranteed not to throw, so a serialize/post failure can never skip
	// ack/nack below or crash the poll loop.
	await reportConversation(
		hubUrl,
		token,
		tracker.capturedMessages.filter((e) => e.gen === thisGeneration).map((e) => e.message),
		{ agentName, invocationId, correlationId },
		task.id,
	);

	if (succeeded) {
		await ackTask(hubUrl, token, agentName, task.id);
		logInfo("event processed", {
			event_type: evCtx.type,
			duration_ms: durationMs,
			task_id: task.id,
		});
	} else {
		await nackTask(hubUrl, token, agentName, task.id, errMsg);
		logError("event failed, nacked with delay", {
			event_type: evCtx.type,
			duration_ms: durationMs,
			nak_delay_ms: NAK_DELAY_MS,
			task_id: task.id,
		});
	}
}

async function runPollLoop(
	pi: ExtensionAPI,
	ctx: ExtensionContext,
	agentName: string,
	model: string,
	hubUrl: string,
	token: string,
	tracker: TurnTracker,
) {
	logInfo("poll loop started", { hub_url: hubUrl, agent: agentName });

	while (true) {
		let resp: Response;
		try {
			resp = await fetch(
				`${hubUrl}/api/internal/agents/${agentName}/next-task?timeout=30s`,
				{ headers: { "X-Internal-Token": token } },
			);
		} catch (err) {
			logError("poll request failed, retrying in 5s", {
				err: err instanceof Error ? err.message : String(err),
			});
			await sleep(5000);
			continue;
		}

		if (resp.status === 204) {
			// Timeout, no task available. Loop immediately.
			continue;
		}

		if (!resp.ok) {
			logError("poll returned unexpected status, retrying in 5s", {
				status: resp.status,
			});
			await sleep(5000);
			continue;
		}

		let task: Task;
		try {
			task = (await resp.json()) as Task;
		} catch (err) {
			logError("failed to parse task JSON", {
				err: err instanceof Error ? err.message : String(err),
			});
			await sleep(1000);
			continue;
		}

		await processTask(pi, ctx, task, agentName, model, hubUrl, token, tracker);
	}
}

export default function ainselRunnerExtension(pi: ExtensionAPI) {
	const tracker = createTurnTracker();

	// turn_end fires after EACH LLM response in the tool-use loop — too early
	// to resolve the task. We only track it for observability.
	pi.on("turn_end", (event) => {
		logInfo("turn ended", { turn_index: event.turnIndex });
	});

	// agent_settled fires once when the entire agent run is complete (all
	// turns, retries, compaction). This is the correct signal to resolve
	// the task promise and ACK/NACK.
	pi.on("agent_settled", () => {
		endTurn(tracker);
		markSettled(tracker);
	});

	pi.on("message_end", (event) => {
		const msg = event.message;
		captureMessage(tracker, msg);
		if (msg.role !== "assistant") return;

		const errMsg = msg.stopReason === "error"
			? (msg.errorMessage ?? "LLM returned stopReason=error with no message")
			: null;
		if (errMsg) {
			logError("assistant message ended with error", {
				stopReason: msg.stopReason,
				errorMessage: errMsg,
			});
		}
		recordAssistantEnd(tracker, errMsg);
		if (msg.usage && msg.usage.totalTokens > 0) {
			const agent = process.env.AINSEL_EVENT_AGENT ?? "";
			const repo = process.env.AINSEL_EVENT_REPO ?? "";
			const org = process.env.AINSEL_EVENT_ORG ?? "";
			const eventType = process.env.AINSEL_EVENT_TYPE ?? "";
			const model = msg.model ?? "";
			tokenCounter.inc(
				{ agent, repo, org, event_type: eventType, token_type: "input", model },
				msg.usage.input,
			);
			tokenCounter.inc(
				{ agent, repo, org, event_type: eventType, token_type: "output", model },
				msg.usage.output,
			);
			logInfo("token usage recorded", {
				agent,
				event_type: eventType,
				input: msg.usage.input,
				output: msg.usage.output,
				model,
			});
		}
	});

	// Strip accumulated conversation history between events.
	pi.on("context", (event) => {
		const msgs = event.messages;
		let startIdx = 0;
		for (let i = msgs.length - 1; i >= 0; i--) {
			const msg = msgs[i] as { role?: string; content?: unknown };
			if (msg.role === "user") {
				const text =
					typeof msg.content === "string"
						? msg.content
						: Array.isArray(msg.content)
							? ((msg.content as Array<{ type?: string; text?: string }>).find(
									(c) => c.type === "text",
								)?.text ?? "")
							: "";
				if (text.startsWith("<event>")) {
					startIdx = i;
					break;
				}
			}
		}
		if (startIdx > 0) {
			return { messages: msgs.slice(startIdx) };
		}
		return {};
	});

	let started = false;
	pi.on("session_start", async (_event, ctx) => {
		if (started) return;
		started = true;

		let agentName: string;
		let hubUrl: string;
		let token: string;
		let model: string;
		try {
			agentName = required("AGENT_NAME");
			hubUrl = required("HUB_URL");
			token = resolveInternalToken();
			model = required("OLLAMA_CLOUD_MODEL");
		} catch {
			return;
		}

		logInfo("ainsel-runner starting", {
			agent: agentName,
			hub_url: hubUrl,
			model,
		});

		startMetricsServer();

		// Run the poll loop in the background.
		runPollLoop(pi, ctx, agentName, model, hubUrl, token, tracker).catch((err) => {
			logError("poll loop crashed", {
				err: err instanceof Error ? err.message : String(err),
			});
		});
	});
}
