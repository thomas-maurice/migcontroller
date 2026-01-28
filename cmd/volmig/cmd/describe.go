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
	"k8s.io/apimachinery/pkg/types"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

var describeCmd = &cobra.Command{
	Use:   "describe <name>",
	Short: "Show detailed information about a VolumeResize",
	Long: `Show detailed information about a VolumeResize resource including
its spec, status, and per-volume migration progress.

Examples:
  # Describe a volume resize
  volmig describe resize-weaviate

  # Describe in specific namespace
  volmig describe resize-weaviate -n production`,
	Args: cobra.ExactArgs(1),
	Run:  runDescribe,
}

func init() {
	rootCmd.AddCommand(describeCmd)
}

func runDescribe(cmd *cobra.Command, args []string) {
	name := args[0]
	ctx := context.Background()

	c, err := getClient()
	if err != nil {
		exitWithError("failed to create kubernetes client", err)
	}

	vr := &storagev1alpha1.VolumeResize{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, vr); err != nil {
		exitWithError("failed to get volumeresize", err)
	}

	fmt.Printf("Name:         %s\n", vr.Name)
	fmt.Printf("Namespace:    %s\n", vr.Namespace)
	fmt.Printf("Created:      %s\n", vr.CreationTimestamp.Format("2006-01-02 15:04:05"))
	fmt.Println()

	fmt.Println("Spec:")
	fmt.Printf("  StatefulSet:  %s\n", vr.Spec.StatefulSetName)
	fmt.Println("  Volumes:")
	for _, vol := range vr.Spec.Volumes {
		fmt.Printf("    - Name:     %s\n", vol.Name)
		fmt.Printf("      NewSize:  %s\n", vol.NewSize.String())
		if vol.StorageClass != nil {
			fmt.Printf("      StorageClass: %s\n", *vol.StorageClass)
		}
	}
	fmt.Println()

	fmt.Println("Status:")
	phase := vr.Status.Phase
	if phase == "" {
		phase = "Pending"
	}
	fmt.Printf("  Phase:        %s\n", phase)

	if vr.Status.CurrentReplica != nil {
		fmt.Printf("  CurrentReplica: %d\n", *vr.Status.CurrentReplica)
	}
	if vr.Status.CurrentVolume != "" {
		fmt.Printf("  CurrentVolume:  %s\n", vr.Status.CurrentVolume)
	}
	if vr.Status.Message != "" {
		fmt.Printf("  Message:      %s\n", vr.Status.Message)
	}
	if vr.Status.BackupConfigMapName != "" {
		fmt.Printf("  BackupConfigMap: %s\n", vr.Status.BackupConfigMapName)
	}
	if vr.Status.StartTime != nil {
		fmt.Printf("  StartTime:    %s\n", vr.Status.StartTime.Format("2006-01-02 15:04:05"))
	}
	if vr.Status.CompletionTime != nil {
		fmt.Printf("  CompletionTime: %s\n", vr.Status.CompletionTime.Format("2006-01-02 15:04:05"))
		if vr.Status.StartTime != nil {
			duration := vr.Status.CompletionTime.Sub(vr.Status.StartTime.Time)
			fmt.Printf("  Duration:     %s\n", duration.Round(1))
		}
	}

	if len(vr.Status.VolumeStatuses) > 0 {
		fmt.Println()
		fmt.Println("Volume Statuses:")
		for _, vs := range vr.Status.VolumeStatuses {
			fmt.Printf("  - Volume: %s, Replica: %d\n", vs.VolumeName, vs.Replica)
			fmt.Printf("    Phase:    %s\n", vs.Phase)
			if vs.OldPVCName != "" {
				fmt.Printf("    OldPVC:   %s\n", vs.OldPVCName)
			}
			if vs.NewPVCName != "" {
				fmt.Printf("    NewPVC:   %s\n", vs.NewPVCName)
			}
			if vs.OldPVName != "" {
				fmt.Printf("    OldPV:    %s\n", vs.OldPVName)
			}
			if vs.Message != "" {
				fmt.Printf("    Message:  %s\n", vs.Message)
			}
		}
	}

	if len(vr.Status.Conditions) > 0 {
		fmt.Println()
		fmt.Println("Conditions:")
		for _, cond := range vr.Status.Conditions {
			fmt.Printf("  - Type: %s\n", cond.Type)
			fmt.Printf("    Status: %s\n", cond.Status)
			if cond.Reason != "" {
				fmt.Printf("    Reason: %s\n", cond.Reason)
			}
			if cond.Message != "" {
				fmt.Printf("    Message: %s\n", cond.Message)
			}
		}
	}
}
