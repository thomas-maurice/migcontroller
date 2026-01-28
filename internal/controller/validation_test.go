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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

func TestValidateStatefulSetNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	sts, result := validateStatefulSetExists(ctx, c, "default", "nonexistent")
	assert.Nil(t, sts)
	assert.False(t, result.Valid)
	assert.Contains(t, result.Message, "not found")
}

func TestValidateStatefulSetExists(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
	ctx := context.Background()

	foundSts, result := validateStatefulSetExists(ctx, c, "default", "test-sts")
	assert.NotNil(t, foundSts)
	assert.True(t, result.Valid)
}

func TestValidateVolumeTargetNotFound(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
				},
			},
		},
	}

	volumes := []storagev1alpha1.VolumeResizeTarget{
		{Name: "nonexistent", NewSize: resource.MustParse("500Mi")},
	}

	result := validateVolumeTargets(sts, volumes)
	assert.False(t, result.Valid)
	assert.Contains(t, result.Message, "nonexistent")
	assert.Contains(t, result.Message, "not found")
}

func TestValidateVolumeTargetFound(t *testing.T) {
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "logs"},
				},
			},
		},
	}

	volumes := []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("500Mi")},
	}

	result := validateVolumeTargets(sts, volumes)
	assert.True(t, result.Valid)
}

func TestValidateSizeReductionInvalid(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Current PVC has 500Mi, trying to resize to 1Gi (larger)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-sts-0",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("500Mi"),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	ctx := context.Background()

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("1Gi"),
	}

	result := validateSizeReduction(ctx, c, "default", "test-sts", vol)
	assert.False(t, result.Valid)
	assert.Contains(t, result.Message, "must be smaller")
}

func TestValidateSizeReductionValid(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Current PVC has 1Gi, trying to resize to 500Mi (smaller)
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-test-sts-0",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
	ctx := context.Background()

	vol := storagev1alpha1.VolumeResizeTarget{
		Name:    "data",
		NewSize: resource.MustParse("500Mi"),
	}

	result := validateSizeReduction(ctx, c, "default", "test-sts", vol)
	assert.True(t, result.Valid)
}

func TestValidatePDBBlockingDisruption(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
			},
		},
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pdb",
			Namespace: "default",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 0,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pdb).Build()
	ctx := context.Background()

	result := validatePDBAllowsDisruption(ctx, c, sts)
	assert.False(t, result.Valid)
	assert.Contains(t, result.Message, "does not allow disruptions")
}

func TestValidatePDBAllowingDisruption(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
			},
		},
	}

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pdb",
			Namespace: "default",
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			DisruptionsAllowed: 1,
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pdb).Build()
	ctx := context.Background()

	result := validatePDBAllowsDisruption(ctx, c, sts)
	assert.True(t, result.Valid)
}

func TestValidateNoPDBPresent(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, policyv1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts",
			Namespace: "default",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	result := validatePDBAllowsDisruption(ctx, c, sts)
	assert.True(t, result.Valid)
}
