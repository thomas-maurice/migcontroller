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

// Phase constants for VolumeResize status
const (
	PhasePending    = "Pending"
	PhaseValidating = "Validating"
	PhaseSyncing    = "Syncing"
	PhaseReplacing  = "Replacing"
	PhaseCompleted  = "Completed"
	PhaseFailed     = "Failed"
)

// Volume-level phase constants
const (
	VolumeStatusPending   = "Pending"
	VolumeStatusSyncing   = "Syncing"
	VolumeStatusSynced    = "Synced"
	VolumeStatusReplacing = "Replacing"
	VolumeStatusCompleted = "Completed"
	VolumeStatusFailed    = "Failed"
)

// Condition type constants
const (
	ConditionTypeReady       = "Ready"
	ConditionTypeValidated   = "Validated"
	ConditionTypeProgressing = "Progressing"
)

// Annotation keys
const (
	AnnotationOldPVName  = "storage.maurice.fr/old-pv-name"
	AnnotationManagedBy  = "storage.maurice.fr/managed-by"
	AnnotationSTSDeleted = "storage.maurice.fr/sts-deleted"
	AnnotationSTSBackup  = "storage.maurice.fr/sts-backup-cm"
)

// ConfigMap key for STS backup
const (
	ConfigMapKeySTSSpec = "statefulset.json"
)

// Label keys
const (
	LabelMigrationName = "storage.maurice.fr/migration-name"
	LabelReplica       = "storage.maurice.fr/replica"
	LabelVolumeName    = "storage.maurice.fr/volume-name"
)

// Finalizer name
const (
	FinalizerName = "storage.maurice.fr/finalizer"
)

// Default values
const (
	DefaultMigratorImage = "mauricethomas/migcontroller-migrator:latest"
)

// Status messages
const (
	MessageMigrationCompleted = "Migration completed successfully"
)
