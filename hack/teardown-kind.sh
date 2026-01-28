#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="volresize-test"

echo "Deleting Kind cluster: ${CLUSTER_NAME}"

if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    kind delete cluster --name "${CLUSTER_NAME}"
    echo "Cluster '${CLUSTER_NAME}' deleted"
else
    echo "Cluster '${CLUSTER_NAME}' does not exist"
fi
