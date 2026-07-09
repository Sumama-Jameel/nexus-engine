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

package dotfiles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateManagedPath(t *testing.T) {
	// Anchor all test paths under the user's actual $HOME so the
	// containment check can succeed on every test machine.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home directory: %v", err)
	}

	cases := []struct {
		name           string
		path           string
		allowSensitive bool
		wantErr        bool
		errContains    string
	}{
		// Valid cases
		{
			name:    "valid file under home",
			path:    filepath.Join(home, ".zshrc"),
			wantErr: false,
		},
		{
			name:    "valid nested file under home",
			path:    filepath.Join(home, ".config", "nvim", "init.vim"),
			wantErr: false,
		},

		// Invalid cases
		{
			name:        "empty path",
			path:        "",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "relative path",
			path:        "zshrc",
			wantErr:     true,
			errContains: "absolute",
		},
		{
			name:        "relative path with dot",
			path:        "./.zshrc",
			wantErr:     true,
			errContains: "absolute",
		},
		{
			name:        "traversal segment via Join (collapsed by Clean, caught by containment)",
			path:        filepath.Join(home, "..", "etc", "passwd"),
			wantErr:     true,
			errContains: "outside",
		},
		{
			name:        "raw traversal segment (caught by Clean normalization)",
			path:        home + "/../etc/passwd",
			wantErr:     true,
			errContains: "normalized",
		},
		{
			name:        "path with semicolon",
			path:        filepath.Join(home, ".zshrc; rm -rf /"),
			wantErr:     true,
			errContains: "metacharacter",
		},
		{
			name:        "path with backtick",
			path:        filepath.Join(home, ".zshrc`whoami`"),
			wantErr:     true,
			errContains: "metacharacter",
		},
		{
			name:        "path outside home",
			path:        "/etc/passwd",
			wantErr:     true,
			errContains: "outside",
		},
		{
			name:        "sensitive path without force",
			path:        filepath.Join(home, ".ssh", "id_rsa"),
			wantErr:     true,
			errContains: "sensitive",
		},
		{
			name:           "sensitive path WITH force",
			path:           filepath.Join(home, ".ssh", "id_rsa"),
			allowSensitive: true,
			wantErr:        false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateManagedPath(tc.path, tc.allowSensitive)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for path %q, got nil", tc.path)
					return
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error for path %q: %v", tc.path, err)
			}
		})
	}
}

func TestIsSensitivePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Sensitive
		{"/home/user/.ssh/id_rsa", true},
		{"/home/user/.ssh/id_ed25519", true},
		{"/home/user/.ssh/known_hosts", true},
		{"/home/user/.gnupg/private-keys-v1.d/foo.key", true},
		{"/home/user/.aws/credentials", true},
		{"/home/user/.config/gh/hosts.yml", true},
		{"/home/user/.netrc", true},
		{"/home/user/.docker/config.json", true},

		// Not sensitive
		{"/home/user/.zshrc", false},
		{"/home/user/.gitconfig", false},
		{"/home/user/.config/nvim/init.vim", false},
		{"/home/user/.ssh/config", false}, // config (not id_* or known_hosts)
		{"/home/user/.ssh/rc", false},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := isSensitivePath(tc.path); got != tc.want {
				t.Errorf("isSensitivePath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestHasShellMetacharacters(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		// Safe
		{"/home/user/.zshrc", false},
		{"/home/user/.config/nvim/init.vim", false},
		{"/home/user/file with spaces.txt", false},

		// Dangerous
		{"/home/user/.zshrc;rm", true},
		{"/home/user/.zshrc|grep foo", true},
		{"/home/user/.zshrc&echo hi", true},
		{"/home/user/$HOME/.zshrc", true},
		{"/home/user/`whoami`.zshrc", true},
		{"/home/user/.zshrc\nfoo", true},
		{"/home/user/.zshrc'foo'", true},
		{"/home/user/.zshrc\"foo\"", true},
		{"/home/user/.zshrc\\foo", true},
		{"/home/user/.zshrc!foo", true},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := hasShellMetacharacters(tc.in); got != tc.want {
				t.Errorf("hasShellMetacharacters(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAdd_NilExecFunc(t *testing.T) {
	_, err := Add(nil, "", AddDeps{ExecFn: nil}, false)
	if err == nil {
		t.Fatal("expected error when ExecFunc is nil")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestVerify_NilExecFunc(t *testing.T) {
	_, err := Verify(nil, AddDeps{ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestVerify_Success(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 1 && args[0] == "verify" {
			return "ok\n", nil
		}
		t.Errorf("unexpected ExecFn call: %s %v", cmd, args)
		return "", nil
	}

	report, err := Verify(context.Background(), AddDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.OK {
		t.Errorf("expected OK=true on chezmoi verify success")
	}
	if report.Message != "ok" {
		t.Errorf("expected message to be trimmed, got %q", report.Message)
	}
}

func TestVerify_DriftDetected(t *testing.T) {
	// chezmoi verify exits 1 when there are differences — that's drift,
	// not a hard failure. Verify must return a populated report with OK=false
	// and a nil error.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: exit status 1")
	}

	report, err := Verify(context.Background(), AddDeps{ExecFn: execFn})
	if err != nil {
		t.Fatalf("expected nil error for drift (exit 1), got: %v", err)
	}
	if report.OK {
		t.Errorf("expected OK=false when drift is detected")
	}
	if !strings.Contains(report.Message, "drift detected") {
		t.Errorf("expected 'drift detected' message, got %q", report.Message)
	}
}

func TestVerify_RealError(t *testing.T) {
	// Errors other than "exit status 1" (timeout, not found, etc.) are real.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("TIMEOUT: chezmoi verify exceeded 60s")
	}

	_, err := Verify(context.Background(), AddDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for non-exit-1 failure")
	}
	if !strings.Contains(err.Error(), "chezmoi verify failed") {
		t.Errorf("expected 'chezmoi verify failed' error, got: %v", err)
	}
}

func TestVerify_ReportsManagedCount(t *testing.T) {
	// Verify should report the count of managed files from state.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "ok", nil
	}
	tracker := withTempHome(t, "")
	// Manually populate managed files via the public RecordDotfilesAdd API.
	for _, p := range []string{"/tmp/a", "/tmp/b", "/tmp/c"} {
		_ = tracker.RecordDotfilesAdd(p)
	}

	report, err := Verify(context.Background(),
		AddDeps{ExecFn: execFn, State: tracker})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ManagedCount != 3 {
		t.Errorf("expected ManagedCount=3, got %d", report.ManagedCount)
	}
}
