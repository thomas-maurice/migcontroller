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

// buildMigratorPod creates the pod spec for the rclone migration pod
func buildMigratorPod(vr *storagev1alpha1.VolumeResize, vol storagev1alpha1.VolumeResizeTarget, replica int32, oldPVCName, newPVCName string) *corev1.Pod {
	podName := fmt.Sprintf("%s-migrator-%d-%s", vr.Name, replica, vol.Name)

	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
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
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			// Run as root to be able to read files owned by any user
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:  new(int64), // 0 = root
				RunAsGroup: new(int64), // 0 = root
			},
			Containers: []corev1.Container{
				{
					Name:            "migrator",
					Image:           DefaultMigratorImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
					Env: []corev1.EnvVar{
						{Name: "SOURCE_PATH", Value: "/source"},
						{Name: "DEST_PATH", Value: "/dest"},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "source",
							MountPath: "/source",
						},
						{
							Name:      "dest",
							MountPath: "/dest",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "source",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: oldPVCName,
						},
					},
				},
				{
					Name: "dest",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: newPVCName,
						},
					},
				},
			},
		},
	}
}

// createMigratorPod creates and runs the migration pod
func createMigratorPod(ctx context.Context, c client.Client, vr *storagev1alpha1.VolumeResize, vol storagev1alpha1.VolumeResizeTarget, replica int32, oldPVCName, newPVCName string) (*corev1.Pod, error) {
	pod := buildMigratorPod(vr, vol, replica, oldPVCName, newPVCName)

	// Check if pod already exists (idempotency)
	existingPod := &corev1.Pod{}
	err := c.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, existingPod)
	if err == nil {
		return existingPod, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for existing migrator pod: %w", err)
	}

	if err := c.Create(ctx, pod); err != nil {
		return nil, fmt.Errorf("failed to create migrator pod: %w", err)
	}

	return pod, nil
}

// waitForMigrationComplete waits for the migration pod to complete
func waitForMigrationComplete(ctx context.Context, c client.Client, podName, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		pod := &corev1.Pod{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: podName}, pod); err != nil {
			return fmt.Errorf("failed to get migration pod status: %w", err)
		}

		switch pod.Status.Phase {
		case corev1.PodSucceeded:
			return nil
		case corev1.PodFailed:
			return fmt.Errorf("migration pod failed")
		}

		time.Sleep(time.Second * 2)
	}

	return fmt.Errorf("timeout waiting for migration to complete")
}

// cleanupMigratorPod deletes the migration pod
func cleanupMigratorPod(ctx context.Context, c client.Client, podName, namespace string) error {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
	}

	if err := c.Delete(ctx, pod); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete migrator pod: %w", err)
	}

	return nil
}

// getMigratorPodName returns the expected name of the migrator pod
func getMigratorPodName(vrName, volName string, replica int32) string {
	return fmt.Sprintf("%s-migrator-%d-%s", vrName, replica, volName)
}
