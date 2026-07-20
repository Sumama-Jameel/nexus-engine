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
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/Sumama-Jameel/nexus-engine/internal/installer"
	"github.com/Sumama-Jameel/nexus-engine/internal/mode"
	"github.com/Sumama-Jameel/nexus-engine/pkg/manifest"
	"github.com/spf13/cobra"
)

func runModeList(cmd *cobra.Command, args []string) error {
	_, err := initDeps(context.Background())
	if err != nil {
		return err
	}
	modes, err := mode.List()
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(modes)
	}
	for _, m := range modes {
		builtin := ""
		if m.Builtin {
			builtin = " (built-in)"
		}
		fmt.Printf("%s — %s%s\n", m.Name, m.Description, builtin)
	}
	return nil
}

func runModeCurrent(cmd *cobra.Command, args []string) error {
	nd, err := initDeps(context.Background())
	if err != nil {
		return err
	}
	active := nd.state.GetActiveMode()
	if outputJSON {
		return jsonOutput(map[string]string{"active_mode": active})
	}
	if active == "" {
		fmt.Println("No mode has been applied yet.")
	} else {
		fmt.Printf("Active mode: %s\n", active)
	}
	return nil
}

func runModeShow(cmd *cobra.Command, args []string) error {
	m, err := mode.Resolve(args[0])
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(m)
	}
	fmt.Printf("Mode: %s (%s)\n", m.Name, m.Description)
	return nil
}

func runModeApply(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}

	deps := mode.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		GOOS:   runtime.GOOS,
		ApplyProfile: func(ctx context.Context, pName string, dryRun bool) error {
			store, storeErr := initProfileStore()
			if storeErr != nil {
				return storeErr
			}
			profile, profileErr := store.LoadProfileWithExtends(pName)
			if profileErr != nil {
				return profileErr
			}
			target := manifest.ResolveTarget(profile, nd.env.PackageManager)
			if target == nil {
				return nil
			}
			orch := installer.NewOrchestrator(nd.pm, nd.execFn, nd.state, nd.audit, pName, dryRun)
			_, installErr := orch.Install(ctx, target.Packages)
			return installErr
		},
		BindDotfiles: func(ctx context.Context, source string) error { return nil },
	}

	opts := mode.ApplyOpts{
		DryRun:                dryRun,
		AllowUnlistedServices: allowUnlistedServices,
	}

	report, applyErr := mode.Apply(ctx, deps, args[0], opts)
	if applyErr != nil {
		return applyErr
	}

	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Mode %q applied (previous: %q)\n", report.Mode, report.Previous)
	return nil
}

func runModeRollback(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := mode.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		GOOS:   runtime.GOOS,
	}
	report, err := mode.Rollback(ctx, deps, mode.ApplyOpts{})
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Rolled back to: %s\n", report.Mode)
	return nil
}

func runModeDefine(cmd *cobra.Command, args []string) error {
	input := mode.DefineInput{
		In:             bufio.NewReader(os.Stdin),
		Out:            os.Stdout,
		NonInteractive: false,
		Draft:          mode.Mode{Name: args[0]},
	}
	m, defineErr := mode.Define(input)
	if defineErr != nil {
		return defineErr
	}
	fmt.Printf("Mode %q created at %s\n", m.Name, m.SourcePath)
	return nil
}
