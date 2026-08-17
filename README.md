# MuxCore Kubernetes Operator

Scaffold for [MASTER-ROADMAP §4.1](https://github.com/Muxcore-Media/mvp/blob/main/deploy/README.md) — reconciles a namespaced **`MuxCorePlatform`** CR into `muxcored` + sidecar Deployments/Services.

This is an early **v0.1.0** controller (controller-runtime).

## Day-1 preference: installer / compose, not the operator

**Do not start here for a first laptop install.** Day-1 is:

1. [`muxcore-installer`](https://github.com/Muxcore-Media/muxcore-installer) (release binaries), or
2. Compose / host sidecars on one machine (see wiki [Deployment](https://github.com/Muxcore-Media/muxcore-docs/blob/main/wiki/Deployment.md) and `_mvp/`)

Use this operator only when you already need Kubernetes orchestration. Prefer installer/compose until then. Helm chart packaging of the operator itself is a later follow-up.

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

For kind/k3d, use the **local-image** sample instead of the GHCR sample (below).

## Kind / k3d with local images only

Samples under `config/samples/kind/` reference **`localhost:5000/...`** (or a cluster-loaded local tag) — no GHCR/paid registry dependency.

This environment may not have Docker, kind, or k3d; when those tools are available:

```bash
# 1) Local registry + module images (workspace helper)
#    from MuxCore/_mvp:
./local-registry.sh push v0.5.4   # example: muxcored → localhost:5000/muxcore/muxcored:…

# 2) Build and load the operator into the cluster
docker build -t muxcore-operator:v0.1.0 .
kind load docker-image muxcore-operator:v0.1.0   # or: k3d image import muxcore-operator:v0.1.0

# 3) Install CRD/RBAC, then operator with local image
kubectl apply -f config/crd/muxcore.media_muxcoreplatforms.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/samples/kind/manager.yaml

# 4) Platform CR with localhost:5000 module images
kubectl create namespace muxcore
kubectl -n muxcore create secret generic muxcore-auth \
  --from-literal=admin-password='change-me'
kubectl apply -f config/samples/kind/muxcoreplatform.yaml
```

**kind note:** nodes cannot always pull `localhost:5000` from the host. Either mirror/push into a registry the cluster can reach, or `kind load docker-image` / `k3d image import` each `localhost:5000/muxcore/…` tag (and set `imagePullPolicy: IfNotPresent` if you retag without the registry host).

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
- Image publish from a self-hosted runner (GHCR or private registry)
