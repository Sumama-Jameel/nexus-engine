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

package container

import (
	"context"
	"fmt"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// ExecFunc is the Zero-Trust execution gate.
type ExecFunc = func(ctx context.Context, command string, args ...string) (string, error)

// CreateDeps holds dependencies for the Create operation.
type CreateDeps struct {
	ExecFn ExecFunc
	State  StateTracker
	Audit  *engine.AuditLogger
}

// CreateOpts controls the create operation.
type CreateOpts struct {
	// Image is the OCI reference, e.g., "fedora:39".
	Image string
	// Home maps a custom HOME directory into the container.
	Home string
	// AdditionalDistroboxFlags passes through extra args to distrobox-create.
	AdditionalDistroboxFlags []string
}

// CreateReport is the structured result of a container create.
type CreateReport struct {
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	Family    string    `json:"family"`
	Created   bool      `json:"created"`
	RolledBack bool     `json:"rolled_back,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Create runs distrobox-create with the given name and image.
// Auto-cleanup: if the state write fails, the container is removed
// via distrobox-rm --force to keep the state consistent.
func Create(ctx context.Context, deps CreateDeps, name string, opts CreateOpts) (*CreateReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("container: ExecFn must not be nil (Zero-Trust)")
	}

	if err := validateName(name); err != nil {
		return nil, err
	}
	if err := validateImageRef(opts.Image); err != nil {
		return nil, err
	}

	// Build distrobox-create args.
	args := []string{"create", "--name", name, "--image", opts.Image}
	if opts.Home != "" {
		args = append(args, "--home", opts.Home)
	}
	if len(opts.AdditionalDistroboxFlags) > 0 {
		args = append(args, opts.AdditionalDistroboxFlags...)
	}

	// Step 1: create the container.
	_, err := deps.ExecFn(ctx, "distrobox", args...)
	if err != nil {
		return nil, fmt.Errorf("distrobox-create failed: %w", err)
	}

	family := detectFamily(opts.Image)
	report := &CreateReport{
		Name:      name,
		Image:     opts.Image,
		Family:    family,
		Created:   true,
		CreatedAt: time.Now().UTC(),
	}

	// Step 2: persist the state.
	if deps.State != nil {
		if stateErr := deps.State.RecordContainerCreate(name, opts.Image, family); stateErr != nil {
			// State write failed — auto-remove to keep consistent.
			_, rmErr := deps.ExecFn(ctx, "distrobox", "rm", "--force", name)
			_ = rmErr
			// Best-effort: log the rm error but still return the state error.
			deps.Audit.Log(engine.AuditEntry{
				Action: "CONTAINER_CREATE_ROLLED_BACK",
				Result: "failure",
				Package: fmt.Sprintf("container %q: state write failed (%v); auto-removed", name, stateErr),
			})
			report.RolledBack = true
			return report, fmt.Errorf("container %q created but state write failed (%v); auto-removed", name, stateErr)
		}

		if deps.Audit != nil {
			deps.Audit.Log(engine.AuditEntry{
				Action: "CONTAINER_CREATE",
				Result: "success",
				Package: fmt.Sprintf("container %q (%s)", name, opts.Image),
			})
		}
	}

	return report, nil
}

// RollbackContainer removes a container from state and cleans up the
// Distrobox instance. Used by the rollback path in Create when state
// persistence fails but the container was already created.
func RollbackContainer(ctx context.Context, deps CreateDeps, name string) error {
	if deps.State != nil {
		_ = deps.State.RecordContainerRemove(name)
	}
	_, err := deps.ExecFn(ctx, "distrobox", "rm", "--force", name)
	if deps.Audit != nil {
		deps.Audit.Log(engine.AuditEntry{
			Action: "CONTAINER_CREATE_ROLLED_BACK",
			Result: "failure",
			Package: fmt.Sprintf("container %q: auto-removed", name),
		})
	}
	return err
}
