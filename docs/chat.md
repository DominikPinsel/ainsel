# Chat Sessions

AInsel's chat feature lets you talk directly to an agent from the hub UI — no webhook or trigger required. This is useful for ad-hoc questions, debugging an agent's behaviour, or testing a persona before wiring it to a trigger.

## Starting a chat

1. Navigate to **Chat** (`/chat`) in the sidebar.
2. Click **New Chat** and select an agent from the dropdown.
3. A new session is created and you're taken to the chat view (`/chat/<session-id>`).

The agent uses its configured persona, model, and tools — the same configuration it uses when responding to events. Messages you send are delivered to the agent runtime; the agent's reply is streamed back and rendered as markdown.

## The chat view

- **Message input** — Type a message and press **Enter** to send. Use **Shift+Enter** for a newline.
- **Message history** — Previous messages in the session are displayed with timestamps and token counts.
- **Status messages** — System events (session created, agent started, etc.) appear as centred status lines.
- **Delete** — Click **Delete Chat** in the titleblock to permanently remove the session and all its messages.

## Managing sessions

The chat list (`/chat`) shows all sessions with the agent name, creation date, and last activity. You can filter by agent. Sessions are per-user — each operator sees only their own chats.

## When to use chat vs triggers

- **Chat** — interactive, on-demand conversations. Good for questions, testing, and debugging.
- **Triggers** — automated, event-driven. Good for production workflows like PR review and issue triage.

Chat does not invoke triggers; it's a separate interaction path that goes directly to the agent runtime.