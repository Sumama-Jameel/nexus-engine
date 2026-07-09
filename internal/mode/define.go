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
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefineInput is the structured input to Define. When In is nil, Define
// reads from stdin (interactive); when In is provided (tests), prompts
// are answered in order from the supplied strings.
type DefineInput struct {
	// In is the line reader. nil = os.Stdin.
	In *bufio.Reader
	// Out is the prompt writer. nil = os.Stdout.
	Out *os.File
	// NonInteractive suppresses prompts when set; Name and the rest of
	// the fields must already be populated on Draft.
	NonInteractive bool
	// Draft is the partial Mode to fill in. Only Name is required as
	// input; other fields may be pre-populated (e.g., from a flag).
	Draft Mode
}

// Define creates a user-defined mode at ~/.nexus/modes/<name>.yaml.
//
// The flow mirrors profileCreate from main.go: prompt for the missing
// fields, validate, then write the YAML. When NonInteractive is true,
// Define validates the existing Draft and writes it without prompting —
// useful for scripted / dashboard flows and for unit tests.
//
// If a mode with the same name already exists, Define refuses unless the
// caller deletes it first. This prevents accidental overwrite of a
// per-machine override that took time to tune.
func Define(input DefineInput) (*Mode, error) {
	in := input.In
	if in == nil {
		in = bufio.NewReader(os.Stdin)
	}
	out := input.Out
	if out == nil {
		out = os.Stdout
	}

	m := input.Draft

	if m.Name == "" && !input.NonInteractive {
		name, err := prompt(in, out, "Mode name (lowercase, no spaces): ")
		if err != nil {
			return nil, err
		}
		m.Name = strings.TrimSpace(name)
	}
	if m.Name == "" {
		return nil, errors.New("mode name is required")
	}

	if !input.NonInteractive {
		if m.Description == "" {
			desc, err := prompt(in, out, "Description: ")
			if err != nil {
				return nil, err
			}
			m.Description = strings.TrimSpace(desc)
		}
		if m.Profile == "" {
			prof, err := prompt(in, out, "Profile name (must exist in profile store): ")
			if err != nil {
				return nil, err
			}
			m.Profile = strings.TrimSpace(prof)
		}
		if m.DotfilesSource == "" {
			ds, err := prompt(in, out, "Dotfiles source (URL or path, blank to skip): ")
			if err != nil {
				return nil, err
			}
			m.DotfilesSource = strings.TrimSpace(ds)
		}
		if len(m.StopServices) == 0 {
			ss, err := prompt(in, out, "Services to STOP (comma-separated, blank for none): ")
			if err != nil {
				return nil, err
			}
			m.StopServices = splitCSV(ss)
		}
		if len(m.StartServices) == 0 {
			ss, err := prompt(in, out, "Services to START (comma-separated, blank for none): ")
			if err != nil {
				return nil, err
			}
			m.StartServices = splitCSV(ss)
		}
		if m.OSTweaks.Linux.CPUGovernor == "" && m.OSTweaks.Windows.PowerPlan == "" {
			gov, err := prompt(in, out, "Linux CPU governor (powersave/balanced/performance, blank to skip): ")
			if err != nil {
				return nil, err
			}
			m.OSTweaks.Linux.CPUGovernor = strings.TrimSpace(gov)
			pp, err := prompt(in, out, "Windows power plan (balanced/high_performance/power_saver, blank to skip): ")
			if err != nil {
				return nil, err
			}
			m.OSTweaks.Windows.PowerPlan = strings.TrimSpace(pp)
		}
	}

	if err := validate(&m); err != nil {
		return nil, fmt.Errorf("invalid mode: %w", err)
	}

	dir, err := userModeDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create user mode dir %s: %w", dir, err)
	}
	path := filepath.Join(dir, m.Name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("mode %q already exists at %s — delete it first to redefine", m.Name, path)
	}

	data, err := yaml.Marshal(&m)
	if err != nil {
		return nil, fmt.Errorf("cannot serialize mode: %w", err)
	}
	// Header comment makes the file self-documenting when an operator
	// opens it in an editor.
	header := "# Nexus user-defined mode — edit safely; `nexus mode apply` re-reads on every invocation.\n"
	if err := os.WriteFile(path, []byte(header+string(data)), 0644); err != nil {
		return nil, fmt.Errorf("cannot write mode file: %w", err)
	}

	m.Builtin = false
	m.SourcePath = path
	return &m, nil
}

// prompt writes the prompt to out and reads one trimmed line from in.
// Empty input is allowed and returned as "" so callers can distinguish
// "user pressed enter" from "io error".
func prompt(in *bufio.Reader, out *os.File, label string) (string, error) {
	if _, err := fmt.Fprint(out, label); err != nil {
		return "", err
	}
	line, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// splitCSV parses a comma-separated line into a trimmed, non-empty slice.
// Returns nil (not an empty slice) for blank input so YAML serialization
// renders `stop_services: []` correctly.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
