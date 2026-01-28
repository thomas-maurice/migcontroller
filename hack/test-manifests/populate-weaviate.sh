#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
WEAVIATE_URL="${WEAVIATE_URL:-}"
NUM_OBJECTS="${NUM_OBJECTS:-100}"

# Port-forward if no URL provided
if [ -z "$WEAVIATE_URL" ]; then
  echo "Starting port-forward to Weaviate..."
  kubectl port-forward svc/weaviate -n "${NAMESPACE}" 8080:80 >/dev/null 2>&1 &
  PF_PID=$!
  trap 'kill $PF_PID 2>/dev/null || true' EXIT
  sleep 3
  WEAVIATE_URL="http://localhost:8080"
fi

echo "Weaviate URL: ${WEAVIATE_URL}"

# Wait for Weaviate to be ready
echo "Waiting for Weaviate to be ready..."
for i in $(seq 1 30); do
  # Weaviate returns empty body with 200 OK when ready
  HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${WEAVIATE_URL}/v1/.well-known/ready" 2>/dev/null || echo "000")
  if [ "$HTTP_CODE" = "200" ]; then
    echo "Weaviate is ready!"
    break
  fi
  echo "  Waiting... ($i/30) - HTTP $HTTP_CODE"
  sleep 1
done

# Check cluster status
echo ""
echo "Checking cluster nodes..."
curl -s "${WEAVIATE_URL}/v1/nodes" | python3 -m json.tool 2>/dev/null || curl -s "${WEAVIATE_URL}/v1/nodes"
echo ""

# Delete existing schema if present
echo "Deleting existing TestArticle schema (if exists)..."
curl -s -X DELETE "${WEAVIATE_URL}/v1/schema/TestArticle" 2>/dev/null || true
sleep 1

# Create a test schema
echo "Creating test schema..."
SCHEMA_RESULT=$(curl -s -X POST "${WEAVIATE_URL}/v1/schema" \
  -H "Content-Type: application/json" \
  -d '{
    "class": "TestArticle",
    "description": "Test articles for migration testing",
    "properties": [
      {
        "name": "title",
        "dataType": ["text"],
        "description": "Article title"
      },
      {
        "name": "content",
        "dataType": ["text"],
        "description": "Article content"
      },
      {
        "name": "articleNumber",
        "dataType": ["int"],
        "description": "Article number for verification"
      }
    ],
    "replicationConfig": {
      "factor": 3
    }
  }')
echo "Schema result: $SCHEMA_RESULT"

echo ""
echo "Populating ${NUM_OBJECTS} test objects using batch API..."

# Build batch payload
BATCH_OBJECTS=""
for i in $(seq 0 $((NUM_OBJECTS - 1))); do
  if [ -n "$BATCH_OBJECTS" ]; then
    BATCH_OBJECTS="${BATCH_OBJECTS},"
  fi
  BATCH_OBJECTS="${BATCH_OBJECTS}{\"class\":\"TestArticle\",\"properties\":{\"title\":\"Test Article ${i}\",\"content\":\"This is test content for article number ${i}. Created for volume resize testing.\",\"articleNumber\":${i}}}"
done

# Single batch import
curl -s -X POST "${WEAVIATE_URL}/v1/batch/objects" \
  -H "Content-Type: application/json" \
  -d "{\"objects\":[${BATCH_OBJECTS}]}" > /dev/null

echo "Batch import complete."
echo ""
echo "Verifying data..."
sleep 1

# Use GraphQL aggregate to get count
OBJECT_COUNT=$(curl -s -X POST "${WEAVIATE_URL}/v1/graphql" \
  -H "Content-Type: application/json" \
  -d '{"query":"{Aggregate{TestArticle{meta{count}}}}"}' | grep -o '"count":[0-9]*' | cut -d: -f2 || echo "0")
echo "Total objects in TestArticle class: ${OBJECT_COUNT}"

# Verify a few specific objects using GraphQL
echo ""
echo "Verifying sample objects..."
for i in 0 50 99; do
  RESULT=$(curl -s -X POST "${WEAVIATE_URL}/v1/graphql" \
    -H "Content-Type: application/json" \
    -d "{\"query\":\"{Get{TestArticle(where:{path:[\\\"articleNumber\\\"],operator:Equal,valueInt:${i}}){title articleNumber}}}\"}" 2>/dev/null || echo "{}")
  if echo "$RESULT" | grep -q "Test Article ${i}"; then
    echo "  Article ${i}: OK"
  else
    echo "  Article ${i}: NOT FOUND - $RESULT"
  fi
done

echo ""
echo "Data population complete!"
