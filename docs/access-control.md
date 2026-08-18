# Access Control

AInsel uses **group-based access control** to isolate resources between
teams. Every resource belongs to a group, and a user's permissions are
determined by their role in that group.

## Authentication

Interactive users authenticate either through an external OIDC provider or
with hub-managed local accounts (`auth.mode: local`), depending on the
deployment. In addition, every user can create personal access tokens for
programmatic use. All methods resolve to the same user identity and grant
the same permissions.

### OIDC (interactive)

Browser-based authentication via an external provider (e.g. Zitadel). The
frontend handles the OAuth flow automatically. API requests carry a standard
`Authorization: Bearer <jwt>` header.

### Local users (interactive)

When the platform runs with `auth.mode: local`, users sign in with
username/password against the hub itself. An `admin` account is bootstrapped
on first start; further users are managed by admins via the UI (**Admin →
Users**) or the `/api/v1/users` API. Successful logins receive a session
JWT, used exactly like the OIDC one below.

### User tokens (programmatic)

Personal access tokens (PATs) for scripts, CI pipelines, and MCP clients.
Tokens are prefixed with `ainsel_` and are created via the API or the
frontend profile page.

```bash
curl -H "Authorization: Bearer ainsel_abc123..." \
  https://ainsel.example.com/api/v1/agents
```

A user token has the **same permissions as the user who created it**. It
cannot be scoped further.

Create a token:

```bash
curl -X POST https://ainsel.example.com/api/v1/user-tokens \
  -H "Authorization: Bearer <jwt>" \
  -H "Content-Type: application/json" \
  -d '{"name": "ci-token", "expiresInDays": 90}'
```

The plaintext token is returned **once** in the response. Store it securely.

## Groups

Groups are the tenancy boundary. They are flat — no nesting or hierarchy.

Any authenticated user can create a group and becomes its **owner**. All
authenticated users can browse the group directory (list groups, view
memberships).

### Roles

Each group membership carries one of three roles:

| Role   | Permissions |
|--------|-------------|
| reader | View all resources in the group and their observability data |
| writer | Create, update, and delete resources in the group |
| owner  | Writer permissions, plus manage group membership and toggle resource visibility |

Roles apply to **all resources in the group** — there are no per-resource
permission overrides.

### Managing groups

```bash
# Create a group (you become the owner)
curl -X POST https://ainsel.example.com/api/v1/groups \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "platform-team", "description": "Platform engineering"}'

# Add a member as writer
curl -X POST https://ainsel.example.com/api/v1/groups/<group-id>/members \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"userId": "<user-id>", "role": "writer"}'

# List members
curl https://ainsel.example.com/api/v1/groups/<group-id> \
  -H "Authorization: Bearer <token>"
```

## Resources and Groups

Every resource (agent, agent-image, connector, trigger, cron-trigger,
MCP server, persona, skill) belongs to exactly one group. You choose the
group when creating the resource, from the groups you are a member of.

```bash
curl -X POST https://ainsel.example.com/api/v1/agents \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"name": "code-reviewer", "groupId": "<group-id>", ...}'
```

### Visibility rules

| Action | Who can do it |
|--------|---------------|
| Read a resource | Members of the resource's group (reader or above), any user if the resource is public, admins |
| Modify a resource | Members with writer or owner role in the resource's group, admins |
| Delete a resource | Members with writer or owner role in the resource's group, admins |
| Toggle public flag | Members with owner role in the resource's group, admins |

### Public resources

Any resource can be marked as **public**, granting read access to every
authenticated user regardless of group membership. Write access still
requires group membership.

```bash
# Mark a resource as public
curl -X PUT https://ainsel.example.com/api/v1/agents/<id> \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"public": true}'
```

### Listing resources

List endpoints return only resources in **your groups** by default. Add
`?includePublic=true` to also include public resources from other groups.

```bash
# Only my groups' agents
curl https://ainsel.example.com/api/v1/agents \
  -H "Authorization: Bearer <token>"

# My agents + public agents
curl "https://ainsel.example.com/api/v1/agents?includePublic=true" \
  -H "Authorization: Bearer <token>"
```

Admins see all resources regardless of group membership.

## Chat Sessions

Chat sessions are **personal** — they belong to the user who created them.
They are not associated with a group and cannot be made public. Only the
session owner and platform admins can view or manage a session.

## Observability

Observability data (events, invocations, logs, metrics, conversations) is
scoped to the resources you can read. For example, you only see invocation
logs for agents in your groups. Aggregate dashboards only include data from
accessible resources.

## Platform Admins

Users with the admin flag bypass all authorization checks. Admins can:

- Access any resource in any group
- Manage any group's membership
- Promote/demote other admins
- See all observability data

Admin status is managed via the users API:

```bash
curl -X PATCH https://ainsel.example.com/api/v1/users/<user-id> \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{"isAdmin": true}'
```

## Internal Service Authentication

Service-to-service communication (webhook-receiver → hub, agent runtimes →
hub, MCP service → hub) uses a shared secret via the `X-Internal-Token`
header. This is scoped to `/api/internal/*` endpoints only and does not
grant access to the user-facing API.
