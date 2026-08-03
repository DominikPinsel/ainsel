# Development Guide

## Prerequisites

- Go 1.22+
- Kubebuilder 4+
- kubectl
- Access to a Kubernetes cluster (or use envtest)

## Project Structure

```
api/
  v1alpha1/
    webhookconnector_types.go  # WebhookConnector CRD types
    groupversion_info.go       # API group registration
    zz_generated.deepcopy.go   # Generated DeepCopy methods
cmd/
  main.go                      # Operator entry point
config/
  crd/                         # Generated CRD manifests
  rbac/                        # RBAC rules
  manager/                     # Manager deployment
  samples/                     # Sample CRs
internal/
  controller/                  # Reconciliation logic
```

## Building

```bash
# Build the operator binary
make build

# Build Docker image
make docker-build IMG=<registry>/ainsel-k8s-event-source-gateway-operator:tag

# Generate CRD manifests
make manifests

# Generate DeepCopy methods
make generate
```

## Testing

```bash
# Run all tests
make test
```

## Running Locally

```bash
# Install CRDs
make install

# Run the operator
make run
```

## Code Generation

After modifying CRD types in `api/v1alpha1/`:

```bash
make generate   # DeepCopy methods
make manifests  # CRD YAML
```

## Adding a New API (kubebuilder create api)

The `PROJECT` file uses `domain: dev` + `group: ainsel`, which kubebuilder
composes as `<group>.<domain>` — so new APIs scaffold under the
`ainsel.dev` group, matching the existing CRDs and the `+groupName=ainsel.dev`
marker in `shared/api/api/v1alpha1/groupversion_info.go`. When running
`kubebuilder create api`, pass `--group ainsel` (not `ainsel.dev`) so the
resulting group resolves to `ainsel.dev` rather than the doubled
`ainsel.ainsel.dev`.
