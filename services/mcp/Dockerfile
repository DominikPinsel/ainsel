FROM golang:1.27-alpine AS build

WORKDIR /workspace

# Build context = monorepo root. mcp depends on shared/auth/oidc via a
# replace directive in its go.mod — copy that in alongside.
COPY shared/auth ./shared/auth
COPY services/mcp ./services/mcp

WORKDIR /workspace/services/mcp

RUN CGO_ENABLED=0 go build -o /bin/ainsel-mcp ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/ainsel-mcp /bin/ainsel-mcp
USER nonroot:nonroot
ENTRYPOINT ["/bin/ainsel-mcp"]
