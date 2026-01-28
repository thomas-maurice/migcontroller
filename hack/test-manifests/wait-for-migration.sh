#!/usr/bin/env bash
set -euo pipefail

MIGRATION_NAME="${1:-resize-weaviate}"
TIMEOUT="${TIMEOUT:-600}"  # 10 minutes default

echo "Waiting for VolumeResize '${MIGRATION_NAME}' to complete (timeout: ${TIMEOUT}s)..."

start_time=$(date +%s)
while true; do
  current_time=$(date +%s)
  elapsed=$((current_time - start_time))

  if [ $elapsed -ge $TIMEOUT ]; then
    echo "ERROR: Timeout waiting for migration to complete"
    kubectl get volumeresizes.storage.maurice.fr "${MIGRATION_NAME}" -o yaml
    exit 1
  fi

  phase=$(kubectl get volumeresizes.storage.maurice.fr "${MIGRATION_NAME}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "")
  message=$(kubectl get volumeresizes.storage.maurice.fr "${MIGRATION_NAME}" -o jsonpath='{.status.message}' 2>/dev/null || echo "")
  replica=$(kubectl get volumeresizes.storage.maurice.fr "${MIGRATION_NAME}" -o jsonpath='{.status.currentReplica}' 2>/dev/null || echo "")

  echo "  [${elapsed}s] Phase: ${phase}, Replica: ${replica}, Message: ${message}"

  if [ "$phase" = "Completed" ]; then
    echo "Migration completed successfully!"
    exit 0
  fi

  if [ "$phase" = "Failed" ]; then
    echo "ERROR: Migration failed: ${message}"
    kubectl describe volumeresizes.storage.maurice.fr "${MIGRATION_NAME}"
    exit 1
  fi

  sleep 5
done
