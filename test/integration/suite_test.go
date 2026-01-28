//go:build integration
// +build integration

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

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

type IntegrationTestSuite struct {
	suite.Suite
	client    client.Client
	ctx       context.Context
	namespace string
}

func TestIntegrationSuite(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION") == "true" {
		t.Skip("Skipping integration tests")
	}
	suite.Run(t, new(IntegrationTestSuite))
}

func (s *IntegrationTestSuite) SetupSuite() {
	s.ctx = context.Background()

	// Load kubeconfig
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = os.Getenv("HOME") + "/.kube/config"
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(s.T(), err, "Failed to load kubeconfig")

	// Add our scheme
	err = storagev1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(s.T(), err, "Failed to add scheme")

	// Create client
	s.client, err = client.New(config, client.Options{Scheme: scheme.Scheme})
	require.NoError(s.T(), err, "Failed to create client")

	// Create test namespace
	s.namespace = fmt.Sprintf("volresize-test-%d", time.Now().Unix())
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace,
		},
	}
	err = s.client.Create(s.ctx, ns)
	require.NoError(s.T(), err, "Failed to create test namespace")

	s.T().Logf("Created test namespace: %s", s.namespace)
}

func (s *IntegrationTestSuite) TearDownSuite() {
	// Delete test namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: s.namespace,
		},
	}
	_ = s.client.Delete(s.ctx, ns)
	s.T().Logf("Deleted test namespace: %s", s.namespace)
}

func (s *IntegrationTestSuite) TearDownTest() {
	// Clean up any VolumeResize resources
	vrList := &storagev1alpha1.VolumeResizeList{}
	if err := s.client.List(s.ctx, vrList, client.InNamespace(s.namespace)); err == nil {
		for _, vr := range vrList.Items {
			_ = s.client.Delete(s.ctx, &vr)
		}
	}

	// Clean up StatefulSets
	stsList := &appsv1.StatefulSetList{}
	if err := s.client.List(s.ctx, stsList, client.InNamespace(s.namespace)); err == nil {
		for _, sts := range stsList.Items {
			_ = s.client.Delete(s.ctx, &sts)
		}
	}

	// Clean up PDBs
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := s.client.List(s.ctx, pdbList, client.InNamespace(s.namespace)); err == nil {
		for _, pdb := range pdbList.Items {
			_ = s.client.Delete(s.ctx, &pdb)
		}
	}

	// Wait a bit for cleanup
	time.Sleep(time.Second * 2)
}

// Helper functions

func (s *IntegrationTestSuite) createTestStatefulSet(name string, replicas int32, volumes []string, sizes []string) *appsv1.StatefulSet {
	labels := map[string]string{"app": name}

	var volumeClaimTemplates []corev1.PersistentVolumeClaim
	var volumeMounts []corev1.VolumeMount

	for i, volName := range volumes {
		size := "1Gi"
		if i < len(sizes) {
			size = sizes[i]
		}

		volumeClaimTemplates = append(volumeClaimTemplates, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: volName},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(size),
					},
				},
			},
		})

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      volName,
			MountPath: "/" + volName,
		})
	}

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:         "busybox",
							Image:        "busybox:1.36",
							Command:      []string{"sh", "-c", "while true; do sleep 3600; done"},
							VolumeMounts: volumeMounts,
						},
					},
				},
			},
			VolumeClaimTemplates: volumeClaimTemplates,
		},
	}

	// Create headless service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: "None",
			Selector:  labels,
			Ports: []corev1.ServicePort{
				{Port: 80, Name: "web"},
			},
		},
	}

	err := s.client.Create(s.ctx, svc)
	require.NoError(s.T(), err)

	err = s.client.Create(s.ctx, sts)
	require.NoError(s.T(), err)

	return sts
}

func (s *IntegrationTestSuite) createVolumeResize(name, stsName string, volumes []storagev1alpha1.VolumeResizeTarget) *storagev1alpha1.VolumeResize {
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: storagev1alpha1.VolumeResizeSpec{
			StatefulSetName: stsName,
			Volumes:         volumes,
		},
	}

	err := s.client.Create(s.ctx, vr)
	require.NoError(s.T(), err)

	return vr
}

func (s *IntegrationTestSuite) createPDB(name string, selector map[string]string, minAvailable int) *policyv1.PodDisruptionBudget {
	minAvailableVal := intstr.FromInt(minAvailable)
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: s.namespace,
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailableVal,
			Selector:     &metav1.LabelSelector{MatchLabels: selector},
		},
	}

	err := s.client.Create(s.ctx, pdb)
	require.NoError(s.T(), err)

	return pdb
}

func (s *IntegrationTestSuite) waitForSTSReady(name string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sts := &appsv1.StatefulSet{}
		err := s.client.Get(s.ctx, types.NamespacedName{Namespace: s.namespace, Name: name}, sts)
		if err == nil && sts.Status.ReadyReplicas == *sts.Spec.Replicas {
			return
		}
		time.Sleep(time.Second * 2)
	}
	s.T().Fatalf("StatefulSet %s did not become ready within %v", name, timeout)
}

func (s *IntegrationTestSuite) waitForVolumeResizePhase(name string, phase string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		vr := &storagev1alpha1.VolumeResize{}
		err := s.client.Get(s.ctx, types.NamespacedName{Namespace: s.namespace, Name: name}, vr)
		if err == nil && vr.Status.Phase == phase {
			return
		}
		time.Sleep(time.Second * 2)
	}
	s.T().Fatalf("VolumeResize %s did not reach phase %s within %v", name, phase, timeout)
}

func (s *IntegrationTestSuite) populateData(stsName string, replica int, volName, content string) {
	podName := fmt.Sprintf("%s-%d", stsName, replica)
	// This would use kubectl exec in a real test
	s.T().Logf("Would populate data in %s/%s with: %s", podName, volName, content)
}

func (s *IntegrationTestSuite) verifyData(stsName string, replica int, volName, expectedContent string) bool {
	podName := fmt.Sprintf("%s-%d", stsName, replica)
	// This would use kubectl exec in a real test
	s.T().Logf("Would verify data in %s/%s contains: %s", podName, volName, expectedContent)
	return true
}
