#!/usr/bin/env bash
set -euo pipefail

STS_NAME="${1:-test-sts}"
NAMESPACE="${2:-default}"

echo "Populating test data for StatefulSet: ${STS_NAME} in namespace: ${NAMESPACE}"

# Get the number of replicas
REPLICAS=$(kubectl get sts "${STS_NAME}" -n "${NAMESPACE}" -o jsonpath='{.spec.replicas}')

echo "Found ${REPLICAS} replicas"

for ((i=0; i<REPLICAS; i++)); do
    POD_NAME="${STS_NAME}-${i}"
    echo ""
    echo "Populating data for pod: ${POD_NAME}"

    # Wait for pod to be ready
    echo "  Waiting for pod to be ready..."
    kubectl wait --for=condition=ready pod/"${POD_NAME}" -n "${NAMESPACE}" --timeout=120s

    # Create marker files with unique content per replica
    echo "  Creating marker files..."
    kubectl exec "${POD_NAME}" -n "${NAMESPACE}" -- sh -c "echo 'data-marker-replica-${i}-$(date +%s)' > /data/marker.txt"
    kubectl exec "${POD_NAME}" -n "${NAMESPACE}" -- sh -c "echo 'logs-marker-replica-${i}-$(date +%s)' > /logs/marker.txt"

    # Create some random data
    echo "  Creating random data files..."
    kubectl exec "${POD_NAME}" -n "${NAMESPACE}" -- sh -c "dd if=/dev/urandom of=/data/random.bin bs=1024 count=100 2>/dev/null"
    kubectl exec "${POD_NAME}" -n "${NAMESPACE}" -- sh -c "dd if=/dev/urandom of=/logs/random.bin bs=1024 count=50 2>/dev/null"

    # Create a directory structure
    echo "  Creating directory structure..."
    kubectl exec "${POD_NAME}" -n "${NAMESPACE}" -- sh -c "mkdir -p /data/subdir && echo 'nested-file-${i}' > /data/subdir/nested.txt"

    echo "  Done with ${POD_NAME}"
done

echo ""
echo "Data population complete!"
echo ""
echo "Verify with:"
echo "  kubectl exec ${STS_NAME}-0 -n ${NAMESPACE} -- cat /data/marker.txt"
echo "  kubectl exec ${STS_NAME}-0 -n ${NAMESPACE} -- cat /logs/marker.txt"
