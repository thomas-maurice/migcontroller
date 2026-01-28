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

package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

var (
	statefulSetName string
	volumeName      string
	newSize         string
	storageClass    string
	watch           bool
)

var createCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a VolumeResize for a StatefulSet",
	Long: `Create a VolumeResize custom resource to migrate StatefulSet PVCs to a new size.

Examples:
  # Resize weaviate-data volume to 500Mi
  volmig create resize-weaviate --statefulset weaviate --volume weaviate-data --size 500Mi

  # Resize with a different storage class
  volmig create resize-db --statefulset postgres --volume data --size 10Gi --storage-class fast-ssd

  # Create and watch progress
  volmig create resize-weaviate --statefulset weaviate --volume weaviate-data --size 500Mi --watch`,
	Args: cobra.ExactArgs(1),
	Run:  runCreate,
}

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(&statefulSetName, "statefulset", "s", "", "name of the StatefulSet to resize (required)")
	createCmd.Flags().StringVarP(&volumeName, "volume", "v", "", "name of the volume to resize (required)")
	createCmd.Flags().StringVar(&newSize, "size", "", "target size for the volume (required, e.g., 500Mi, 10Gi)")
	createCmd.Flags().StringVar(&storageClass, "storage-class", "",
		"storage class for the new PVC (optional, defaults to original)")
	createCmd.Flags().BoolVarP(&watch, "watch", "w", false, "watch migration progress after creation")

	_ = createCmd.MarkFlagRequired("statefulset")
	_ = createCmd.MarkFlagRequired("volume")
	_ = createCmd.MarkFlagRequired("size")
}

func runCreate(cmd *cobra.Command, args []string) {
	name := args[0]
	ctx := context.Background()

	// Validate size
	quantity, err := resource.ParseQuantity(newSize)
	if err != nil {
		exitWithError("invalid size format", err)
	}

	// Get client
	c, err := getClient()
	if err != nil {
		exitWithError("failed to create kubernetes client", err)
	}

	// Check if VolumeResize already exists
	existing := &storagev1alpha1.VolumeResize{}
	err = c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, existing)
	if err == nil {
		exitWithError("volumeresize already exists", fmt.Errorf("name: %s", name))
	}

	// Build the VolumeResize object
	vr := &storagev1alpha1.VolumeResize{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: storagev1alpha1.VolumeResizeSpec{
			StatefulSetName: statefulSetName,
			Volumes: []storagev1alpha1.VolumeResizeTarget{
				{
					Name:    volumeName,
					NewSize: quantity,
				},
			},
		},
	}

	// Add storage class if specified
	if storageClass != "" {
		vr.Spec.Volumes[0].StorageClass = &storageClass
	}

	// Create the VolumeResize
	if err := c.Create(ctx, vr); err != nil {
		exitWithError("failed to create volumeresize", err)
	}

	fmt.Printf("VolumeResize '%s' created successfully\n", name)
	fmt.Printf("  StatefulSet: %s\n", statefulSetName)
	fmt.Printf("  Volume:      %s\n", volumeName)
	fmt.Printf("  New Size:    %s\n", newSize)
	if storageClass != "" {
		fmt.Printf("  StorageClass: %s\n", storageClass)
	}
	fmt.Println()
	fmt.Printf("Monitor progress with:\n")
	fmt.Printf("  volmig watch %s -n %s\n", name, namespace)
	fmt.Printf("  kubectl get volumeresize %s -n %s -w\n", name, namespace)

	if watch {
		fmt.Println()
		runWatch(cmd, []string{name})
	}
}
