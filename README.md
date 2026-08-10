# MuxCore Kubernetes Operator

Scaffold for [MASTER-ROADMAP §4.1](https://github.com/Muxcore-Media/mvp/blob/main/deploy/README.md) — reconciles a namespaced **`MuxCorePlatform`** CR into `muxcored` + sidecar Deployments/Services.

This is an early **v0.1.0** controller (controller-runtime). Prefer Helm/Kustomize under `mvp/deploy/` for day-1 installs until GHCR images and mTLS injection land.

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

## CRD summary

| Field | Purpose |
|-------|---------|
| `spec.coreImage` | muxcored image (default `ghcr.io/muxcore-media/muxcored:v0.5.4`) |
| `spec.meshAddr` | Value for `MUXCORE_MESH_ADDR` on sidecars |
| `spec.insecureDisableTLS` | Sets `MUXCORE_INSECURE_DISABLE_TLS=true` |
| `spec.modules[]` | Sidecar Deployments (`name`, `image`, optional `port`/`env`/`envFromSecret`) |

Status reports `desiredModules` / `readyModules` and a `Ready` condition.

## Develop

```bash
make test
make build
```

Requires Go 1.26+. Cluster run needs a kubeconfig; unit tests use the controller-runtime fake client.

## Out of scope (follow-ups)

- Full media stack modules
- PVC / StorageClass wiring
- mTLS cert injection (staging parity)
- Helm chart packaging of the operator itself
- GHCR image publish (org `write:packages` P0)
