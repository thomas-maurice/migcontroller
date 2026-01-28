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
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

var followStatus bool

var statusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show quick status of a VolumeResize",
	Long: `Show a quick status overview of a VolumeResize migration including
phase, progress, and per-replica status.

Examples:
  # Get status of a volume resize
  volmig status resize-weaviate

  # Follow status updates until completion
  volmig status resize-weaviate --follow

  # Get status in specific namespace
  volmig status resize-weaviate -n production`,
	Args: cobra.ExactArgs(1),
	Run:  runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.Flags().BoolVarP(&followStatus, "follow", "f", false, "follow status updates until completion")
}

func runStatus(cmd *cobra.Command, args []string) {
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

	// Print header
	phase := vr.Status.Phase
	if phase == "" {
		phase = phasePending
	}

	// Status icon
	icon := "⏳"
	switch phase {
	case phaseCompleted:
		icon = "✅"
	case phaseFailed:
		icon = "❌"
	case "Syncing":
		icon = "🔄"
	case "Validating":
		icon = "🔍"
	}

	fmt.Printf("%s VolumeResize: %s\n", icon, name)
	fmt.Printf("   Phase: %s\n", phase)
	if vr.Status.Message != "" {
		fmt.Printf("   Message: %s\n", vr.Status.Message)
	}
	fmt.Println()

	// Print spec summary
	fmt.Printf("Target: StatefulSet/%s\n", vr.Spec.StatefulSetName)
	for _, vol := range vr.Spec.Volumes {
		sc := "(default)"
		if vol.StorageClass != nil {
			sc = *vol.StorageClass
		}
		fmt.Printf("   Volume: %s → %s [%s]\n", vol.Name, vol.NewSize.String(), sc)
	}
	fmt.Println()

	// Print per-replica status if available
	if len(vr.Status.VolumeStatuses) > 0 {
		fmt.Println("Replica Status:")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  REPLICA\tVOLUME\tPHASE\tMESSAGE")

		for _, vs := range vr.Status.VolumeStatuses {
			statusIcon := ""
			switch vs.Phase {
			case phaseCompleted:
				statusIcon = "✅"
			case phaseFailed:
				statusIcon = "❌"
			case "Syncing", "Replacing":
				statusIcon = "🔄"
			default:
				statusIcon = "⏳"
			}
			msg := vs.Message
			if len(msg) > 40 {
				msg = msg[:37] + "..."
			}
			_, _ = fmt.Fprintf(w, "  %d\t%s\t%s %s\t%s\n", vs.Replica, vs.VolumeName, statusIcon, vs.Phase, msg)
		}
		_ = w.Flush()
		fmt.Println()
	}

	// Print timing info
	if vr.Status.StartTime != nil {
		fmt.Printf("Started: %s\n", vr.Status.StartTime.Format("2006-01-02 15:04:05"))
	}
	if vr.Status.CompletionTime != nil {
		fmt.Printf("Completed: %s\n", vr.Status.CompletionTime.Format("2006-01-02 15:04:05"))
		if vr.Status.StartTime != nil {
			duration := vr.Status.CompletionTime.Sub(vr.Status.StartTime.Time)
			fmt.Printf("Duration: %s\n", duration.Round(1))
		}
	}

	// Print backup info
	if vr.Status.BackupConfigMapName != "" {
		fmt.Printf("Backup ConfigMap: %s\n", vr.Status.BackupConfigMapName)
	}

	// If following, continue watching
	if followStatus && phase != phaseCompleted && phase != phaseFailed {
		fmt.Println()
		fmt.Println("Following status updates (Ctrl+C to stop)...")
		fmt.Println()
		runWatch(cmd, args)
		return
	}

	// Exit with error if failed
	if phase == phaseFailed {
		os.Exit(1)
	}
}
