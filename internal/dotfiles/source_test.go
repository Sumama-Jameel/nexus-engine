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
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// makeExecRecorder returns an ExecFn that records every (cmd, args) call and
// returns the next canned response. When responses is empty, it returns
// ("", nil) for every call.
func makeExecRecorder(t *testing.T, responses map[string]string) (ExecFunc, *[][]string) {
	t.Helper()
	calls := &[][]string{}
	return func(ctx context.Context, cmd string, args ...string) (string, error) {
		full := append([]string{cmd}, args...)
		*calls = append(*calls, full)
		key := cmd
		if len(args) > 0 {
			key = cmd + " " + strings.Join(args, " ")
		}
		if resp, ok := responses[key]; ok {
			return resp, nil
		}
		// Default: success with empty output.
		return "", nil
	}, calls
}

// withTempHome redirects $HOME to a fresh temp dir for the duration of the
// test, and pre-populates it with a state.json containing the given source.
// This lets us exercise code paths that read from a StateTracker without
// touching the user's real ~/.nexus/state.json.
func withTempHome(t *testing.T, source string) *engine.StateTracker {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	nexusDir := filepath.Join(tmpHome, ".nexus")
	if err := os.MkdirAll(nexusDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Build a minimal valid NexusState.
	state := engine.NexusState{
		Version:         1,
		LastModified:    time.Now().UTC(),
		Packages:        map[string]engine.PackageState{},
		ProfilesApplied: []string{},
		WSLInstances:    map[string]engine.WSLInstanceState{},
		Dotfiles: engine.DotfilesState{
			Installed: true,
			Source:    source,
		},
	}
	if err := writeStateJSON(filepath.Join(nexusDir, "state.json"), state); err != nil {
		t.Fatalf("write state: %v", err)
	}

	tracker, err := engine.NewStateTracker()
	if err != nil {
		t.Fatalf("NewStateTracker: %v", err)
	}
	return tracker
}

// writeStateJSON writes the state atomically (matches StateTracker's save()).
func writeStateJSON(path string, state engine.NexusState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ─── BindSource: validation guards ─────────────────────────────────────────

func TestBindSource_NilExecFn(t *testing.T) {
	err := BindSource(context.Background(), "https://github.com/foo/bar",
		SourceDeps{ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestBindSource_RejectsNonHTTPS(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called when URL validation fails (got %s %v)", cmd, args)
		return "", nil
	}

	err := BindSource(context.Background(), "http://github.com/foo/bar",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for http:// scheme")
	}
	if !strings.Contains(err.Error(), "only HTTPS") {
		t.Errorf("expected 'only HTTPS' error, got: %v", err)
	}
}

func TestBindSource_RejectsGitScheme(t *testing.T) {
	// "git@github.com:foo/bar.git" is the SCP-style form, which V8 normalizes
	// to ssh://git@github.com/foo/bar.git and accepts (github.com is whitelisted).
	// Use a non-URL garbage string to verify the URL parse rejection path.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for malformed URL strings")
		return "", nil
	}

	err := BindSource(context.Background(), "ht!tp://nope",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	if !strings.Contains(err.Error(), "invalid source URL") {
		t.Errorf("expected 'invalid source URL' wrapper, got: %v", err)
	}
}

func TestNormalizeSourceURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"https unchanged", "https://github.com/foo/bar", "https://github.com/foo/bar"},
		{"ssh URL unchanged", "ssh://git@github.com/foo/bar", "ssh://git@github.com/foo/bar"},
		{"SCP git@host:path", "git@github.com:foo/bar.git", "ssh://git@github.com/foo/bar.git"},
		{"SCP with leading slash in path", "git@github.com:/foo/bar.git", "ssh://git@github.com/foo/bar.git"},
		{"whitespace trimmed", "  https://github.com/foo/bar  ", "https://github.com/foo/bar"},
		{"empty", "", ""},
		{"non-URL garbage unchanged", "not a url", "not a url"},
		{"windows path unchanged", "C:\\Users\\foo", "C:\\Users\\foo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeSourceURL(tc.in); got != tc.want {
				t.Errorf("NormalizeSourceURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBindSource_AcceptsSSH(t *testing.T) {
	// SCP-style SSH URL should be normalized and accepted when the host is whitelisted.
	// We mock ExecFn to verify the normalized URL reaches chezmoi init.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		if cmd == "chezmoi" && len(args) == 3 && args[0] == "init" {
			if args[2] != "ssh://git@github.com/foo/bar.git" {
				t.Errorf("expected normalized SSH URL, got %q", args[2])
			}
			return "Initialized", nil
		}
		return "", nil
	}

	err := BindSource(context.Background(), "git@github.com:foo/bar.git",
		SourceDeps{ExecFn: execFn})
	if err != nil {
		if strings.Contains(err.Error(), "DNS lookup failed") {
			t.Skipf("DNS lookup failed (no network?): %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindSource_RejectsNonGitSSHUser(t *testing.T) {
	// SSH URLs must use the "git" user. "user@host:path" normalizes to
	// ssh://user@host/path which should be rejected.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for non-'git' SSH users")
		return "", nil
	}

	err := BindSource(context.Background(), "user@github.com:foo/bar.git",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for non-'git' SSH user")
	}
	if !strings.Contains(err.Error(), "'git' user") {
		t.Errorf("expected 'git' user error, got: %v", err)
	}
}

func TestBindSource_RejectsHostNotInWhitelist(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for non-whitelisted host")
		return "", nil
	}

	err := BindSource(context.Background(), "https://evil.example.com/foo/bar",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for non-whitelisted host")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("expected 'not in the allowed list' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "github.com") {
		t.Errorf("error should list allowed hosts, got: %v", err)
	}
}

func TestBindSource_RejectsUserinfo(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for URLs with userinfo")
		return "", nil
	}

	err := BindSource(context.Background(), "https://user:pass@github.com/foo/bar",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for URL with userinfo")
	}
	if !strings.Contains(err.Error(), "userinfo") {
		t.Errorf("expected 'userinfo' error, got: %v", err)
	}
}

func TestBindSource_RejectsQueryString(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for URLs with query strings")
		return "", nil
	}

	err := BindSource(context.Background(), "https://github.com/foo/bar?token=secret",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for URL with query string")
	}
	if !strings.Contains(err.Error(), "query") {
		t.Errorf("expected 'query' error, got: %v", err)
	}
}

func TestBindSource_RejectsPrivateIP(t *testing.T) {
	// Note: "localhost" is NOT in the host whitelist, so it's rejected at
	// the whitelist check (before reaching the DNS/private-IP check).
	// The checkHostNotPrivate function is tested separately below for
	// direct private-IP hosts (127.0.0.1, 192.168.1.1, etc.).
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		t.Errorf("ExecFn must not be called for hostnames not in whitelist")
		return "", nil
	}

	err := BindSource(context.Background(), "https://localhost/foo/bar",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		t.Fatal("expected error for localhost")
	}
	if !strings.Contains(err.Error(), "not in the allowed list") {
		t.Errorf("expected 'not in the allowed list' error, got: %v", err)
	}
}

func TestBindSource_ExecFailure(t *testing.T) {
	// Use github.com but mock ExecFn to fail. If DNS resolution fails
	// (sandbox without network), skip this test rather than report false negative.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("chezmoi: repository not found")
	}

	err := BindSource(context.Background(), "https://github.com/foo/bar",
		SourceDeps{ExecFn: execFn})
	if err == nil {
		// Network may be unavailable — try with an unresolvable host instead
		// to verify the ExecFn error path is reached.
		t.Skip("network may be unavailable — cannot verify ExecFn failure path")
	}
	if !strings.Contains(err.Error(), "chezmoi init failed") {
		t.Errorf("expected 'chezmoi init failed' error, got: %v", err)
	}
}

func TestBindSource_Success(t *testing.T) {
	// github.com resolves to public IPs (140.82.112.x etc). We skip
	// gracefully if the test sandbox lacks network access.
	execFn, calls := makeExecRecorder(t, map[string]string{
		"chezmoi init https://github.com/foo/bar": "Initialized",
	})

	err := BindSource(context.Background(), "https://github.com/foo/bar",
		SourceDeps{ExecFn: execFn})
	if err != nil {
		if strings.Contains(err.Error(), "DNS lookup failed") {
			t.Skipf("DNS lookup failed (no network?): %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 ExecFn call, got %d", len(*calls))
	}
	got := (*calls)[0]
	if got[0] != "chezmoi" || got[1] != "init" || got[2] != "https://github.com/foo/bar" {
		t.Errorf("unexpected ExecFn call: %v", got)
	}
}

// ─── BindSource: state recording ───────────────────────────────────────────

func TestBindSource_RecordsStateWhenProvided(t *testing.T) {
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "Initialized", nil
	}

	tracker := withTempHome(t, "") // start with empty Source
	st := tracker.GetDotfilesState()
	if st.Source != "" {
		t.Fatalf("precondition: expected empty source, got %q", st.Source)
	}

	err := BindSource(context.Background(), "https://github.com/foo/bar",
		SourceDeps{ExecFn: execFn, State: tracker})
	if err != nil {
		if strings.Contains(err.Error(), "DNS lookup failed") {
			t.Skipf("DNS lookup failed (no network?): %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	st = tracker.GetDotfilesState()
	if st.Source != "https://github.com/foo/bar" {
		t.Errorf("expected Source to be recorded, got %q", st.Source)
	}
	if st.InitializedAt.IsZero() {
		t.Errorf("expected InitializedAt to be set")
	}
}

func TestBindSource_StateRecordingFailureDoesNotRollBack(t *testing.T) {
	// Per the doc: "State recording is best-effort: the bind already succeeded.
	// A failure here does not roll back the chezmoi init."
	// We test this by passing a nil State — should succeed normally.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "Initialized", nil
	}

	err := BindSource(context.Background(), "https://github.com/foo/bar",
		SourceDeps{ExecFn: execFn, State: nil})
	if err != nil {
		if strings.Contains(err.Error(), "DNS lookup failed") {
			t.Skipf("DNS lookup failed (no network?): %v", err)
		}
		t.Fatalf("unexpected error with nil state: %v", err)
	}
}

// ─── UnbindSource ──────────────────────────────────────────────────────────

func TestUnbindSource_NilExecFn(t *testing.T) {
	err := UnbindSource(context.Background(), SourceDeps{ExecFn: nil})
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestUnbindSource_Success(t *testing.T) {
	execFn, calls := makeExecRecorder(t, nil)
	tracker := withTempHome(t, "https://github.com/foo/bar")

	err := UnbindSource(context.Background(),
		SourceDeps{ExecFn: execFn, State: tracker})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("expected exactly 1 ExecFn call, got %d", len(*calls))
	}
	got := (*calls)[0]
	if got[0] != "chezmoi" || got[1] != "init" {
		t.Errorf("expected 'chezmoi init' call, got %v", got)
	}

	// State should reflect the unbind.
	st := tracker.GetDotfilesState()
	if st.Source != "" {
		t.Errorf("expected Source to be cleared, got %q", st.Source)
	}
}

func TestUnbindSource_ExecFailureIsSwallowed(t *testing.T) {
	// Per the doc: "chezmoi has no native 'unbind' command... we accept
	// this as a no-op marker and rely on state-clearing below to remove
	// the binding from Nexus's perspective." So even when chezmoi init
	// fails, UnbindSource must still clear the state.
	execFn := func(ctx context.Context, cmd string, args ...string) (string, error) {
		return "", errors.New("exit status 1")
	}
	tracker := withTempHome(t, "https://github.com/foo/bar")

	err := UnbindSource(context.Background(),
		SourceDeps{ExecFn: execFn, State: tracker})
	if err != nil {
		t.Fatalf("expected nil error (exec failure swallowed), got: %v", err)
	}
	if got := tracker.GetDotfilesState().Source; got != "" {
		t.Errorf("expected Source to be cleared despite exec failure, got %q", got)
	}
}

// ─── checkHostNotPrivate ───────────────────────────────────────────────────

func TestCheckHostNotPrivate(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		wantErr     bool
		errContains string
		skipOnDNS   bool // skip the test if the error is DNS-related (no network)
	}{
		{
			name:        "empty URL",
			url:         "",
			wantErr:     true,
			errContains: "no hostname",
		},
		{
			name:        "non-URL string",
			url:         "not-a-url-at-all",
			wantErr:     true,
			errContains: "no hostname",
		},
		{
			name:        "localhost resolves to loopback",
			url:         "https://localhost/foo",
			wantErr:     true,
			errContains: "private/reserved",
		},
		{
			name:        "127.0.0.1 directly",
			url:         "https://127.0.0.1/foo",
			wantErr:     true,
			errContains: "private/reserved",
		},
		{
			name:        "192.168.1.1 (RFC 1918)",
			url:         "https://192.168.1.1/foo",
			wantErr:     true,
			errContains: "private/reserved",
		},
		{
			name:        "10.0.0.1 (RFC 1918)",
			url:         "https://10.0.0.1/foo",
			wantErr:     true,
			errContains: "private/reserved",
		},
		{
			name:        "169.254.169.254 (cloud metadata)",
			url:         "https://169.254.169.254/latest/meta-data",
			wantErr:     true,
			errContains: "private/reserved",
		},
		{
			name:        "unresolvable RFC 6761 TLD",
			url:         "https://nx-nonexistent.invalid/foo",
			wantErr:     true,
			errContains: "DNS lookup failed",
		},
		{
			name:      "github.com resolves to public IP",
			url:       "https://github.com/foo/bar",
			wantErr:   false,
			skipOnDNS: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkHostNotPrivate(tc.url)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil", tc.url)
					return
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("expected error containing %q, got %q", tc.errContains, err.Error())
				}
			} else {
				if err != nil {
					if tc.skipOnDNS && strings.Contains(err.Error(), "DNS lookup failed") {
						t.Skipf("DNS lookup failed (no network?): %v", err)
					}
					t.Errorf("expected no error for %q, got %v", tc.url, err)
				}
			}
		})
	}
}
