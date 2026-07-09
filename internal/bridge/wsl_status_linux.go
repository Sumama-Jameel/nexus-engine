//go:build linux

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

package bridge

import (
	"context"
	"runtime"
)

// DetectWSL2Status is the Linux implementation of WSL2 detection.
// On Linux, WSL2 detection from the Windows side is not applicable.
// Instead, we check if we're running INSIDE WSL2 (which the existing
// bridge_linux.go already handles).
//
// Per V4: "A Windows .exe version of your Go tool that checks if WSL2 is enabled."
// On Linux, this returns a status indicating the command is not applicable,
// with guidance to use `nexus probe` instead.
func DetectWSL2Status(ctx context.Context) *WSL2Status {
	return &WSL2Status{
		WSLAvailable:    false,
		WSLVersion:      "n/a",
		Architecture:    runtime.GOARCH,
		Ready:           false,
		Blockers:        []string{"WSL2 detection is only available on Windows"},
		Recommendations: []string{"On Linux, use 'nexus probe' to detect WSL2 from inside the guest"},
	}
}

// IsWSLCommandAvailable returns false on Linux — the `nexus wsl`
// commands are designed for the Windows host, not for Linux guests.
func IsWSLCommandAvailable() bool {
	return false
}
