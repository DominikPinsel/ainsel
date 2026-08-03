# Changelog

All notable changes to this project will be documented in this file.

### Bug Fixes

- Forward SIGTERM to pi process in RPC mode (53f7c5c)
- Correct SIGTERM trap syntax in entrypoint.sh (ff8d3ab)
- Background pipeline + wait so SIGTERM trap fires (bc18bec)
- Remove deleted tool binary stages from pi/Dockerfile (2961aba)
- Redirect stdout to /dev/null to prevent pipe deadlock (357356b)
- Move stdout redirect comment outside single-quoted sh -c block (3aded85)
- Redirect stdout to tmp file instead of /dev/null (30c8189)

### Documentation

- Trim dead cross-references and align with template (286c186)
- Correct reconciled-resources and pi-extensions layout (41f5a21)

### Features

- Add env field to AgentImage for image-level default env vars (7c4b391)
- Add OpenCode as selectable LLM provider (2a36939)
- Add Go variant Dockerfile with matrix CI build (7090678)
- Add .NET MAUI variant Dockerfile with CI build (3e8b90e)
- Use separate repos for variant images (81e489e)
- Migrate image registry from zot/ghcr to Docker Hub (bbebd71)

