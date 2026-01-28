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
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

// deepCopySTSSpec creates a deep copy of the StatefulSet for later recreation
func deepCopySTSSpec(sts *appsv1.StatefulSet) *appsv1.StatefulSet {
	return sts.DeepCopy()
}

// deleteSTSOrphan deletes the StatefulSet with orphan propagation policy
// This leaves the pods running while the StatefulSet is deleted
func deleteSTSOrphan(ctx context.Context, c client.Client, sts *appsv1.StatefulSet) (*appsv1.StatefulSet, error) {
	// Deep copy the STS spec before deletion
	stsCopy := deepCopySTSSpec(sts)

	// Delete with orphan propagation
	propagation := metav1.DeletePropagationOrphan
	if err := c.Delete(ctx, sts, &client.DeleteOptions{
		PropagationPolicy: &propagation,
	}); err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to delete StatefulSet with orphan policy: %w", err)
	}

	return stsCopy, nil
}

// getPodName returns the pod name for a StatefulSet replica
func getPodName(stsName string, replica int32) string {
	return fmt.Sprintf("%s-%d", stsName, replica)
}

// deletePod deletes a specific pod by name
func deletePod(ctx context.Context, c client.Client, namespace, stsName string, replica int32) error {
	podName := getPodName(stsName, replica)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
	}

	if err := c.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete pod %s: %w", podName, err)
	}

	return nil
}

// waitForPodTermination waits for a pod to be fully terminated
func waitForPodTermination(ctx context.Context, c client.Client, namespace, podName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pod := &corev1.Pod{}
		err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod)
		if apierrors.IsNotFound(err) {
			return nil // Pod is gone
		}
		if err != nil {
			return fmt.Errorf("error checking pod status: %w", err)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for pod %s to terminate", podName)
}

// getSTSBackupConfigMapName returns the name of the ConfigMap used to backup the STS spec
func getSTSBackupConfigMapName(vrName string) string {
	return fmt.Sprintf("%s-sts-backup", vrName)
}

// backupSTSToConfigMap creates a ConfigMap containing the StatefulSet spec for safe recovery
func backupSTSToConfigMap(ctx context.Context, c client.Client, vr *storagev1alpha1.VolumeResize, sts *appsv1.StatefulSet) error {
	cmName := getSTSBackupConfigMapName(vr.Name)

	// Check if backup already exists
	existing := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Namespace: vr.Namespace, Name: cmName}, existing)
	if err == nil {
		// Already exists, nothing to do
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check for existing backup ConfigMap: %w", err)
	}

	// Serialize the STS
	stsJSON, err := json.Marshal(sts)
	if err != nil {
		return fmt.Errorf("failed to serialize StatefulSet: %w", err)
	}

	// Create ConfigMap with the backup
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: vr.Namespace,
			Labels: map[string]string{
				LabelMigrationName: vr.Name,
			},
			Annotations: map[string]string{
				AnnotationManagedBy: "volume-resize-operator",
			},
		},
		Data: map[string]string{
			ConfigMapKeySTSSpec: string(stsJSON),
		},
	}

	if err := c.Create(ctx, cm); err != nil {
		return fmt.Errorf("failed to create backup ConfigMap: %w", err)
	}

	return nil
}

// getSTSFromBackupConfigMap retrieves the StatefulSet spec from the backup ConfigMap
func getSTSFromBackupConfigMap(ctx context.Context, c client.Client, namespace, vrName string) (*appsv1.StatefulSet, error) {
	cmName := getSTSBackupConfigMapName(vrName)

	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: cmName}, cm); err != nil {
		return nil, fmt.Errorf("failed to get backup ConfigMap: %w", err)
	}

	stsJSON, ok := cm.Data[ConfigMapKeySTSSpec]
	if !ok {
		return nil, fmt.Errorf("backup ConfigMap missing %s key", ConfigMapKeySTSSpec)
	}

	sts := &appsv1.StatefulSet{}
	if err := json.Unmarshal([]byte(stsJSON), sts); err != nil {
		return nil, fmt.Errorf("failed to deserialize StatefulSet: %w", err)
	}

	return sts, nil
}

// recreateSTS recreates a StatefulSet from a stored spec
func recreateSTS(ctx context.Context, c client.Client, stsSpec *appsv1.StatefulSet) error {
	// Clear server-set fields
	newSTS := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        stsSpec.Name,
			Namespace:   stsSpec.Namespace,
			Labels:      stsSpec.Labels,
			Annotations: stsSpec.Annotations,
		},
		Spec: stsSpec.Spec,
	}

	// Remove resourceVersion, UID, and other server-generated fields
	newSTS.ResourceVersion = ""
	newSTS.UID = ""
	newSTS.CreationTimestamp = metav1.Time{}
	newSTS.Generation = 0
	newSTS.ManagedFields = nil

	if err := c.Create(ctx, newSTS); err != nil {
		return fmt.Errorf("failed to recreate StatefulSet: %w", err)
	}

	return nil
}
