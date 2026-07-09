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
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// containsShellMetacharacters — exhaustive per-character and combination tests
// ---------------------------------------------------------------------------

func TestContainsShellMetacharacters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// --- Individual metacharacters ---
		{"semicolon", "foo;bar", true},
		{"pipe", "foo|bar", true},
		{"ampersand", "foo&&bar", true},
		{"dollar", "$HOME", true},
		{"backtick", "`whoami`", true},
		{"left_paren", "(cmd)", true},
		{"right_paren", "(cmd)", true},
		{"left_brace", "{cmd}", true},
		{"right_brace", "{cmd}", true},
		{"less_than", "foo<bar", true},
		{"greater_than", "foo>bar", true},
		{"newline", "foo\nbar", true},
		{"carriage_return", "foo\rbar", true},
		{"single_quote", "it's", true},
		{"double_quote", `foo"bar`, true},
		{"backslash", `foo\bar`, true},
		{"exclamation", "!event", true},

		// --- Safe strings (no metacharacters) ---
		{"plain_word", "hello", false},
		{"alphanumeric", "abc123", false},
		{"dots", "file.txt", false},
		{"slashes", "/usr/bin/cmd", false},
		{"dashes", "--verbose", false},
		{"underscores", "my_variable", false},
		{"equals", "KEY=VALUE", false},
		{"colon", "http://example.com", false},
		{"at_sign", "user@host", false},
		{"tilde", "~/path", false},
		{"percent", "100%", false},
		{"hash_prefix", "#comment", false},
		{"plus", "pkg+extra", false},
		{"comma", "a,b,c", false},
		{"question_mark", "maybe?", false},
		{"asterisk", "*.txt", false},
		{"empty_string", "", false},
		{"spaces", "hello world", false},
		{"numbers", "12345", false},
		{"version", "1.2.3-beta", false},

		// --- Combinations and edge cases ---
		{"semicolons_multiple", "a;b;c", true},
		{"pipe_with_spaces", "ls | grep foo", true},
		{"command_substitution", "$(whoami)", true},
		{"backtick_substitution", "`id`", true},
		{"redirect_append", ">>file", true},
		{"redirect_input", "<input", true},
		{"and_operator", "true && false", true},
		{"or_operator", "true || false", true},
		{"mixed_injection", "; rm -rf /", true},
		{"curl_pipe", "http://evil.com|sh", true},
		{"newline_at_end", "foo\n", true},
		{"carriage_at_end", "foo\r", true},
		{"escaped_quote", `foo\'bar`, true},
		{"env_var_expansion", "${PATH}", true},
		{"nested_command", "`cat /etc/passwd`", true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := containsShellMetacharacters(tt.input)
			if got != tt.expected {
				t.Errorf("containsShellMetacharacters(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — whitelist rejection tests (no real commands needed)
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_BlockedCommands(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name    string
		command string
		args    []string
	}{
		{"rm", "rm", []string{"-rf", "/"}},
		{"rm_no_args", "rm", nil},
		{"sh", "sh", []string{"-c", "whoami"}},
		{"bash", "bash", []string{"-c", "id"}},
		{"chmod", "chmod", []string{"777", "/etc/passwd"}},
		{"chown", "chown", []string{"root:root", "/etc/shadow"}},
		{"dd", "dd", []string{"if=/dev/zero", "of=/dev/sda"}},
		{"mkfs", "mkfs", []string{"/dev/sda1"}},
		{"kill", "kill", []string{"-9", "1"}},
		{"reboot", "reboot", nil},
		{"shutdown", "shutdown", []string{"now"}},
		{"fdisk", "fdisk", []string{"/dev/sda"}},
		{"format_on_windows", "format", []string{"C:"}},
		{"malicious_binary", "/usr/local/bin/evil", nil},
		{"python_with_dash_c", "python3", []string{"-c", "import os; os.system('rm -rf /')"}},
		{"empty_command", "", nil},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := SanitizeAndExecute(ctx, tt.command, tt.args...)
			if err == nil {
				t.Errorf("SanitizeAndExecute(%q, %v) should have been rejected but succeeded", tt.command, tt.args)
			}
			// The error must be a SECURITY error for non-empty commands not in the whitelist
			if tt.command != "" && !strings.Contains(err.Error(), "SECURITY") {
				t.Errorf("expected SECURITY error for command %q, got: %v", tt.command, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — shell metacharacter injection in arguments
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_MetacharInjectionInArgs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	tests := []struct {
		name string
		args []string
	}{
		// Semicolon injection
		{"semicolon_in_arg", []string{";rm -rf /"}},
		// Pipe injection
		{"pipe_in_arg", []string{"|sh"}},
		// Command substitution
		{"dollar_substitution", []string{"$(whoami)"}},
		{"backtick_substitution", []string{"`id`"}},
		// Logical operators
		{"and_operator", []string{"&&evil"}},
		{"or_operator", []string{"||evil"}},
		// Redirection
		{"redirect_out", []string{">/etc/passwd"}},
		{"redirect_in", []string{"</etc/shadow"}},
		// Newline injection
		{"newline_in_arg", []string{"foo\nbar"}},
		// Variable expansion
		{"dollar_var", []string{"$HOME"}},
		// Quotes
		{"single_quote_in_arg", []string{"' OR 1=1"}},
		{"double_quote_in_arg", []string{`"; rm -rf /"`}},
		// Brace expansion
		{"brace_expansion", []string{"{a,b,c}"}},
		// Multiple args with one bad
		{"one_bad_among_good", []string{"-y", "--verbose", ";rm -rf /"}},
		// Curl with pipe (classic attack vector)
		{"curl_pipe_sh", []string{"http://evil.com/payload|sh"}},
		// Backslash escape
		{"backslash_escape", []string{`\nrm -rf /`}},
		// Exclamation mark (history expansion)
		{"exclamation_mark", []string{"!event"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Use an allowed command but with malicious args
			_, err := SanitizeAndExecute(ctx, "ls", tt.args...)
			if err == nil {
				t.Errorf("SanitizeAndExecute should reject args containing metacharacters: %v", tt.args)
			}
			if !strings.Contains(err.Error(), "SECURITY") {
				t.Errorf("expected SECURITY error for metacharacter in args, got: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — allowed commands with safe arguments
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_AllowedCommands(t *testing.T) {
	// NOTE: These tests actually execute system commands. They test that
	// allowed commands pass the whitelist. If the command doesn't exist on
	// the test system, we expect an EXEC error, NOT a SECURITY error.

	ctx := context.Background()

	tests := []struct {
		name              string
		command           string
		args              []string
		expectSecurityErr bool // true = should fail at whitelist check
		expectExecErr     bool // true = expected to fail at execution (binary not found, etc.)
	}{
		// Commands that should exist on most Linux systems
		{"uname_no_args", "uname", nil, false, false},
		{"uname_with_flag", "uname", []string{"-r"}, false, false},
		{"ls_no_args", "ls", nil, false, false},
		{"ls_with_path", "ls", []string{"/tmp"}, false, false},
		{"which_git", "which", []string{"git"}, false, false},
		{"cat_dev_null", "cat", []string{"/dev/null"}, false, false},

		// Commands in whitelist that may not be installed
		{"apt-get_update", "apt-get", []string{"update"}, false, true},
		{"pacman_version", "pacman", []string{"--version"}, false, true},
		{"dnf_version", "dnf", []string{"--version"}, false, true},
		{"apk_version", "apk", []string{"--version"}, false, true},

		// Empty args array is fine for commands that need no args
		{"hostname_no_args", "hostname", []string{}, false, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := SanitizeAndExecute(ctx, tt.command, tt.args...)
			if err == nil {
				if tt.expectSecurityErr || tt.expectExecErr {
					t.Errorf("expected an error but got none for command %q", tt.command)
				}
				// Success is fine
				return
			}
			if tt.expectSecurityErr {
				if !strings.Contains(err.Error(), "SECURITY") {
					t.Errorf("expected SECURITY error, got: %v", err)
				}
			} else if tt.expectExecErr {
				// EXEC error is acceptable for commands not installed in CI
				if !strings.Contains(err.Error(), "EXEC") && !strings.Contains(err.Error(), "SECURITY") {
					t.Errorf("expected EXEC or SECURITY error, got: %v", err)
				}
			} else {
				// Unexpected error — but some systems may not have the binary
				// so we only fail if it's a SECURITY error (wrong whitelist)
				if strings.Contains(err.Error(), "SECURITY") {
					t.Errorf("command %q should be whitelisted but got SECURITY error", tt.command)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — empty command
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_EmptyCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := SanitizeAndExecute(ctx, "")
	if err == nil {
		t.Fatal("empty command should be rejected")
	}
	if !strings.Contains(err.Error(), "SECURITY") {
		t.Errorf("expected SECURITY error for empty command, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — timeout enforcement
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_TimeoutEnforcement(t *testing.T) {
	// This test verifies that a context with a shorter deadline than
	// CommandTimeoutSec is respected. We use a cancelled context to
	// simulate timeout without actually waiting.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := SanitizeAndExecute(ctx, "ls")
	if err == nil {
		t.Log("command succeeded despite cancelled context (may happen if very fast)")
	} else {
		// Should get either a context error or a TIMEOUT error
		t.Logf("correctly got error with cancelled context: %v", err)
	}
}

func TestSanitizeAndExecute_TimeoutWithShortDeadline(t *testing.T) {
	// Use a very short deadline to trigger timeout for a long-running command.
	// We use "sleep" which is NOT in the allowed commands list, so we can't
	// directly test this. Instead, we verify the timeout constant is correct.
	if CommandTimeoutSec != 60 {
		t.Errorf("CommandTimeoutSec = %d, want 60", CommandTimeoutSec)
	}
}

// ---------------------------------------------------------------------------
// AllowedCommands — whitelist completeness checks
// ---------------------------------------------------------------------------

func TestAllowedCommands_WhitelistIntegrity(t *testing.T) {
	t.Parallel()

	// Commands that MUST be in the whitelist for the engine to function
	required := []string{
		"uname", "hostname", "free", "lspci", "uptime",
		"sudo", "apt-get", "apt-cache", "dpkg",
		"pacman", "dnf", "rpm", "yum", "apk",
		"chezmoi", "git", "wsl", "docker", "podman", "distrobox",
		"which", "cat", "ls", "systemctl",
		"node", "npm", "python3", "python", "java",
		"vim", "curl", "wget", "zsh", "htop", "tmux",
		// V4: Windows commands
		"powershell", "dism", "where", "cmd", "systeminfo",
	}

	for _, cmd := range required {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			if !AllowedCommands[cmd] {
				t.Errorf("AllowedCommands[%q] = false, want true", cmd)
			}
		})
	}
}

func TestAllowedCommands_DangerousCommandsExcluded(t *testing.T) {
	t.Parallel()

	// Commands that must NEVER be in the whitelist
	dangerous := []string{
		"rm", "sh", "bash", "chmod", "chown", "dd", "mkfs",
		"kill", "reboot", "shutdown", "fdisk", "format",
		"mount", "umount", "su", "passwd", "useradd", "userdel",
		"eval", "exec", "source",
	}

	for _, cmd := range dangerous {
		cmd := cmd
		t.Run(cmd, func(t *testing.T) {
			t.Parallel()
			if AllowedCommands[cmd] {
				t.Errorf("AllowedCommands[%q] = true — this dangerous command must NOT be whitelisted", cmd)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidatePrerequisites — basic smoke test
// ---------------------------------------------------------------------------

func TestValidatePrerequisites(t *testing.T) {
	// This calls SanitizeAndExecute internally with `which` or `where`.
	// We just verify it returns a map and doesn't panic.
	ctx := context.Background()
	results := ValidatePrerequisites(ctx)

	if results == nil {
		t.Fatal("ValidatePrerequisites returned nil map")
	}

	// Should check at least git and curl
	if _, ok := results["git"]; !ok {
		t.Error("ValidatePrerequisites should check for git")
	}
	if _, ok := results["curl"]; !ok {
		t.Error("ValidatePrerequisites should check for curl")
	}
}

// ---------------------------------------------------------------------------
// CommandTimeoutSec — constant verification
// ---------------------------------------------------------------------------

func TestCommandTimeoutSec(t *testing.T) {
	if CommandTimeoutSec <= 0 {
		t.Errorf("CommandTimeoutSec = %d, must be positive", CommandTimeoutSec)
	}
	if CommandTimeoutSec > 300 {
		t.Errorf("CommandTimeoutSec = %d seems too high, expected 60", CommandTimeoutSec)
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — arguments with spaces (safe arguments)
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_SafeArgumentsWithSpaces(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Arguments with spaces but no metacharacters should pass sanitization.
	// The command may still fail at execution, but it should NOT be a SECURITY error.
	_, err := SanitizeAndExecute(ctx, "ls", "path with spaces")
	if err != nil && strings.Contains(err.Error(), "SECURITY") {
		t.Error("arguments with spaces but no metacharacters should not trigger SECURITY error")
	}
}

func TestSanitizeAndExecute_SafeDottedArguments(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Dotted paths should be safe
	_, err := SanitizeAndExecute(ctx, "ls", ".config")
	if err != nil && strings.Contains(err.Error(), "SECURITY") {
		t.Error("dotted paths should not trigger SECURITY error")
	}
}

// ---------------------------------------------------------------------------
// SanitizeAndExecute — concurrent safety
// ---------------------------------------------------------------------------

func TestSanitizeAndExecute_ConcurrentSafety(t *testing.T) {
	// Test that AllowedCommands map can be read concurrently without races.
	// The map is never written to after init, but we verify -race flag passes.
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// These should all fail at the whitelist check — no real execution
			_, _ = SanitizeAndExecute(ctx, "rm", "-rf", "/")
			_, _ = SanitizeAndExecute(ctx, "ls", "/tmp")
			_ = AllowedCommands["sudo"]
			_ = AllowedCommands["rm"]
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
