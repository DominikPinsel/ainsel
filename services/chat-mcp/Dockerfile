FROM golang:1.26-bookworm AS builder

WORKDIR /workspace

# Build context = monorepo root. Copy only the modules we need and
# initialize a focused go.work so Go resolves shared packages locally.
COPY shared/api ./shared/api
COPY services/chat-mcp ./services/chat-mcp
RUN go work init ./shared/api ./services/chat-mcp

WORKDIR /workspace/services/chat-mcp

ARG VERSION=dev VCS_REF=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.Commit=${VCS_REF}" \
    -o /chat-mcp ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /chat-mcp /chat-mcp
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/chat-mcp"]
