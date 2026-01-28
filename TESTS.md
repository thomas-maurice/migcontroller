# Testing Guide

**Watch your PVCs shrink in real-time with a 3-node Weaviate cluster.**

This guide walks you through an end-to-end test: deploy a distributed database, fill it with data, shrink the volumes, verify nothing was lost.

---

## TL;DR

```bash
make test-weaviate
```

Creates a Kind cluster, deploys everything, migrates 3 replicas from 1Gi to 500Mi, verifies all 100 test objects survive. Cleanup with `make cleanup-test-weaviate`.

---

## Prerequisites

| Tool | Purpose |
|------|---------|
| Docker | Container runtime |
| Kind | Local Kubernetes cluster |
| kubectl | Cluster management |
| curl | API verification |

---

## Manual Testing (Step by Step)

### 1. Create a Kind Cluster

```bash
kind create cluster --name volresize-test --config - <<'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
EOF
```

### 2. Build and Load Images

```bash
# Build controller (defaults to mauricethomas/migcontroller:latest)
make docker-build

# Build migrator (the rclone container)
docker build -t mauricethomas/migcontroller-migrator:latest -f build/migrator/Dockerfile .

# Load into Kind
kind load docker-image mauricethomas/migcontroller:latest --name volresize-test
kind load docker-image mauricethomas/migcontroller-migrator:latest --name volresize-test
```

### 3. Deploy the Operator

```bash
make install
make deploy  # Uses mauricethomas/migcontroller:latest by default

# Wait for it
kubectl rollout status deployment/volume-resize-operator-controller-manager \
  -n volume-resize-operator-system --timeout=60s
```

### 4. Deploy Weaviate

```bash
./hack/test-manifests/setup-weaviate.sh
```

This creates a 3-node vector database with 1Gi volumes each.

### 5. Fill It With Data

```bash
./hack/test-manifests/populate-weaviate.sh
```

Creates 100 test articles replicated across all 3 nodes.

### 6. Verify Starting State

```bash
kubectl get pvc
# NAME                       CAPACITY
# weaviate-data-weaviate-0   1Gi
# weaviate-data-weaviate-1   1Gi
# weaviate-data-weaviate-2   1Gi
```

### 7. Trigger the Migration

```bash
kubectl apply -f hack/test-manifests/volumeresize-weaviate.yaml

# Watch it happen
kubectl get volumeresize resize-weaviate -w
```

### 8. Verify Success

```bash
kubectl get pvc
# NAME                       CAPACITY
# weaviate-data-weaviate-0   500Mi  <-- Shrunk!
# weaviate-data-weaviate-1   500Mi
# weaviate-data-weaviate-2   500Mi

./hack/test-manifests/verify-weaviate.sh
# All 3 nodes HEALTHY
# 100 objects intact
```

---

## How the Migration Works

The magic happens in 4 phases:

### Phase 1: Preparation

```
1. Validate the VolumeResize spec
2. Verify StatefulSet and volumes exist
3. Backup StatefulSet spec to ConfigMap (for rollback)
4. Delete StatefulSet with orphan policy
   └── Pods keep running, just unmanaged
```

### Phase 2: Per-Replica Migration

For each replica (0, 1, 2...), sequentially:

```
┌─────────────────────────────────────────────────────────────────┐
│  1. Delete the pod (it's orphaned, won't respawn)               │
│  2. Set PV reclaim policy to Retain (safety net)                │
│  3. Create new PVC with target size                             │
│  4. Spin up migrator pod with rclone                            │
│  5. Wait for data sync to complete                              │
│  6. Swap PVCs (delete old, rename new)                          │
│  7. Recreate StatefulSet                                        │
│  8. Wait for pod ready                                          │
│  9. Next replica...                                             │
└─────────────────────────────────────────────────────────────────┘
```

### Phase 3: Completion

All replicas migrated. StatefulSet running. New volumes bound. Done.

### Visual Timeline

```
Replica 0:  [Kill] → [Migrate] → [Swap PVC] → [Ready] ─────────────────────
Replica 1:                                            [Kill] → [Migrate] → [Swap PVC] → [Ready] ─────
Replica 2:                                                                              [Kill] → ...

StatefulSet: [Backup & Orphan] ────── [Recreate] ────────────── [Recreate] ────── [Recreate] ───
```

Sequential migration keeps quorum alive for distributed databases.

---

## The Migrator Pod

The migrator is the workhorse that actually moves data between volumes.

### What It Does

```
┌──────────────────────────────────────────────────────────┐
│  Migrator Pod                                            │
│  ┌─────────────┐        rclone sync        ┌──────────┐  │
│  │ Old PVC     │ ──────────────────────────▶│ New PVC  │  │
│  │ /source     │        (preserves         │ /dest    │  │
│  │ (1Gi)       │        all attrs)         │ (500Mi)  │  │
│  └─────────────┘                           └──────────┘  │
└──────────────────────────────────────────────────────────┘
```

### The Image

Built from `build/migrator/Dockerfile` and published as `mauricethomas/migcontroller-migrator:latest`:

```dockerfile
FROM alpine:3.19
RUN apk add --no-cache rclone bash coreutils
COPY build/migrator/migrate.sh /usr/local/bin/migrate.sh
ENTRYPOINT ["/usr/local/bin/migrate.sh"]
```

The script runs `rclone sync` with these flags:
- `sync` - One-way sync, destination matches source exactly
- `/source` - Mount point for old (larger) PVC
- `/dest` - Mount point for new (smaller) PVC
- `-v` - Verbose logging (visible in pod logs)
- `--checksum` - Verify integrity with checksums, not just timestamps

### Why rclone?

| Feature | Why it matters |
|---------|----------------|
| Preserves permissions | Files keep their ownership and modes |
| Handles any filesystem | Works with ext4, xfs, whatever |
| Checksum verification | Data integrity guaranteed |
| Incremental sync | Fast retries if interrupted |
| Battle-tested | Used in production for years |

### Security

The migrator runs as root (`runAsUser: 0`) to read/write files with any ownership. It has no network access - just mounts two volumes and copies.

---

## Rollback Capability

Things go wrong. The operator is ready:

| What's preserved | Where | Purpose |
|------------------|-------|---------|
| Original PV | Cluster (Retain policy) | Data recovery |
| StatefulSet spec | ConfigMap `{name}-sts-backup` | Manual recreation |

**To recover manually:**

```bash
# Find the backup
kubectl get configmap -l storage.maurice.fr/migration-name=resize-weaviate

# Check original PVs
kubectl get pv | grep Retain

# Recreate PVCs pointing to original PVs, restore StatefulSet from ConfigMap
```

---

## Troubleshooting

### Operator Logs

```bash
kubectl logs -n volume-resize-operator-system \
  deployment/volume-resize-operator-controller-manager -f
```

### Migrator Logs

```bash
kubectl logs -l storage.maurice.fr/migration-name=resize-weaviate
```

### VolumeResize Details

```bash
kubectl describe volumeresize resize-weaviate
```

### Check Backup

```bash
kubectl get configmap -l storage.maurice.fr/migration-name=resize-weaviate -o yaml
```

---

## Cleanup

```bash
kubectl delete volumeresize resize-weaviate
kubectl delete -f hack/test-manifests/weaviate-helm-values.yaml  # if using helm
kubectl delete pvc -l app.kubernetes.io/name=weaviate
kind delete cluster --name volresize-test
```

Or just:

```bash
make cleanup-test-weaviate
```
