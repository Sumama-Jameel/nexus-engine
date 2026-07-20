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

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ConfigureResult tracks the outcome of the Configure step, providing a
// detailed breakdown of each sub-operation for user feedback and debugging.
type ConfigureResult struct {
	// ChezmoiInstalled indicates whether the Chezmoi dotfile manager was found
	// on the system PATH.
	ChezmoiInstalled bool `json:"chezmoi_installed"`
	// ChezmoiInitialized indicates whether Chezmoi's data directory exists
	// (or was successfully created) at ~/.local/share/chezmoi.
	ChezmoiInitialized bool `json:"chezmoi_initialized"`
	// ShellConfigWritten indicates whether the Nexus-optimized shell
	// configuration block was successfully injected into the user's rc file.
	ShellConfigWritten bool `json:"shell_config_written"`
	// ShellConfigPath is the filesystem path to the modified shell rc file
	// (e.g., ~/.zshrc or ~/.bashrc).
	ShellConfigPath string `json:"shell_config_path"`
	// ShellType is the detected shell family ("zsh" or "bash"), determined
	// by inspecting the SHELL environment variable.
	ShellType string `json:"shell_type"`
	// NexusDirCreated indicates whether the ~/.nexus directory was created
	// during this run (false if it already existed).
	NexusDirCreated bool `json:"nexus_dir_created"`
	// NexusDir is the absolute path to the Nexus configuration directory,
	// typically ~/.nexus.
	NexusDir string `json:"nexus_dir"`
	// EnvVarsApplied is the count of profile environment variables that were
	// injected into the shell configuration block.
	EnvVarsApplied int `json:"env_vars_applied"`
	// Messages is a list of human-readable status messages describing each
	// sub-operation performed or skipped during configuration.
	Messages []string `json:"messages"`
}

// Configure executes the CONFIGURE step of nexus init:
// 1. Creates the ~/.nexus directory
// 2. Checks for Chezmoi and initializes dotfile tracking
// 3. Injects the Nexus-Optimized ZSH/Bash config WITH profile env vars
//
// Per the V3 plan: "When a profile is applied, the env: map becomes
// export KEY=VALUE lines inside the Nexus shell config block."
// This is idempotent — re-applying the same profile updates values
// without duplication.
func Configure(ctx context.Context, envVars map[string]string) *ConfigureResult {
	result := &ConfigureResult{
		Messages: []string{},
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		result.Messages = append(result.Messages, "Could not determine home directory")
		return result
	}

	// Step 1: Create ~/.nexus directory
	nexusDir := filepath.Join(homeDir, ".nexus")
	if err := os.MkdirAll(nexusDir, 0755); err != nil {
		result.Messages = append(result.Messages, fmt.Sprintf("Failed to create %s: %v", nexusDir, err))
	} else {
		result.NexusDirCreated = true
		result.NexusDir = nexusDir
		result.Messages = append(result.Messages, fmt.Sprintf("Created %s", nexusDir))
	}

	// Step 2: Check for Chezmoi and initialize
	_, err = SanitizeAndExecute(ctx, "which", "chezmoi")
	if err != nil {
		result.ChezmoiInstalled = false
		result.Messages = append(result.Messages, "Chezmoi not found — dotfile tracking will be available after installation")
		result.Messages = append(result.Messages, "Install Chezmoi: sh -c \"$(curl -fsLS get.chezmoi.io)\"")
	} else {
		result.ChezmoiInstalled = true
		// Attempt chezmoi init if not already initialized
		chezmoiDir := filepath.Join(homeDir, ".local", "share", "chezmoi")
		if _, err := os.Stat(chezmoiDir); os.IsNotExist(err) {
			// Initialize chezmoi with a minimal config
			output, err := SanitizeAndExecute(ctx, "chezmoi", "init")
			if err != nil {
				result.ChezmoiInitialized = false
				result.Messages = append(result.Messages, fmt.Sprintf("Chezmoi init failed: %v", err))
			} else {
				result.ChezmoiInitialized = true
				result.Messages = append(result.Messages, "Chezmoi initialized successfully")
				if strings.TrimSpace(output) != "" {
					result.Messages = append(result.Messages, output)
				}
			}
		} else {
			result.ChezmoiInitialized = true
			result.Messages = append(result.Messages, "Chezmoi already initialized")
		}
	}

	// Step 3: Inject Nexus-Optimized Shell Config WITH profile env vars
	shellType := detectShell()
	result.ShellType = shellType

	var shellConfig string
	switch shellType {
	case "zsh":
		shellConfig = generateZshConfig(envVars)
		result.ShellConfigPath = filepath.Join(homeDir, ".zshrc")
	case "bash":
		shellConfig = generateBashConfig(envVars)
		result.ShellConfigPath = filepath.Join(homeDir, ".bashrc")
	default:
		shellConfig = generateBashConfig(envVars)
		result.ShellConfigPath = filepath.Join(homeDir, ".bashrc")
		result.ShellType = "bash"
	}

	if shellConfig != "" {
		if err := injectShellConfig(result.ShellConfigPath, shellConfig); err != nil {
			result.ShellConfigWritten = false
			result.Messages = append(result.Messages, fmt.Sprintf("Failed to write shell config: %v", err))
		} else {
			result.ShellConfigWritten = true
			result.EnvVarsApplied = len(envVars)
			result.Messages = append(result.Messages, fmt.Sprintf("Nexus config injected into %s (%d env vars)", result.ShellConfigPath, len(envVars)))
		}
	}

	return result
}

// detectShell determines the user's default shell.
func detectShell() string {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	if strings.Contains(shell, "bash") {
		return "bash"
	}
	// Default to bash
	return "bash"
}

// generateEnvExports converts a map of env vars to shell export lines.
// The env vars are sorted alphabetically for deterministic output.
// Per V3: NEXUS_PROFILE is ALWAYS set to the active profile name.
func generateEnvExports(envVars map[string]string) string {
	if len(envVars) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		v := envVars[k]
		// Sanitize: reject env vars with shell metacharacters in the value
		if containsShellMetacharacters(v) {
			sb.WriteString(fmt.Sprintf("# WARNING: %s skipped — value contains shell metacharacters\n", k))
			continue
		}
		sb.WriteString(fmt.Sprintf("export %s=\"%s\"\n", k, v))
	}
	return sb.String()
}

// generateZshConfig creates the Nexus-Optimized ZSH configuration block.
func generateZshConfig(envVars map[string]string) string {
	envBlock := generateEnvExports(envVars)

	return `

# ─── NEXUS PROTOCOL — Optimized ZSH Config ───
# Auto-injected by 'nexus init'
# Do not remove the marker lines — Nexus uses them to manage this block
# ─── NEXUS_START ───

# Nexus Environment
export NEXUS_HOME="$HOME/.nexus"
export PATH="$NEXUS_HOME/bin:$PATH"
` + envBlock + `
# Prompt — Clean, informative, Nexus-branded
NEXUS_PROMPT='%F{cyan}⬡%f %F{green}%~%f %F{yellow}▶%f '
PROMPT="$NEXUS_PROMPT"

# History — Never lose a command
HISTSIZE=10000
SAVEHIST=10000
HISTFILE="$NEXUS_HOME/.zsh_history"
setopt SHARE_HISTORY
setopt HIST_IGNORE_DUPS

# Aliases — Developer Efficiency
alias ll='ls -alF --color=auto'
alias la='ls -A --color=auto'
alias l='ls -CF --color=auto'
alias gs='git status'
alias gd='git diff'
alias gp='git pull'
alias gc='git commit'
alias nprobe='nexus probe'
alias ninit='nexus init'

# Key bindings
bindkey -e  # Emacs mode
bindkey '^[[A' history-search-backward
bindkey '^[[B' history-search-forward

# ─── NEXUS_END ───
`
}

// generateBashConfig creates the Nexus-Optimized Bash configuration block.
func generateBashConfig(envVars map[string]string) string {
	envBlock := generateEnvExports(envVars)

	return `

# ─── NEXUS PROTOCOL — Optimized Bash Config ───
# Auto-injected by 'nexus init'
# Do not remove the marker lines — Nexus uses them to manage this block
# ─── NEXUS_START ───

# Nexus Environment
export NEXUS_HOME="$HOME/.nexus"
export PATH="$NEXUS_HOME/bin:$PATH"
` + envBlock + `
# Prompt — Clean, informative, Nexus-branded
NEXUS_PROMPT='\[\033[0;36m\]⬡\[\033[0m\] \[\033[0;32m\]\w\[\033[0m\] \[\033[0;33m\]▶\[\033[0m\] '
PS1="$NEXUS_PROMPT"

# History — Never lose a command
HISTSIZE=10000
HISTFILESIZE=10000
HISTCONTROL=ignoredups:erasedups
shopt -s histappend
PROMPT_COMMAND="history -a; history -c; history -r; $PROMPT_COMMAND"

# Aliases — Developer Efficiency
alias ll='ls -alF --color=auto'
alias la='ls -A --color=auto'
alias l='ls -CF --color=auto'
alias gs='git status'
alias gd='git diff'
alias gp='git pull'
alias gc='git commit'
alias nprobe='nexus probe'
alias ninit='nexus init'

# ─── NEXUS_END ───
`
}

// injectShellConfig writes the Nexus config block into the shell config file.
// It checks if the block already exists (by looking for the NEXUS_START marker)
// and either updates it or appends it.
func injectShellConfig(configPath string, config string) error {
	// Read existing config
	existing := ""
	data, err := os.ReadFile(configPath)
	if err == nil {
		existing = string(data)
	}

	// Check if Nexus block already exists
	startMarker := "# ─── NEXUS_START ───"
	endMarker := "# ─── NEXUS_END ───"

	if strings.Contains(existing, startMarker) {
		// Replace existing Nexus block
		startIdx := strings.Index(existing, startMarker)
		// Find the beginning of the block (include the header comment line before NEXUS_START)
		blockStart := strings.Index(existing, "# ─── NEXUS PROTOCOL")
		if blockStart == -1 || blockStart > startIdx {
			blockStart = startIdx
		}
		endIdx := strings.Index(existing, endMarker) + len(endMarker)

		if endIdx > startIdx {
			existing = existing[:blockStart] + existing[endIdx:]
		}
	}

	// Append the new config
	newContent := strings.TrimSuffix(existing, "\n") + config

	return os.WriteFile(configPath, []byte(newContent), 0644)
}
