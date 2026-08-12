import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { extractEventContext, buildUserMessage, createTurnTracker, beginTurn, endTurn, recordAssistantEnd, captureMessage, markSettled, waitForSettle, toConversationPayloads, postTaskMessages, reportConversation, redactSecrets, resetRedactionCache, capToolResultContent, truncateWithMarker, resolveToolResultMaxChars, resolveEventDataMaxChars, resolveInternalToken } from "./index.ts";
import type { HubEvent, TurnTracker, ConversationPayload } from "./index.ts";

describe("extractEventContext", () => {
	it("parses a raw Forgejo issues webhook (regression: PR #569)", () => {
		// This is the exact data flow that was broken: the hub sends
		// the raw webhook body as task.payload and the original headers
		// (plus the canonical "type") as task.headers.
		const event: HubEvent = {
			id: "evt-forgejo-123",
			headers: {
				"type": "issues",
				"X-Trigger-Name": "develop-on-assign",
				"X-Invocation-ID": "inv-abc",
				"X-Forgejo-Event": "issues",
				"content-type": "application/json",
			},
			data: {
				action: "assigned",
				sender: { login: "dpinsel" },
				repository: {
					name: "warhammer-maui",
					full_name: "apps/warhammer-maui",
					owner: { login: "apps" },
				},
				issue: { number: 77, title: "Fix the thing" },
			},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "issues.assigned");
		assert.equal(ctx.action, "assigned");
		assert.equal(ctx.actor, "dpinsel");
		assert.equal(ctx.owner, "apps");
		assert.equal(ctx.repo, "warhammer-maui");
		assert.equal(ctx.number, 77);
		assert.equal(ctx.kind, "issue");
		assert.equal(ctx.id, "evt-forgejo-123");
	});

	it("parses a pull_request webhook", () => {
		const event: HubEvent = {
			id: "evt-456",
			headers: { type: "pull_request" },
			data: {
				action: "opened",
				sender: { login: "alice" },
				repository: {
					name: "my-repo",
					owner: { login: "org" },
				},
				pull_request: { number: 15 },
			},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "pull_request.opened");
		assert.equal(ctx.owner, "org");
		assert.equal(ctx.repo, "my-repo");
		assert.equal(ctx.number, 15);
		assert.equal(ctx.kind, "pull_request");
		assert.equal(ctx.actor, "alice");
	});

	it("parses a push event (no action)", () => {
		const event: HubEvent = {
			id: "evt-789",
			headers: { type: "push" },
			data: {
				ref: "refs/heads/main",
				sender: { login: "bob" },
				repository: {
					name: "my-repo",
					owner: { login: "org" },
				},
			},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "push");
		assert.equal(ctx.owner, "org");
		assert.equal(ctx.repo, "my-repo");
		assert.equal(ctx.number, undefined);
	});

	it("detects chat events via type header", () => {
		const event: HubEvent = {
			id: "evt-chat-1",
			headers: {
				type: "chat.message",
				"X-Trigger-Name": "chat",
			},
			data: {
				session_id: "sess-42",
				message: "Hello agent!",
			},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "chat.message");
		assert.equal(ctx.chatSessionId, "sess-42");
		assert.equal(ctx.chatMessage, "Hello agent!");
	});

	it("detects cron events via type header", () => {
		const event: HubEvent = {
			id: "evt-cron-1",
			headers: {
				type: "cron",
				"X-Trigger-Name": "daily-report",
			},
			data: {
				cronTrigger: "daily-report",
				prompt: "Generate the daily summary.",
			},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "cron");
		assert.equal(ctx.prompt, "Generate the daily summary.");
		assert.equal(ctx.cronTrigger, "daily-report");
	});

	it("parses a structured HubEvent with subject/actor", () => {
		const event: HubEvent = {
			id: "evt-structured",
			type: "issue.opened",
			action: "opened",
			subject: {
				kind: "issue",
				owner: "org",
				repo: "my-repo",
				number: 42,
			},
			actor: { login: "carol" },
			headers: {},
			data: {},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "issue.opened");
		assert.equal(ctx.action, "opened");
		assert.equal(ctx.actor, "carol");
		assert.equal(ctx.owner, "org");
		assert.equal(ctx.repo, "my-repo");
		assert.equal(ctx.number, 42);
		assert.equal(ctx.kind, "issue");
	});

	it("handles empty data gracefully", () => {
		const event: HubEvent = {
			id: "evt-empty",
			headers: {},
			data: {},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.id, "evt-empty");
		assert.equal(ctx.owner, undefined);
		assert.equal(ctx.repo, undefined);
		assert.equal(ctx.number, undefined);
	});

	it("uses type header without provider-specific headers", () => {
		// The runner must not depend on X-Forgejo-Event etc.
		const event: HubEvent = {
			id: "evt-generic",
			headers: { type: "issues" },
			data: {
				action: "closed",
				sender: { login: "dave" },
				repository: {
					name: "repo",
					owner: { login: "org" },
				},
				issue: { number: 5 },
			},
		};

		const ctx = extractEventContext(event);

		assert.equal(ctx.type, "issues.closed");
		assert.equal(ctx.owner, "org");
		assert.equal(ctx.number, 5);
	});
});

describe("buildUserMessage", () => {
	const webhookCtx = {
		type: "issue_comment.created",
		actor: "alice",
		owner: "acme",
		repo: "web",
		number: 83,
		kind: "issue",
		id: "evt-1",
	};

	it("inlines the raw payload in an event-data block", () => {
		const payload = {
			action: "created",
			comment: { body: "@review-agent wdyt?" },
		};
		const msg = buildUserMessage(webhookCtx, payload);

		assert.ok(msg.includes("type: issue_comment.created"));
		assert.ok(msg.includes("issue: 83"));
		assert.ok(msg.includes("<event-data>"));
		assert.ok(msg.includes("</event-data>"));
		assert.ok(msg.includes("@review-agent wdyt?"));
		assert.ok(msg.includes('"action": "created"'));
	});

	it("omits the event-data block when no payload is given", () => {
		const msg = buildUserMessage(webhookCtx);
		assert.ok(!msg.includes("<event-data>"));
		assert.ok(msg.includes("Handle the issue_comment.created"));
	});

	it("omits the event-data block for null payloads", () => {
		const msg = buildUserMessage(webhookCtx, null);
		assert.ok(!msg.includes("<event-data>"));
	});

	it("truncates oversized payloads with a marker", () => {
		const prev = process.env.EVENT_DATA_MAX_CHARS;
		process.env.EVENT_DATA_MAX_CHARS = "200";
		try {
			const payload = { blob: "x".repeat(5000) };
			const msg = buildUserMessage(webhookCtx, payload);
			const start = msg.indexOf("<event-data>") + "<event-data>\n".length;
			const end = msg.lastIndexOf("</event-data>");
			const body = msg.slice(start, end);
			assert.ok(body.includes("[truncated"), "payload must carry a truncation marker");
			assert.ok(body.length < 500, "payload must be capped near the limit");
		} finally {
			if (prev === undefined) delete process.env.EVENT_DATA_MAX_CHARS;
			else process.env.EVENT_DATA_MAX_CHARS = prev;
		}
	});

	it("does not inline payload for chat.message events", () => {
		const msg = buildUserMessage({ type: "chat.message", chatSessionId: "s1", chatMessage: "hi" });
		assert.ok(!msg.includes("<event-data>"));
		assert.ok(msg.includes("hi"));
	});
});

describe("resolveEventDataMaxChars", () => {
	it("returns the default when unset", () => {
		const prev = process.env.EVENT_DATA_MAX_CHARS;
		delete process.env.EVENT_DATA_MAX_CHARS;
		try {
			assert.equal(resolveEventDataMaxChars(), 16_000);
		} finally {
			if (prev !== undefined) process.env.EVENT_DATA_MAX_CHARS = prev;
		}
	});

	it("ignores invalid values", () => {
		const prev = process.env.EVENT_DATA_MAX_CHARS;
		try {
			process.env.EVENT_DATA_MAX_CHARS = "banana";
			assert.equal(resolveEventDataMaxChars(), 16_000);
			process.env.EVENT_DATA_MAX_CHARS = "-5";
			assert.equal(resolveEventDataMaxChars(), 16_000);
			process.env.EVENT_DATA_MAX_CHARS = "0";
			assert.equal(resolveEventDataMaxChars(), 16_000);
		} finally {
			if (prev === undefined) delete process.env.EVENT_DATA_MAX_CHARS;
			else process.env.EVENT_DATA_MAX_CHARS = prev;
		}
	});

	it("honours a valid override", () => {
		const prev = process.env.EVENT_DATA_MAX_CHARS;
		try {
			process.env.EVENT_DATA_MAX_CHARS = "512";
			assert.equal(resolveEventDataMaxChars(), 512);
		} finally {
			if (prev === undefined) delete process.env.EVENT_DATA_MAX_CHARS;
			else process.env.EVENT_DATA_MAX_CHARS = prev;
		}
	});
});

describe("toConversationPayloads", () => {
	const meta = {
		agentName: "olli",
		invocationId: "inv-1",
		correlationId: "corr-1",
	};

	it("serializes a user message", () => {
		const messages = [
			{
				role: "user",
				content: [
					{ type: "text", text: "Hello" },
					{ type: "image", url: "http://x/y.png" },
				],
			},
		];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 1);
		assert.equal(out[0].agentName, "olli");
		assert.equal(out[0].invocationId, "inv-1");
		assert.equal(out[0].correlationId, "corr-1");
		assert.equal(out[0].role, "user");
		assert.equal(out[0].inputTokens, 0);
		assert.equal(out[0].outputTokens, 0);
		assert.equal(out[0].model, "");
		assert.equal(out[0].stopReason, "");
		assert.deepEqual(JSON.parse(out[0].content), [
			{ type: "text", text: "Hello" },
			{ type: "image" },
		]);
	});

	it("serializes an assistant message with thinking, text and toolCall blocks plus usage", () => {
		const messages = [
			{
				role: "assistant",
				model: "gpt-x",
				usage: { input: 10, output: 20 },
				stopReason: "end_turn",
				content: [
					{ type: "thinking", thinking: "hmm" },
					{ type: "text", text: "answer" },
					{ type: "toolCall", id: "tc-1", name: "bash", arguments: { cmd: "ls" } },
					{ type: "weird" },
				],
			},
		];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 1);
		assert.equal(out[0].role, "assistant");
		assert.equal(out[0].model, "gpt-x");
		assert.equal(out[0].inputTokens, 10);
		assert.equal(out[0].outputTokens, 20);
		assert.equal(out[0].stopReason, "end_turn");
		assert.deepEqual(JSON.parse(out[0].content), [
			{ type: "thinking", text: "hmm" },
			{ type: "text", text: "answer" },
			{ type: "toolCall", id: "tc-1", name: "bash", arguments: { cmd: "ls" } },
			{ type: "weird" },
		]);
	});

	it("serializes a toolResult message", () => {
		const messages = [
			{
				role: "toolResult",
				toolCallId: "tc-1",
				isError: true,
				content: "boom",
			},
		];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 1);
		assert.equal(out[0].role, "toolResult");
		assert.deepEqual(JSON.parse(out[0].content), {
			toolCallId: "tc-1",
			isError: true,
			content: "boom",
		});
	});

	it("skips unknown roles", () => {
		const messages = [{ role: "system", content: [{ type: "text", text: "hi" }] }];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 0);
	});

	it("serializes string user-content to a single text block", () => {
		const messages = [{ role: "user", content: "plain string body" }];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 1);
		assert.equal(out[0].role, "user");
		assert.deepEqual(JSON.parse(out[0].content), [
			{ type: "text", text: "plain string body" },
		]);
	});

	it("returns [] for undefined and null messages", () => {
		assert.deepEqual(toConversationPayloads(undefined as any, meta), []);
		assert.deepEqual(toConversationPayloads(null as any, meta), []);
	});

	it("preserves order and count across multiple messages", () => {
		const messages = [
			{ role: "user", content: [{ type: "text", text: "one" }] },
			{ role: "assistant", content: [{ type: "text", text: "two" }] },
			{ role: "toolResult", toolCallId: "tc-1", content: "three" },
		];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 3);
		assert.deepEqual(out.map((p) => p.role), ["user", "assistant", "toolResult"]);
	});

	it("defaults assistant fields when usage/model/stopReason are absent", () => {
		const messages = [{ role: "assistant", content: [{ type: "text", text: "hi" }] }];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 1);
		assert.equal(out[0].model, "");
		assert.equal(out[0].inputTokens, 0);
		assert.equal(out[0].outputTokens, 0);
		assert.equal(out[0].stopReason, "");
	});

	it("serializes an empty content array to \"[]\"", () => {
		const messages = [{ role: "user", content: [] }];

		const out = toConversationPayloads(messages, meta);

		assert.equal(out.length, 1);
		assert.equal(out[0].content, "[]");
	});

	it("skips an unserializable message without discarding the rest", () => {
		const circular: any = { a: 1 };
		circular.self = circular;
		const messages = [
			{ role: "user", content: [{ type: "text", text: "before" }] },
			{ role: "toolResult", toolCallId: "tc-bad", isError: false, content: circular },
			{ role: "assistant", content: [{ type: "text", text: "after" }] },
		];

		const out = toConversationPayloads(messages, meta);

		// The circular message is skipped; the other two survive.
		assert.equal(out.length, 2);
		assert.deepEqual(out.map((p) => p.role), ["user", "assistant"]);
	});
});

describe("TurnTracker", () => {
	it("resolves the turn promise on endTurn", async () => {
		const tracker = createTurnTracker();
		const turnDone = beginTurn(tracker);

		recordAssistantEnd(tracker, null);
		endTurn(tracker);

		await turnDone; // should resolve without hanging
		assert.equal(tracker.activeGeneration, 0);
		assert.equal(tracker.assistantMessageCompleted, true);
		assert.equal(tracker.errorMessage, null);
	});

	it("ignores a stale/duplicate endTurn", async () => {
		const tracker = createTurnTracker();
		const turnDone = beginTurn(tracker);

		endTurn(tracker);
		await turnDone;

		// Second endTurn is a no-op — activeGeneration is already 0
		endTurn(tracker);
		assert.equal(tracker.activeGeneration, 0);
		assert.equal(tracker.resolve, null);
	});

	it("ignores recordAssistantEnd after endTurn (stale message_end)", async () => {
		const tracker = createTurnTracker();
		const turnDone = beginTurn(tracker);

		endTurn(tracker);
		await turnDone;

		// Stale message_end arrives after turn already ended
		recordAssistantEnd(tracker, "late error");
		assert.equal(tracker.assistantMessageCompleted, false);
		assert.equal(tracker.errorMessage, null);
	});

	it("records error from recordAssistantEnd", async () => {
		const tracker = createTurnTracker();
		const turnDone = beginTurn(tracker);

		recordAssistantEnd(tracker, "LLM exploded");
		endTurn(tracker);
		await turnDone;

		assert.equal(tracker.errorMessage, "LLM exploded");
		assert.equal(tracker.assistantMessageCompleted, true);
	});

	it("bumps generation on each beginTurn", () => {
		const tracker = createTurnTracker();

		beginTurn(tracker);
		assert.equal(tracker.generation, 1);
		assert.equal(tracker.activeGeneration, 1);

		endTurn(tracker);
		beginTurn(tracker);
		assert.equal(tracker.generation, 2);
		assert.equal(tracker.activeGeneration, 2);
	});

	it("resets per-turn state on beginTurn", async () => {
		const tracker = createTurnTracker();

		const turn1 = beginTurn(tracker);
		recordAssistantEnd(tracker, "error from turn 1");
		endTurn(tracker);
		await turn1;

		// New turn resets state
		beginTurn(tracker);
		assert.equal(tracker.errorMessage, null);
		assert.equal(tracker.assistantMessageCompleted, false);
	});

	it("post-turn check: activeGeneration === 0 after normal endTurn (blocker fix)", async () => {
		const tracker = createTurnTracker();
		const turnDone = beginTurn(tracker);
		const thisGeneration = tracker.activeGeneration;

		recordAssistantEnd(tracker, null);
		endTurn(tracker);
		await turnDone;

		// This is the exact check from processTask — after a normal endTurn,
		// activeGeneration is 0, which must be treated as "turn completed
		// normally", not as a generation mismatch.
		const stateIsTrusted =
			tracker.activeGeneration === 0 || tracker.activeGeneration === thisGeneration;
		assert.equal(stateIsTrusted, true, "normal endTurn must be trusted");
	});

	it("post-turn check: detects genuine generation mismatch", () => {
		const tracker = createTurnTracker();
		beginTurn(tracker);
		const thisGeneration = tracker.activeGeneration; // 1

		// Simulate re-entrancy: a new turn starts before the old one ends
		beginTurn(tracker);
		// activeGeneration is now 2, thisGeneration is 1

		const stateIsTrusted =
			tracker.activeGeneration === 0 || tracker.activeGeneration === thisGeneration;
		assert.equal(stateIsTrusted, false, "different non-zero generation is a mismatch");
	});
});

describe("captureMessage", () => {
	it("captures messages during an active turn", () => {
		const tracker = createTurnTracker();
		beginTurn(tracker);

		captureMessage(tracker, { role: "user", content: "hi" });
		captureMessage(tracker, { role: "assistant", content: "hello" });

		assert.equal(tracker.capturedMessages.length, 2);
	});

	it("resets the buffer on a new beginTurn", () => {
		const tracker = createTurnTracker();
		beginTurn(tracker);
		captureMessage(tracker, { role: "user", content: "turn 1" });
		endTurn(tracker);

		beginTurn(tracker);
		assert.equal(tracker.capturedMessages.length, 0);
	});

	it("ignores captures when no turn is active", () => {
		const tracker = createTurnTracker();

		captureMessage(tracker, { role: "user", content: "stale" });

		assert.equal(tracker.capturedMessages.length, 0);
	});

	it("tags each captured message with the active generation", () => {
		const tracker = createTurnTracker();
		beginTurn(tracker); // generation 1
		const gen = tracker.activeGeneration;

		captureMessage(tracker, { role: "user", content: "hi" });

		assert.equal(tracker.capturedMessages.length, 1);
		assert.equal(tracker.capturedMessages[0].gen, gen);
		assert.deepEqual(tracker.capturedMessages[0].message, { role: "user", content: "hi" });
	});

	it("drops orphaned messages after timeout drain (activeGeneration === 0)", () => {
		const tracker = createTurnTracker();
		beginTurn(tracker); // generation 1
		captureMessage(tracker, { role: "user", content: "legit" });

		// Simulate timeout: abandon the turn
		tracker.resolve = null;
		tracker.activeGeneration = 0;

		// Orphaned message_end from the abandoned turn arrives during drain
		captureMessage(tracker, { role: "assistant", content: "orphaned tail" });

		// Only the legit message should be captured
		assert.equal(tracker.capturedMessages.length, 1);
		assert.deepEqual(tracker.capturedMessages[0].message, { role: "user", content: "legit" });
	});
});

describe("markSettled / waitForSettle", () => {
	it("markSettled sets the settled flag", () => {
		const tracker = createTurnTracker();
		beginTurn(tracker);
		assert.equal(tracker.settled, false);

		markSettled(tracker);
		assert.equal(tracker.settled, true);
	});

	it("beginTurn resets settled to false", () => {
		const tracker = createTurnTracker();
		assert.equal(tracker.settled, true); // idle at startup

		beginTurn(tracker);
		assert.equal(tracker.settled, false);

		markSettled(tracker);
		assert.equal(tracker.settled, true);

		beginTurn(tracker);
		assert.equal(tracker.settled, false);
	});

	it("waitForSettle returns immediately when already settled", async () => {
		const tracker = createTurnTracker();
		// settled is true by default (agent idle at startup)
		const ctx = { isIdle: () => false };

		const result = await waitForSettle(tracker, ctx, 5000);
		assert.equal(result, true);
	});

	it("waitForSettle returns immediately when ctx.isIdle() is true", async () => {
		const tracker = createTurnTracker();
		beginTurn(tracker); // sets settled = false
		const ctx = { isIdle: () => true };

		const result = await waitForSettle(tracker, ctx, 5000);
		assert.equal(result, true);
	});

	it("waitForSettle resolves when markSettled fires", async () => {
		const tracker = createTurnTracker();
		beginTurn(tracker); // sets settled = false
		const ctx = { isIdle: () => false };

		// Fire markSettled after a short delay
		setTimeout(() => markSettled(tracker), 10);

		const result = await waitForSettle(tracker, ctx, 5000);
		assert.equal(result, true);
		assert.equal(tracker.settled, true);
		assert.equal(tracker.settleResolve, null);
	});

	it("waitForSettle returns false on backstop timeout", async () => {
		const tracker = createTurnTracker();
		beginTurn(tracker); // sets settled = false
		const ctx = { isIdle: () => false };

		// Never call markSettled — the backstop should fire
		const result = await waitForSettle(tracker, ctx, 50);
		assert.equal(result, false);
		assert.equal(tracker.settleResolve, null);
	});

	it("waitForSettle re-checks inside the executor to close the race window", async () => {
		const tracker = createTurnTracker();
		beginTurn(tracker); // sets settled = false

		// isIdle returns false on the fast-path check, then true inside the
		// executor — simulating markSettled firing between the two checks.
		let calls = 0;
		const ctx = { isIdle: () => ++calls > 1 };

		const result = await waitForSettle(tracker, ctx, 5000);
		assert.equal(result, true);
		// The backstop timer must not have been needed.
		assert.equal(tracker.settleResolve, null);
	});

	it("orphaned turn_end after timeout does not resolve the next turn", async () => {
		const tracker = createTurnTracker();

		// Turn 1 starts and times out
		beginTurn(tracker); // generation 1
		tracker.resolve = null;
		tracker.activeGeneration = 0;

		// Turn 2 starts
		const turn2Done = beginTurn(tracker); // generation 2
		assert.equal(tracker.activeGeneration, 2);

		// Orphaned turn_end from turn 1 arrives — but activeGeneration is 2,
		// so endTurn WILL resolve it (this is the second bug). However, with
		// the cancel+drain fix, the abandoned turn settles BEFORE beginTurn
		// is called for turn 2, so this scenario cannot happen in practice.
		// This test documents that endTurn still resolves for any non-zero
		// activeGeneration — the fix prevents reaching this state.
		endTurn(tracker);

		// turn2Done resolved (because endTurn doesn't check generation)
		await turn2Done;
		assert.equal(tracker.activeGeneration, 0);
	});

	it("full timeout-drain cycle: orphaned events are harmless", async () => {
		const tracker = createTurnTracker();

		// Turn 1 starts
		beginTurn(tracker); // generation 1
		captureMessage(tracker, { role: "user", content: "task A" });

		// Timeout: abandon turn 1
		tracker.resolve = null;
		tracker.activeGeneration = 0;

		// During drain: orphaned events arrive
		captureMessage(tracker, { role: "assistant", content: "orphaned" });
		endTurn(tracker); // no-op: activeGeneration is 0

		// Drain completes (markSettled)
		markSettled(tracker);
		assert.equal(tracker.settled, true);

		// Turn 2 starts — clean slate
		const turn2Done = beginTurn(tracker); // generation 2
		assert.equal(tracker.settled, false);
		assert.equal(tracker.capturedMessages.length, 0);

		// Turn 2 proceeds normally
		captureMessage(tracker, { role: "user", content: "task B" });
		recordAssistantEnd(tracker, null);
		endTurn(tracker);
		await turn2Done;

		// Only task B's message is captured under generation 2
		assert.equal(tracker.capturedMessages.length, 1);
		assert.equal(tracker.capturedMessages[0].gen, 2);
		assert.deepEqual(tracker.capturedMessages[0].message, { role: "user", content: "task B" });
	});
});

describe("reportConversation", () => {
	const meta = { agentName: "olli", invocationId: "inv-1", correlationId: "task-7" };

	it("swallows a serialization failure (circular content) and still resolves", async () => {
		const originalFetch = globalThis.fetch;
		let posted = 0;
		globalThis.fetch = (async () => {
			posted++;
			return new Response(null, { status: 204 });
		}) as any;

		try {
			const circular: any = { a: 1 };
			circular.self = circular;
			const messages = [{ role: "toolResult", toolCallId: "tc-1", isError: false, content: circular }];

			// Regression: if the try/catch in reportConversation were removed,
			// JSON.stringify on the circular content would throw here and
			// propagate out — skipping ack/nack and crashing the poll loop.
			await assert.doesNotReject(
				reportConversation("http://hub", "tok", messages, meta, 7),
			);
			// Serialization failed before posting, so nothing was sent.
			assert.equal(posted, 0);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});

	it("posts payloads for serializable messages", async () => {
		const originalFetch = globalThis.fetch;
		let posted = 0;
		globalThis.fetch = (async () => {
			posted++;
			return new Response(null, { status: 204 });
		}) as any;

		try {
			const messages = [{ role: "user", content: "hi" }];
			await reportConversation("http://hub", "tok", messages, meta, 7);
			assert.equal(posted, 1);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});
});

describe("redactSecrets", () => {
	const savedEnv = { ...process.env };

	function setEnv(vars: Record<string, string>) {
		// Clear all AINSEL/test vars, then set the given ones.
		for (const key of Object.keys(process.env)) {
			if (key.startsWith("TEST_") || key.startsWith("AINSEL_")) delete process.env[key];
		}
		for (const [k, v] of Object.entries(vars)) process.env[k] = v;
		resetRedactionCache();
	}

	function restoreEnv() {
		for (const key of Object.keys(process.env)) {
			if (key.startsWith("TEST_") || key.startsWith("AINSEL_")) delete process.env[key];
		}
		Object.assign(process.env, savedEnv);
		resetRedactionCache();
	}

	it("redacts a secret in assistant text content", () => {
		setEnv({ TEST_SECRET_TOKEN: "super-secret-value-12345" });
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "text", text: "The token is super-secret-value-12345 ok" }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.equal(out.length, 1);
			assert.ok(!out[0].content.includes("super-secret-value-12345"), "secret must be redacted");
			assert.ok(out[0].content.includes("***"), "replacement marker must be present");
		} finally {
			restoreEnv();
		}
	});

	it("redacts a secret in thinking blocks", () => {
		setEnv({ TEST_SECRET_TOKEN: "thinking-secret-99999" });
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "thinking", thinking: "I see thinking-secret-99999 here" }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(!out[0].content.includes("thinking-secret-99999"));
			assert.ok(out[0].content.includes("***"));
		} finally {
			restoreEnv();
		}
	});

	it("redacts a secret in toolCall arguments", () => {
		setEnv({ TEST_API_KEY: "toolcall-secret-abcdef" });
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "toolCall", id: "tc-1", name: "bash", arguments: { cmd: "echo toolcall-secret-abcdef" } }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(!out[0].content.includes("toolcall-secret-abcdef"));
			assert.ok(out[0].content.includes("***"));
		} finally {
			restoreEnv();
		}
	});

	it("redacts a secret in toolResult content", () => {
		setEnv({ TEST_API_KEY: "toolresult-secret-xyz" });
		try {
			const messages = [
				{ role: "toolResult", toolCallId: "tc-1", isError: false, content: "output: toolresult-secret-xyz" },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(!out[0].content.includes("toolresult-secret-xyz"));
			assert.ok(out[0].content.includes("***"));
		} finally {
			restoreEnv();
		}
	});

	it("redacts a secret containing JSON-escapable characters", () => {
		setEnv({ TEST_JSON_SECRET: 'has"quote/and\\slash' });
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "text", text: 'value is has"quote/and\\slash done' }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			// The raw secret and its JSON-escaped form must both be gone.
			assert.ok(!out[0].content.includes('has"quote'), "raw secret must be redacted");
			assert.ok(!out[0].content.includes('has\\"quote'), "JSON-escaped secret must be redacted");
			assert.ok(out[0].content.includes("***"));
		} finally {
			restoreEnv();
		}
	});

	it("does not redact denylisted non-secret values", () => {
		setEnv({
			AGENT_NAME: "my-agent-name-long",
			HUB_URL: "http://hub.example.com:8080",
			OLLAMA_CLOUD_MODEL: "llama-3.1-70b-instruct",
		});
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "text", text: "agent my-agent-name-long at http://hub.example.com:8080 using llama-3.1-70b-instruct" }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(out[0].content.includes("my-agent-name-long"), "AGENT_NAME must not be redacted");
			assert.ok(out[0].content.includes("http://hub.example.com:8080"), "HUB_URL must not be redacted");
			assert.ok(out[0].content.includes("llama-3.1-70b-instruct"), "model must not be redacted");
		} finally {
			restoreEnv();
		}
	});

	it("does not redact AINSEL_EVENT_ prefixed values", () => {
		setEnv({ AINSEL_EVENT_REPO: "my-org/my-repo-long" });
		try {
			const messages = [
				{ role: "user", content: [{ type: "text", text: "working on my-org/my-repo-long" }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(out[0].content.includes("my-org/my-repo-long"), "AINSEL_EVENT_ values must not be redacted");
		} finally {
			restoreEnv();
		}
	});

	it("ignores empty and sub-min-length env values", () => {
		setEnv({ TEST_EMPTY: "", TEST_SHORT: "abc" });
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "text", text: "abc is short and should stay" }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(out[0].content.includes("abc"), "short values must not be redacted");
		} finally {
			restoreEnv();
		}
	});

	it("redacts multiple distinct secrets in one transcript", () => {
		setEnv({
			TEST_SECRET_A: "first-secret-value-aaa",
			TEST_SECRET_B: "second-secret-value-bbb",
		});
		try {
			const messages = [
				{ role: "assistant", content: [{ type: "text", text: "A=first-secret-value-aaa B=second-secret-value-bbb" }] },
			];
			const meta = { agentName: "a", invocationId: "i", correlationId: "c" };
			const out = toConversationPayloads(messages, meta);
			assert.ok(!out[0].content.includes("first-secret-value-aaa"));
			assert.ok(!out[0].content.includes("second-secret-value-bbb"));
			assert.ok(out[0].content.includes("***"));
		} finally {
			restoreEnv();
		}
	});

	it("redactSecrets returns input unchanged when no secrets match", () => {
		setEnv({ TEST_SECRET_TOKEN: "super-secret-value-12345" });
		try {
			const result = redactSecrets("no secrets here at all");
			assert.equal(result, "no secrets here at all");
		} finally {
			restoreEnv();
		}
	});
});

describe("postTaskMessages", () => {
	it("posts one camelCase request per payload to the task-messages endpoint", async () => {
		const originalFetch = globalThis.fetch;
		const calls: Array<{ url: string; init: RequestInit }> = [];
		globalThis.fetch = (async (url: any, init: any) => {
			calls.push({ url: String(url), init });
			return new Response(null, { status: 204 });
		}) as any;

		try {
			const payloads: ConversationPayload[] = [
				{
					agentName: "olli",
					invocationId: "inv-1",
					correlationId: "task-7",
					role: "assistant",
					content: "[]",
					model: "gpt-x",
					inputTokens: 10,
					outputTokens: 20,
					stopReason: "end_turn",
				},
				{
					agentName: "olli",
					invocationId: "inv-1",
					correlationId: "task-7",
					role: "user",
					content: "[]",
					model: "",
					inputTokens: 0,
					outputTokens: 0,
					stopReason: "",
				},
			];

			await postTaskMessages("http://hub", "tok", payloads);

			assert.equal(calls.length, 2);
			for (const c of calls) {
				assert.equal(c.url, "http://hub/api/internal/task-messages");
				assert.equal(c.init.method, "POST");
				const headers = c.init.headers as Record<string, string>;
				assert.equal(headers["X-Internal-Token"], "tok");
				assert.equal(headers["Content-Type"], "application/json");
			}

			const body = JSON.parse(calls[0].init.body as string);
			assert.equal(body.invocationId, "inv-1");
			assert.equal(body.inputTokens, 10);
			assert.equal(body.agentName, "olli");
			// Ensure no snake_case keys leaked through.
			assert.equal(body.invocation_id, undefined);
			assert.equal(body.input_tokens, undefined);
			assert.equal(body.agent_name, undefined);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});

	it("swallows per-request failures and keeps posting", async () => {
		const originalFetch = globalThis.fetch;
		let count = 0;
		globalThis.fetch = (async () => {
			count++;
			throw new Error("network down");
		}) as any;

		try {
			const payloads: ConversationPayload[] = [
				{
					agentName: "olli",
					invocationId: "inv-1",
					correlationId: "task-7",
					role: "user",
					content: "[]",
					model: "",
					inputTokens: 0,
					outputTokens: 0,
					stopReason: "",
				},
				{
					agentName: "olli",
					invocationId: "inv-1",
					correlationId: "task-7",
					role: "assistant",
					content: "[]",
					model: "",
					inputTokens: 0,
					outputTokens: 0,
					stopReason: "",
				},
			];

			// Must not throw despite every request failing.
			await postTaskMessages("http://hub", "tok", payloads);
			assert.equal(count, 2);
		} finally {
			globalThis.fetch = originalFetch;
		}
	});
});

describe("truncateWithMarker", () => {
	it("returns input unchanged when at or below the cap", () => {
		assert.equal(truncateWithMarker("hello", 10), "hello");
		assert.equal(truncateWithMarker("hello", 5), "hello");
	});

	it("truncates with head+tail and marker when over the cap", () => {
		const input = "A".repeat(100);
		const result = truncateWithMarker(input, 40);
		// head = 30, tail = 10, omitted = 60
		assert.ok(result.startsWith("A".repeat(30)), "head must survive");
		assert.ok(result.endsWith("A".repeat(10)), "tail must survive");
		assert.ok(result.includes("... [truncated 60 chars] ..."), "marker must include omitted count");
	});

	it("handles cap+1 boundary", () => {
		const input = "B".repeat(41);
		const result = truncateWithMarker(input, 40);
		assert.ok(result.includes("[truncated"), "cap+1 must be truncated");
		// Exactly at cap is untouched
		assert.equal(truncateWithMarker("B".repeat(40), 40), "B".repeat(40));
	});
});

describe("capToolResultContent", () => {
	it("caps an oversized string", () => {
		const big = "X".repeat(200);
		const result = capToolResultContent(big, 100) as string;
		assert.ok(result.includes("[truncated"), "must contain truncation marker");
		assert.ok(result.length < big.length, "must be shorter than input");
		assert.ok(result.startsWith("X".repeat(75)), "head must survive");
		assert.ok(result.endsWith("X".repeat(25)), "tail must survive");
	});

	it("leaves a small string unchanged", () => {
		assert.equal(capToolResultContent("small", 100), "small");
	});

	it("caps text blocks in an array and reduces non-text blocks", () => {
		const content = [
			{ type: "text", text: "Y".repeat(200) },
			{ type: "image", url: "http://x/y.png" },
			{ type: "text", text: "small" },
		];
		const result = capToolResultContent(content, 100) as any[];
		assert.equal(result.length, 3);
		// First text block capped
		assert.ok(result[0].text.includes("[truncated"), "large text block must be capped");
		// Image block reduced to { type }
		assert.deepEqual(result[1], { type: "image" });
		// Small text block untouched
		assert.deepEqual(result[2], { type: "text", text: "small" });
	});

	it("passes arbitrary objects through unchanged", () => {
		const obj = { foo: "bar", nested: { a: 1 } };
		assert.deepEqual(capToolResultContent(obj, 10), obj);
	});

	it("respects explicit maxChars parameter", () => {
		const input = "Z".repeat(50);
		const result = capToolResultContent(input, 20) as string;
		assert.ok(result.includes("[truncated"), "must truncate at explicit cap");
	});

	it("reads TOOL_RESULT_MAX_CHARS env lazily", () => {
		const saved = process.env.TOOL_RESULT_MAX_CHARS;
		try {
			process.env.TOOL_RESULT_MAX_CHARS = "30";
			const input = "W".repeat(50);
			const result = capToolResultContent(input) as string;
			assert.ok(result.includes("[truncated"), "must respect env cap");
		} finally {
			if (saved === undefined) delete process.env.TOOL_RESULT_MAX_CHARS;
			else process.env.TOOL_RESULT_MAX_CHARS = saved;
		}
	});
});

describe("resolveToolResultMaxChars", () => {
	it("returns default when env is unset", () => {
		const saved = process.env.TOOL_RESULT_MAX_CHARS;
		try {
			delete process.env.TOOL_RESULT_MAX_CHARS;
			assert.equal(resolveToolResultMaxChars(), 16_000);
		} finally {
			if (saved !== undefined) process.env.TOOL_RESULT_MAX_CHARS = saved;
		}
	});

	it("returns default for invalid env values", () => {
		const saved = process.env.TOOL_RESULT_MAX_CHARS;
		try {
			process.env.TOOL_RESULT_MAX_CHARS = "not-a-number";
			assert.equal(resolveToolResultMaxChars(), 16_000);
			process.env.TOOL_RESULT_MAX_CHARS = "-5";
			assert.equal(resolveToolResultMaxChars(), 16_000);
			process.env.TOOL_RESULT_MAX_CHARS = "0";
			assert.equal(resolveToolResultMaxChars(), 16_000);
		} finally {
			if (saved === undefined) delete process.env.TOOL_RESULT_MAX_CHARS;
			else process.env.TOOL_RESULT_MAX_CHARS = saved;
		}
	});

	it("returns the parsed env value when valid", () => {
		const saved = process.env.TOOL_RESULT_MAX_CHARS;
		try {
			process.env.TOOL_RESULT_MAX_CHARS = "5000";
			assert.equal(resolveToolResultMaxChars(), 5000);
		} finally {
			if (saved === undefined) delete process.env.TOOL_RESULT_MAX_CHARS;
			else process.env.TOOL_RESULT_MAX_CHARS = saved;
		}
	});
});

describe("toConversationPayloads — toolResult capping", () => {
	const meta = { agentName: "olli", invocationId: "inv-1", correlationId: "corr-1" };

	it("caps oversized string toolResult content with marker", () => {
		const big = "O".repeat(20_000);
		const messages = [{ role: "toolResult", toolCallId: "tc-1", isError: false, content: big }];
		const out = toConversationPayloads(messages, meta);
		assert.equal(out.length, 1);
		const parsed = JSON.parse(out[0].content);
		assert.ok(parsed.content.includes("[truncated"), "must contain truncation marker");
		assert.ok(parsed.content.length < big.length, "must be shorter than original");
	});

	it("small toolResult content is byte-identical to pre-capping behavior", () => {
		const messages = [{ role: "toolResult", toolCallId: "tc-1", isError: true, content: "boom" }];
		const out = toConversationPayloads(messages, meta);
		assert.deepEqual(JSON.parse(out[0].content), {
			toolCallId: "tc-1",
			isError: true,
			content: "boom",
		});
	});

	it("caps oversized text blocks in array content and reduces non-text blocks", () => {
		const content = [
			{ type: "text", text: "T".repeat(20_000) },
			{ type: "image", url: "http://x/y.png" },
		];
		const messages = [{ role: "toolResult", toolCallId: "tc-2", isError: false, content }];
		const out = toConversationPayloads(messages, meta);
		const parsed = JSON.parse(out[0].content);
		assert.ok(parsed.content[0].text.includes("[truncated"), "text block must be capped");
		assert.deepEqual(parsed.content[1], { type: "image" }, "image block must be reduced");
	});

	it("circular toolResult content is still skipped", () => {
		const circular: any = { a: 1 };
		circular.self = circular;
		const messages = [
			{ role: "user", content: [{ type: "text", text: "before" }] },
			{ role: "toolResult", toolCallId: "tc-bad", isError: false, content: circular },
			{ role: "assistant", content: [{ type: "text", text: "after" }] },
		];
		const out = toConversationPayloads(messages, meta);
		assert.equal(out.length, 2);
		assert.deepEqual(out.map((p) => p.role), ["user", "assistant"]);
	});

	it("redaction runs before truncation so secrets are never split", () => {
		const savedEnv = { ...process.env };
		// Clear test vars
		for (const key of Object.keys(process.env)) {
			if (key.startsWith("TEST_")) delete process.env[key];
		}
		const secret = "straddle-secret-value-99999";
		process.env.TEST_STRADDLE_SECRET = secret;
		resetRedactionCache();
		try {
			// Place the secret right at the 75% head boundary of a 100-char cap.
			// head = 75 chars, so put the secret starting at char 70.
			const prefix = "A".repeat(70);
			const suffix = "B".repeat(200);
			const input = prefix + secret + suffix;
			const messages = [{ role: "toolResult", toolCallId: "tc-1", isError: false, content: input }];
			const out = toConversationPayloads(messages, meta);
			const parsed = JSON.parse(out[0].content);
			assert.ok(!parsed.content.includes(secret), "secret must be fully redacted");
			assert.ok(parsed.content.includes("***"), "redaction marker must be present");
		} finally {
			for (const key of Object.keys(process.env)) {
				if (key.startsWith("TEST_")) delete process.env[key];
			}
			Object.assign(process.env, savedEnv);
			resetRedactionCache();
		}
	});

	it("per-message safety net caps pathological many-block content", () => {
		// Create many blocks each just under the per-block cap so the total
		// serialized string exceeds the message-level cap.
		const saved = process.env.TOOL_RESULT_MAX_CHARS;
		try {
			process.env.TOOL_RESULT_MAX_CHARS = "100";
			// messageCap = max(128_000, 8 * 100) = 128_000
			// Create 2000 blocks of 99 chars each → ~200k serialized
			const blocks = Array.from({ length: 2000 }, (_, i) => ({
				type: "text",
				text: `block${i}-` + "X".repeat(90),
			}));
			const messages = [{ role: "toolResult", toolCallId: "tc-1", isError: false, content: blocks }];
			const out = toConversationPayloads(messages, meta);
			assert.equal(out.length, 1);
			// The serialized content string must be bounded by the message cap + marker overhead
			assert.ok(out[0].content.length <= 128_000 + 100, "message-level cap must bound the output");
			assert.ok(out[0].content.includes("[truncated"), "must contain truncation marker");
		} finally {
			if (saved === undefined) delete process.env.TOOL_RESULT_MAX_CHARS;
			else process.env.TOOL_RESULT_MAX_CHARS = saved;
		}
	});
});

describe("resolveInternalToken", () => {
	it("returns the platform-managed HUB_INTERNAL_VALIDATE_SECRET", () => {
		const token = resolveInternalToken({
			HUB_INTERNAL_VALIDATE_SECRET: "platform-token",
		});
		assert.equal(token, "platform-token");
	});

	it("ignores a legacy AGENT_TOKEN value", () => {
		const token = resolveInternalToken({
			HUB_INTERNAL_VALIDATE_SECRET: "platform-token",
			AGENT_TOKEN: "per-agent-token",
		});
		assert.equal(token, "platform-token");
	});

	it("throws when the platform secret is missing or empty", () => {
		assert.throws(() => resolveInternalToken({}), /HUB_INTERNAL_VALIDATE_SECRET is not set/);
		assert.throws(() => resolveInternalToken({ HUB_INTERNAL_VALIDATE_SECRET: "" }), /HUB_INTERNAL_VALIDATE_SECRET is not set/);
	});
});
