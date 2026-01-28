#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUTPUT_DIR="${PROJECT_ROOT}/dist"
OUTPUT_FILE="${OUTPUT_DIR}/install.yaml"
IMG="${IMG:-ghcr.io/thomas-maurice/migcontroller:latest}"

cd "${PROJECT_ROOT}"

echo "Generating release manifest..."

# Create output directory
mkdir -p "${OUTPUT_DIR}"

# Generate manifests
CC=/usr/bin/cc make manifests

# Build the combined manifest
echo "# VolumeResize Operator Installation Manifest" > "${OUTPUT_FILE}"
echo "# Generated: $(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "${OUTPUT_FILE}"
echo "# Image: ${IMG}" >> "${OUTPUT_FILE}"
echo "" >> "${OUTPUT_FILE}"

# Add namespace
cat >> "${OUTPUT_FILE}" << 'EOF'
---
apiVersion: v1
kind: Namespace
metadata:
  name: migcontroller-system
  labels:
    control-plane: controller-manager
EOF

echo "" >> "${OUTPUT_FILE}"

# Add CRDs
echo "# Custom Resource Definitions" >> "${OUTPUT_FILE}"
cat "${PROJECT_ROOT}/config/crd/bases/"*.yaml >> "${OUTPUT_FILE}"

echo "" >> "${OUTPUT_FILE}"

# Add RBAC
echo "# RBAC Configuration" >> "${OUTPUT_FILE}"

# Service Account
cat >> "${OUTPUT_FILE}" << 'EOF'
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: migcontroller-controller-manager
  namespace: migcontroller-system
EOF

# Add role and role binding
cat "${PROJECT_ROOT}/config/rbac/role.yaml" >> "${OUTPUT_FILE}"

cat >> "${OUTPUT_FILE}" << 'EOF'
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: migcontroller-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: manager-role
subjects:
  - kind: ServiceAccount
    name: migcontroller-controller-manager
    namespace: migcontroller-system
EOF

echo "" >> "${OUTPUT_FILE}"

# Add Deployment
echo "# Controller Deployment" >> "${OUTPUT_FILE}"
cat >> "${OUTPUT_FILE}" << EOF
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: migcontroller-controller-manager
  namespace: migcontroller-system
  labels:
    control-plane: controller-manager
spec:
  replicas: 1
  selector:
    matchLabels:
      control-plane: controller-manager
  template:
    metadata:
      labels:
        control-plane: controller-manager
    spec:
      serviceAccountName: migcontroller-controller-manager
      containers:
        - name: manager
          image: ${IMG}
          command:
            - /manager
          args:
            - --leader-elect
          resources:
            limits:
              cpu: 500m
              memory: 128Mi
            requests:
              cpu: 10m
              memory: 64Mi
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
      securityContext:
        runAsNonRoot: true
      terminationGracePeriodSeconds: 10
EOF

echo ""
echo "Release manifest generated: ${OUTPUT_FILE}"
echo ""
echo "To install:"
echo "  kubectl apply -f ${OUTPUT_FILE}"
