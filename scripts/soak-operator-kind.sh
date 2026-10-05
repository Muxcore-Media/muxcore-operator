#!/usr/bin/env bash
# Controller unit tests + optional kind reconcile smoke (no GHCR).
#
# Usage:
#   ./scripts/soak-operator-kind.sh              # tests only
#   ./scripts/soak-operator-kind.sh v0.1.0       # tests + kind when docker+kind available
set -euo pipefail

TAG="${1:-v0.1.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

die() { echo "FAIL: $*" >&2; exit 1; }

echo "==> go test ./..."
go test -count=1 -timeout 10m ./...

if ! command -v docker >/dev/null 2>&1 && ! command -v podman >/dev/null 2>&1; then
  echo "skip kind soak: no container runtime"
  exit 0
fi
if ! command -v kind >/dev/null 2>&1; then
  echo "skip kind soak: kind not installed (install kind for cluster reconcile smoke)"
  exit 0
fi

CLUSTER="${MUXCORE_KIND_CLUSTER:-muxcore-operator-soak}"
RUNTIME="${CONTAINER_RUNTIME:-}"
if [[ -z "$RUNTIME" ]]; then
  if command -v podman >/dev/null 2>&1; then RUNTIME=podman; else RUNTIME=docker; fi
fi

echo "==> kind cluster $CLUSTER"
if ! kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  kind create cluster --name "$CLUSTER"
fi

echo "==> build operator image"
BUILD_ONLY=1 CONTAINER_RUNTIME="$RUNTIME" ./scripts/publish-operator-local.sh "$TAG"
"$RUNTIME" tag "localhost/muxcore-operator:${TAG}" "muxcore-operator:${TAG}"
kind load docker-image "muxcore-operator:${TAG}" --name "$CLUSTER"

CORE_TAG="${MUXCORE_CORE_TAG:-v0.6.7}"
CORE_LOCAL="localhost/muxcore/muxcored:${CORE_TAG}"
CORE_KIND="muxcored:${CORE_TAG}"
if "$RUNTIME" image inspect "$CORE_LOCAL" >/dev/null 2>&1; then
  echo "==> load muxcored image from $CORE_LOCAL"
  "$RUNTIME" tag "$CORE_LOCAL" "$CORE_KIND"
  kind load docker-image "$CORE_KIND" --name "$CLUSTER"
elif "$RUNTIME" image inspect "$CORE_KIND" >/dev/null 2>&1; then
  echo "==> load muxcored image $CORE_KIND"
  kind load docker-image "$CORE_KIND" --name "$CLUSTER"
else
  echo "skip muxcored preload: build/push $CORE_LOCAL or tag $CORE_KIND (soak uses muxcored-only CR)"
fi

echo "==> apply CRD/RBAC/operator (kind sample)"
kubectl apply -f config/crd/muxcore.media_muxcoreplatforms.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/samples/kind/manager.yaml
kubectl create namespace muxcore --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/kind/muxcoreplatform.yaml

echo "==> wait for operator deployment"
kubectl -n muxcore-system rollout status deployment/muxcore-operator --timeout=120s

echo "==> wait for platform Ready condition (minimal-local, muxcored-only)"
for _ in $(seq 1 60); do
  if kubectl -n muxcore get muxcoreplatform minimal-local -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; then
    echo "soak PASS: MuxCorePlatform minimal-local Ready"
    exit 0
  fi
  sleep 2
done
kubectl -n muxcore describe muxcoreplatform minimal-local || true
kubectl -n muxcore get deploy,pods || true
die "timeout waiting for Ready (preload muxcored:${CORE_TAG} for muxcored-only soak)"
