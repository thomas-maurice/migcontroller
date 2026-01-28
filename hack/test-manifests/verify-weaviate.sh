#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
EXPECTED_OBJECTS="${EXPECTED_OBJECTS:-100}"
EXPECTED_SIZE="${EXPECTED_SIZE:-500Mi}"

echo "Verifying Weaviate data integrity..."

# Start port-forward (suppress output)
kubectl port-forward svc/weaviate -n "${NAMESPACE}" 8080:80 >/dev/null 2>&1 &
PF_PID=$!
trap 'kill $PF_PID 2>/dev/null || true' EXIT
sleep 3

WEAVIATE_URL="http://localhost:8080"

# Check cluster health with retries (nodes may take time to rejoin after migration)
echo "Checking cluster health (waiting for all nodes to be healthy)..."
MAX_RETRIES=30
for i in $(seq 1 $MAX_RETRIES); do
  NODES_RESPONSE=$(curl -s "${WEAVIATE_URL}/v1/nodes")
  HEALTHY_COUNT=$(echo "$NODES_RESPONSE" | grep -o '"status":"HEALTHY"' | wc -l | tr -d ' ')

  if [ "$HEALTHY_COUNT" -eq 3 ]; then
    echo "  All 3 nodes are HEALTHY"
    break
  fi

  if [ "$i" -eq "$MAX_RETRIES" ]; then
    echo "ERROR: Expected 3 healthy nodes, found ${HEALTHY_COUNT} after ${MAX_RETRIES} retries"
    echo "$NODES_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$NODES_RESPONSE"
    exit 1
  fi

  echo "  Waiting for all nodes to be healthy... (${HEALTHY_COUNT}/3, attempt ${i}/${MAX_RETRIES})"
  sleep 1
done

# Check object count
echo "Checking object count..."
OBJECT_COUNT=$(curl -s -X POST "${WEAVIATE_URL}/v1/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query":"{Aggregate{TestArticle{meta{count}}}}"}' | grep -o '"count":[0-9]*' | cut -d: -f2 || echo "0")

if [ "$OBJECT_COUNT" -ne "$EXPECTED_OBJECTS" ]; then
  echo "ERROR: Expected ${EXPECTED_OBJECTS} objects, found ${OBJECT_COUNT}"
  exit 1
fi
echo "  Found ${OBJECT_COUNT} objects (expected: ${EXPECTED_OBJECTS})"

# Verify sample objects
echo "Verifying sample objects..."
for i in 0 50 99; do
  RESULT=$(curl -s -X POST "${WEAVIATE_URL}/v1/graphql" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{Get{TestArticle(where:{path:[\\\"articleNumber\\\"],operator:Equal,valueInt:${i}}){title articleNumber}}}\"}" 2>/dev/null || echo "{}")
  if echo "$RESULT" | grep -q "Test Article ${i}"; then
    echo "  Article ${i}: OK"
  else
    echo "  Article ${i}: NOT FOUND"
    echo "  Response: $RESULT"
    exit 1
  fi
done

# Check PVC sizes
echo "Checking PVC sizes..."
PVCS=$(kubectl get pvc -n "${NAMESPACE}" -l app.kubernetes.io/name=weaviate -o jsonpath='{range .items[*]}{.metadata.name}={.status.capacity.storage}{"\n"}{end}')
echo "$PVCS"

WRONG_SIZE=0
while IFS= read -r line; do
  if [ -n "$line" ]; then
    size=$(echo "$line" | cut -d= -f2)
    if [ "$size" != "$EXPECTED_SIZE" ]; then
      echo "  ERROR: PVC has size ${size}, expected ${EXPECTED_SIZE}"
      WRONG_SIZE=1
    fi
  fi
done <<< "$PVCS"

if [ "$WRONG_SIZE" -eq 1 ]; then
  exit 1
fi
echo "  All PVCs are ${EXPECTED_SIZE}"

echo ""
echo "=== Data integrity verification PASSED ==="
