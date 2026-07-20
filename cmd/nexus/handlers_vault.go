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

	"github.com/Sumama-Jameel/nexus-engine/internal/dotfiles"
	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
	"github.com/spf13/cobra"
)

func runDotfilesVaultAdd(cmd *cobra.Command, file string, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultAddDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		DryRun: force,
	}
	_, err = dotfiles.VaultAdd(ctx, file, deps)
	return err
}

func runDotfilesVaultInit(cmd *cobra.Command, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultInitDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.VaultInit(ctx, deps)
	return err
}

func runDotfilesVaultList(cmd *cobra.Command) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	result, err := dotfiles.VaultList(struct {
		State *engine.StateTracker
	}{State: nd.state})
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(result)
	}
	for _, entry := range result.Files {
		fmt.Printf("%s -> %s\n", entry.Original, entry.Encrypted)
	}
	return nil
}

func runDotfilesVaultStatus(cmd *cobra.Command) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	result, err := dotfiles.VaultStatus(struct {
		State *engine.StateTracker
	}{State: nd.state})
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(result)
	}
	fmt.Printf("Vault status: initialized=%v files=%d\n", result.Status.Initialized, result.Status.FileCount)
	return nil
}

func runDotfilesVaultUnlock(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultUnlockDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.VaultUnlock(ctx, deps)
	return err
}

func runDotfilesVaultRemove(cmd *cobra.Command, file string, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.VaultRemoveDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		Force:  force,
	}
	_, err = dotfiles.VaultRemove(ctx, file, deps)
	return err
}
