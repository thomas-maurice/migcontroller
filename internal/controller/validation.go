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
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

// ValidationResult represents the result of a validation check
type ValidationResult struct {
	Valid   bool
	Message string
}

// validateStatefulSetExists checks if the target StatefulSet exists
func validateStatefulSetExists(ctx context.Context, c client.Client, namespace, name string) (*appsv1.StatefulSet, ValidationResult) {
	sts := &appsv1.StatefulSet{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, sts)
	if err != nil {
		return nil, ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("StatefulSet %s not found: %v", name, err),
		}
	}
	return sts, ValidationResult{Valid: true}
}

// validateVolumeTargets checks that all volume targets match volumeClaimTemplates in the StatefulSet
func validateVolumeTargets(sts *appsv1.StatefulSet, volumes []storagev1alpha1.VolumeResizeTarget) ValidationResult {
	templateNames := make(map[string]bool)
	for _, vct := range sts.Spec.VolumeClaimTemplates {
		templateNames[vct.Name] = true
	}

	for _, vol := range volumes {
		if !templateNames[vol.Name] {
			return ValidationResult{
				Valid:   false,
				Message: fmt.Sprintf("volume %q not found in StatefulSet volumeClaimTemplates", vol.Name),
			}
		}
	}

	return ValidationResult{Valid: true}
}

// validateSizeReduction checks that newSize is smaller than the current PVC size
func validateSizeReduction(ctx context.Context, c client.Client, namespace, stsName string, vol storagev1alpha1.VolumeResizeTarget) ValidationResult {
	// Get the PVC for replica 0 to check current size
	pvcName := getOriginalPVCName(vol.Name, stsName, 0)
	pvc, err := getPVC(ctx, c, namespace, pvcName)
	if err != nil {
		return ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("failed to get PVC %s: %v", pvcName, err),
		}
	}

	currentSize := pvc.Spec.Resources.Requests.Storage()
	if currentSize == nil {
		return ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("PVC %s has no storage request", pvcName),
		}
	}

	// Compare sizes: newSize must be strictly less than currentSize
	if vol.NewSize.Cmp(*currentSize) >= 0 {
		return ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("newSize (%s) must be smaller than current size (%s) for volume %s", vol.NewSize.String(), currentSize.String(), vol.Name),
		}
	}

	return ValidationResult{Valid: true}
}

// validatePDBAllowsDisruption checks that no PDB blocks pod disruption for the StatefulSet
func validatePDBAllowsDisruption(ctx context.Context, c client.Client, sts *appsv1.StatefulSet) ValidationResult {
	// List all PDBs in the namespace
	pdbList := &policyv1.PodDisruptionBudgetList{}
	if err := c.List(ctx, pdbList, client.InNamespace(sts.Namespace)); err != nil {
		return ValidationResult{
			Valid:   false,
			Message: fmt.Sprintf("failed to list PodDisruptionBudgets: %v", err),
		}
	}

	// Get StatefulSet pod labels
	podLabels := sts.Spec.Template.Labels

	for _, pdb := range pdbList.Items {
		if pdb.Spec.Selector == nil {
			continue
		}

		selector, err := labels.ValidatedSelectorFromSet(pdb.Spec.Selector.MatchLabels)
		if err != nil {
			continue
		}

		// Check if PDB selector matches StatefulSet pods
		if selector.Matches(labels.Set(podLabels)) {
			// Check if disruptions are allowed
			if pdb.Status.DisruptionsAllowed < 1 {
				return ValidationResult{
					Valid:   false,
					Message: fmt.Sprintf("PodDisruptionBudget %s does not allow disruptions (disruptionsAllowed=%d)", pdb.Name, pdb.Status.DisruptionsAllowed),
				}
			}
		}
	}

	return ValidationResult{Valid: true}
}
