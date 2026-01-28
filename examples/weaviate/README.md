# Weaviate VolumeResize Example

This example demonstrates how to resize PVCs for a 3-node Weaviate cluster.

## Prerequisites

- Kubernetes cluster with the VolumeResize operator installed
- `volmig` CLI (optional, but recommended)

## Deploy Weaviate

```bash
kubectl apply -f weaviate-cluster.yaml
kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=weaviate --timeout=120s
```

Verify the cluster:
```bash
kubectl get pods -l app.kubernetes.io/name=weaviate
kubectl get pvc -l app.kubernetes.io/name=weaviate
```

## Resize Volumes

### Option 1: Using the CLI (Recommended)

```bash
# Create resize operation
volmig create resize-weaviate \
  --statefulset weaviate \
  --volume weaviate-data \
  --size 500Mi

# Follow the progress
volmig status resize-weaviate --follow

# Or watch in real-time
volmig watch resize-weaviate
```

### Option 2: Using kubectl

```bash
kubectl apply -f volumeresize.yaml
kubectl get volumeresize resize-weaviate -w
```

## Verify Migration

```bash
# Check PVCs are resized
kubectl get pvc -l app.kubernetes.io/name=weaviate

# Check all pods are running
kubectl get pods -l app.kubernetes.io/name=weaviate

# Check cluster health
kubectl port-forward svc/weaviate 8080:80 &
curl http://localhost:8080/v1/nodes
```

## Cleanup

```bash
kubectl delete volumeresize resize-weaviate
kubectl delete -f weaviate-cluster.yaml
kubectl delete pvc -l app.kubernetes.io/name=weaviate
```
