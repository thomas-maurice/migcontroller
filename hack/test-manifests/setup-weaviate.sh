#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAMESPACE="${NAMESPACE:-default}"

echo "Setting up 3-node Weaviate cluster..."

# Add Weaviate Helm repo
helm repo add weaviate https://weaviate.github.io/weaviate-helm 2>/dev/null || true
helm repo update

# Install Weaviate with 3 replicas
echo "Installing Weaviate with 3 replicas..."
helm upgrade --install weaviate weaviate/weaviate \
  --namespace "${NAMESPACE}" \
  --set replicas=3 \
  --set image.tag=1.25.2 \
  --set storage.size=1Gi \
  --set storage.storageClassName=standard \
  --set resources.requests.cpu=100m \
  --set resources.requests.memory=256Mi \
  --set resources.limits.cpu=500m \
  --set resources.limits.memory=512Mi \
  --set authentication.anonymous_access.enabled=true \
  --set modules.default_vectorizer_module=none

echo "Waiting for all Weaviate pods to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=weaviate -n "${NAMESPACE}" --timeout=180s

echo ""
echo "Weaviate cluster status:"
kubectl get pods -l app.kubernetes.io/name=weaviate -n "${NAMESPACE}"
echo ""
kubectl get pvc -l app.kubernetes.io/name=weaviate -n "${NAMESPACE}"

echo ""
echo "Weaviate cluster is ready!"
echo ""
echo "To populate data, run:"
echo "  ${SCRIPT_DIR}/populate-weaviate.sh"
echo ""
echo "To start health monitoring, run:"
echo "  ${SCRIPT_DIR}/monitor-weaviate.sh"
