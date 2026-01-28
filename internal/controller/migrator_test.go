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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

func TestBuildMigratorPodVolumeMounts(t *testing.T) {
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	pod := buildMigratorPod(vr, vol, 0, "data-test-sts-0", "data-test-sts-0-new")

	// Verify volume mounts
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 2)

	sourceMount := pod.Spec.Containers[0].VolumeMounts[0]
	assert.Equal(t, "source", sourceMount.Name)
	assert.Equal(t, "/source", sourceMount.MountPath)

	destMount := pod.Spec.Containers[0].VolumeMounts[1]
	assert.Equal(t, "dest", destMount.Name)
	assert.Equal(t, "/dest", destMount.MountPath)
}

func TestBuildMigratorPodEnvVars(t *testing.T) {
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	pod := buildMigratorPod(vr, vol, 0, "data-test-sts-0", "data-test-sts-0-new")

	// Verify env vars
	envVars := pod.Spec.Containers[0].Env
	require.Len(t, envVars, 2)

	var sourceEnv, destEnv corev1.EnvVar
	for _, env := range envVars {
		if env.Name == "SOURCE_PATH" {
			sourceEnv = env
		}
		if env.Name == "DEST_PATH" {
			destEnv = env
		}
	}

	assert.Equal(t, "/source", sourceEnv.Value)
	assert.Equal(t, "/dest", destEnv.Value)
}

func TestBuildMigratorPodImage(t *testing.T) {
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	pod := buildMigratorPod(vr, vol, 0, "data-test-sts-0", "data-test-sts-0-new")

	assert.Equal(t, DefaultMigratorImage, pod.Spec.Containers[0].Image)
}

func TestBuildMigratorPodLabels(t *testing.T) {
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	pod := buildMigratorPod(vr, vol, 0, "data-test-sts-0", "data-test-sts-0-new")

	assert.Equal(t, "test-resize", pod.Labels[LabelMigrationName])
	assert.Equal(t, "0", pod.Labels[LabelReplica])
	assert.Equal(t, "data", pod.Labels[LabelVolumeName])
}

func TestMigratorPodNameFormat(t *testing.T) {
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	pod := buildMigratorPod(vr, vol, 1, "data-test-sts-1", "data-test-sts-1-new")

	assert.Equal(t, "test-resize-migrator-1-data", pod.Name)
}

func TestGetMigratorPodName(t *testing.T) {
	tests := []struct {
		vrName   string
		volName  string
		replica  int32
		expected string
	}{
		{"resize-job", "data", 0, "resize-job-migrator-0-data"},
		{"resize-job", "data", 1, "resize-job-migrator-1-data"},
		{"my-resize", "logs", 2, "my-resize-migrator-2-logs"},
	}

	for _, tt := range tests {
		result := getMigratorPodName(tt.vrName, tt.volName, tt.replica)
		assert.Equal(t, tt.expected, result)
	}
}

func TestCreateMigratorPodIdempotent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, storagev1alpha1.AddToScheme(scheme))

	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize-migrator-0-data",
			Namespace: "default",
		},
	}

	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPod).Build()
	ctx := context.Background()

	// Should return existing pod without error
	pod, err := createMigratorPod(ctx, c, vr, vol, 0, "data-test-sts-0", "data-test-sts-0-new")
	require.NoError(t, err)
	assert.NotNil(t, pod)
	assert.Equal(t, "test-resize-migrator-0-data", pod.Name)
}

func TestCleanupMigratorPod(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	ctx := context.Background()

	err := cleanupMigratorPod(ctx, c, "test-pod", "default")
	require.NoError(t, err)
}

func TestCleanupMigratorPodNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	// Should not error if pod doesn't exist
	err := cleanupMigratorPod(ctx, c, "nonexistent", "default")
	require.NoError(t, err)
}
