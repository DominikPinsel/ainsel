# Naming Convention

Every named entity in the platform follows one scheme, so a name tells you
**what kind of thing it is, what role it plays, and which scope it belongs
to** — in that order, lowercase kebab. Identity is always the id (`a-…`,
`c-…`, `ch-…`); names are labels. Both are safe to change: renames go
through the hub API / MCP (`display_name`), never by recreating the CRD.

## The rule

`^[a-z0-9-]{1,48}$` — lowercase letters, digits, hyphens. A name **starts
with the entity kind** and **never repeats a type word** (no "Agent",
"Connector", "Channel" inside the label — the kind prefix carries that).
Role words, not handles (`developer`, never `dev-agent`); at most three
words after the prefix.

| Kind | Pattern | Role of each part | Examples |
|------|---------|-------------------|----------|
| Agent | `agent-<role>[-<scope>]` | what it does; where it acts | `agent-developer` · `agent-reviewer-gh` · `agent-developer-local` |
| Connector | `connector-<platform>-<scope>` | which system; which org/repo | `connector-forgejo-ainsel` · `connector-github-ainsel` |
| Channel (provisioned) | — | mirrors its entity, always | renaming a connector renames its channel on the next sync |
| Channel (custom) | `channel-<purpose>[-<scope>]` | what it groups | `channel-warhammer-backlog` |
| Trigger | `trigger-<role>-<event>[-<scope>]` | who reacts; to what; where | `trigger-developer-mention-apps` · `trigger-refiner-issue-opened-ainsel` |
| Cron trigger | `cron-<role>-<purpose>` | who runs; why | `cron-developer-pr-check` |
| Persona | `persona-<role>` | the role the persona speaks as | `persona-product-owner` |
| Agent image | `image-<role>[-<scope>]` (display name) | runtime behind the role | `image-refiner-gh` |
| Skill / MCP server | `kebab-purpose` | existing convention, unchanged | `commit-hygiene-and-prs` |

**Scope tokens** are fixed vocabulary, taken from the connector's last
segment: `ainsel` (Forgejo org), `apps`, `my-ide-app`, `gh` (the AInsel
repo on GitHub). A trigger's scope suffix must be the scope token of the
connector it binds — never a platform name in the middle (`refiner-mention-
ainsel-github` is wrong order; `trigger-refiner-mention-gh` is the shape).

## Why the prefix

The console, MCP tool results, activity rows and event timelines all show
bare labels side by side. With kind prefixes, `agent-reviewer` in a search
box needs no hover to disambiguate from `connector-reviewer` or an
`image-reviewer`, channel lists sort by kind naturally, and a name copied
into a filter, commit message, or Slack thread still says what it is.

## Applying it

- **Renaming:** MCP `update_agent` / `update_connector` (`display_name`),
  `update_trigger` / `update_cron_trigger` (`displayName`), the console
  edit forms, or `PUT /api/v1/{agents,connectors,triggers}/{id}`. Ids,
  refs, channels and history follow automatically — a rename is a label
  change, not a new stream.
- **Forge mentions are not names.** Trigger filters like
  `comment.body contains @dev-agent` match *forge usernames* — renaming an
  agent never changes which comments fire it, and vice versa.
- **New entities:** pick the role first (`who does this act as?`), then
  scope if the role alone collides. If a name needs a sentence to explain
  it, the description field is the place for that, not the name.

The whole platform was aligned to this scheme in September 2026 (56
entities); the tables above are the surviving reference.
