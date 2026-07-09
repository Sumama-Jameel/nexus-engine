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
	"encoding/json"
	"fmt"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// AppsReport is the structured result of listing apps inside a container.
type AppsReport struct {
	Name    string   `json:"name"`
	Image   string   `json:"image"`
	Apps    []string `json:"apps"`
	Managed bool     `json:"managed"`
}

// Apps lists applications installed inside a Distrobox container.
// Returns the list of host-integrated apps that were exported.
func Apps(ctx context.Context, state *engine.StateTracker, execFn ExecFunc, name string) (*AppsReport, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	out, err := execFn(ctx, "distrobox", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("distrobox-list failed: %w", err)
	}

	// Parse and filter for the specific container
	// distrobox-list --json returns all containers. We extract apps
	// from the container matching the given name.
	var allContainers []struct {
		Name string   `json:"name"`
		Apps []string `json:"apps"`
	}
	if err := jsonUnmarshal([]byte(out), &allContainers); err != nil {
		return nil, fmt.Errorf("distrobox-list JSON parse failed: %w", err)
	}

	for _, c := range allContainers {
		if c.Name == name {
			report := &AppsReport{
				Name:    name,
				Apps:    c.Apps,
				Managed: IsManaged(state, name),
			}
			return report, nil
		}
	}

	return nil, fmt.Errorf("container %q not found in distrobox list", name)
}

// jsonUnmarshal is a small wrapper for readability — standard library.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
