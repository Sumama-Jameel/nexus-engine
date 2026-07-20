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
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// StateTracker defines the subset of state operations needed by the container
// package. Using this interface instead of a concrete engine.StateTracker
// prevents circular imports and enables testing with mocks.
type StateTracker interface {
	RecordContainerEnter(name string) error
	RecordContainerRemove(name string) error
	RecordContainerCreate(name, image, family string) error
	IsContainerManaged(name string) bool
	GetContainerNames() []string
}

// Container records a Nexus-managed Distrobox container. Every field is
// validated before being persisted — tampered state files fail on load.
type Container struct {
	// Name is the unique identifier used by `nexus container <name>`.
	// Must match the Distrobox naming convention.
	Name string `json:"name"`
	// Image is the OCI reference, e.g., "fedora:39" or "ubuntu:22.04".
	Image string `json:"image"`
	// Family is the OS family for future OS-specific tweaks.
	Family string `json:"family"`
	// CreatedAt is the UTC timestamp of the create.
	CreatedAt time.Time `json:"created_at"`
	// SourcePath is where the container lives in the filesystem.
	// Always under $HOME/.local/share/distrobox/.
	SourcePath string `json:"source_path,omitempty"`
}

// validateName checks the Distrobox naming convention:
// - starts with alphanumeric
// - only alphanumeric, hyphen, underscore
// - max 64 chars
// - no shell metacharacters
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// validateImage checks the OCI reference:
// - starts with alphanumeric
// - after that: alphanumeric, ., _, /, -, :
// - tag part after : is alphanumeric + hyphen + underscore + dot
// - max 128 chars before tag, max 128 after tag

func validateImageRef(img string) error {
	if img == "" {
		return errors.New("image reference is required")
	}
	if len(img) > 256 {
		return errors.New("image reference too long (max 256)")
	}

	// Reject path traversal and CLI injection patterns
	if strings.Contains(img, "..") {
		return errors.New("image ref cannot contain '..'")
	}
	if strings.HasPrefix(img, "--") {
		return errors.New("image ref cannot start with '--'")
	}

	// Split on : for tag
	parts := strings.SplitN(img, ":", 2)
	base := parts[0]
	tag := ""
	if len(parts) == 2 {
		tag = parts[1]
	}

	// Validate base
	if base == "" {
		return errors.New("image base is required")
	}
	if len(base) > 128 {
		return errors.New("image base too long")
	}
	for _, c := range base {
		if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '.' && c != '_' && c != '/' && c != '-' {
			return fmt.Errorf("invalid character %q in image base", c)
		}
	}

	// Validate tag
	if tag != "" {
		if len(tag) > 128 {
			return errors.New("image tag too long")
		}
		// Only one : allowed (tag is the part after the first colon)
		// a:b:c is invalid because tag would contain ':'
		if strings.Contains(tag, ":") {
			return errors.New("image tag cannot contain ':'")
		}
		for _, c := range tag {
			if !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') && c != '.' && c != '_' && c != '-' {
				return fmt.Errorf("invalid character %q in image tag", c)
			}
		}
	}

	return nil
}

// validateName enforces the container name convention:
// - alphanumeric, hyphen, underscore
// - no path separators, no shell metachars
func validateName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > 64 {
		return errors.New("name too long (max 64)")
	}
	if !nameRe.MatchString(name) {
		return errors.New("name must start with alphanumeric; only a-z, A-Z, 0-9, hyphen, underscore allowed")
	}
	if strings.ContainsAny(name, "../\\") {
		return errors.New("name contains path separators")
	}
	return nil
}

// detectFamily maps common base images to their OS families.
// Used for state tracking and future OS-specific tweaks.
func detectFamily(img string) string {
	img = strings.ToLower(img)
	switch {
	case strings.Contains(img, "alpine"):
		return "alpine"
	case strings.Contains(img, "arch") || strings.Contains(img, "archlinux"):
		return "arch"
	case strings.Contains(img, "debian") || strings.Contains(img, "ubuntu"):
		return "debian"
	case strings.Contains(img, "fedora") || strings.Contains(img, "centos") || strings.Contains(img, "rhel"):
		return "fedora"
	case strings.Contains(img, "opensuse") || strings.Contains(img, "suse"):
		return "suse"
	default:
		return "unknown"
	}
}
