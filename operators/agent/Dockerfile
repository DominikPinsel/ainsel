# Build the manager binary
FROM golang:1.26-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace

# Build context = monorepo root. Copy only the modules we need and
# initialize a focused go.work so Go resolves shared/api locally.
COPY shared/api ./shared/api
COPY operators/agent ./operators/agent
RUN go work init ./shared/api ./operators/agent

WORKDIR /workspace/operators/agent

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -a -o /manager ./cmd

# Runtime
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=builder /manager .
USER nonroot:nonroot
ENTRYPOINT ["/manager"]
