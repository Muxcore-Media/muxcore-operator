# MuxCore Kubernetes Operator

Scaffold for [MASTER-ROADMAP §4.1](https://github.com/Muxcore-Media/mvp/blob/main/deploy/README.md) — reconciles a namespaced **`MuxCorePlatform`** CR into `muxcored` + sidecar Deployments/Services.

This is an early **v0.1.0** controller (controller-runtime).

## Day-1 preference: installer / compose, not the operator

**Do not start here for a first laptop install.** Day-1 is:

1. [`muxcore-installer`](https://github.com/Muxcore-Media/muxcore-installer) (release binaries), or
2. Compose / host sidecars on one machine (see wiki [Deployment](https://github.com/Muxcore-Media/muxcore-docs/blob/main/wiki/Deployment.md) and `_mvp/`)

Use this operator only when you already need Kubernetes orchestration.

## Install (cluster)

```bash
kubectl apply -f config/crd/muxcore.media_muxcoreplatforms.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/manager/manager.yaml

kubectl create namespace muxcore
kubectl -n muxcore create secret generic muxcore-auth \
  --from-literal=admin-password='change-me'

kubectl apply -f config/samples/muxcore_v1alpha1_muxcoreplatform.yaml
```

Optional GHCR mirror sample: `config/samples/muxcore_v1alpha1_muxcoreplatform_ghcr.yaml`.

## Kind / k3d with local images only

Samples under `config/samples/kind/` use cluster-loaded tags — no registry pull.

```bash
BUILD_ONLY=1 ./scripts/publish-operator-local.sh v0.1.0
kind load docker-image muxcore-operator:v0.1.0

kubectl apply -f config/crd/muxcore.media_muxcoreplatforms.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/samples/kind/manager.yaml
kubectl apply -f config/samples/kind/muxcoreplatform.yaml   # muxcored-only soak CR
```

Soak gate: `./scripts/soak-operator-kind.sh v0.1.0`

## CRD summary

| Field | Purpose |
|-------|---------|
| `spec.coreImage` | muxcored image (default `ghcr.io/muxcore-media/muxcored:v0.6.7`) |
| `spec.meshAddr` | Sidecar `MUXCORE_GRPC_ADDR` dial target (default `<platform>-muxcored:9090`) |
| `spec.insecureDisableTLS` | Sets `MUXCORE_INSECURE_DISABLE_TLS=true` when true |
| `spec.meshTLSSecret` | Secret with `tls.crt`, `tls.key`, `ca.crt` when TLS enabled |
| `spec.modules[]` | Sidecars (`name`, `image`, ports, env, PVC, probes) |

Owned Deployments/Services are named `<platform>-<module>` (e.g. `minimal-api-rest`). Inter-module HTTP URLs in `env` must use those Service DNS names.

Sidecars receive `MUXCORE_GRPC_ADDR` and `MUXCORE_MODULE_ID`. muxcored listens on `MUXCORE_MESH_ADDR=:9090`.

Status reports `desiredModules` / `readyModules` and a `Ready` condition. Reconcile/prune events are recorded on the Platform.

## Security / RBAC

- Operator `--watch-namespace=muxcore` (see `config/manager/manager.yaml`).
- Namespaced **Role** in `muxcore` for Deployments/Services/PVCs (not cluster-wide).
- Metrics bind to `127.0.0.1:8080` (loopback only).
- Manager and managed pods run non-root with dropped capabilities.

## Develop

```bash
make test
make build
golangci-lint run ./...
```

Requires Go 1.26+. Unit tests use the controller-runtime fake client.

## Image publish (LAN / GHCR)

```bash
BUILD_ONLY=1 ./scripts/publish-operator-local.sh v0.1.0
./scripts/publish-operator-local.sh v0.1.0   # push to the LAN registry (default localhost:5000/muxcore)
MUXCORE_REGISTRY=localhost:5000/muxcore ./scripts/publish-operator-local.sh v0.1.0
```

Default images in the CRD and samples come from `ghcr.io/muxcore-media/*`; override with `MUXCORE_REGISTRY` when publishing to a LAN registry.

## Out of scope (follow-ups)

- Full media/acquisition stack in one CR (see acquisition sample for optional modules)
- Helm chart packaging of the operator itself
- cert-manager integration (bring your own `meshTLSSecret`)
