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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPhaseConstants(t *testing.T) {
	phases := []string{
		PhasePending,
		PhaseValidating,
		PhaseSyncing,
		PhaseReplacing,
		PhaseCompleted,
		PhaseFailed,
	}

	for _, phase := range phases {
		assert.NotEmpty(t, phase, "Phase constant should not be empty")
	}
}

func TestVolumeStatusConstants(t *testing.T) {
	statuses := []string{
		VolumeStatusPending,
		VolumeStatusSyncing,
		VolumeStatusSynced,
		VolumeStatusReplacing,
		VolumeStatusCompleted,
		VolumeStatusFailed,
	}

	for _, status := range statuses {
		assert.NotEmpty(t, status, "Volume status constant should not be empty")
	}
}

func TestConditionTypeConstants(t *testing.T) {
	conditions := []string{
		ConditionTypeReady,
		ConditionTypeValidated,
		ConditionTypeProgressing,
	}

	for _, condition := range conditions {
		assert.NotEmpty(t, condition, "Condition type constant should not be empty")
	}
}

func TestAnnotationKeysFollowKubernetesNaming(t *testing.T) {
	annotations := []string{
		AnnotationOldPVName,
		AnnotationManagedBy,
		AnnotationSTSDeleted,
		AnnotationSTSBackup,
	}

	for _, annotation := range annotations {
		assert.NotEmpty(t, annotation, "Annotation key should not be empty")
		assert.Contains(t, annotation, "maurice.fr", "Annotation should contain domain")
		assert.Contains(t, annotation, "/", "Annotation should have prefix/name format")
	}
}

func TestLabelKeysFollowKubernetesNaming(t *testing.T) {
	labels := []string{
		LabelMigrationName,
		LabelReplica,
		LabelVolumeName,
	}

	for _, label := range labels {
		assert.NotEmpty(t, label, "Label key should not be empty")
		assert.Contains(t, label, "maurice.fr", "Label should contain domain")
		assert.Contains(t, label, "/", "Label should have prefix/name format")
	}
}

func TestFinalizerFollowsKubernetesNaming(t *testing.T) {
	assert.NotEmpty(t, FinalizerName, "Finalizer name should not be empty")
	assert.Contains(t, FinalizerName, "maurice.fr", "Finalizer should contain domain")
}

func TestDefaultMigratorImage(t *testing.T) {
	assert.NotEmpty(t, DefaultMigratorImage, "Default migrator image should not be empty")
	assert.True(t, strings.Contains(DefaultMigratorImage, ":"), "Image should have a tag")
}
