// Copyright 2024-2026 Nexus Protocol Contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package container

import (
	"context"
	"fmt"
)

// RemoveOpts controls the remove operation.
type RemoveOpts struct {
	// Force skips the managed-state check.
	Force bool
}

// RemoveReport is the structured result of a container removal.
type RemoveReport struct {
	Name    string `json:"name"`
	Removed bool   `json:"removed"`
}

// Remove deletes a Distrobox container. Gated on the container being
// Nexus-managed (i.e., present in state). Use --force to skip this
// check for containers created outside Nexus.
func Remove(ctx context.Context, deps StateTracker, execFn ExecFunc, name string, opts RemoveOpts) (*RemoveReport, error) {
	if execFn == nil {
		return nil, fmt.Errorf("container: ExecFn must not be nil (Zero-Trust)")
	}
	if err := validateName(name); err != nil {
		return nil, err
	}

	if !opts.Force && !deps.IsContainerManaged(name) {
		return nil, fmt.Errorf("container %q is not managed by Nexus — add --force to override", name)
	}

	_, err := execFn(ctx, "distrobox", "rm", "--force", name)
	if err != nil {
		return nil, fmt.Errorf("distrobox-rm failed: %w", err)
	}

	deps.RecordContainerRemove(name)

	return &RemoveReport{Name: name, Removed: true}, nil
}

// IsManaged checks if a container is tracked in Nexus state.
func IsManaged(state StateTracker, name string) bool {
	if state == nil {
		return false
	}
	return state.IsContainerManaged(name)
}
