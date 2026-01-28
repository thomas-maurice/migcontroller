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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeepCopySTSSpec(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sts",
			Namespace:       "default",
			ResourceVersion: "12345",
			UID:             "abc-123",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptrInt32(3),
		},
	}

	copy := deepCopySTSSpec(sts)
	assert.Equal(t, sts.Name, copy.Name)
	assert.Equal(t, sts.Namespace, copy.Namespace)
	assert.Equal(t, *sts.Spec.Replicas, *copy.Spec.Replicas)

	// Verify it's a deep copy
	*copy.Spec.Replicas = 5
	assert.NotEqual(t, *sts.Spec.Replicas, *copy.Spec.Replicas)
}

func TestGetPodName(t *testing.T) {
	tests := []struct {
		stsName  string
		replica  int32
		expected string
	}{
		{"myapp", 0, "myapp-0"},
		{"myapp", 1, "myapp-1"},
		{"test-sts", 2, "test-sts-2"},
	}

	for _, tt := range tests {
		result := getPodName(tt.stsName, tt.replica)
		assert.Equal(t, tt.expected, result)
	}
}

func TestDeleteSTSOrphanReturnsValidSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sts",
			Namespace:       "default",
			ResourceVersion: "12345",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptrInt32(2),
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts).Build()
	ctx := context.Background()

	specCopy, err := deleteSTSOrphan(ctx, c, sts)
	require.NoError(t, err)
	assert.NotNil(t, specCopy)
	assert.Equal(t, "test-sts", specCopy.Name)
	assert.Equal(t, int32(2), *specCopy.Spec.Replicas)
}

func TestRecreateSTS(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))

	stsSpec := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sts",
			Namespace:       "default",
			ResourceVersion: "12345",
			UID:             "old-uid",
			Labels:          map[string]string{"app": "test"},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptrInt32(2),
			ServiceName: "test-svc",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "busybox"},
					},
				},
			},
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	ctx := context.Background()

	err := recreateSTS(ctx, c, stsSpec)
	require.NoError(t, err)

	// Verify STS was created
	newSTS := &appsv1.StatefulSet{}
	err = c.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test-sts"}, newSTS)
	require.NoError(t, err)
	assert.Equal(t, "test-sts", newSTS.Name)
	assert.NotEqual(t, "old-uid", string(newSTS.UID)) // UID should be different
	assert.Equal(t, int32(2), *newSTS.Spec.Replicas)
	assert.Equal(t, "test", newSTS.Labels["app"])
}

func TestDeletePod(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-sts-0",
			Namespace: "default",
		},
	}

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	ctx := context.Background()

	err := deletePod(ctx, c, "default", "test-sts", 0)
	require.NoError(t, err)
}
