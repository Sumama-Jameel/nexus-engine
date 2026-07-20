// Copyright 2024-2026 Nexus Protocol Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"

	"github.com/Sumama-Jameel/nexus-engine/internal/container"
	"github.com/spf13/cobra"
)

func runContainerList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	list, err := container.List(ctx, nd.state, nd.execFn)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(list)
	}
	fmt.Println("Containers:")
	for _, c := range list.Containers {
		managed := ""
		if c.Managed {
			managed = " (managed)"
		}
		fmt.Printf("  %s — %s%s\n", c.Name, c.Status, managed)
	}
	return nil
}

func runContainerInfo(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	info, err := container.Info(ctx, nd.state, nd.execFn, args[0])
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(info)
	}
	fmt.Printf("Name: %s\nStatus: %s\nImage: %s\nManaged: %v\n", info.Name, info.Status, info.Image, info.Managed)
	return nil
}

func runContainerCreate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	image, _ := cmd.Flags().GetString("image")
	deps := container.CreateDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	opts := container.CreateOpts{Image: image}
	report, err := container.Create(ctx, deps, args[0], opts)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Container %q created\n", report.Name)
	return nil
}

func runContainerEnter(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	enterCmd, err := container.EnterCommand(args[0], nd.state)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(map[string]string{"command": enterCmd})
	}
	fmt.Println("To enter this container, run:")
	fmt.Printf("  %s\n", enterCmd)
	return nil
}

func runContainerApps(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	apps, err := container.Apps(ctx, nd.state, nd.execFn, args[0])
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(apps)
	}
	fmt.Printf("Apps in %s:\n", apps.Name)
	for _, a := range apps.Apps {
		fmt.Printf("  %s\n", a)
	}
	return nil
}

func runContainerRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	force, _ := cmd.Flags().GetBool("force")
	opts := container.RemoveOpts{Force: force}
	report, err := container.Remove(ctx, nd.state, nd.execFn, args[0], opts)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Removed: %s\n", report.Name)
	return nil
}
