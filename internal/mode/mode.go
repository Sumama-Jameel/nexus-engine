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

package mode

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mode is a declarative, switchable unit that bundles a profile reference,
// optional dotfile overlay, service toggles, and bounded OS tweaks.
//
// Per ADR 010: the same schema applies to built-in modes (embedded via
// //go:embed) and user-defined modes at ~/.nexus/modes/*.yaml. Users may
// override a built-in by placing a YAML file with the same Name.
type Mode struct {
	// Name is the unique identifier used in `nexus mode apply <name>`.
	Name string `yaml:"name"`
	// Description is a one-line summary shown in `nexus mode list`.
	Description string `yaml:"description"`
	// Profile is the name of the Nexus profile that will be applied when
	// this mode is activated. Must resolve in the local profile store.
	Profile string `yaml:"profile"`
	// DotfilesSource, when non-empty, re-binds the chezmoi source to this
	// URL or path before applying. Leave empty to leave dotfiles untouched.
	DotfilesSource string `yaml:"dotfiles_source"`
	// StopServices lists systemd / sc service names to stop.
	StopServices []string `yaml:"stop_services"`
	// StartServices lists systemd / sc service names to start.
	StartServices []string `yaml:"start_services"`
	// OSTweaks contains bounded, OS-specific tweaks (CPU governor on
	// Linux, power plan on Windows). Empty fields are no-ops.
	OSTweaks OSTweaks `yaml:"os_tweaks"`
	// Builtin is set by the loader; true for embedded defaults, false
	// for user-defined overrides. Not serialized.
	Builtin bool `yaml:"-"`
	// SourcePath is the on-disk path this Mode was loaded from; empty for
	// built-ins. Not serialized.
	SourcePath string `yaml:"-"`
}

// OSTweaks groups the bounded, OS-specific tweaks. Only the tweak matching
// the running OS is consulted by Apply; the other is ignored. This keeps
// a single Mode YAML portable across machines.
type OSTweaks struct {
	Linux   LinuxTweaks   `yaml:"linux"`
	Windows WindowsTweaks `yaml:"windows"`
}

// LinuxTweaks holds the tweaks applied when running on Linux.
type LinuxTweaks struct {
	// CPUGovernor maps to `cpupower frequency-set -g <value>`.
	// Allowed values: powersave, balanced, performance, schedutil.
	CPUGovernor string `yaml:"cpu_governor"`
}

// WindowsTweaks holds the tweaks applied when running on Windows.
type WindowsTweaks struct {
	// PowerPlan maps to `powercfg /setactive <value>`.
	// Allowed aliases: balanced, high_performance, power_saver.
	// Resolved to the canonical GUID at apply time.
	PowerPlan string `yaml:"power_plan"`
}

//go:embed builtins/*.yaml
var builtinFS embed.FS

// BuiltinNames lists the names of the three embedded defaults in a stable
// order (alphabetical). Used by `nexus mode list` to render the built-in
// section before user-defined modes.
func BuiltinNames() []string {
	return []string{"dev", "gamer", "work"}
}

// loadBuiltin parses one embedded YAML file into a Mode and stamps it as
// a built-in. Errors are surfaced verbatim so a corrupted embed fails
// loudly at startup rather than silently dropping the mode.
func loadBuiltin(name string) (*Mode, error) {
	data, err := builtinFS.ReadFile("builtins/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("builtin mode %q missing from embed: %w", name, err)
	}
	var m Mode
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("builtin mode %q has invalid YAML: %w", name, err)
	}
	m.Builtin = true
	if m.Name == "" {
		m.Name = name
	}
	if err := validate(&m); err != nil {
		return nil, fmt.Errorf("builtin mode %q invalid: %w", name, err)
	}
	return &m, nil
}

// loadAllBuiltins loads every embedded default. Failures are returned as a
// joined error so a single broken embed does not silently drop one mode.
func loadAllBuiltins() ([]*Mode, error) {
	var (
		out  []*Mode
		errs []string
	)
	for _, name := range BuiltinNames() {
		m, err := loadBuiltin(name)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		out = append(out, m)
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("builtin mode errors: %s", strings.Join(errs, "; "))
	}
	return out, nil
}

// userModeDir returns the per-machine override directory. Created lazily
// by the writer side (define.go); loaders tolerate absence.
func userModeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine HOME for user modes: %w", err)
	}
	return filepath.Join(home, ".nexus", "modes"), nil
}

// loadUserMode reads a single user-defined mode from ~/.nexus/modes/.
func loadUserMode(name string) (*Mode, error) {
	dir, err := userModeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Mode
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("user mode %q has invalid YAML: %w", name, err)
	}
	m.Builtin = false
	m.SourcePath = path
	if m.Name == "" {
		m.Name = name
	}
	if err := validate(&m); err != nil {
		return nil, fmt.Errorf("user mode %q invalid: %w", name, err)
	}
	return &m, nil
}

// loadAllUserModes scans ~/.nexus/modes/ for *.yaml files. Returns an
// empty slice (not an error) when the directory does not exist yet —
// first-run users have only the built-ins.
func loadAllUserModes() ([]*Mode, error) {
	dir, err := userModeDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read user mode dir %s: %w", dir, err)
	}
	var out []*Mode
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		m, err := loadUserMode(name)
		if err != nil {
			// Skip broken user files but keep loading the rest. The
			// operator should see the broken file in `nexus mode list`
			// diagnostics, so we surface the error in the SourcePath.
			out = append(out, &Mode{
				Name:        name,
				Description: "(invalid: " + err.Error() + ")",
				SourcePath:  filepath.Join(dir, e.Name()),
			})
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// Resolve returns the mode named `name`, preferring the per-machine user
// override over the embedded built-in. Returns an error if neither exists.
func Resolve(name string) (*Mode, error) {
	if name == "" {
		return nil, errors.New("mode name is required")
	}
	if user, err := loadUserMode(name); err == nil {
		return user, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		// Read or parse error on the user file — surface it instead of
		// silently falling back to a built-in that the user might have
		// intended to override.
		return nil, err
	}
	for _, bname := range BuiltinNames() {
		if bname == name {
			return loadBuiltin(name)
		}
	}
	return nil, fmt.Errorf("mode %q not found (built-ins: %s; user dir: %s)",
		name, strings.Join(BuiltinNames(), ", "), mustDir(userModeDir))
}

// mustDir is a tiny helper that calls f() and returns the path or
// "<unavailable>" — only used for error message formatting.
func mustDir(f func() (string, error)) string {
	p, err := f()
	if err != nil {
		return "<unavailable>"
	}
	return p
}

// List returns every available mode: built-ins plus user-defined, with
// user definitions taking precedence when names collide. The result is
// sorted by name for deterministic CLI output.
//
// Failures to enumerate user modes degrade gracefully: the built-ins are
// still returned and the error is included as the second return value so
// the CLI can warn without blocking the listing.
func List() ([]*Mode, error) {
	builtins, bErr := loadAllBuiltins()
	users, uErr := loadAllUserModes()

	byName := make(map[string]*Mode)
	for _, m := range users {
		byName[m.Name] = m
	}
	for _, m := range builtins {
		if _, exists := byName[m.Name]; !exists {
			byName[m.Name] = m
		}
	}

	out := make([]*Mode, 0, len(byName))
	for _, m := range byName {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	var err error
	switch {
	case bErr != nil && uErr != nil:
		err = fmt.Errorf("built-ins: %v; user modes: %v", bErr, uErr)
	case bErr != nil:
		err = bErr
	case uErr != nil:
		err = uErr
	}
	return out, err
}

// validate enforces the minimum invariants of a Mode. Both built-in and
// user-defined modes run through this — corruption is caught at load
// time, not at apply time when the damage is harder to roll back.
func validate(m *Mode) error {
	if m.Name == "" {
		return errors.New("name is required")
	}
	if strings.ContainsAny(m.Name, "/\\\n\r\t") {
		return fmt.Errorf("name %q contains path separators or whitespace", m.Name)
	}
	if m.Profile == "" {
		return fmt.Errorf("profile is required (referenced by %q)", m.Name)
	}
	for _, svc := range m.StopServices {
		if strings.TrimSpace(svc) == "" {
			return errors.New("stop_services contains an empty entry")
		}
	}
	for _, svc := range m.StartServices {
		if strings.TrimSpace(svc) == "" {
			return errors.New("start_services contains an empty entry")
		}
	}
	return nil
}
