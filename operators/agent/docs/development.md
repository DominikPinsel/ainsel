# Development Guide

## Prerequisites

- Go 1.22+
- Kubebuilder 4+
- kubectl
- Access to a Kubernetes cluster (or use envtest for testing)

## Project Structure

```
api/
  v1alpha1/
    agent_types.go           # Agent CRD types
    trigger_types.go         # Trigger CRD types
    groupversion_info.go     # API group registration
    zz_generated.deepcopy.go # Generated DeepCopy methods
cmd/
  main.go                    # Operator entry point
config/
  crd/                       # Generated CRD manifests
  rbac/                      # RBAC rules
  manager/                   # Manager deployment
  samples/                   # Sample CRs
internal/
  controller/
    agent_controller.go      # Agent reconciliation logic
    trigger_controller.go    # Trigger reconciliation logic
    suite_test.go            # Test suite setup (envtest)
```

## Building

```bash
# Build the operator binary
make build

# Build Docker image
make docker-build IMG=<registry>/ainsel-k8s-ai-agent-operator:tag

# Generate CRD manifests from types
make manifests

# Generate DeepCopy methods
make generate
```

## Testing

```bash
# Run unit and integration tests (uses envtest)
make test

# Run with verbose output
go test -v ./internal/controller/...
```

The tests use `envtest` which runs a real Kubernetes API server and etcd locally, so no cluster is needed.

## Running Locally

To run the operator against your current kubeconfig cluster:

```bash
# Install CRDs first
make install

# Run the operator
make run
```

## Code Generation

After modifying CRD types in `api/v1alpha1/`:

```bash
# Regenerate DeepCopy methods
make generate

# Regenerate CRD manifests
make manifests
```

## Adding a New API (kubebuilder create api)

The `PROJECT` file uses `domain: dev` + `group: ainsel`, which kubebuilder
composes as `<group>.<domain>` — so new APIs scaffold under the
`ainsel.dev` group, matching the existing CRDs and the `+groupName=ainsel.dev`
marker in `shared/api/api/v1alpha1/groupversion_info.go`. When running
`kubebuilder create api`, pass `--group ainsel` (not `ainsel.dev`) so the
resulting group resolves to `ainsel.dev` rather than the doubled
`ainsel.ainsel.dev`.

## Adding a New Field to a CRD

1. Add the field to the type in `api/v1alpha1/agent_types.go` or `trigger_types.go`
2. Run `make generate` to update DeepCopy
3. Run `make manifests` to update CRD YAML
4. Update the controller reconciliation logic
5. Add tests
6. Update the ainsel-chart CRD files
