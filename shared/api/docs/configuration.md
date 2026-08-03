# Configuration

The `ainsel-api-shared` module is a library and does not have its own configuration. Instead, it provides constants and types that consuming components configure.

## NATS Constants

These constants are used by all components to connect to the same streams and subjects:

| Constant | Value | Used By |
|----------|-------|---------|
| `StreamEvents` | `EVENTS` | ainsel-event-source-gateway-forgejo (publish), ainsel-hub-backend (consume) |
| `StreamAgents` | `AGENTS` | ainsel-hub-backend (publish), ainsel-ai-agent (consume) |
| `StreamHub` | `HUB` | ainsel-ai-agent (publish), ainsel-hub-backend (consume) |
| `SubjectEventsAll` | `events.>` | ainsel-hub-backend consumer subscription |
| `SubjectAgentsAll` | `agent.>` | ainsel-ai-agent consumer subscription |
| `SubjectHubAll` | `hub.>` | ainsel-hub-backend completion consumer |
| `SubjectHubInvocationCompleted` | `hub.invocation.completed` | ainsel-ai-agent publishes on completion |

## Helper Functions

| Function | Format | Example |
|----------|--------|---------|
| `EventsSubject(connector, eventType)` | `events.<connector>.<type>` | `events.forgejo.issue.opened` |
| `AgentSubject(agentName)` | `agent.<name>` | `agent.code-reviewer` |
