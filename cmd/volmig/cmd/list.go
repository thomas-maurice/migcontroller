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
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/thomas-maurice/migcontroller/api/v1alpha1"
)

var (
	allNamespaces bool
	outputFormat  string
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List VolumeResize resources",
	Long: `List all VolumeResize resources in a namespace or across all namespaces.

Examples:
  # List in current namespace
  volmig list

  # List in all namespaces
  volmig list -A

  # List with extended info (volumes, age)
  volmig list -o wide

  # List in specific namespace
  volmig list -n production`,
	Run: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "list across all namespaces")
	listCmd.Flags().StringVarP(&outputFormat, "output", "o", "", "output format (wide)")
}

func runList(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	c, err := getClient()
	if err != nil {
		exitWithError("failed to create kubernetes client", err)
	}

	vrList := &storagev1alpha1.VolumeResizeList{}
	listOpts := []client.ListOption{}

	if !allNamespaces {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := c.List(ctx, vrList, listOpts...); err != nil {
		exitWithError("failed to list volumeresizes", err)
	}

	if len(vrList.Items) == 0 {
		if allNamespaces {
			fmt.Println("No VolumeResize resources found in any namespace")
		} else {
			fmt.Printf("No VolumeResize resources found in namespace '%s'\n", namespace)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Build header
	header := ""
	if allNamespaces {
		header = "NAMESPACE\t"
	}
	header += "NAME\tSTATEFULSET\tPHASE\tREPLICA\tMESSAGE"
	if outputFormat == outputFormatWide {
		header += "\tVOLUMES\tAGE"
	}
	_, _ = fmt.Fprintln(w, header)

	for _, vr := range vrList.Items {
		phase := vr.Status.Phase
		if phase == "" {
			phase = phasePending
		}

		replica := "-"
		if vr.Status.CurrentReplica != nil {
			replica = fmt.Sprintf("%d", *vr.Status.CurrentReplica)
		}

		message := vr.Status.Message
		maxMsgLen := 40
		if outputFormat == outputFormatWide {
			maxMsgLen = 60
		}
		if len(message) > maxMsgLen {
			message = message[:maxMsgLen-3] + "..."
		}

		// Build row
		row := ""
		if allNamespaces {
			row = vr.Namespace + "\t"
		}
		row += fmt.Sprintf("%s\t%s\t%s\t%s\t%s", vr.Name, vr.Spec.StatefulSetName, phase, replica, message)

		if outputFormat == outputFormatWide {
			// Get volume names and sizes
			var volumes strings.Builder
			for i, vol := range vr.Spec.Volumes {
				if i > 0 {
					volumes.WriteString(",")
				}
				volumes.WriteString(fmt.Sprintf("%s:%s", vol.Name, vol.NewSize.String()))
			}

			// Calculate age
			age := formatAge(time.Since(vr.CreationTimestamp.Time))
			row += fmt.Sprintf("\t%s\t%s", volumes.String(), age)
		}

		_, _ = fmt.Fprintln(w, row)
	}

	_ = w.Flush()
}

func formatAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
