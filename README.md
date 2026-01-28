# VolumeResize Operator

**Shrink your Kubernetes StatefulSet volumes without downtime.**

Kubernetes doesn't let you reduce PVC sizes. This operator does it anyway - safely, with rolling migrations and automatic rollback capability.

---

## The Problem

You provisioned 100Gi for your database. Turns out you only need 10Gi. Kubernetes says "tough luck" - PVCs can only grow, never shrink. You're stuck paying for storage you don't need.

## The Solution

```bash
volmig create shrink-db --statefulset postgres --volume data --size 10Gi --watch
```

That's it. The operator handles everything:
- Rolling migration (one replica at a time)
- Zero data loss (rclone sync with verification)
- Automatic rollback on failure
- Works with any StatefulSet

---

## Quick Start

### 1. Install

```bash
# Install CRDs
make install

# Deploy operator (uses mauricethomas/migcontroller:latest by default)
make deploy
```

### 2. Install CLI

```bash
make build-cli
# Binary at ./bin/volmig
```

### 3. Resize Something

```bash
# Create a resize operation
volmig create my-resize \
  --statefulset my-app \
  --volume data \
  --size 500Mi

# Watch it happen
volmig status my-resize --follow
```

---

## Features

| Feature | Description |
|---------|-------------|
| **Rolling Migration** | One replica at a time - your app stays up |
| **Data Safety** | Old PVs retained with `Retain` policy for rollback |
| **Any Storage Class** | Change storage class during resize |
| **CLI + CRD** | Use `volmig` CLI or apply YAML directly |
| **Real-time Status** | Watch progress with `volmig watch` |
| **Idempotent** | Safe to retry, handles interruptions |

---

## CLI Reference

### Create a Resize

```bash
volmig create <name> \
  --statefulset <sts-name> \
  --volume <volume-name> \
  --size <new-size> \
  [--storage-class <sc>] \
  [--watch]
```

### Monitor Progress

```bash
volmig list                    # List all resizes
volmig list -A                 # All namespaces
volmig list -o wide            # Extended info

volmig status <name>           # Quick status
volmig status <name> --follow  # Follow until done
volmig watch <name>            # Real-time updates
volmig describe <name>         # Full details
```

### Cleanup

```bash
volmig delete <name>
```

---

## Example: Resize a Weaviate Cluster

```bash
# Deploy 3-node Weaviate with 1Gi volumes
kubectl apply -f examples/weaviate/weaviate-cluster.yaml
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=weaviate --timeout=120s

# Check current PVC sizes
kubectl get pvc
# NAME                       CAPACITY
# weaviate-data-weaviate-0   1Gi
# weaviate-data-weaviate-1   1Gi
# weaviate-data-weaviate-2   1Gi

# Resize to 500Mi
volmig create resize-weaviate \
  --statefulset weaviate \
  --volume weaviate-data \
  --size 500Mi \
  --watch

# Verify
kubectl get pvc
# NAME                       CAPACITY
# weaviate-data-weaviate-0   500Mi
# weaviate-data-weaviate-1   500Mi
# weaviate-data-weaviate-2   500Mi
```

---

## Using YAML Instead

```yaml
apiVersion: storage.maurice.fr/v1alpha1
kind: VolumeResize
metadata:
  name: resize-my-app
spec:
  statefulSetName: my-app
  volumes:
    - name: data
      newSize: 500Mi
    - name: logs
      newSize: 100Mi
      storageClass: standard  # Optional: change storage class
```

```bash
kubectl apply -f volumeresize.yaml
kubectl get volumeresize -w
```

---

## How It Works

```
For each replica (0, 1, 2, ...):

1. Backup StatefulSet spec to ConfigMap
2. Delete StatefulSet (orphan mode - pods keep running)
3. Delete target pod
4. Set Retain policy on old PV
5. Create new PVC with target size
6. Run migrator pod (rclone sync)
7. Replace old PVC with new one
8. Recreate StatefulSet
9. Wait for pod ready
10. Next replica...
```

Migration is sequential to maintain quorum for distributed systems.

**Rollback**: If anything fails, old PVs are retained. The backup ConfigMap contains the original StatefulSet spec for manual recovery.

---

## The Migrator Pod

Data transfer happens via a lightweight migrator container that syncs between the old and new PVCs.

```
┌──────────────────────────────────────────────────────────┐
│  Migrator Pod                                            │
│  ┌─────────────┐        rclone sync        ┌──────────┐  │
│  │ Old PVC     │ ──────────────────────────▶│ New PVC  │  │
│  │ /source     │     (checksum verified)   │ /dest    │  │
│  │ (1Gi)       │                           │ (500Mi)  │  │
│  └─────────────┘                           └──────────┘  │
└──────────────────────────────────────────────────────────┘
```

**What's inside:**
- Alpine Linux (~5MB base)
- rclone for data sync
- Runs as root to handle files with any ownership

**The sync command:**
```bash
rclone sync /source/ /dest/ --progress --transfers 4 --checkers 8 --verbose
```

| Flag | Purpose |
|------|---------|
| `--progress` | Real-time transfer progress in logs |
| `--transfers 4` | 4 parallel file transfers |
| `--checkers 8` | 8 parallel integrity checkers |
| `--verbose` | Detailed logging for debugging |

**Why rclone?**
- Preserves permissions, ownership, timestamps
- Checksum verification (not just timestamps)
- Handles interruptions gracefully
- Battle-tested in production for years

---

## Requirements

- Kubernetes 1.25+
- RWO (ReadWriteOnce) volumes
- Dynamic storage provisioner
- StatefulSet without PDB blocking all disruptions

---

## Development

```bash
# Run locally against cluster
make run

# Run tests
make test

# Full e2e test with Weaviate
make test-weaviate

# Cleanup
make cleanup-test-weaviate
```

---

## License

Apache License 2.0
