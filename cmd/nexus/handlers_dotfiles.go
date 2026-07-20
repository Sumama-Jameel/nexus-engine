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
	"github.com/spf13/cobra"
)

func runDotfilesDetect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	report, err := dotfiles.Detect(ctx, nd.execFn)
	if err != nil {
		return err
	}
	if outputJSON {
		return jsonOutput(report)
	}
	fmt.Printf("Chezmoi: installed=%v version=%s\n", report.Installed, report.Version)
	return nil
}

func runDotfilesInstall(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.InstallDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.InstallChezmoi(ctx, deps)
	return err
}

func runDotfilesInit(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SourceDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	return dotfiles.BindSource(ctx, args[0], deps)
}

func runDotfilesRemove(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SourceDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	return dotfiles.UnbindSource(ctx, deps)
}

func runDotfilesApply(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.Apply(ctx, deps)
	return err
}

func runDotfilesStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	status, err := dotfiles.Status(ctx, deps)
	if err != nil {
		return err
	}
	fmt.Println(status)
	return nil
}

func runDotfilesDiff(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.ApplyDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	diff, err := dotfiles.Diff(ctx, deps)
	if err != nil {
		return err
	}
	fmt.Println(diff)
	return nil
}

func runDotfilesAdd(cmd *cobra.Command, args []string, force bool) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.AddDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.Add(ctx, args[0], deps, force)
	return err
}

func runDotfilesPush(cmd *cobra.Command, message string, force bool, token string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SyncDeps{
		ExecFn:         nd.execFn,
		State:          nd.state,
		Audit:          nd.audit,
		Token:          resolveSyncToken(token),
		SkipSecretScan: force,
	}
	_, err = dotfiles.Push(ctx, deps, message)
	return err
}

// runDotfilesPull pulls from remote.
// Test signature: runDotfilesPull(cmd, rebase bool, token string).
func runDotfilesPull(cmd *cobra.Command, rebase bool, token string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SyncDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
		Token:  resolveSyncToken(token),
	}
	_, err = dotfiles.Pull(ctx, deps, rebase)
	return err
}

// runDotfilesSync does pull + apply + push.
// Test signature: runDotfilesSync(cmd, message string, force bool, token string).
func runDotfilesSync(cmd *cobra.Command, message string, force bool, token string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.SyncDeps{
		ExecFn:         nd.execFn,
		State:          nd.state,
		Audit:          nd.audit,
		Token:          resolveSyncToken(token),
		SkipSecretScan: force,
	}
	_, err = dotfiles.Sync(ctx, deps, message, false)
	return err
}

func runDotfilesVerify(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	nd, err := initDeps(ctx)
	if err != nil {
		return err
	}
	deps := dotfiles.AddDeps{
		ExecFn: nd.execFn,
		State:  nd.state,
		Audit:  nd.audit,
	}
	_, err = dotfiles.Verify(ctx, deps)
	return err
}
