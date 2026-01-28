#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CLUSTER_NAME="volresize-test"
IMG="${IMG:-controller:latest}"

cd "${PROJECT_ROOT}"

echo "Building operator image..."
CC=/usr/bin/cc make docker-build IMG="${IMG}"

echo ""
echo "Loading image into Kind cluster..."
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    kind load docker-image "${IMG}" --name "${CLUSTER_NAME}"
else
    echo "Warning: Kind cluster '${CLUSTER_NAME}' not found"
fi

echo ""
echo "Installing CRDs..."
CC=/usr/bin/cc make install

echo ""
echo "Deploying operator..."
CC=/usr/bin/cc make deploy IMG="${IMG}"

echo ""
echo "Waiting for deployment..."
kubectl wait --for=condition=available deployment/migcontroller-controller-manager -n migcontroller-system --timeout=120s || true

echo ""
echo "Deployment status:"
kubectl get pods -n migcontroller-system

echo ""
echo "Done!"
