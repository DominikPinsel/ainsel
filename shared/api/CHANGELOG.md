# Changelog

All notable changes to this project will be documented in this file.

### Features

- Simplify ForgejoConnector — user-supplied bot token, drop admin/webhook flow (555f893)


### Features

- Remove Agent.spec.forgejo and AgentForgejo type (382af46)

### Release

- V1.0.0 (12f0a5f)


### Features

- Add optional ForgejoConnector.spec.adminCredentials (additive) (a903179)

### Release

- V0.7.0 (94cc520)


### Bug Fixes

- Align botUserId JSON tag and add nil deepcopy test (fa04830)

### Features

- Add optional ForgejoConnector.spec.botIdentity (additive) (140d91c)

### Release

- V0.6.0 (325eaeb)


### Features

- Add agentTokens and reactionConsumerName fields (61343aa)
- Add AgentImage CRD types (5fe631b)
- Replace Agent runtime.image and skills with imageRef and enabledTools (4fae790)

### Refactoring

- Replace AgentTokens with ReactionBot (bdbb326)


### Bug Fixes

- Run notify-dependents on go-1.26 runner (4321b99)

### Features

- Add APIURL field to GitHubConnectorSpec for GHE support (d0342df)

### Release

- V0.4.0 (69a5bc3)


### Features

- Add Organization field to ForgejoConnectorSpec (15308ef)
- Add DisplayName field to connector and trigger CRD specs (8d746a1)


### Bug Fixes

- Update dependent repo names in notify-dependents workflow (7ceb885)
- Update common.Filter to ainselapishared.Filter in v1alpha1 (3c5a547)

### Documentation

- Update repo name references after rename (6ffe5b4)

### Features

- Add workflow to notify dependents on new release (19112d3)
- Add GitHubConnector CRD types (c5f8df5)

### Refactoring

- Rename package from common to ainselapishared (a0fadd9)


### Bug Fixes

- Use triple-quoted string in cliff.toml header (89fd7a7)
- Use GITHUB_HEAD_REF for PR checkout branch (72c7798)

### Release

- V0.2.1 (8c24e1a)


### Bug Fixes

- Release-pr closes stale PRs when version changes (87aa47c)
- Use go install for golangci-lint, make git-cliff install fault-tolerant (24748e9)
- Pin Go version in CI instead of go-version-file (9f9c982)
- Replace setup-go action with manual Go installation (7d80635)
- Remove race detector from CI tests (not supported in DinD containers) (46d7d25)
- Set Go PATH at job level, remove sudo (05f0d0f)
- Inline PATH export in each CI step for Forgejo compatibility (c8e29a4)
- Use go-1.26 runner image (Go pre-installed, no download needed) (27992d1)
- Use ubuntu-latest with inline Go install, fix git-cliff download URL (1faf7f6)
- Set GOPROXY=off for common (zero external deps) (bd54ae3)
- Use internal Forgejo URL for checkout (bypass Authelia) (84890da)
- Add auth token to internal git clone URLs (private repos) (50b697c)
- Use release-tools runner label instead of container block (bf20453)
- Replace actions/checkout with internal git clone (2638932)

### Documentation

- Add comprehensive documentation with architecture diagrams (9507459)

### Features

- Use custom go-1.26 runner image with pre-installed Go and golangci-lint (2163a97)
- Add CRD types (Agent, Trigger, ForgejoConnector) to common (ce12589)

### Debug

- Add verbose output to test step (6904481)


### Features

- Add canonical event schema (32a387d)
- Add typed data payloads for all event types (18b116f)
- Add filter engine with wildcard event type matching (9dbb91d)
- Add NATS stream and subject constants (1c8a1d6)
- Add DeepCopyInto/DeepCopy methods for Filter (kubebuilder compat) (afbd7c0)

