# Ainsel API Shared Architecture

## Overview

The `ainsel-api-shared` module is the shared foundation for all Ainsel platform components. It provides three core capabilities: the canonical event schema, the filter engine, and NATS stream constants.

## Module Structure

```mermaid
graph TD
    subgraph ainsel-api-shared
        ES[Event Schema<br/>event.go, event_types.go, event_data.go]
        FE[Filter Engine<br/>filter.go]
        NC[NATS Constants<br/>nats.go]
    end

    ES -->|Event struct used by| FE
    NC -->|stream names used by| PUB[Publishers]
    NC -->|subject patterns used by| CON[Consumers]
```

## Event Schema

The canonical `Event` struct is the normalized format that all connectors produce and ainsel-hub-backend consumes. Every event flowing through the platform conforms to this schema.

```mermaid
classDiagram
    class Event {
        +string ID
        +string Version
        +string Source
        +string Connector
        +string Type
        +time.Time Timestamp
        +EventSubject Subject
        +EventActor Actor
        +string Action
        +RawJSON Data
        +string Raw
    }

    class EventSubject {
        +string Kind
        +string Owner
        +string Repo
        +int Number
    }

    class EventActor {
        +string Login
        +string DisplayName
        +bool IsBot
    }

    Event --> EventSubject
    Event --> EventActor

    class IssueData {
        +IssuePayload Issue
    }

    class PullRequestData {
        +PullRequestPayload PullRequest
    }

    class PushData {
        +string Ref
        +string Before
        +string After
        +[]CommitPayload Commits
    }

    Event ..> IssueData : Data field
    Event ..> PullRequestData : Data field
    Event ..> PushData : Data field
```

## Filter Engine

The filter engine evaluates conditions against the event `Data` payload using dotted field paths and a set of operators. Multiple filters are combined with AND logic.

```mermaid
flowchart TD
    A[Incoming Event] --> B[Extract Data payload]
    B --> C{For each Filter}
    C --> D[Resolve dotted field path]
    D --> E{Field exists?}
    E -->|No| F[Return false]
    E -->|Yes| G{Apply operator}
    G -->|Match| H{More filters?}
    G -->|No match| F
    H -->|Yes| C
    H -->|No| I[Return true - all matched]
```

### Field Resolution

Fields use dotted notation to navigate nested JSON:
- `issue.title` resolves to `data.issue.title`
- `comment.body` resolves to `data.comment.body`

### Event Type Matching

The `MatchEventType` function supports wildcard patterns:
- `*` matches all event types
- `issue.*` matches `issue.opened`, `issue.comment.created`, etc.
- `push` matches only `push`

## NATS Subject Layout

```mermaid
graph LR
    subgraph "EVENTS stream"
        E1[events.forgejo.issue.opened]
        E2[events.forgejo.push]
        E3[events.forgejo.pull_request.opened]
    end

    subgraph "AGENTS stream"
        A1[agent.code-reviewer]
        A2[agent.issue-triager]
    end

    subgraph "HUB stream"
        H1[hub.invocation.completed]
    end
```

Subject format:
- Events: `events.<connectorName>.<eventType>`
- Agents: `agent.<agentName>`
- Hub: `hub.invocation.completed`
