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

package wsl

import (
	"context"
	"fmt"
	"time"
)

// IsImportAvailable returns false on Linux — WSL2 import is a Windows-only
// operation. Linux users are already on Linux and don't need to import
// a rootfs into WSL2.
func IsImportAvailable() bool {
	return false
}

// RootFSImage is a stub on Linux. See rootfs.go (windows) for the real type.
type RootFSImage struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	Arch        string `json:"arch"`
	Family      string `json:"family"`
	Description string `json:"description"`
	DefaultUser string `json:"default_user"`
}

// ImportConfig is a stub on Linux. See import.go (windows) for the real type.
type ImportConfig struct {
	DistroName    string
	InstallPath   string
	TarballPath   string
	Image         *RootFSImage
	SkipDownload  bool
	SkipVerify    bool
	DryRun        bool
}

// ImportResult is a stub on Linux. See import.go (windows) for the real type.
type ImportResult struct {
	DistroName     string        `json:"distro_name"`
	ImageName      string        `json:"image_name"`
	ImageVersion   string        `json:"image_version"`
	TarballPath    string        `json:"tarball_path"`
	TarballSHA256  string        `json:"tarball_sha256"`
	InstallPath    string        `json:"install_path"`
	Duration       time.Duration `json:"duration"`
	StepsCompleted []string      `json:"steps_completed"`
	Warnings       []string      `json:"warnings"`
	Aborted        bool          `json:"aborted"`
	AbortReason    string        `json:"abort_reason,omitempty"`
}

// NexusDistro is a stub on Linux.
type NexusDistro struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Version string `json:"version"`
	Default bool   `json:"default"`
}

// ExecFunc is a stub type on Linux for compilation compatibility.
type ExecFunc func(ctx context.Context, command string, args ...string) (string, error)

// AuditFunc is a stub type on Linux for compilation compatibility.
type AuditFunc func(action, target, result string, durationMs int64, err error)

// StateRecordFunc is a stub type on Linux for compilation compatibility.
type StateRecordFunc func(name, image, version, sha256, installPath, family string) error

// DefaultRootFSRegistry returns an empty registry on Linux.
func DefaultRootFSRegistry() []RootFSImage {
	return nil
}

// FindImage always returns an error on Linux.
func FindImage(name string) (*RootFSImage, error) {
	return nil, fmt.Errorf("WSL2 import is only available on Windows")
}

// ValidateDistroName is a no-op on Linux.
func ValidateDistroName(name string) error {
	return fmt.Errorf("WSL2 import is only available on Windows")
}

// ValidateInstallPath is a no-op on Linux.
func ValidateInstallPath(path string) error {
	return fmt.Errorf("WSL2 import is only available on Windows")
}

// GenerateWSLConf returns an empty string on Linux.
func GenerateWSLConf(defaultUser string) string {
	return ""
}

// NotAvailableError is returned when WSL2 import commands are invoked on Linux.
func NotAvailableError() error {
	return fmt.Errorf("WSL2 import commands are only available on Windows. " +
		"On Linux, you're already running natively — use 'nexus init' instead")
}

// FormatImportResult returns a "not available" message on Linux.
func FormatImportResult(result *ImportResult) string {
	return "\n  ⛔ WSL2 import is only available on Windows\n\n"
}

// WSL2Importer is a stub on Linux for compilation compatibility.
// All methods return not-available errors.
type WSL2Importer struct{}

// NewWSL2Importer creates a Linux stub importer.
// This exists for compilation compatibility — on Linux, IsImportAvailable()
// returns false, so the importer is never actually used.
func NewWSL2Importer(execFn ExecFunc) *WSL2Importer {
	return &WSL2Importer{}
}

// SetAuditFunc is a no-op on Linux.
func (w *WSL2Importer) SetAuditFunc(fn AuditFunc) {}

// SetStateRecordFunc is a no-op on Linux.
func (w *WSL2Importer) SetStateRecordFunc(fn StateRecordFunc) {}

// Import returns a not-available error on Linux.
func (w *WSL2Importer) Import(ctx context.Context, config *ImportConfig) (*ImportResult, error) {
	return &ImportResult{Aborted: true, AbortReason: "WSL2 import is only available on Windows"}, NotAvailableError()
}

// Remove returns a not-available error on Linux.
func (w *WSL2Importer) Remove(ctx context.Context, distroName string, force bool) error {
	return NotAvailableError()
}

// ListNexusDistros returns a not-available error on Linux.
func (w *WSL2Importer) ListNexusDistros(ctx context.Context) ([]NexusDistro, error) {
	return nil, NotAvailableError()
}
