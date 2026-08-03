# Skills

Skills are reusable prompt fragments that agents can reference. A skill encapsulates a piece of expertise — how to review code, how to triage an issue, how to write a commit message — as a named, versioned block of text that any persona can include.

## Why skills?

Without skills, every persona has to contain all the instructions it needs inline. If two agents both need to know how to write a good PR review, you duplicate that knowledge in both personas. Skills solve this:

- **Reuse** — define a skill once, reference it from any persona.
- **Consistency** — update a skill and every persona that uses it picks up the change.
- **Separation** — keep personas focused on role and tone; move domain expertise into skills.

## Managing skills

Skills live at `/skills` in the UI.

- **List** (`/skills`) — shows all skills with name, description, and last-modified date.
- **Create** (`/skills/new`) — define a skill with a name, a short description, and a body (the prompt text).
- **Edit** (`/skills/:id/edit`) — modify the name, description, or body.
- **Detail** (`/skills/:id`) — view the full skill content.

A skill's **body** is free-form text — typically markdown instructions or a prompt template. How it's consumed depends on the agent runtime; the persona references the skill by name and the runtime injects the body into the prompt at invocation time.

## Using skills in personas

When writing a persona, reference skills by name. The agent runtime resolves skill references at invocation time, so updating a skill immediately affects all personas that use it — no persona republish needed.