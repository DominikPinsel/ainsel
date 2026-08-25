FROM golang:1.27-bookworm AS builder

WORKDIR /workspace

# Build context = monorepo root. Copy only the modules we need and
# initialize a focused go.work so Go resolves shared/api locally.
COPY shared/api ./shared/api
COPY services/webhook-receiver ./services/webhook-receiver
RUN go work init ./shared/api ./services/webhook-receiver

WORKDIR /workspace/services/webhook-receiver

ARG VERSION=dev VCS_REF=unknown
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} \
              -X main.Commit=${VCS_REF}" \
    -o /webhook-receiver ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /webhook-receiver /webhook-receiver
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/webhook-receiver"]
