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
// See the License for the specific language governing permissions
// limitations under the License.

package container

import (
	"context"
	"fmt"
)

// EnterCommand returns the shell command to enter a container.
// We don't execute it — the Tauri dashboard is not a terminal.
// Instead, we return the command string so the user can copy and
// paste it into their own terminal, or the CLI can print it.
//
// The state tracker records the "last entered" time for visibility.
func EnterCommand(name string, state StateTracker) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}

	if state != nil {
		state.RecordContainerEnter(name)
	}

	return fmt.Sprintf("distrobox enter %s", name), nil
}

// EnterDirect runs `distrobox enter <name>` directly. Used for
// non-interactive entry points like `nexus container apps`.
func EnterDirect(name string, execFn ExecFunc) (string, error) {
	if err := validateName(name); err != nil {
		return "", err
	}
	return execFn(context.Background(), "distrobox", "enter", name)
}
