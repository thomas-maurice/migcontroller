#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="volresize-test"
CONFIG_FILE="${SCRIPT_DIR}/kind-config.yaml"

echo "Setting up Kind cluster: ${CLUSTER_NAME}"

# Check if cluster already exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster '${CLUSTER_NAME}' already exists"
else
    echo "Creating cluster '${CLUSTER_NAME}'..."
    kind create cluster --config "${CONFIG_FILE}"
fi

# Set kubectl context
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

echo ""
echo "Cluster nodes:"
kubectl get nodes

echo ""
echo "Cluster is ready!"
echo "Context: kind-${CLUSTER_NAME}"
