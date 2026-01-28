/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

// VolumeResizeReconciler reconciles a VolumeResize object
type VolumeResizeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=storage.maurice.fr,resources=volumeresizes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.maurice.fr,resources=volumeresizes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=storage.maurice.fr,resources=volumeresizes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;delete;create
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;delete;create
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;delete;create
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is the main reconciliation loop
func (r *VolumeResizeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the VolumeResize instance
	vr := &storagev1alpha1.VolumeResize{}
	if err := r.Get(ctx, req.NamespacedName, vr); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !vr.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, vr)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(vr, FinalizerName) {
		controllerutil.AddFinalizer(vr, FinalizerName)
		if err := r.Update(ctx, vr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Route to phase handler
	switch vr.Status.Phase {
	case "", PhasePending:
		return r.handlePending(ctx, vr)
	case PhaseValidating:
		return r.handleValidating(ctx, vr)
	case PhaseSyncing:
		return r.handleSyncing(ctx, vr)
	case PhaseReplacing:
		return r.handleReplacing(ctx, vr)
	case PhaseCompleted, PhaseFailed:
		// Terminal states, no action needed
		return ctrl.Result{}, nil
	default:
		log.Error(nil, "Unknown phase", "phase", vr.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// handlePending initializes the migration and transitions to Validating
func (r *VolumeResizeReconciler) handlePending(ctx context.Context, vr *storagev1alpha1.VolumeResize) (ctrl.Result, error) {
	now := metav1.Now()
	vr.Status.StartTime = &now
	vr.Status.VolumeStatuses = []storagev1alpha1.VolumeStatus{}
	vr.Status.Phase = PhaseValidating
	vr.Status.Message = "Starting validation"

	if err := r.Status().Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// handleValidating runs all validation checks
func (r *VolumeResizeReconciler) handleValidating(ctx context.Context, vr *storagev1alpha1.VolumeResize) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Validate StatefulSet exists
	sts, result := validateStatefulSetExists(ctx, r.Client, vr.Namespace, vr.Spec.StatefulSetName)
	if !result.Valid {
		return r.setFailed(ctx, vr, result.Message)
	}

	// Validate volume targets
	result = validateVolumeTargets(sts, vr.Spec.Volumes)
	if !result.Valid {
		return r.setFailed(ctx, vr, result.Message)
	}

	// Validate size reduction for each volume
	for _, vol := range vr.Spec.Volumes {
		result = validateSizeReduction(ctx, r.Client, vr.Namespace, vr.Spec.StatefulSetName, vol)
		if !result.Valid {
			return r.setFailed(ctx, vr, result.Message)
		}
	}

	// Validate PDB allows disruption
	result = validatePDBAllowsDisruption(ctx, r.Client, sts)
	if !result.Valid {
		return r.setFailed(ctx, vr, result.Message)
	}

	log.Info("Validation passed, starting sync phase")

	// Initialize volume statuses
	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}

	for i := int32(0); i < replicas; i++ {
		for _, vol := range vr.Spec.Volumes {
			vr.Status.VolumeStatuses = append(vr.Status.VolumeStatuses, storagev1alpha1.VolumeStatus{
				VolumeName: vol.Name,
				Replica:    i,
				Phase:      VolumeStatusPending,
			})
		}
	}

	// Transition to Syncing
	vr.Status.Phase = PhaseSyncing
	vr.Status.CurrentReplica = ptrInt32(0)
	vr.Status.CurrentVolume = vr.Spec.Volumes[0].Name
	vr.Status.Message = "Validation complete, starting sync"

	if err := r.Status().Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// handleSyncing handles the data synchronization phase
//
//nolint:gocyclo // Complex state machine with multiple phases - refactoring would increase risk
func (r *VolumeResizeReconciler) handleSyncing(ctx context.Context, vr *storagev1alpha1.VolumeResize) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	replica := *vr.Status.CurrentReplica
	volName := vr.Status.CurrentVolume

	// Check if current volume/replica is already completed - if so, advance to next
	for _, vs := range vr.Status.VolumeStatuses {
		if vs.VolumeName == volName && vs.Replica == replica && vs.Phase == VolumeStatusCompleted {
			log.Info("Volume already completed, advancing to next", "replica", replica, "volume", volName)
			nextReplica, nextVol, done := r.getNextVolumeReplica(vr, replica, volName)
			if done {
				vr.Status.Phase = PhaseCompleted
				now := metav1.Now()
				vr.Status.CompletionTime = &now
				vr.Status.Message = MessageMigrationCompleted
				vr.Status.CurrentReplica = nil
				vr.Status.CurrentVolume = ""
			} else {
				vr.Status.CurrentReplica = &nextReplica
				vr.Status.CurrentVolume = nextVol
				vr.Status.Message = fmt.Sprintf("Migrating replica %d volume %s", nextReplica, nextVol)
			}
			if err := r.Status().Update(ctx, vr); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	}

	// Find the volume target
	var vol storagev1alpha1.VolumeResizeTarget
	for _, v := range vr.Spec.Volumes {
		if v.Name == volName {
			vol = v
			break
		}
	}

	log.Info("Processing volume", "replica", replica, "volume", volName)

	// Get original PVC
	originalPVCName := getOriginalPVCName(volName, vr.Spec.StatefulSetName, replica)
	originalPVC, err := getPVC(ctx, r.Client, vr.Namespace, originalPVCName)
	if err != nil {
		return r.setFailed(ctx, vr, fmt.Sprintf("failed to get original PVC: %v", err))
	}

	// Create temp PVC if not exists
	tempPVCName := getTempPVCName(volName, vr.Spec.StatefulSetName, replica)
	tempPVC, err := createTempPVC(ctx, r.Client, vr, vol, originalPVC, replica)
	if err != nil {
		return r.setFailed(ctx, vr, fmt.Sprintf("failed to create temp PVC: %v", err))
	}

	// Check if we need to wait for temp PVC to be bound
	// Skip waiting for WaitForFirstConsumer storage classes - the migrator pod will trigger binding
	waitForBinding := true
	if tempPVC.Spec.StorageClassName != nil && *tempPVC.Spec.StorageClassName != "" {
		sc := &storagev1.StorageClass{}
		if err := r.Get(ctx, types.NamespacedName{Name: *tempPVC.Spec.StorageClassName}, sc); err == nil {
			if sc.VolumeBindingMode != nil && *sc.VolumeBindingMode == storagev1.VolumeBindingWaitForFirstConsumer {
				waitForBinding = false
			}
		}
	}

	if waitForBinding && tempPVC.Status.Phase != corev1.ClaimBound {
		log.Info("Waiting for temp PVC to be bound", "pvc", tempPVCName)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Set Retain on old PV
	oldPVName := originalPVC.Spec.VolumeName
	if err := setRetainOnPV(ctx, r.Client, oldPVName); err != nil {
		return r.setFailed(ctx, vr, fmt.Sprintf("failed to set retain on PV: %v", err))
	}

	// Update volume status
	r.updateVolumeStatus(vr, volName, replica, VolumeStatusSyncing, "Preparing migration")
	vr.Status.VolumeStatuses = updateVolumeStatusInList(vr.Status.VolumeStatuses, volName, replica, func(vs *storagev1alpha1.VolumeStatus) {
		vs.OldPVCName = originalPVCName
		vs.NewPVCName = tempPVCName
		vs.OldPVName = oldPVName
	})

	// Delete STS with orphan if not already deleted
	if vr.Annotations == nil || vr.Annotations[AnnotationSTSDeleted] != "true" {
		sts := &appsv1.StatefulSet{}
		err := r.Get(ctx, types.NamespacedName{Namespace: vr.Namespace, Name: vr.Spec.StatefulSetName}, sts)
		if err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
		if err == nil {
			// FIRST: Backup STS spec to ConfigMap BEFORE any changes
			if err := backupSTSToConfigMap(ctx, r.Client, vr, sts); err != nil {
				return r.setFailed(ctx, vr, fmt.Sprintf("failed to backup STS to ConfigMap: %v", err))
			}
			log.Info("StatefulSet spec backed up to ConfigMap", "configmap", getSTSBackupConfigMapName(vr.Name))

			// Now safe to delete STS with orphan policy
			if _, err := deleteSTSOrphan(ctx, r.Client, sts); err != nil {
				return r.setFailed(ctx, vr, fmt.Sprintf("failed to delete STS: %v", err))
			}

			// Mark STS as deleted and record backup ConfigMap name
			if vr.Annotations == nil {
				vr.Annotations = make(map[string]string)
			}
			vr.Annotations[AnnotationSTSDeleted] = "true"
			vr.Annotations[AnnotationSTSBackup] = getSTSBackupConfigMapName(vr.Name)
			if err := r.Update(ctx, vr); err != nil {
				return ctrl.Result{}, err
			}

			// Update status with backup ConfigMap name
			vr.Status.BackupConfigMapName = getSTSBackupConfigMapName(vr.Name)
			if err := r.Status().Update(ctx, vr); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	// Delete target pod
	podName := getPodName(vr.Spec.StatefulSetName, replica)
	if err := deletePod(ctx, r.Client, vr.Namespace, vr.Spec.StatefulSetName, replica); err != nil {
		log.Error(err, "Failed to delete pod", "pod", podName)
	}

	// Wait for pod termination
	if err := waitForPodTermination(ctx, r.Client, vr.Namespace, podName, time.Minute*2); err != nil {
		log.Info("Waiting for pod termination", "pod", podName)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Create migrator pod
	migratorPod, err := createMigratorPod(ctx, r.Client, vr, vol, replica, originalPVCName, tempPVCName)
	if err != nil {
		return r.setFailed(ctx, vr, fmt.Sprintf("failed to create migrator pod: %v", err))
	}

	// Wait for migration to complete
	if err := waitForMigrationComplete(ctx, r.Client, migratorPod.Name, vr.Namespace, time.Hour); err != nil {
		// Check if still running
		pod := &corev1.Pod{}
		if getErr := r.Get(ctx, types.NamespacedName{Namespace: vr.Namespace, Name: migratorPod.Name}, pod); getErr == nil {
			if pod.Status.Phase == corev1.PodRunning || pod.Status.Phase == corev1.PodPending {
				log.Info("Migration in progress", "pod", migratorPod.Name)
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}
			if pod.Status.Phase == corev1.PodFailed {
				return r.setFailed(ctx, vr, "migration pod failed")
			}
		}
	}

	// Cleanup migrator pod
	if err := cleanupMigratorPod(ctx, r.Client, migratorPod.Name, vr.Namespace); err != nil {
		log.Error(err, "Failed to cleanup migrator pod")
	}

	// Update volume status to synced
	r.updateVolumeStatus(vr, volName, replica, VolumeStatusSynced, "Migration complete")

	// IMMEDIATELY replace PVC for this replica (don't wait for all replicas)
	log.Info("Replacing PVC for replica", "replica", replica, "volume", volName)
	if err := replacePVC(ctx, r.Client, vr, vol, replica); err != nil {
		return r.setFailed(ctx, vr, fmt.Sprintf("failed to replace PVC: %v", err))
	}
	r.updateVolumeStatus(vr, volName, replica, VolumeStatusCompleted, "PVC replaced")

	// IMPORTANT: Persist the status NOW before recreating STS
	// This ensures we don't re-process this replica if reconcile is interrupted
	if err := r.Status().Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}

	// Recreate STS to bring this replica back online
	// (other replicas are still running with old PVCs - that's fine)
	stsSpec, err := getSTSFromBackupConfigMap(ctx, r.Client, vr.Namespace, vr.Name)
	if err != nil {
		return r.setFailed(ctx, vr, fmt.Sprintf("failed to get STS from backup: %v", err))
	}

	// Update volumeClaimTemplates with new sizes
	for i := range stsSpec.Spec.VolumeClaimTemplates {
		vct := &stsSpec.Spec.VolumeClaimTemplates[i]
		for _, v := range vr.Spec.Volumes {
			if vct.Name == v.Name {
				vct.Spec.Resources.Requests[corev1.ResourceStorage] = v.NewSize
				if v.StorageClass != nil {
					vct.Spec.StorageClassName = v.StorageClass
				}
			}
		}
	}

	if err := recreateSTS(ctx, r.Client, stsSpec); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return r.setFailed(ctx, vr, fmt.Sprintf("failed to recreate STS: %v", err))
		}
	}
	log.Info("StatefulSet recreated, waiting for pod to come back", "replica", replica)

	// Clear the STS deleted annotation so next replica can delete it again
	delete(vr.Annotations, AnnotationSTSDeleted)
	if err := r.Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}

	// Wait for the pod to come back online before proceeding
	podName = getPodName(vr.Spec.StatefulSetName, replica)
	pod := &corev1.Pod{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: vr.Namespace, Name: podName}, pod); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Waiting for pod to be recreated", "pod", podName)
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}
	if pod.Status.Phase != corev1.PodRunning {
		log.Info("Waiting for pod to be running", "pod", podName, "phase", pod.Status.Phase)
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	log.Info("Replica migration complete, pod is back online", "replica", replica)

	// Advance to next volume/replica
	nextReplica, nextVol, done := r.getNextVolumeReplica(vr, replica, volName)
	if done {
		// All replicas done - go to Completed (skip Replacing phase)
		vr.Status.Phase = PhaseCompleted
		now := metav1.Now()
		vr.Status.CompletionTime = &now
		vr.Status.Message = MessageMigrationCompleted
		vr.Status.CurrentReplica = nil
		vr.Status.CurrentVolume = ""
		log.Info(MessageMigrationCompleted)
	} else {
		vr.Status.CurrentReplica = &nextReplica
		vr.Status.CurrentVolume = nextVol
		vr.Status.Message = fmt.Sprintf("Migrating replica %d volume %s", nextReplica, nextVol)
	}

	if err := r.Status().Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{Requeue: true}, nil
}

// handleReplacing is kept for backwards compatibility but the new flow
// handles PVC replacement directly in handleSyncing after each replica
func (r *VolumeResizeReconciler) handleReplacing(ctx context.Context, vr *storagev1alpha1.VolumeResize) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("In Replacing phase - this should not happen with new flow, marking complete")

	// If we end up here, just mark as complete
	now := metav1.Now()
	vr.Status.Phase = PhaseCompleted
	vr.Status.CompletionTime = &now
	vr.Status.Message = MessageMigrationCompleted

	if err := r.Status().Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// handleDeletion handles cleanup when VolumeResize is deleted
func (r *VolumeResizeReconciler) handleDeletion(ctx context.Context, vr *storagev1alpha1.VolumeResize) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Handling deletion")

	// List and delete temp PVCs
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList, client.InNamespace(vr.Namespace), client.MatchingLabels{
		LabelMigrationName: vr.Name,
	}); err == nil {
		for _, pvc := range pvcList.Items {
			if err := r.Delete(ctx, &pvc); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete temp PVC", "pvc", pvc.Name)
			}
		}
	}

	// List and delete migrator pods
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(vr.Namespace), client.MatchingLabels{
		LabelMigrationName: vr.Name,
	}); err == nil {
		for _, pod := range podList.Items {
			if err := r.Delete(ctx, &pod); err != nil && !apierrors.IsNotFound(err) {
				log.Error(err, "Failed to delete migrator pod", "pod", pod.Name)
			}
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(vr, FinalizerName)
	if err := r.Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// Helper functions

func ptrInt32(i int32) *int32 {
	return &i
}

func (r *VolumeResizeReconciler) setFailed(ctx context.Context, vr *storagev1alpha1.VolumeResize, message string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Error(nil, "Migration failed", "message", message)

	vr.Status.Phase = PhaseFailed
	vr.Status.Message = message

	if err := r.Status().Update(ctx, vr); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *VolumeResizeReconciler) updateVolumeStatus(vr *storagev1alpha1.VolumeResize, volName string, replica int32, phase, message string) {
	for i := range vr.Status.VolumeStatuses {
		vs := &vr.Status.VolumeStatuses[i]
		if vs.VolumeName == volName && vs.Replica == replica {
			vs.Phase = phase
			vs.Message = message
			return
		}
	}
}

func updateVolumeStatusInList(statuses []storagev1alpha1.VolumeStatus, volName string, replica int32, updateFn func(*storagev1alpha1.VolumeStatus)) []storagev1alpha1.VolumeStatus {
	for i := range statuses {
		if statuses[i].VolumeName == volName && statuses[i].Replica == replica {
			updateFn(&statuses[i])
		}
	}
	return statuses
}

func (r *VolumeResizeReconciler) getNextVolumeReplica(vr *storagev1alpha1.VolumeResize, currentReplica int32, currentVol string) (int32, string, bool) {
	// Find current volume index
	volIdx := 0
	for i, v := range vr.Spec.Volumes {
		if v.Name == currentVol {
			volIdx = i
			break
		}
	}

	// Try next volume for same replica
	if volIdx+1 < len(vr.Spec.Volumes) {
		return currentReplica, vr.Spec.Volumes[volIdx+1].Name, false
	}

	// Try next replica with first volume
	// Get total replicas from volume statuses
	maxReplica := int32(0)
	for _, vs := range vr.Status.VolumeStatuses {
		if vs.Replica > maxReplica {
			maxReplica = vs.Replica
		}
	}

	if currentReplica < maxReplica {
		return currentReplica + 1, vr.Spec.Volumes[0].Name, false
	}

	// All done
	return 0, "", true
}

// SetupWithManager sets up the controller with the Manager.
func (r *VolumeResizeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1alpha1.VolumeResize{}).
		Named("volumeresize").
		Complete(r)
}
