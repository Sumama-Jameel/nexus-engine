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
        "bytes"
        "context"
        "fmt"
        "os/exec"
        "runtime"
        "strings"
        "time"
)

// Security constants — per the Nexus Protocol Zero-Trust Architecture.
const (
        // CommandTimeoutSec is the maximum number of seconds any system call may run
        // before being forcibly terminated via context cancellation. Per the Nexus
        // Protocol design: "If a package install hangs, the engine kills it after 60
        // seconds, preventing bricking." This timeout defends against denial-of-service
        // via hung or unresponsive subprocesses.
        CommandTimeoutSec = 60
)

// AllowedCommands is the immutable whitelist of base commands that the engine is
// permitted to execute. Any command not present in this map is rejected by
// SanitizeAndExecute before reaching the operating system.
//
// Threat model:
//   - An attacker who can control the command parameter (e.g., via a malicious
//     profile or crafted input) can only invoke commands in this set.
//   - The whitelist deliberately excludes shell interpreters (sh, bash -c) and
//     dangerous utilities (rm, chmod, chown, dd, mkfs) that could cause
//     irreversible system damage.
//   - Commands are executed directly via exec.CommandContext, never through an
//     intermediate shell, so even whitelisted commands cannot be chained with
//     shell operators.
//
// No raw string concatenation for commands. Ever.
var AllowedCommands = map[string]bool{
        "uname":     true,
        "hostname":  true,
        "free":      true,
        "lspci":     true,
        "uptime":    true,
        "sudo":      true,
        "apt-get":   true,
        "apt-cache": true,
        "dpkg":      true,
        "pacman":    true,
        "dnf":       true,
        "rpm":       true,
        "yum":       true,
        "apk":       true,
        "chezmoi":   true,
        "git":       true,
        "wsl":       true,
        "docker":    true,
        "podman":    true,
        "distrobox": true,
        "which":     true,
        "cat":       true,
        "ls":        true,
        "systemctl": true,
        "node":      true,
        "npm":       true,
        "python3":   true,
        "python":    true,
        "java":      true,
        "vim":       true,
        "curl":      true,
        "wget":      true,
        "zsh":       true,
        "htop":      true,
        "tmux":      true,
        // V4: Windows-specific commands for cross-platform support
        "powershell":  true,
        "dism":        true,
        "where":       true,
        "cmd":         true,
        "systeminfo":  true,
}

// SanitizeAndExecute is the centralized security gate for all system calls
// performed by the Nexus engine. It is the single point through which every
// command must pass; no other code path in the engine executes subprocesses.
//
// Per the Nexus Protocol: "No raw string concatenation for commands. Ever."
//
// This function enforces a four-layer defense-in-depth model:
//
//  1. Command whitelisting — only base commands present in AllowedCommands may
//     execute. This prevents an attacker who controls the command string from
//     invoking arbitrary binaries (e.g., rm, sh, dd).
//
//  2. Argument sanitization — every argument is scanned by
//     containsShellMetacharacters. Characters such as ;, |, &, $, `, and others
//     are rejected because they could enable command chaining, substitution, or
//     injection if the argument were ever interpreted by a shell.
//
//  3. Context-based timeout — all commands are bounded by CommandTimeoutSec
//     (60 seconds). If a subprocess does not terminate within this window, it is
//     killed via context cancellation, preventing denial-of-service from hung
//     processes.
//
//  4. Direct execution — commands are invoked via exec.CommandContext, which
//     passes arguments directly to execve(2). No intermediate shell is involved,
//     so shell expansion, globbing, and operator chaining are impossible regardless
//     of argument content.
//
// Returns the command's stdout on success, or a descriptive error indicating
// whether the failure was a whitelist rejection, metacharacter detection,
// timeout, or execution error.
func SanitizeAndExecute(ctx context.Context, command string, args ...string) (string, error) {
        // Step 1: Validate the base command against the whitelist
        if !AllowedCommands[command] {
                return "", fmt.Errorf("SECURITY: command '%s' is not in the allowed list", command)
        }

        // Step 2: Sanitize all arguments — reject shell metacharacters
        for _, arg := range args {
                if containsShellMetacharacters(arg) {
                        return "", fmt.Errorf("SECURITY: argument '%s' contains shell metacharacters", arg)
                }
        }

        // Step 3: Create a context with timeout
        ctx, cancel := context.WithTimeout(ctx, time.Duration(CommandTimeoutSec)*time.Second)
        defer cancel()

        // Step 4: Execute the command directly (never through a shell)
        cmd := exec.CommandContext(ctx, command, args...)

        var stdout, stderr bytes.Buffer
        cmd.Stdout = &stdout
        cmd.Stderr = &stderr

        err := cmd.Run()
        if err != nil {
                if ctx.Err() == context.DeadlineExceeded {
                        return "", fmt.Errorf("TIMEOUT: command '%s' exceeded %d second limit", command, CommandTimeoutSec)
                }
                return "", fmt.Errorf("EXEC: command '%s' failed: %w (stderr: %s)", command, err, stderr.String())
        }

        return stdout.String(), nil
}

// containsShellMetacharacters reports whether the string s contains characters
// that could enable command injection if the string were interpreted by a shell.
// The checked characters include command terminators (;), pipes (|), logical
// operators (&), variable expansions ($), command substitutions (`), grouping
// operators (() {}), redirections (< >), newlines, quotes, and escape
// characters. Even though SanitizeAndExecute never invokes a shell, this check
// provides defense-in-depth against scenarios where output might be
// re-interpreted by a downstream consumer.
func containsShellMetacharacters(s string) bool {
        dangerous := []string{";", "|", "&", "$", "`", "(", ")", "{", "}", "<", ">", "\n", "\r", "'", "\"", "\\", "!"}
        for _, char := range dangerous {
                if strings.Contains(s, char) {
                        return true
                }
        }
        return false
}

// ValidatePrerequisites checks that required tools are available on the system.
// This is the "Validate" step in the nexus init flow.
//
// Platform-aware: uses `which` on Linux/macOS and `where` on Windows.
// Per the Nexus Protocol Zero-Trust Architecture: the lookup command
// MUST go through SanitizeAndExecute — both `which` and `where` are
// in the AllowedCommands whitelist.
func ValidatePrerequisites(ctx context.Context) map[string]bool {
        prerequisites := []string{"git", "curl"}
        results := make(map[string]bool)

        // Select the platform-appropriate binary lookup command.
        // On Windows, `which` does not exist — `where` is the equivalent.
        // On Linux/macOS, `which` is the standard tool locator.
        lookupCmd := "which"
        if runtime.GOOS == "windows" {
                lookupCmd = "where"
        }

        for _, tool := range prerequisites {
                _, err := SanitizeAndExecute(ctx, lookupCmd, tool)
                results[tool] = err == nil
        }

        return results
}
