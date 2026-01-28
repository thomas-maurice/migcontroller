#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
WEAVIATE_URL="${WEAVIATE_URL:-}"
CHECK_INTERVAL="${CHECK_INTERVAL:-5}"
EXPECTED_OBJECTS="${EXPECTED_OBJECTS:-100}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Port-forward if no URL provided
if [ -z "$WEAVIATE_URL" ]; then
  echo "Starting port-forward to Weaviate..."
  kubectl port-forward svc/weaviate -n "${NAMESPACE}" 8080:80 &
  PF_PID=$!
  trap "kill $PF_PID 2>/dev/null || true" EXIT
  sleep 3
  WEAVIATE_URL="http://localhost:8080"
fi

echo "Weaviate Cluster Health Monitor"
echo "================================"
echo "URL: ${WEAVIATE_URL}"
echo "Check interval: ${CHECK_INTERVAL}s"
echo "Expected objects: ${EXPECTED_OBJECTS}"
echo ""
echo "Press Ctrl+C to stop monitoring"
echo ""

check_count=0
errors=0
last_healthy_nodes=0
last_object_count=0

while true; do
  check_count=$((check_count + 1))
  timestamp=$(date '+%Y-%m-%d %H:%M:%S')

  # Check readiness
  ready=$(curl -s --max-time 5 "${WEAVIATE_URL}/v1/.well-known/ready" 2>/dev/null || echo "false")
  if echo "$ready" | grep -q "true"; then
    ready_status="${GREEN}READY${NC}"
  else
    ready_status="${RED}NOT READY${NC}"
    errors=$((errors + 1))
  fi

  # Check cluster nodes
  nodes_json=$(curl -s --max-time 5 "${WEAVIATE_URL}/v1/nodes" 2>/dev/null || echo '{"nodes":[]}')
  healthy_nodes=$(echo "$nodes_json" | grep -o '"status":"HEALTHY"' | wc -l | tr -d ' ')

  if [ "$healthy_nodes" -eq 3 ]; then
    nodes_status="${GREEN}${healthy_nodes}/3${NC}"
  elif [ "$healthy_nodes" -gt 0 ]; then
    nodes_status="${YELLOW}${healthy_nodes}/3${NC}"
  else
    nodes_status="${RED}${healthy_nodes}/3${NC}"
    errors=$((errors + 1))
  fi

  # Track node changes
  if [ "$healthy_nodes" -ne "$last_healthy_nodes" ] && [ "$last_healthy_nodes" -ne 0 ]; then
    echo -e "${YELLOW}[${timestamp}] Node count changed: ${last_healthy_nodes} -> ${healthy_nodes}${NC}"
  fi
  last_healthy_nodes=$healthy_nodes

  # Check object count
  object_count=$(curl -s --max-time 5 "${WEAVIATE_URL}/v1/objects?class=TestArticle&limit=1" 2>/dev/null | grep -o '"totalResults":[0-9]*' | cut -d: -f2 || echo "0")

  if [ "$object_count" -ge "$EXPECTED_OBJECTS" ]; then
    objects_status="${GREEN}${object_count}/${EXPECTED_OBJECTS}${NC}"
  elif [ "$object_count" -gt 0 ]; then
    objects_status="${YELLOW}${object_count}/${EXPECTED_OBJECTS}${NC}"
  else
    objects_status="${RED}${object_count}/${EXPECTED_OBJECTS}${NC}"
    errors=$((errors + 1))
  fi

  # Track object count changes
  if [ "$object_count" -ne "$last_object_count" ] && [ "$last_object_count" -ne 0 ]; then
    echo -e "${YELLOW}[${timestamp}] Object count changed: ${last_object_count} -> ${object_count}${NC}"
  fi
  last_object_count=$object_count

  # Verify a random object
  random_id=$((RANDOM % EXPECTED_OBJECTS))
  verify_result=$(curl -s --max-time 5 "${WEAVIATE_URL}/v1/objects?class=TestArticle&limit=1&where={\"path\":[\"articleNumber\"],\"operator\":\"Equal\",\"valueInt\":${random_id}}" 2>/dev/null || echo "{}")
  if echo "$verify_result" | grep -q "Test Article ${random_id}"; then
    verify_status="${GREEN}OK (article ${random_id})${NC}"
  else
    verify_status="${RED}FAILED (article ${random_id})${NC}"
    errors=$((errors + 1))
  fi

  # Print status line
  printf "\r[%s] #%d | Ready: %b | Nodes: %b | Objects: %b | Verify: %b | Errors: %d    " \
    "$timestamp" "$check_count" "$ready_status" "$nodes_status" "$objects_status" "$verify_status" "$errors"

  sleep "$CHECK_INTERVAL"
done
