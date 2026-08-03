# Development Guide

## Prerequisites

- Go 1.22+
- NATS server with JetStream
- Access to a Kubernetes cluster with CRDs installed

## Project Structure

```
cmd/
  hub/
    main.go                     # Entry point, wiring
internal/
  api/
    server.go                   # HTTP server, routing
    handlers_agents.go          # Agent CRUD handlers
    handlers_connectors.go      # Connector CRUD handlers
    handlers_triggers.go        # Trigger CRUD handlers
  router/
    router.go                   # NATS event consumer + publisher
  trigger/
    index.go                    # In-memory trigger index
    index_test.go               # Index tests
  metrics/
    metrics.go                  # Prometheus metrics
```

## Building

```bash
make build
```

## Testing

```bash
make test

# Verbose
go test -v ./...
```

## Running Locally

1. Ensure NATS is running with JetStream:
   ```bash
   nats-server -js
   ```

2. Ensure CRDs are installed in your cluster:
   ```bash
   kubectl apply -f ../ainsel-chart/crds/
   ```

3. Run the hub backend:
   ```bash
   export NATS_URL="nats://localhost:4222"
   export HUB_NAMESPACE="ainsel"
   go run ./cmd/hub/
   ```

4. Test the API:
   ```bash
   curl http://localhost:8080/health
   curl http://localhost:8080/api/v1/agents
   ```

## Key Design Decisions

- Uses `controller-runtime` Manager for Kubernetes informers (not a full operator, just cache)
- Trigger index is in-memory for fast matching -- rebuilt from informer on startup
- REST API is a thin HTTP wrapper over `controller-runtime` client CRUD
- Event routing is push-based: subscribe to EVENTS, match, publish to AGENTS
