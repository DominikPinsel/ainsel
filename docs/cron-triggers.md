# Cron Triggers

Cron triggers (also called "scheduled prompts") let you invoke an agent on a recurring schedule — no webhook required. A cron trigger fires the agent with a fixed prompt at times you define, using standard cron expressions.

Common use cases:

- **Daily summary** — "Summarise yesterday's PR activity and post a digest."
- **Periodic sweep** — "Check for stale issues older than 7 days and label them."
- **Health check** — "Verify the connector is receiving webhooks and report if not."

## Creating a cron trigger

Cron triggers are managed on the **agent detail page** (`/agents/:id`), under the **Schedules** tab.

1. Navigate to an agent's detail page.
2. Open the **Schedules** tab.
3. Click **Add Schedule**.
4. Fill in:
   - **Name** — a unique identifier for this trigger.
   - **Schedule** — a cron expression (e.g. `0 9 * * 1-5` for 09:00 on weekdays).
   - **Prompt** — the message sent to the agent on each fire.
   - **Enabled** — toggle on to activate immediately.
5. Save.

## How it works

When a cron trigger fires:

1. The hub publishes an event to the agent's NATS subject with the configured prompt.
2. The agent runtime picks up the event, processes it using its persona, model, and tools.
3. The result is recorded as an invocation — visible in `/activity` with the trigger name as the source.

The trigger's **status** panel shows whether the agent reference and schedule are valid, plus the last and next run times.

## Cron expressions

AInsel uses standard 5-field cron syntax:

```
┌───── minute (0–59)
│ ┌───── hour (0–23)
│ │ ┌───── day of month (1–31)
│ │ │ ┌───── month (1–12)
│ │ │ │ ┌───── day of week (0–6, 0 = Sunday)
│ │ │ │ │
* * * * *
```

Examples:

| Expression | Meaning |
|---|---|
| `0 9 * * 1-5` | 09:00 Monday–Friday |
| `0 */2 * * *` | Every 2 hours |
| `30 18 * * 0` | 18:30 every Sunday |
| `0 0 1 * *` | Midnight on the 1st of every month |

## Disabling

Toggle **Enabled** off to pause a trigger without deleting it. The schedule is preserved and can be re-enabled at any time.