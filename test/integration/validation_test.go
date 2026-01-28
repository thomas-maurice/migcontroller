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
	"time"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

// TestValidationFailureSizeNotSmaller tests size validation enforcement
func (s *IntegrationTestSuite) TestValidationFailureSizeNotSmaller() {
	// Create STS with 500Mi volume
	s.createTestStatefulSet("test-sts-size", 1, []string{"data"}, []string{"500Mi"})
	s.waitForSTSReady("test-sts-size", time.Minute*2)

	// Create VolumeResize with newSize: 1Gi (larger than current)
	s.createVolumeResize("resize-size", "test-sts-size", []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("1Gi")},
	})

	// Wait for Failed phase
	s.waitForVolumeResizePhase("resize-size", "Failed", time.Minute*2)

	// Verify message mentions size
	vr := &storagev1alpha1.VolumeResize{}
	err := s.client.Get(s.ctx, types.NamespacedName{Namespace: s.namespace, Name: "resize-size"}, vr)
	require.NoError(s.T(), err)
	s.Contains(vr.Status.Message, "smaller")
}

// TestValidationFailureVolumeNotFound tests volume target validation
func (s *IntegrationTestSuite) TestValidationFailureVolumeNotFound() {
	// Create STS with volume named "data"
	s.createTestStatefulSet("test-sts-vol", 1, []string{"data"}, []string{"1Gi"})
	s.waitForSTSReady("test-sts-vol", time.Minute*2)

	// Create VolumeResize targeting volume "nonexistent"
	s.createVolumeResize("resize-vol", "test-sts-vol", []storagev1alpha1.VolumeResizeTarget{
		{Name: "nonexistent", NewSize: resource.MustParse("500Mi")},
	})

	// Wait for Failed phase
	s.waitForVolumeResizePhase("resize-vol", "Failed", time.Minute*2)

	// Verify message mentions volume not found
	vr := &storagev1alpha1.VolumeResize{}
	err := s.client.Get(s.ctx, types.NamespacedName{Namespace: s.namespace, Name: "resize-vol"}, vr)
	require.NoError(s.T(), err)
	s.Contains(vr.Status.Message, "not found")
}

// TestValidationFailurePDBBlocking tests PDB enforcement
func (s *IntegrationTestSuite) TestValidationFailurePDBBlocking() {
	// Create STS with 2 replicas
	s.createTestStatefulSet("test-sts-pdb", 2, []string{"data"}, []string{"1Gi"})
	s.waitForSTSReady("test-sts-pdb", time.Minute*2)

	// Create PDB with minAvailable: 2 (no disruption allowed)
	s.createPDB("test-pdb", map[string]string{"app": "test-sts-pdb"}, 2)

	// Wait for PDB status to update
	time.Sleep(time.Second * 5)

	// Create VolumeResize
	s.createVolumeResize("resize-pdb", "test-sts-pdb", []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("500Mi")},
	})

	// Wait for Failed phase
	s.waitForVolumeResizePhase("resize-pdb", "Failed", time.Minute*2)

	// Verify message mentions PDB
	vr := &storagev1alpha1.VolumeResize{}
	err := s.client.Get(s.ctx, types.NamespacedName{Namespace: s.namespace, Name: "resize-pdb"}, vr)
	require.NoError(s.T(), err)
	s.Contains(vr.Status.Message, "PodDisruptionBudget")
}

// TestValidationFailureStatefulSetNotFound tests StatefulSet validation
func (s *IntegrationTestSuite) TestValidationFailureStatefulSetNotFound() {
	// Create VolumeResize targeting non-existent StatefulSet
	s.createVolumeResize("resize-noexist", "nonexistent-sts", []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("500Mi")},
	})

	// Wait for Failed phase
	s.waitForVolumeResizePhase("resize-noexist", "Failed", time.Minute*2)

	// Verify message mentions StatefulSet not found
	vr := &storagev1alpha1.VolumeResize{}
	err := s.client.Get(s.ctx, types.NamespacedName{Namespace: s.namespace, Name: "resize-noexist"}, vr)
	require.NoError(s.T(), err)
	s.Contains(vr.Status.Message, "not found")
}
