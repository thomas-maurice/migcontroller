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

	"k8s.io/apimachinery/pkg/api/resource"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

// TestHappyPathSingleVolumeSingleReplica tests the simplest migration scenario
func (s *IntegrationTestSuite) TestHappyPathSingleVolumeSingleReplica() {
	// Create STS with 1 replica, 1 volume (1Gi)
	s.createTestStatefulSet("test-sts-1", 1, []string{"data"}, []string{"1Gi"})
	s.waitForSTSReady("test-sts-1", time.Minute*2)

	// Populate data with marker file
	s.populateData("test-sts-1", 0, "data", "marker-replica-0")

	// Create VolumeResize CR (newSize: 500Mi)
	s.createVolumeResize("resize-1", "test-sts-1", []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("500Mi")},
	})

	// Wait for Completed phase
	s.waitForVolumeResizePhase("resize-1", "Completed", time.Minute*10)

	// Verify marker file exists in migrated volume
	s.True(s.verifyData("test-sts-1", 0, "data", "marker-replica-0"))
}

// TestHappyPathSingleVolumeMultipleReplicas tests sequential replica processing
func (s *IntegrationTestSuite) TestHappyPathSingleVolumeMultipleReplicas() {
	// Create STS with 3 replicas, 1 volume
	s.createTestStatefulSet("test-sts-3", 3, []string{"data"}, []string{"1Gi"})
	s.waitForSTSReady("test-sts-3", time.Minute*3)

	// Create unique marker per replica
	for i := 0; i < 3; i++ {
		s.populateData("test-sts-3", i, "data", "marker-replica-"+string(rune('0'+i)))
	}

	// Create VolumeResize CR
	s.createVolumeResize("resize-3", "test-sts-3", []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("500Mi")},
	})

	// Wait for completion
	s.waitForVolumeResizePhase("resize-3", "Completed", time.Minute*15)

	// Verify data integrity per replica
	for i := 0; i < 3; i++ {
		s.True(s.verifyData("test-sts-3", i, "data", "marker-replica-"+string(rune('0'+i))))
	}
}

// TestHappyPathMultipleVolumes tests multi-volume migration
func (s *IntegrationTestSuite) TestHappyPathMultipleVolumes() {
	// Create STS with 2 volumeClaimTemplates
	s.createTestStatefulSet("test-sts-mv", 2, []string{"data", "logs"}, []string{"1Gi", "500Mi"})
	s.waitForSTSReady("test-sts-mv", time.Minute*2)

	// Create VolumeResize targeting both volumes
	s.createVolumeResize("resize-mv", "test-sts-mv", []storagev1alpha1.VolumeResizeTarget{
		{Name: "data", NewSize: resource.MustParse("500Mi")},
		{Name: "logs", NewSize: resource.MustParse("250Mi")},
	})

	// Wait for completion
	s.waitForVolumeResizePhase("resize-mv", "Completed", time.Minute*15)
}
