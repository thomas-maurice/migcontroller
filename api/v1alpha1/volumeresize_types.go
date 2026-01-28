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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VolumeResizeTarget specifies a volume to resize
type VolumeResizeTarget struct {
	// Name is the name of the volumeClaimTemplate in the StatefulSet
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// NewSize is the target size for the volume (must be smaller than current size)
	// +kubebuilder:validation:Required
	NewSize resource.Quantity `json:"newSize"`

	// StorageClass is the storage class to use for the new PVC. Defaults to the original PVC's storage class.
	// +optional
	StorageClass *string `json:"storageClass,omitempty"`
}

// VolumeResizeSpec defines the desired state of VolumeResize
type VolumeResizeSpec struct {
	// StatefulSetName is the name of the StatefulSet to migrate
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	StatefulSetName string `json:"statefulSetName"`

	// Volumes specifies which volumes to resize and their target sizes
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Volumes []VolumeResizeTarget `json:"volumes"`
}

// VolumeStatus tracks the migration status for a specific volume on a specific replica
type VolumeStatus struct {
	// VolumeName is the name of the volume being migrated
	VolumeName string `json:"volumeName"`

	// Replica is the replica index this status is for
	Replica int32 `json:"replica"`

	// Phase is the current phase of this volume's migration
	// +kubebuilder:validation:Enum=Pending;Syncing;Synced;Replacing;Completed;Failed
	Phase string `json:"phase"`

	// OldPVCName is the name of the original PVC
	// +optional
	OldPVCName string `json:"oldPVCName,omitempty"`

	// NewPVCName is the name of the temporary new PVC
	// +optional
	NewPVCName string `json:"newPVCName,omitempty"`

	// OldPVName is the name of the original PV (retained for rollback)
	// +optional
	OldPVName string `json:"oldPVName,omitempty"`

	// Message provides additional details about the current phase
	// +optional
	Message string `json:"message,omitempty"`
}

// VolumeResizeStatus defines the observed state of VolumeResize.
type VolumeResizeStatus struct {
	// Phase is the current phase of the migration
	// +kubebuilder:validation:Enum=Pending;Validating;Syncing;Replacing;Completed;Failed
	// +optional
	Phase string `json:"phase,omitempty"`

	// Conditions represent the current state of the VolumeResize resource
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// VolumeStatuses tracks the status of each volume being migrated
	// +optional
	VolumeStatuses []VolumeStatus `json:"volumeStatuses,omitempty"`

	// CurrentReplica is the replica index currently being processed
	// +optional
	CurrentReplica *int32 `json:"currentReplica,omitempty"`

	// CurrentVolume is the volume name currently being processed
	// +optional
	CurrentVolume string `json:"currentVolume,omitempty"`

	// Message provides additional details about the current phase
	// +optional
	Message string `json:"message,omitempty"`

	// StartTime is when the migration started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the migration completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// BackupConfigMapName is the name of the ConfigMap containing the StatefulSet backup
	// +optional
	BackupConfigMapName string `json:"backupConfigMapName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="StatefulSet",type=string,JSONPath=`.spec.statefulSetName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Replica",type=integer,JSONPath=`.status.currentReplica`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`,priority=0
// +kubebuilder:printcolumn:name="Backup",type=string,JSONPath=`.status.backupConfigMapName`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VolumeResize is the Schema for the volumeresizes API
type VolumeResize struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of VolumeResize
	// +required
	Spec VolumeResizeSpec `json:"spec"`

	// status defines the observed state of VolumeResize
	// +optional
	Status VolumeResizeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// VolumeResizeList contains a list of VolumeResize
type VolumeResizeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []VolumeResize `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VolumeResize{}, &VolumeResizeList{})
}
