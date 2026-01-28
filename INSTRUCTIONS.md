# VolumeResize Operator - Implementation Instructions

## Project Overview

Build a Kubernetes operator that migrates StatefulSet volumes to smaller PVCs using rclone for data synchronization.

### Key Design Decisions

- **CRD Scope**: One VolumeResize CR per StatefulSet, with explicit volume targets specified
- **Migration Strategy**: Spin up a separate rclone pod mounting both old and new volumes
- **StatefulSet Handling**: Delete with `--cascade=orphan`, delete target pod, migrate, recreate STS
- **Volume Replacement**: Set old PV to Retain, delete old PVC, create new PVC bound to new PV with original name
- **Replica Processing**: Sequential, one at a time
- **Rollback**: Old PV is retained for manual recovery
- **Validation**: Trust rclone exit code, refuse if newSize >= currentSize, refuse if PDB blocks disruption
- **Image**: `mauricethomas/volmig`
- **Environments**: kind, AWS, GCP (RWO volumes only)

---

## Progress Tracking

After completing each task:
1. Update `PROGRESS.md` with task ID, status (DONE/BLOCKED/SKIPPED), and notes
2. Run relevant tests
3. Commit changes

---

## Phase 1: Project Scaffolding

### Task 1.1: Initialize Kubebuilder Project

**Goal**: Create the base kubebuilder project structure.

**Actions**:
- Create directory `volume-resize-operator`
- Run `go mod init github.com/mauricethomas/volume-resize-operator`
- Run `kubebuilder init --domain maurice.fr --repo github.com/mauricethomas/volume-resize-operator`

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 1.2: Create VolumeResize API

**Goal**: Scaffold the CRD and controller.

**Actions**:
- Run `kubebuilder create api --group storage --version v1alpha1 --kind VolumeResize --resource --controller`
- Answer yes to both prompts

**Validation**: Files exist at `api/v1alpha1/volumeresize_types.go` and `internal/controller/volumeresize_controller.go`

**Update PROGRESS.md**

---

### Task 1.3: Define CRD Spec

**Goal**: Define the VolumeResize spec schema.

**Actions**:
- Edit `api/v1alpha1/volumeresize_types.go`
- Add `VolumeResizeSpec` with fields:
  - `StatefulSetName` (string, required)
  - `Volumes` ([]VolumeResizeTarget, required, minItems=1)
- Add `VolumeResizeTarget` struct with fields:
  - `Name` (string, required) - matches volumeClaimTemplate name
  - `NewSize` (resource.Quantity, required)
  - `StorageClass` (*string, optional) - defaults to original if not specified
- Run `make generate && make manifests`

**Validation**: `make build` succeeds, CRD YAML generated in `config/crd/bases/`

**Update PROGRESS.md**

---

### Task 1.4: Define CRD Status

**Goal**: Define comprehensive status reporting.

**Actions**:
- Add `VolumeResizeStatus` with fields:
  - `Phase` (string, enum: Pending/Validating/Syncing/Replacing/Completed/Failed)
  - `Conditions` ([]metav1.Condition)
  - `VolumeStatuses` ([]VolumeStatus)
  - `CurrentReplica` (*int32)
  - `CurrentVolume` (string)
  - `Message` (string)
  - `StartTime` (*metav1.Time)
  - `CompletionTime` (*metav1.Time)
- Add `VolumeStatus` struct with fields:
  - `VolumeName`, `Replica`, `Phase`, `OldPVCName`, `NewPVCName`, `OldPVName`, `Message`
- Add kubebuilder markers for status subresource and printcolumns
- Run `make generate && make manifests`

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 1.5: Add Testify Dependency

**Goal**: Set up testing framework.

**Actions**:
- Run `go get github.com/stretchr/testify`
- Run `go mod tidy`

**Validation**: `go mod graph | grep testify` shows dependency

**Update PROGRESS.md**

---

### Task 1.6: Create PROGRESS.md Template

**Goal**: Create the progress tracking file.

**Actions**:
- Create `PROGRESS.md` with structure:
  - Current Status section (phase, last updated)
  - Completed Tasks table (Task, Status, Date, Notes)
  - Blocked Tasks table (Task, Blocker, Notes)
  - Next Steps list

**Validation**: File exists and is valid markdown

**Update PROGRESS.md** (this is the first entry!)

---

## Phase 2: Kind Test Infrastructure

### Task 2.1: Create Kind Cluster Config

**Goal**: Define kind cluster configuration.

**Actions**:
- Create `hack/kind-config.yaml` with:
  - 1 control-plane node
  - 1 worker node
  - Extra mounts for volume testing if needed

**Validation**: File is valid YAML

**Update PROGRESS.md**

---

### Task 2.2: Create Kind Setup Script

**Goal**: Automate kind cluster creation.

**Actions**:
- Create `hack/setup-kind.sh` that:
  - Creates cluster named `volresize-test` if not exists
  - Uses config from `hack/kind-config.yaml`
  - Sets kubectl context
  - Prints cluster info
- Make script executable

**Validation**: Running script creates cluster, `kubectl get nodes` shows nodes ready

**Update PROGRESS.md**

---

### Task 2.3: Create Kind Teardown Script

**Goal**: Automate kind cluster deletion.

**Actions**:
- Create `hack/teardown-kind.sh` that deletes cluster `volresize-test`
- Make script executable

**Validation**: Running script deletes cluster

**Update PROGRESS.md**

---

### Task 2.4: Create Test StatefulSet Manifest

**Goal**: Create a test StatefulSet for development.

**Actions**:
- Create `hack/test-manifests/statefulset.yaml` with:
  - Headless Service
  - StatefulSet named `test-sts` with 2 replicas
  - Two volumeClaimTemplates: `data` (1Gi) and `logs` (500Mi)
  - Simple busybox container mounting both volumes

**Validation**: `kubectl apply -f hack/test-manifests/statefulset.yaml` creates resources

**Update PROGRESS.md**

---

### Task 2.5: Create Test Data Population Script

**Goal**: Script to populate test data in StatefulSet volumes.

**Actions**:
- Create `hack/test-manifests/populate-data.sh` that:
  - Writes random data to each pod's volumes
  - Creates marker files with identifiable content per replica
- Make script executable

**Validation**: Running script populates data, can verify with `kubectl exec`

**Update PROGRESS.md**

---

### Task 2.6: Create Sample VolumeResize CR

**Goal**: Sample CR for testing.

**Actions**:
- Create `hack/test-manifests/volumeresize-sample.yaml` targeting:
  - StatefulSet: `test-sts`
  - Volume: `data` with newSize: `500Mi`

**Validation**: File is valid YAML matching CRD schema

**Update PROGRESS.md**

---

## Phase 3: Migrator Image

### Task 3.1: Create Migrator Dockerfile

**Goal**: Build image with rclone and utilities.

**Actions**:
- Create `build/migrator/Dockerfile` with:
  - Base: `alpine:3.19`
  - Install: `rclone`, `bash`, `coreutils`
  - Non-root user

**Validation**: `docker build -t mauricethomas/volmig:latest build/migrator/` succeeds

**Update PROGRESS.md**

---

### Task 3.2: Create Migration Script

**Goal**: Script that runs inside migrator pod.

**Actions**:
- Create `build/migrator/migrate.sh` that:
  - Takes SOURCE_PATH and DEST_PATH env vars
  - Runs `rclone sync` with progress output
  - Exits with rclone's exit code
- Update Dockerfile to include the script

**Validation**: Rebuild image, verify script exists at expected path

**Update PROGRESS.md**

---

### Task 3.3: Create Image Build Script

**Goal**: Automate image building and loading to kind.

**Actions**:
- Create `hack/build-migrator-image.sh` that:
  - Builds `mauricethomas/volmig:latest`
  - Optionally pushes if PUSH=true env var set
  - Loads into kind cluster if it exists
- Make script executable

**Validation**: Running script builds image and loads into kind

**Update PROGRESS.md**

---

### Task 3.4: Test Migrator Image Manually

**Goal**: Verify image works with test data.

**Actions**:
- Create a temporary test pod manifest mounting two PVCs
- Run the migrator image with migrate.sh
- Verify data is synced correctly

**Validation**: Data appears in destination volume with correct contents

**Update PROGRESS.md**

---

## Phase 4: Controller Constants and Types

### Task 4.1: Create Constants File

**Goal**: Define shared constants.

**Actions**:
- Create `internal/controller/constants.go` with:
  - Phase constants (Pending, Validating, Syncing, Replacing, Completed, Failed)
  - Condition type constants (Ready, Validated, Progressing)
  - Annotation keys for tracking old PV names, managed-by
  - Label keys for migration name, replica, volume
  - Finalizer name
  - Default migrator image constant

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 4.2: Create Unit Tests for Constants

**Goal**: Ensure constants are properly defined.

**Actions**:
- Create `internal/controller/constants_test.go`
- Test that all phase constants are non-empty strings
- Test that label/annotation keys follow Kubernetes naming conventions (contain domain)
- Use testify assertions

**Validation**: `go test ./internal/controller/... -run TestConstants` passes

**Update PROGRESS.md**

---

## Phase 5: Validation Logic

### Task 5.1: Create Validation Types

**Goal**: Define validation result types.

**Actions**:
- Create `internal/controller/validation.go`
- Define `ValidationResult` struct with:
  - `Valid` (bool)
  - `Message` (string)

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 5.2: Implement StatefulSet Validation

**Goal**: Validate target StatefulSet exists.

**Actions**:
- Add function `validateStatefulSetExists(ctx, client, namespace, name)` that:
  - Gets the StatefulSet
  - Returns ValidationResult with message if not found

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 5.3: Implement Volume Target Validation

**Goal**: Validate volume targets match volumeClaimTemplates.

**Actions**:
- Add function `validateVolumeTargets(sts, volumes)` that:
  - For each target in volumes, finds matching volumeClaimTemplate in STS
  - Returns error message if any template not found

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 5.4: Implement Size Validation

**Goal**: Ensure newSize < currentSize.

**Actions**:
- Add function `validateSizeReduction(ctx, client, namespace, stsName, volume)` that:
  - Gets actual PVC for replica 0
  - Compares newSize to current PVC size
  - Returns error if newSize >= currentSize

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 5.5: Implement PDB Validation

**Goal**: Check PodDisruptionBudgets allow disruption.

**Actions**:
- Add function `validatePDBAllowsDisruption(ctx, client, sts)` that:
  - Lists PDBs in namespace
  - Checks if any PDB selector matches StatefulSet pod labels
  - Returns error if matching PDB has DisruptionsAllowed < 1

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 5.6: Create Validation Unit Tests

**Goal**: Test all validation functions.

**Actions**:
- Create `internal/controller/validation_test.go`
- Use fake client from controller-runtime
- Test cases:
  - StatefulSet not found → invalid
  - Volume target not found in templates → invalid
  - newSize >= currentSize → invalid
  - newSize < currentSize → valid
  - PDB blocking disruption (DisruptionsAllowed=0) → invalid
  - PDB allowing disruption (DisruptionsAllowed>=1) → valid
  - No PDB present → valid
- Use testify assertions (assert, require)

**Validation**: `go test ./internal/controller/... -run TestValidation` passes

**Update PROGRESS.md**

---

## Phase 6: PVC Operations

### Task 6.1: Create PVC Helper Functions

**Goal**: PVC name generation utilities.

**Actions**:
- Create `internal/controller/pvc.go`
- Add `getOriginalPVCName(volumeName, stsName, replica)` returning `<vol>-<sts>-<replica>`
- Add `getTempPVCName(volumeName, stsName, replica)` returning `<vol>-<sts>-<replica>-new`

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 6.2: Implement Temp PVC Creation

**Goal**: Create temporary PVC for migration target.

**Actions**:
- Add function `createTempPVC(ctx, client, vr, vol, originalPVC, replica)` that:
  - Checks if temp PVC already exists (for idempotency)
  - Creates PVC with newSize from vol spec
  - Uses storageClass from vol spec, or defaults to originalPVC's storageClass
  - Adds labels: migration name, replica, volume name
  - Adds annotation: managed-by

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 6.3: Implement PV Retain Policy

**Goal**: Set PV reclaim policy to Retain before deleting PVC.

**Actions**:
- Add function `setRetainOnPV(ctx, client, pvName)` that:
  - Gets PV by name
  - If already Retain, returns early
  - Sets `PersistentVolumeReclaimPolicy` to `Retain`
  - Updates PV

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 6.4: Implement PVC Replacement

**Goal**: Replace old PVC with new one bound to new PV.

**Actions**:
- Add function `replacePVC(ctx, client, vr, vol, replica)` that:
  - Gets temp PVC to find its bound PV name
  - Deletes temp PVC
  - Clears claimRef on the new PV (so it can be bound again)
  - Deletes original PVC
  - Creates new PVC with original name, setting `volumeName` to bind to new PV
  - Copies labels from original PVC

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 6.5: Create PVC Unit Tests

**Goal**: Test PVC operations.

**Actions**:
- Create `internal/controller/pvc_test.go`
- Use fake client
- Test cases:
  - `getOriginalPVCName` returns correct format
  - `getTempPVCName` returns correct format
  - `createTempPVC` creates with correct size and storageClass
  - `createTempPVC` is idempotent (returns existing if present)
  - `setRetainOnPV` updates policy
  - `setRetainOnPV` is idempotent
- Use testify assertions

**Validation**: `go test ./internal/controller/... -run TestPVC` passes

**Update PROGRESS.md**

---

## Phase 7: StatefulSet Operations

### Task 7.1: Create StatefulSet Helper File

**Goal**: STS manipulation utilities.

**Actions**:
- Create `internal/controller/statefulset.go`
- Add helper to deep copy STS spec for recreation

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 7.2: Implement Orphan Delete

**Goal**: Delete STS without deleting pods.

**Actions**:
- Add function `deleteSTSOrphan(ctx, client, sts)` that:
  - Deep copies STS spec for later recreation
  - Deletes STS with `PropagationPolicy: Orphan`
  - Returns the stored spec

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 7.3: Implement Pod Deletion

**Goal**: Delete specific pod for migration.

**Actions**:
- Add function `deletePod(ctx, client, namespace, stsName, replica)` that:
  - Constructs pod name: `<sts>-<replica>`
  - Deletes pod
  - Optionally waits for pod to be fully terminated

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 7.4: Implement STS Recreation

**Goal**: Recreate StatefulSet after migration.

**Actions**:
- Add function `recreateSTS(ctx, client, stsSpec)` that:
  - Clears resourceVersion, UID, and other server-set fields
  - Clears status
  - Creates STS from cleaned spec

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 7.5: Create StatefulSet Unit Tests

**Goal**: Test STS operations.

**Actions**:
- Create `internal/controller/statefulset_test.go`
- Test cases:
  - Orphan delete returns valid spec copy
  - Pod name construction is correct
  - STS recreation clears server fields
- Use testify assertions

**Validation**: `go test ./internal/controller/... -run TestStatefulSet` passes

**Update PROGRESS.md**

---

## Phase 8: Migration Pod

### Task 8.1: Create Migration Pod Spec Builder

**Goal**: Build pod spec for rclone migration.

**Actions**:
- Create `internal/controller/migrator.go`
- Add function `buildMigratorPod(vr, vol, replica, oldPVCName, newPVCName)` that returns Pod with:
  - Name: `<vr.name>-migrator-<replica>-<vol>`
  - Image: `mauricethomas/volmig:latest` (from constant)
  - Volume mounts: old PVC at /source, new PVC at /dest
  - Env vars: SOURCE_PATH=/source, DEST_PATH=/dest
  - Command: runs migrate.sh
  - RestartPolicy: Never
  - Labels for tracking

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 8.2: Implement Migration Pod Creation

**Goal**: Create and run migration pod.

**Actions**:
- Add function `createMigratorPod(ctx, client, vr, vol, replica, oldPVC, newPVC)` that:
  - Builds pod spec using helper
  - Checks if pod already exists (idempotency)
  - Creates pod
  - Returns pod reference

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 8.3: Implement Migration Pod Monitoring

**Goal**: Wait for migration to complete.

**Actions**:
- Add function `waitForMigrationComplete(ctx, client, podName, namespace, timeout)` that:
  - Polls pod status
  - Returns nil if pod phase is Succeeded
  - Returns error if pod phase is Failed
  - Returns error on timeout

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 8.4: Implement Migration Pod Cleanup

**Goal**: Clean up migration pod after completion.

**Actions**:
- Add function `cleanupMigratorPod(ctx, client, podName, namespace)` that:
  - Deletes the pod
  - Ignores NotFound errors

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 8.5: Create Migrator Unit Tests

**Goal**: Test migration pod operations.

**Actions**:
- Create `internal/controller/migrator_test.go`
- Test cases:
  - Pod spec has correct volume mounts (old at /source, new at /dest)
  - Pod spec has correct env vars
  - Pod spec uses correct image
  - Pod has expected labels
  - Pod name follows expected format
- Use testify assertions

**Validation**: `go test ./internal/controller/... -run TestMigrator` passes

**Update PROGRESS.md**

---

## Phase 9: Main Reconciliation Loop

### Task 9.1: Implement Reconcile Entry Point

**Goal**: Main reconcile function structure.

**Actions**:
- Edit `internal/controller/volumeresize_controller.go`
- Implement `Reconcile()` that:
  - Gets VolumeResize CR (return if not found)
  - Checks for deletion (handle finalizer)
  - Adds finalizer if not present
  - Routes to phase handler based on `status.Phase`
  - Handles empty phase as Pending

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.2: Implement Pending Phase Handler

**Goal**: Initialize migration and transition to Validating.

**Actions**:
- Add function `handlePending(ctx, vr)` that:
  - Sets StartTime to now
  - Initializes empty VolumeStatuses array
  - Sets Phase to Validating
  - Updates status

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.3: Implement Validating Phase Handler

**Goal**: Run all validations.

**Actions**:
- Add function `handleValidating(ctx, vr)` that:
  - Runs validateStatefulSetExists
  - Runs validateVolumeTargets
  - Runs validateSizeReduction for each volume
  - Runs validatePDBAllowsDisruption
  - On any failure: set Phase=Failed, Message=error, return
  - On success: set Phase=Syncing, CurrentReplica=0, CurrentVolume=first volume

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.4: Implement Syncing Phase Handler - Part 1

**Goal**: Set up for data sync (create temp PVC, prepare old PV).

**Actions**:
- Add function `handleSyncing(ctx, vr)` that for current replica/volume:
  - Gets original PVC
  - Creates temp PVC if not exists
  - Waits for temp PVC to be bound
  - Sets Retain policy on old PV
  - Updates VolumeStatus with PVC/PV names

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.5: Implement Syncing Phase Handler - Part 2

**Goal**: Handle STS manipulation and pod deletion.

**Actions**:
- Extend handleSyncing to:
  - Delete STS with orphan if not already deleted (track in annotation)
  - Delete target pod
  - Wait for pod termination

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.6: Implement Syncing Phase Handler - Part 3

**Goal**: Run migration and advance to next volume/replica.

**Actions**:
- Extend handleSyncing to:
  - Create migrator pod
  - Wait for migration to complete
  - Clean up migrator pod
  - Update VolumeStatus to synced
  - Advance to next volume for same replica, or next replica
  - When all done: set Phase=Replacing

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.7: Implement Replacing Phase Handler

**Goal**: Replace PVCs with new ones.

**Actions**:
- Add function `handleReplacing(ctx, vr)` that:
  - For each replica/volume combination:
    - Calls replacePVC
    - Updates VolumeStatus
  - Recreates StatefulSet
  - Sets Phase=Completed
  - Sets CompletionTime

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.8: Implement Status Update Helpers

**Goal**: Utilities for status updates.

**Actions**:
- Add `updatePhase(vr, phase, message)` helper
- Add `updateVolumeStatus(vr, volumeName, replica, phase, message)` helper
- Add `setCondition(vr, conditionType, status, reason, message)` helper
- Ensure all helpers properly update the status subresource

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.9: Add Finalizer Handling

**Goal**: Clean up resources on CR deletion.

**Actions**:
- Add finalizer constant to constants.go
- In Reconcile: add finalizer if missing on new CR
- Add `handleDeletion(ctx, vr)` that:
  - Lists and deletes any temp PVCs with migration label
  - Lists and deletes any migrator pods with migration label
  - Removes finalizer from CR

**Validation**: `make build` succeeds

**Update PROGRESS.md**

---

### Task 9.10: Create Reconciler Unit Tests

**Goal**: Test reconciliation logic.

**Actions**:
- Create `internal/controller/volumeresize_controller_test.go`
- Use envtest or fake client with scheme
- Test cases:
  - CR not found returns no error (no requeue)
  - New CR gets finalizer added
  - Pending transitions to Validating
  - Validation failure sets Failed phase
  - Deletion triggers cleanup
- Use testify assertions

**Validation**: `go test ./internal/controller/... -run TestReconcile` passes

**Update PROGRESS.md**

---

## Phase 10: RBAC and Deployment

### Task 10.1: Configure RBAC Permissions

**Goal**: Set up proper RBAC markers.

**Actions**:
- Add kubebuilder RBAC markers to controller for:
  - volumeresize (all verbs)
  - volumeresize/status (get, update, patch)
  - statefulsets (get, list, watch, delete, create)
  - pods (get, list, watch, delete, create)
  - persistentvolumeclaims (get, list, watch, delete, create)
  - persistentvolumes (get, list, watch, update)
  - poddisruptionbudgets (get, list, watch)
  - events (create, patch)
- Run `make manifests`

**Validation**: Check `config/rbac/role.yaml` includes all permissions

**Update PROGRESS.md**

---

### Task 10.2: Configure Manager Deployment

**Goal**: Review and adjust deployment configuration.

**Actions**:
- Review `config/manager/manager.yaml`
- Set appropriate resource requests/limits
- Ensure leader election is configured
- Set image pull policy appropriately

**Validation**: Manifest is valid

**Update PROGRESS.md**

---

### Task 10.3: Create Deployment Script

**Goal**: Automate deployment to kind.

**Actions**:
- Create `hack/deploy.sh` that:
  - Builds operator image with `make docker-build`
  - Loads image into kind cluster
  - Installs CRDs with `make install`
  - Deploys operator with `make deploy`
- Make script executable

**Validation**: Running script deploys operator, pod is running in operator namespace

**Update PROGRESS.md**

---

## Phase 11: Integration Testing

### Task 11.1: Create Integration Test Framework

**Goal**: Set up integration test structure.

**Actions**:
- Create `test/integration/suite_test.go`
- Set up test suite that:
  - Connects to kind cluster (or uses envtest)
  - Creates test namespace
  - Cleans up after tests
- Add build tag for integration tests

**Validation**: `go test ./test/integration/... -v -tags=integration` runs (even with no tests)

**Update PROGRESS.md**

---

### Task 11.2: Test Happy Path - Single Volume Single Replica

**Goal**: Simplest end-to-end test.

**Actions**:
- Create `test/integration/basic_test.go`
- Test that:
  - Creates STS with 1 replica, 1 volume (1Gi)
  - Populates data with marker file
  - Creates VolumeResize CR (newSize: 500Mi)
  - Waits for Completed phase
  - Verifies marker file exists in migrated volume
  - Verifies new PVC has correct size
  - Verifies old PV has Retain policy
- Use testify assertions

**Validation**: Test passes on kind cluster

**Update PROGRESS.md**

---

### Task 11.3: Test Happy Path - Single Volume Multiple Replicas

**Goal**: Test sequential replica processing.

**Actions**:
- Add test case:
  - Creates STS with 3 replicas, 1 volume
  - Creates unique marker per replica
  - Creates VolumeResize CR
  - Verifies all replicas migrated
  - Verifies data integrity per replica

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.4: Test Happy Path - Multiple Volumes

**Goal**: Test multi-volume migration.

**Actions**:
- Add test case:
  - Creates STS with 2 volumeClaimTemplates
  - Creates VolumeResize targeting both volumes with different sizes
  - Verifies both volumes migrated correctly

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.5: Test Validation Failure - Size Not Smaller

**Goal**: Test size validation enforcement.

**Actions**:
- Create `test/integration/validation_test.go`
- Test:
  - Creates STS with 500Mi volume
  - Creates VolumeResize with newSize: 1Gi
  - Verifies Phase becomes Failed
  - Verifies Message mentions size

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.6: Test Validation Failure - Volume Not Found

**Goal**: Test volume target validation.

**Actions**:
- Add test case:
  - Creates STS with volume named "data"
  - Creates VolumeResize targeting volume "nonexistent"
  - Verifies Phase becomes Failed

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.7: Test Validation Failure - PDB Blocking

**Goal**: Test PDB enforcement.

**Actions**:
- Add test case:
  - Creates STS with 2 replicas
  - Creates PDB with minAvailable: 2 (no disruption allowed)
  - Creates VolumeResize
  - Verifies Phase becomes Failed
  - Verifies Message mentions PDB

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.8: Test StorageClass Override

**Goal**: Test custom storage class specification.

**Actions**:
- Add test case:
  - Creates STS with default storage class
  - Creates alternate StorageClass in cluster
  - Creates VolumeResize specifying the alternate storageClass
  - Verifies new PVC uses specified storage class

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.9: Test Idempotency / Controller Restart

**Goal**: Verify controller handles restarts gracefully.

**Actions**:
- Add test case:
  - Starts migration
  - Deletes controller pod mid-migration (during Syncing phase)
  - Waits for controller to restart
  - Verifies migration completes successfully
  - Verifies no duplicate temp PVCs or migrator pods

**Validation**: Test passes

**Update PROGRESS.md**

---

### Task 11.10: Test Cleanup on CR Deletion

**Goal**: Verify finalizer cleanup works.

**Actions**:
- Add test case:
  - Creates STS and VolumeResize
  - Waits for Syncing phase (temp PVC exists)
  - Deletes VolumeResize CR
  - Verifies temp PVCs are cleaned up
  - Verifies migrator pods are cleaned up
  - Verifies CR is fully deleted

**Validation**: Test passes

**Update PROGRESS.md**

---

## Phase 12: Documentation and Polish

### Task 12.1: Create README.md

**Goal**: Project documentation.

**Actions**:
- Write README.md with:
  - Project description and purpose
  - Prerequisites (Go, kubebuilder, kind, docker)
  - Quick start / installation instructions
  - Usage example with sample CR
  - CRD field reference
  - How rollback works (old PV retained)
  - Troubleshooting section

**Validation**: README is complete and follows good practices

**Update PROGRESS.md**

---

### Task 12.2: Document CRD Fields

**Goal**: Inline documentation for CRD.

**Actions**:
- Add comprehensive godoc comments to all fields in `volumeresize_types.go`
- Run `make manifests` to update CRD YAML descriptions

**Validation**: `kubectl explain volumeresize.spec` shows helpful descriptions

**Update PROGRESS.md**

---

### Task 12.3: Add Structured Logging

**Goal**: Comprehensive, consistent logging.

**Actions**:
- Review all controller functions
- Add structured logging (using logr) with:
  - Operation being performed
  - Resource names (STS, PVC, PV, Pod)
  - Replica index and volume name being processed
  - Phase transitions
  - Errors with full context
- Use consistent log levels (Info for progress, Error for failures)

**Validation**: Logs are informative and parseable during test runs

**Update PROGRESS.md**

---

### Task 12.4: Add Kubernetes Events

**Goal**: Emit events for observability.

**Actions**:
- Add EventRecorder to controller
- Emit events for:
  - Migration started (Normal)
  - Validation passed (Normal)
  - Validation failed (Warning)
  - Replica migration started (Normal)
  - Replica migration completed (Normal)
  - Migration completed (Normal)
  - Errors (Warning)

**Validation**: `kubectl describe volumeresize <name>` shows events

**Update PROGRESS.md**

---

### Task 12.5: Run Full Test Suite

**Goal**: All tests pass.

**Actions**:
- Run `make test` (unit tests)
- Run integration tests on kind
- Fix any failures
- Ensure no race conditions

**Validation**: All tests pass consistently

**Update PROGRESS.md**

---

### Task 12.6: Create Release Manifest Generator

**Goal**: Single-file installation manifest.

**Actions**:
- Create `hack/generate-release.sh` that:
  - Runs `make build-installer`
  - Or manually combines CRD + RBAC + Deployment YAMLs
  - Outputs to `dist/install.yaml`
- Make script executable

**Validation**: `kubectl apply -f dist/install.yaml` deploys everything correctly

**Update PROGRESS.md**

---

## Completion Checklist

Before marking project complete, verify all items:

- [ ] All unit tests pass (`make test`)
- [ ] All integration tests pass on kind
- [ ] CRD fields are documented (godoc + kubectl explain)
- [ ] README.md is complete and accurate
- [ ] RBAC is minimal but sufficient
- [ ] Structured logging is comprehensive
- [ ] Kubernetes events are emitted
- [ ] Finalizer cleanup works correctly
- [ ] Controller handles restarts gracefully
- [ ] Validation catches all error cases
- [ ] Old PV is set to Retain for rollback
- [ ] Works on kind cluster
- [ ] Migrator image builds and works
- [ ] Release manifest can be generated
- [ ] PROGRESS.md is fully updated with all tasks
