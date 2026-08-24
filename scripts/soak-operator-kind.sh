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

echo "==> apply CRD/RBAC/operator (kind sample)"
kubectl apply -f config/crd/muxcore.media_muxcoreplatforms.yaml
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/samples/kind/manager.yaml
kubectl create namespace muxcore --dry-run=client -o yaml | kubectl apply -f -
kubectl -n muxcore create secret generic muxcore-auth \
  --from-literal=admin-password='soak-test' --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/kind/muxcoreplatform.yaml

echo "==> wait for operator deployment"
kubectl -n muxcore-system rollout status deployment/muxcore-operator --timeout=120s

echo "==> wait for platform Ready condition"
for _ in $(seq 1 60); do
  if kubectl -n muxcore get muxcoreplatform muxcore -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q True; then
    echo "soak PASS: MuxCorePlatform Ready"
    exit 0
  fi
  sleep 2
done
kubectl -n muxcore describe muxcoreplatform muxcore || true
die "timeout waiting for Ready"
