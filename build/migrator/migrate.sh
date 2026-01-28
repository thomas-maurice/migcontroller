#!/usr/bin/env bash
set -euo pipefail

# Migration script for volume data sync using rclone
# Expected environment variables:
#   SOURCE_PATH - Path to source volume (e.g., /source)
#   DEST_PATH   - Path to destination volume (e.g., /dest)

SOURCE_PATH="${SOURCE_PATH:-/source}"
DEST_PATH="${DEST_PATH:-/dest}"

echo "Starting volume migration"
echo "  Source: ${SOURCE_PATH}"
echo "  Destination: ${DEST_PATH}"
echo ""

# Verify source exists
if [ ! -d "${SOURCE_PATH}" ]; then
    echo "ERROR: Source path does not exist: ${SOURCE_PATH}"
    exit 1
fi

# Verify destination exists
if [ ! -d "${DEST_PATH}" ]; then
    echo "ERROR: Destination path does not exist: ${DEST_PATH}"
    exit 1
fi

echo "Running rclone sync..."
echo ""

# Run rclone sync with progress output
# --progress: Show progress during transfer
# --transfers: Number of file transfers to run in parallel
# --checkers: Number of checkers to run in parallel
rclone sync \
    "${SOURCE_PATH}/" \
    "${DEST_PATH}/" \
    --progress \
    --transfers 4 \
    --checkers 8 \
    --verbose

RCLONE_EXIT=$?

echo ""
if [ ${RCLONE_EXIT} -eq 0 ]; then
    echo "Migration completed successfully"
else
    echo "Migration failed with exit code: ${RCLONE_EXIT}"
fi

exit ${RCLONE_EXIT}
