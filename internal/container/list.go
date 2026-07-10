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
)

// ListReport is the structured result of `container list`.
type ListReport struct {
	Containers []ContainerInfo `json:"containers"`
}

// ContainerInfo is a single entry from distrobox list --json.
type ContainerInfo struct {
	Name        string `json:"name"`
	Status      string `json:"status"` // "running" | "created" | "exited"
	Image       string `json:"image"`
	Created     string `json:"created"`
	Command     string `json:"command"`
	Managed     bool   `json:"managed"` // true if in Nexus state
}

// List returns all Distrobox containers, annotated with managed status.
// Filters to containers Nexus actually manages via state.
func List(ctx context.Context, state StateTracker, execFn ExecFunc) (*ListReport, error) {
	if execFn == nil {
		return nil, fmt.Errorf("container: ExecFn must not be nil (Zero-Trust)")
	}

	out, err := execFn(ctx, "distrobox", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("distrobox-list failed: %w", err)
	}

	var raw []struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Image   string `json:"image"`
		Created string `json:"created"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("distrobox-list returned invalid JSON: %w", err)
	}

	managed := make(map[string]bool)
	if state != nil {
		for _, name := range state.GetContainerNames() {
			managed[name] = true
		}
	}

	report := &ListReport{}
	for _, c := range raw {
		report.Containers = append(report.Containers, ContainerInfo{
			Name:    c.Name,
			Status:  c.Status,
			Image:   c.Image,
			Created: c.Created,
			Command: c.Command,
			Managed: managed[c.Name],
		})
	}

	return report, nil
}

// Info returns a single container's detail, including managed status.
func Info(ctx context.Context, state StateTracker, execFn ExecFunc, name string) (*ContainerInfo, error) {
	if err := validateName(name); err != nil {
		return nil, err
	}

	list, err := List(ctx, state, execFn)
	if err != nil {
		return nil, err
	}

	for _, c := range list.Containers {
		if c.Name == name {
			return &c, nil
		}
	}

	return nil, fmt.Errorf("container %q not found", name)
}
