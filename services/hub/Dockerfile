FROM golang:1.26-bookworm AS builder

WORKDIR /workspace

# Build context = monorepo root. Copy only the modules we need and
# initialize a focused go.work so Go resolves shared packages locally.
COPY shared/api ./shared/api
COPY shared/auth ./shared/auth
COPY services/hub ./services/hub
RUN go work init ./shared/api ./shared/auth ./services/hub

WORKDIR /workspace/services/hub

ARG VERSION=dev VCS_REF=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/DominikPinsel/ainsel/services/hub/pkg/version.Version=${VERSION} \
              -X github.com/DominikPinsel/ainsel/services/hub/pkg/version.Commit=${VCS_REF}" \
    -o /hub ./cmd/hub

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /hub /hub
USER nonroot:nonroot
EXPOSE 8080 9090
ENTRYPOINT ["/hub"]
