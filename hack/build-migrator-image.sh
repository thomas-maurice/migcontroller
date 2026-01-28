#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
IMAGE_NAME="mauricethomas/volmig"
IMAGE_TAG="${TAG:-latest}"
FULL_IMAGE="${IMAGE_NAME}:${IMAGE_TAG}"
CLUSTER_NAME="volresize-test"

echo "Building migrator image: ${FULL_IMAGE}"

# Build the image
docker build -t "${FULL_IMAGE}" "${PROJECT_ROOT}/build/migrator"

echo ""
echo "Image built successfully: ${FULL_IMAGE}"

# Push if requested
if [ "${PUSH:-false}" = "true" ]; then
    echo ""
    echo "Pushing image to registry..."
    docker push "${FULL_IMAGE}"
    echo "Image pushed successfully"
fi

# Load into kind cluster if it exists
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo ""
    echo "Loading image into Kind cluster: ${CLUSTER_NAME}"
    kind load docker-image "${FULL_IMAGE}" --name "${CLUSTER_NAME}"
    echo "Image loaded into Kind cluster"
else
    echo ""
    echo "Kind cluster '${CLUSTER_NAME}' not found, skipping image load"
fi

echo ""
echo "Done!"
