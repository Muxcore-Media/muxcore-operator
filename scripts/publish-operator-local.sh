#!/usr/bin/env bash
# Build muxcore-operator and tag/push to a LAN OCI registry or ghcr.io/muxcore-media.
#
# Usage:
#   ./scripts/publish-operator-local.sh
#   ./scripts/publish-operator-local.sh v0.1.0
#   BUILD_ONLY=1 ./scripts/publish-operator-local.sh v0.1.0
#   MUXCORE_REGISTRY=ghcr.io/muxcore-media ./scripts/publish-operator-local.sh v0.1.0
#
# Kind/k3d soak (after publish or BUILD_ONLY):
#   ./scripts/soak-operator-kind.sh v0.1.0
set -euo pipefail

TAG="${1:-v0.1.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUILD_ONLY="${BUILD_ONLY:-0}"
REGISTRY="${MUXCORE_REGISTRY:-localhost:5000/muxcore}"

die() { echo "FAIL: $*" >&2; exit 1; }

detect_runtime() {
  if [[ -n "${CONTAINER_RUNTIME:-}" ]]; then
    command -v "$CONTAINER_RUNTIME" >/dev/null 2>&1 || die "CONTAINER_RUNTIME=${CONTAINER_RUNTIME} not found"
    printf '%s\n' "$CONTAINER_RUNTIME"
    return 0
  fi
  if command -v podman >/dev/null 2>&1; then printf '%s\n' podman; return 0; fi
  if command -v docker >/dev/null 2>&1; then printf '%s\n' docker; return 0; fi
  return 1
}

if [[ ! -f "$ROOT/Dockerfile" ]]; then
  die "missing Dockerfile at $ROOT/Dockerfile"
fi

if ! RUNTIME="$(detect_runtime)"; then
  die "neither podman nor docker on PATH"
fi

LOCAL="localhost/muxcore-operator:${TAG}"
REMOTE="${REGISTRY}/muxcore-operator:${TAG}"

echo "runtime=$RUNTIME registry=$REGISTRY tag=$TAG"
echo "building $LOCAL"
"$RUNTIME" build -t "$LOCAL" -f "$ROOT/Dockerfile" "$ROOT"
"$RUNTIME" tag "$LOCAL" "$REMOTE"
"$RUNTIME" tag "$LOCAL" "localhost/muxcore-operator:latest" 2>/dev/null || true

if [[ "$BUILD_ONLY" == "1" ]]; then
  echo "BUILD_ONLY=1 — skip push; images ready:"
  echo "  $LOCAL"
  echo "  $REMOTE"
  exit 0
fi

configure_insecure_local_registry() {
  if [[ "$REGISTRY" != localhost:* && "$REGISTRY" != 127.0.0.1:* ]]; then
    return 0
  fi
  mkdir -p "${HOME}/.config/containers"
  if ! grep -qF "location = \"${REGISTRY%%/*}\"" "${HOME}/.config/containers/registries.conf" 2>/dev/null; then
    cat >>"${HOME}/.config/containers/registries.conf" <<EOF

[[registry]]
location = "${REGISTRY%%/*}"
insecure = true
EOF
  fi
}

configure_insecure_local_registry

echo "pushing $REMOTE"
push_args=()
case "$RUNTIME" in
  podman)
    if [[ "$REGISTRY" == localhost:* || "$REGISTRY" == 127.0.0.1:* ]]; then
      push_args=(--tls-verify=false)
    fi
    ;;
  docker)
  if [[ "$REGISTRY" == localhost:* || "$REGISTRY" == 127.0.0.1:* ]]; then
      push_args=(--tls-verify=false)
    fi
    ;;
esac
if ! "$RUNTIME" push "${push_args[@]}" "$REMOTE"; then
  cat >&2 <<ERR
Push failed for $REMOTE.
GHCR: echo \$TOKEN | $RUNTIME login ghcr.io -u <user> --password-stdin
LAN: MUXCORE_REGISTRY=localhost:5000/muxcore BUILD_ONLY=0 $0 ${TAG}
ERR
  exit 1
fi
echo "published $REMOTE"
