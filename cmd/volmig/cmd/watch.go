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
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/types"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

var watchCmd = &cobra.Command{
	Use:   "watch <name>",
	Short: "Watch VolumeResize migration progress",
	Long: `Watch the progress of a VolumeResize migration until completion or failure.

Examples:
  # Watch a specific volume resize
  volmig watch resize-weaviate

  # Watch in a specific namespace
  volmig watch resize-weaviate -n production`,
	Args: cobra.ExactArgs(1),
	Run:  runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) {
	name := args[0]
	ctx := context.Background()

	c, err := getClient()
	if err != nil {
		exitWithError("failed to create kubernetes client", err)
	}

	fmt.Printf("Watching VolumeResize '%s' in namespace '%s'...\n\n", name, namespace)

	lastPhase := ""
	lastMessage := ""
	var lastReplica *int32

	for {
		vr := &storagev1alpha1.VolumeResize{}
		if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, vr); err != nil {
			exitWithError("failed to get volumeresize", err)
		}

		// Check if status changed
		replicaChanged := (lastReplica == nil && vr.Status.CurrentReplica != nil) ||
			(lastReplica != nil && vr.Status.CurrentReplica == nil) ||
			(lastReplica != nil && vr.Status.CurrentReplica != nil && *lastReplica != *vr.Status.CurrentReplica)

		if vr.Status.Phase != lastPhase || vr.Status.Message != lastMessage || replicaChanged {
			printStatus(vr)
			lastPhase = vr.Status.Phase
			lastMessage = vr.Status.Message
			if vr.Status.CurrentReplica != nil {
				replica := *vr.Status.CurrentReplica
				lastReplica = &replica
			} else {
				lastReplica = nil
			}
		}

		// Check for terminal states
		if vr.Status.Phase == phaseCompleted {
			fmt.Println()
			fmt.Println("Migration completed successfully!")
			if vr.Status.CompletionTime != nil && vr.Status.StartTime != nil {
				duration := vr.Status.CompletionTime.Sub(vr.Status.StartTime.Time)
				fmt.Printf("Duration: %s\n", duration.Round(time.Second))
			}
			os.Exit(0)
		}

		if vr.Status.Phase == phaseFailed {
			fmt.Println()
			fmt.Fprintf(os.Stderr, "Migration failed: %s\n", vr.Status.Message)
			os.Exit(1)
		}

		time.Sleep(2 * time.Second)
	}
}

func printStatus(vr *storagev1alpha1.VolumeResize) {
	timestamp := time.Now().Format("15:04:05")
	phase := vr.Status.Phase
	if phase == "" {
		phase = phasePending
	}

	fmt.Printf("[%s] Phase: %-12s", timestamp, phase)

	if vr.Status.CurrentReplica != nil {
		fmt.Printf(" | Replica: %d", *vr.Status.CurrentReplica)
	}

	if vr.Status.CurrentVolume != "" {
		fmt.Printf(" | Volume: %s", vr.Status.CurrentVolume)
	}

	if vr.Status.Message != "" {
		fmt.Printf(" | %s", vr.Status.Message)
	}

	fmt.Println()
}
