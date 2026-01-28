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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

// getOriginalPVCName returns the PVC name for a StatefulSet volume and replica
// Format: <volumeName>-<stsName>-<replica>
func getOriginalPVCName(volumeName, stsName string, replica int32) string {
	return fmt.Sprintf("%s-%s-%d", volumeName, stsName, replica)
}

// getTempPVCName returns the temporary PVC name used during migration
// Format: <volumeName>-<stsName>-<replica>-new
func getTempPVCName(volumeName, stsName string, replica int32) string {
	return fmt.Sprintf("%s-%s-%d-new", volumeName, stsName, replica)
}

// getPVC retrieves a PVC by namespace and name
func getPVC(ctx context.Context, c client.Client, namespace, name string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, pvc)
	if err != nil {
		return nil, err
	}
	return pvc, nil
}

// createTempPVC creates a temporary PVC for migration with the new size
func createTempPVC(ctx context.Context, c client.Client, vr *storagev1alpha1.VolumeResize, vol storagev1alpha1.VolumeResizeTarget, originalPVC *corev1.PersistentVolumeClaim, replica int32) (*corev1.PersistentVolumeClaim, error) {
	tempPVCName := getTempPVCName(vol.Name, vr.Spec.StatefulSetName, replica)

	// Check if temp PVC already exists (idempotency)
	existingPVC, err := getPVC(ctx, c, vr.Namespace, tempPVCName)
	if err == nil {
		return existingPVC, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for existing temp PVC: %w", err)
	}

	// Determine storage class
	storageClassName := originalPVC.Spec.StorageClassName
	if vol.StorageClass != nil {
		storageClassName = vol.StorageClass
	}

	// Create new PVC with target size
	tempPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tempPVCName,
			Namespace: vr.Namespace,
			Labels: map[string]string{
				LabelMigrationName: vr.Name,
				LabelReplica:       fmt.Sprintf("%d", replica),
				LabelVolumeName:    vol.Name,
			},
			Annotations: map[string]string{
				AnnotationManagedBy: "volume-resize-operator",
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      originalPVC.Spec.AccessModes,
			StorageClassName: storageClassName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: vol.NewSize,
				},
			},
		},
	}

	if err := c.Create(ctx, tempPVC); err != nil {
		return nil, fmt.Errorf("failed to create temp PVC: %w", err)
	}

	return tempPVC, nil
}

// setRetainOnPV sets the reclaim policy of a PV to Retain
func setRetainOnPV(ctx context.Context, c client.Client, pvName string) error {
	pv := &corev1.PersistentVolume{}
	if err := c.Get(ctx, types.NamespacedName{Name: pvName}, pv); err != nil {
		return fmt.Errorf("failed to get PV %s: %w", pvName, err)
	}

	// Already Retain, nothing to do
	if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimRetain {
		return nil
	}

	pv.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	if err := c.Update(ctx, pv); err != nil {
		return fmt.Errorf("failed to update PV %s reclaim policy: %w", pvName, err)
	}

	return nil
}

// replacePVC replaces the original PVC with a new one bound to the new PV
func replacePVC(ctx context.Context, c client.Client, vr *storagev1alpha1.VolumeResize, vol storagev1alpha1.VolumeResizeTarget, replica int32) error {
	originalPVCName := getOriginalPVCName(vol.Name, vr.Spec.StatefulSetName, replica)
	tempPVCName := getTempPVCName(vol.Name, vr.Spec.StatefulSetName, replica)

	// Get temp PVC to find its bound PV
	tempPVC, err := getPVC(ctx, c, vr.Namespace, tempPVCName)
	if err != nil {
		return fmt.Errorf("failed to get temp PVC: %w", err)
	}

	newPVName := tempPVC.Spec.VolumeName
	if newPVName == "" {
		return fmt.Errorf("temp PVC %s is not bound to a PV", tempPVCName)
	}

	// Get original PVC to copy labels
	originalPVC, err := getPVC(ctx, c, vr.Namespace, originalPVCName)
	if err != nil {
		return fmt.Errorf("failed to get original PVC: %w", err)
	}
	originalLabels := originalPVC.Labels

	// IMPORTANT: Set Retain policy on new PV BEFORE deleting temp PVC
	// Otherwise the PV will be deleted along with the temp PVC
	newPV := &corev1.PersistentVolume{}
	if err := c.Get(ctx, types.NamespacedName{Name: newPVName}, newPV); err != nil {
		return fmt.Errorf("failed to get new PV: %w", err)
	}
	newPV.Spec.PersistentVolumeReclaimPolicy = corev1.PersistentVolumeReclaimRetain
	if err := c.Update(ctx, newPV); err != nil {
		return fmt.Errorf("failed to set retain policy on new PV: %w", err)
	}

	// Delete temp PVC (PV is now protected by Retain policy)
	if err := c.Delete(ctx, tempPVC); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete temp PVC: %w", err)
	}

	// Wait for temp PVC to be fully deleted
	for range 60 {
		_, err := getPVC(ctx, c, vr.Namespace, tempPVCName)
		if apierrors.IsNotFound(err) {
			break
		}
		if err != nil {
			return fmt.Errorf("error checking temp PVC deletion: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	// Re-fetch the PV and clear claimRef so it can be bound again
	if err := c.Get(ctx, types.NamespacedName{Name: newPVName}, newPV); err != nil {
		return fmt.Errorf("failed to get new PV after temp PVC deletion: %w", err)
	}
	newPV.Spec.ClaimRef = nil
	if err := c.Update(ctx, newPV); err != nil {
		return fmt.Errorf("failed to clear claimRef on new PV: %w", err)
	}

	// Delete original PVC
	if err := c.Delete(ctx, originalPVC); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete original PVC: %w", err)
	}

	// Wait for original PVC to be fully deleted
	for range 60 {
		_, err := getPVC(ctx, c, vr.Namespace, originalPVCName)
		if apierrors.IsNotFound(err) {
			break
		}
		if err != nil {
			return fmt.Errorf("error checking PVC deletion: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	// Create new PVC with original name, bound to new PV
	newPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      originalPVCName,
			Namespace: vr.Namespace,
			Labels:    originalLabels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      tempPVC.Spec.AccessModes,
			StorageClassName: tempPVC.Spec.StorageClassName,
			VolumeName:       newPVName,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: vol.NewSize,
				},
			},
		},
	}

	if err := c.Create(ctx, newPVC); err != nil {
		return fmt.Errorf("failed to create new PVC with original name: %w", err)
	}

	return nil
}
