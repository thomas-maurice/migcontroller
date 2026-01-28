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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

func TestGetOriginalPVCName(t *testing.T) {
	tests := []struct {
		volumeName string
		stsName    string
		replica    int32
		expected   string
	}{
		{"data", "myapp", 0, "data-myapp-0"},
		{"data", "myapp", 1, "data-myapp-1"},
		{"logs", "test-sts", 2, "logs-test-sts-2"},
	}

	for _, tt := range tests {
		result := getOriginalPVCName(tt.volumeName, tt.stsName, tt.replica)
		assert.Equal(t, tt.expected, result)
	}
}

func TestGetTempPVCName(t *testing.T) {
	tests := []struct {
		volumeName string
		stsName    string
		replica    int32
		expected   string
	}{
		{"data", "myapp", 0, "data-myapp-0-new"},
		{"data", "myapp", 1, "data-myapp-1-new"},
		{"logs", "test-sts", 2, "logs-test-sts-2-new"},
	}

	for _, tt := range tests {
		result := getTempPVCName(tt.volumeName, tt.stsName, tt.replica)
		assert.Equal(t, tt.expected, result)
	}
}

func TestCreateTempPVC(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, storagev1alpha1.AddToScheme(scheme))

	storageClass := "standard"
	originalPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-sts-0",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
		Spec: storagev1alpha1.VolumeResizeSpec{
			StatefulSetName: "test-sts",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(originalPVC).Build()
	ctx := context.Background()

	tempPVC, err := createTempPVC(ctx, c, vr, vol, originalPVC, 0)
	require.NoError(t, err)
	assert.NotNil(t, tempPVC)
	assert.Equal(t, "data-test-sts-0-new", tempPVC.Name)
	assert.Equal(t, resource.MustParse("500Mi"), tempPVC.Spec.Resources.Requests[corev1.ResourceStorage])
	assert.Equal(t, storageClass, *tempPVC.Spec.StorageClassName)
	assert.Equal(t, "test-resize", tempPVC.Labels[LabelMigrationName])
}

func TestCreateTempPVCIdempotent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, storagev1alpha1.AddToScheme(scheme))

	storageClass := "standard"
	originalPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-sts-0",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
		},
	}

	existingTempPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-sts-0-new",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
		},
	}

	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-resize",
			Namespace: "default",
		},
		Spec: storagev1alpha1.VolumeResizeSpec{
			StatefulSetName: "test-sts",
		},
	}

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(originalPVC, existingTempPVC).Build()
	ctx := context.Background()

	// Should return existing PVC without error
	tempPVC, err := createTempPVC(ctx, c, vr, vol, originalPVC, 0)
	require.NoError(t, err)
	assert.NotNil(t, tempPVC)
	assert.Equal(t, "data-test-sts-0-new", tempPVC.Name)
}

func TestSetRetainOnPV(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv-test",
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pv).Build()
	ctx := context.Background()

	err := setRetainOnPV(ctx, c, "pv-test")
	require.NoError(t, err)

	// Verify PV was updated
	updatedPV := &corev1.PersistentVolume{}
	err = c.Get(ctx, client.ObjectKey{Name: "pv-test"}, updatedPV)
	require.NoError(t, err)
	assert.Equal(t, corev1.PersistentVolumeReclaimRetain, updatedPV.Spec.PersistentVolumeReclaimPolicy)
}

func TestSetRetainOnPVIdempotent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pv-test",
		},
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pv).Build()
	ctx := context.Background()

	// Should succeed without error even if already Retain
	err := setRetainOnPV(ctx, c, "pv-test")
	require.NoError(t, err)
}
