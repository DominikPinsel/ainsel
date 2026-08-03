# Security Policy

## Supported Versions

Only the latest release of ainsel is supported with security fixes. We do not backport security patches to older releases.

| Version | Supported |
|---------|-----------|
| latest  | ✓         |
| older   | ✗         |

## Reporting a Vulnerability

**Do not open a public issue for security vulnerabilities.**

Please report security issues by emailing the maintainers at the address listed in the repository's package or commit history. Include:

- A description of the vulnerability and its potential impact
- Steps to reproduce the issue
- Any suggested mitigations you have identified

You should receive an acknowledgement within 72 hours. We aim to provide a fix or mitigation within 90 days of disclosure, and will coordinate a public disclosure date with you.

## Security Architecture

For the architectural security model — trust boundaries, authentication, authorisation, agent isolation, and known gaps — see [docs/architecture.md](docs/architecture.md).

## Scope

The following are in scope for security reports:

- Authentication bypass or token leakage in the hub API
- Privilege escalation between tenants or agent namespaces
- Injection vulnerabilities in webhook payload handling or event routing
- Secrets exposed in logs, API responses, or Kubernetes manifests

The following are **out of scope**:

- Security of the underlying Kubernetes cluster or cloud provider
- Issues in third-party dependencies (report upstream)
- Theoretical attacks without a proof of concept
