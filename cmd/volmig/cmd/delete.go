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

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a VolumeResize resource",
	Long: `Delete a VolumeResize resource by name.

Examples:
  # Delete a volume resize
  volmig delete resize-weaviate

  # Delete in specific namespace
  volmig delete resize-weaviate -n production`,
	Args: cobra.ExactArgs(1),
	Run:  runDelete,
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}

func runDelete(cmd *cobra.Command, args []string) {
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

	if err := c.Delete(ctx, vr); err != nil {
		exitWithError("failed to delete volumeresize", err)
	}

	fmt.Printf("VolumeResize '%s' deleted\n", name)
}
