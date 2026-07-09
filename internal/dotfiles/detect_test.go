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
	"strings"
	"testing"
)

func TestDetect_Installed(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd != "chezmoi" {
			t.Errorf("expected command 'chezmoi', got %q", cmd)
		}
		if len(args) != 1 || args[0] != "--version" {
			t.Errorf("expected args ['--version'], got %v", args)
		}
		return "chezmoi version 2.50.0\ncommit: abc123\n", nil
	}

	report, err := Detect(context.Background(), execFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Installed {
		t.Errorf("expected Installed=true, got false")
	}
	if report.Version != "2.50.0" {
		t.Errorf("expected Version='2.50.0', got %q", report.Version)
	}
	if report.ConfigDir == "" {
		t.Errorf("expected ConfigDir to be populated when installed")
	}
	if report.SourceDir == "" {
		t.Errorf("expected SourceDir to be populated when installed")
	}
	if report.ProbedAt.IsZero() {
		t.Errorf("expected ProbedAt to be set")
	}
}

func TestDetect_NotInstalled(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New(`EXEC: command 'chezmoi' failed: exec: "chezmoi": executable file not found in $PATH`)
	}

	report, err := Detect(context.Background(), execFn)
	if err != nil {
		t.Fatalf("expected nil error when chezmoi is not installed, got: %v", err)
	}
	if report.Installed {
		t.Errorf("expected Installed=false")
	}
	if report.Version != "" {
		t.Errorf("expected empty Version, got %q", report.Version)
	}
	if report.ConfigDir != "" || report.SourceDir != "" {
		t.Errorf("expected empty paths when not installed, got ConfigDir=%q SourceDir=%q",
			report.ConfigDir, report.SourceDir)
	}
}

func TestDetect_RealError(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("TIMEOUT: command 'chezmoi' exceeded 60 second limit")
	}

	_, err := Detect(context.Background(), execFn)
	if err == nil {
		t.Fatal("expected error on real execution failure, got nil")
	}
	if !strings.Contains(err.Error(), "probe failed") {
		t.Errorf("expected wrapped 'probe failed' error, got: %v", err)
	}
}

func TestDetect_NilExecFunc(t *testing.T) {
	_, err := Detect(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when execFn is nil")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestParseChezmoiVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "chezmoi version 2.50.0\n", "2.50.0"},
		{"no newline", "chezmoi version 2.50.0", "2.50.0"},
		{"with commit", "chezmoi version 2.50.0\ncommit: abc123\nbuilt: 2024-01-01\n", "2.50.0"},
		{"dev build", "chezmoi version 2.50.0-1-gabc123-dirty\n", "2.50.0"},
		{"older version", "chezmoi version 1.8.0\n", "1.8.0"},
		// Real chezmoi v2.50.0 output: "chezmoi version v2.50.0, commit ..."
		{"v prefix and comma", "chezmoi version v2.50.0, commit 3ad974381fe57aedbcffef4371aa80970a989aaf, built at 2024-07-02T21:16:33Z, built by goreleaser", "2.50.0"},
		{"v prefix only", "chezmoi version v2.50.0", "2.50.0"},
		{"trailing punctuation", "v1.2.3;", "1.2.3"},
		{"empty", "", ""},
		{"missing version", "chezmoi version\n", ""},
		{"garbage", "not a version string", ""},
		{"single number", "1", ""},
		{"two part", "1.0", ""},
		{"non-numeric", "1.0.x", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseChezmoiVersion(tc.in)
			if got != tc.want {
				t.Errorf("parseChezmoiVersion(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsSemver(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1.0.0", true},
		{"2.50.0", true},
		{"100.200.300", true},
		{"1.0", false},
		{"1.0.0.0", false},
		{"1.0.x", false},
		{"v1.0.0", false}, // 'v' prefix not allowed (stripped earlier)
		{"", false},
		{"1..0", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := isSemver(tc.in); got != tc.want {
				t.Errorf("isSemver(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsNotInstalled(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"executable not found", errors.New(`exec: "chezmoi": executable file not found in $PATH`), true},
		{"not found in", errors.New("command not found in PATH"), true},
		{"exit status 1", errors.New("exit status 1"), true},
		{"exit status 127", errors.New("exit status 127"), true},
		{"timeout", errors.New("TIMEOUT: command 'chezmoi' exceeded 60 second limit"), false},
		{"security rejection", errors.New("SECURITY: command 'foo' is not in the allowed list"), false},
		{"unknown error", errors.New("some other random error"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNotInstalled(tc.err); got != tc.want {
				t.Errorf("isNotInstalled(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
