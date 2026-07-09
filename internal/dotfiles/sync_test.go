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

// pushScript is a programmable ExecFn for push-related tests. It returns
// canned responses based on the (cmd, args) tuple, or an error if one was
// pre-registered.
type pushScript struct {
	t            *testing.T
	responses    map[string]string // "cmd arg1 arg2" → stdout
	errors       map[string]error  // "cmd arg1 arg2" → error
	callLog      []string
	stagedFiles  []string // files to "discover" when git diff --cached is called
	filesOnDisk  map[string]string // fake chezmoi source dir contents
}

func (p *pushScript) run(ctx context.Context, cmd string, args ...string) (string, error) {
	p.t.Helper()
	full := append([]string{cmd}, args...)
	p.callLog = append(p.callLog, strings.Join(full, " "))

	key := strings.Join(full, " ")

	if err, ok := p.errors[key]; ok {
		return "", err
	}
	if resp, ok := p.responses[key]; ok {
		return resp, nil
	}

	// Pattern-based fallbacks for predictable chezmoi git calls.
	// Always length-check before indexing.
	if cmd == "chezmoi" && len(args) >= 2 && args[0] == "git" {
		switch args[1] {
		case "diff":
			// chezmoi git diff --cached --name-only → staged file list
			if len(args) >= 4 && args[2] == "--cached" && args[3] == "--name-only" {
				return strings.Join(p.stagedFiles, "\n"), nil
			}
		case "rev-parse":
			// chezmoi git rev-parse HEAD → commit SHA
			return "abc123def4567890", nil
		}
	}

	// Default: empty success.
	return "", nil
}

// withFakeChezmoiSource sets up HOME so chezmoiSourceDir() returns our
// fake source directory, populated with the given file map (relative
// path → content). We intentionally do NOT set XDG_DATA_HOME — that
// would change the chezmoi source dir resolution to <XDG_DATA_HOME>/chezmoi,
// which doesn't match where we write files below.
func withFakeChezmoiSource(t *testing.T, files map[string]string) string {
	t.Helper()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	// Unset XDG_DATA_HOME so chezmoiSourceDir() falls through to the
	// HOME-based default (<HOME>/.local/share/chezmoi) and matches where
	// we create files below.
	t.Setenv("XDG_DATA_HOME", "")

	sourceDir := filepath.Join(tmpHome, ".local", "share", "chezmoi")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	for rel, content := range files {
		full := filepath.Join(sourceDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
	return sourceDir
}

// ─── Push: input validation ────────────────────────────────────────────────

func TestPush_NilExecFn(t *testing.T) {
	_, err := Push(context.Background(), SyncDeps{ExecFn: nil}, "")
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected 'must not be nil' error, got: %v", err)
	}
}

func TestPush_NoSourceBound(t *testing.T) {
	tracker := withTempHome(t, "")
	script := &pushScript{t: t, responses: map[string]string{}, errors: map[string]error{}}
	_, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "")
	if err == nil {
		t.Fatal("expected error when no source is bound")
	}
	if !strings.Contains(err.Error(), "no dotfile source bound") {
		t.Errorf("expected 'no dotfile source bound' error, got: %v", err)
	}
}

// ─── Push: secret scanning ─────────────────────────────────────────────────

func TestPush_RefusesWhenSecretsDetected(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{
		"dot_zshrc": "export AWS_KEY=AKIAIOSFODNN7EXAMPLE\n",
	})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}

	report, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "test commit")
	if err == nil {
		t.Fatal("expected error when secrets detected")
	}
	if !strings.Contains(err.Error(), "refusing to push") {
		t.Errorf("expected 'refusing to push' error, got: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report even on rejection")
	}
	if len(report.SecretsFound) != 1 {
		t.Errorf("expected 1 secret found, got %d", len(report.SecretsFound))
	}
	if report.Pushed {
		t.Error("Pushed must be false when scan refuses")
	}
}

func TestPush_ForceOverridesSecretScan(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{
		"dot_zshrc": "export AWS_KEY=AKIAIOSFODNN7EXAMPLE\n",
	})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}

	report, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker, SkipSecretScan: true}, "force push")
	if err != nil {
		t.Fatalf("expected success with --force, got: %v", err)
	}
	if !report.Pushed {
		t.Error("expected Pushed=true with --force")
	}
	if len(report.SecretsFound) != 1 {
		t.Errorf("secrets should still be REPORTED in the report even with --force, got %d",
			len(report.SecretsFound))
	}
}

func TestPush_NoSecretsCleanPush(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{
		"dot_zshrc": "export EDITOR=vim\n",
	})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}

	report, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "clean commit")
	if err != nil {
		t.Fatalf("expected clean push, got: %v", err)
	}
	if !report.Pushed {
		t.Error("expected Pushed=true")
	}
	if len(report.SecretsFound) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(report.SecretsFound))
	}
	if report.CommitSHA == "" {
		t.Error("expected CommitSHA to be populated")
	}
}

// ─── Push: error paths ─────────────────────────────────────────────────────

func TestPush_AddFailure(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	script := &pushScript{
		t:         t,
		responses: map[string]string{},
		errors: map[string]error{
			"chezmoi git add -- -A": errors.New("permission denied"),
		},
	}

	_, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "")
	if err == nil {
		t.Fatal("expected error when add fails")
	}
	if !strings.Contains(err.Error(), "chezmoi git add failed") {
		t.Errorf("expected 'chezmoi git add failed' error, got: %v", err)
	}
}

func TestPush_CommitFailure(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{"dot_zshrc": "x"})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}
	script.errors["chezmoi git commit -m test"] = errors.New("author identity unknown")

	_, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "test")
	if err == nil {
		t.Fatal("expected error when commit fails")
	}
	if !strings.Contains(err.Error(), "commit failed") {
		t.Errorf("expected 'commit failed' error, got: %v", err)
	}
}

func TestPush_NothingToCommitIsNoop(t *testing.T) {
	// git commit with nothing staged returns exit 1 + "nothing to commit".
	// We treat this as a no-op (proceed to push) rather than a fatal error.
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{}) // empty source dir
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: nil, // no staged files
	}
	script.errors["chezmoi git commit -m sync"] =
		errors.New("nothing to commit, working tree clean")

	report, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "")
	if err != nil {
		t.Fatalf("expected no error on 'nothing to commit', got: %v", err)
	}
	if !report.Pushed {
		t.Error("expected push to proceed when there's nothing to commit")
	}
}

func TestPush_RemoteAheadFails(t *testing.T) {
	// Simulate "non-fast-forward" error from git push.
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{"dot_zshrc": "x"})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}
	script.errors["chezmoi git push"] = errors.New("non-fast-forward")

	_, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "x")
	if err == nil {
		t.Fatal("expected error on non-fast-forward")
	}
	if !strings.Contains(err.Error(), "push failed") {
		t.Errorf("expected 'push failed' error, got: %v", err)
	}
}

// ─── Push: token injection ─────────────────────────────────────────────────

func TestInjectToken(t *testing.T) {
	cases := []struct {
		name    string
		source  string
		token   string
		wantURL string
		wantOK  bool
	}{
		{
			name:    "HTTPS with token",
			source:  "https://github.com/foo/bar.git",
			token:   "ghp_secret",
			wantURL: "https://x-access-token:ghp_secret@github.com/foo/bar.git",
			wantOK:  true,
		},
		{
			name:    "HTTPS without token",
			source:  "https://github.com/foo/bar.git",
			token:   "",
			wantURL: "https://github.com/foo/bar.git",
			wantOK:  false,
		},
		{
			name:    "SSH with token (no injection)",
			source:  "ssh://git@github.com/foo/bar.git",
			token:   "ghp_secret",
			wantURL: "ssh://git@github.com/foo/bar.git",
			wantOK:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotURL, gotOK := injectToken(tc.source, tc.token)
			if gotURL != tc.wantURL {
				t.Errorf("URL = %q, want %q", gotURL, tc.wantURL)
			}
			if gotOK != tc.wantOK {
				t.Errorf("OK = %v, want %v", gotOK, tc.wantOK)
			}
		})
	}
}

func TestPush_WithTokenInjectsInURL(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{"dot_zshrc": "x"})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}

	_, err := Push(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker, Token: "ghp_supersecret"}, "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the token-bearing URL was passed to chezmoi git push.
	// (chezmoi uses `--` separator before positional args; we just check
	// the URL appears anywhere in the call log.)
	found := false
	for _, call := range script.callLog {
		if strings.Contains(call, "x-access-token:ghp_supersecret@github.com/foo/bar") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("token-bearing URL not used in push call. Calls:\n%s",
			strings.Join(script.callLog, "\n"))
	}
}

// ─── Pull ──────────────────────────────────────────────────────────────────

func TestPull_NilExecFn(t *testing.T) {
	_, err := Pull(context.Background(), SyncDeps{ExecFn: nil}, false)
	if err == nil {
		t.Fatal("expected error when ExecFn is nil")
	}
}

func TestPull_NoSourceBound(t *testing.T) {
	tracker := withTempHome(t, "")
	script := &pushScript{t: t, responses: map[string]string{}, errors: map[string]error{}}
	_, err := Pull(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, false)
	if err == nil {
		t.Fatal("expected error when no source bound")
	}
}

func TestPull_HappyPath(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	script := &pushScript{
		t:         t,
		responses: map[string]string{},
		errors:    map[string]error{},
	}

	report, err := Pull(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Pulled {
		t.Error("expected Pulled=true")
	}
	if !report.Applied {
		t.Error("expected Applied=true (pull triggers apply)")
	}
	if report.CommitSHA == "" {
		t.Error("expected CommitSHA to be populated")
	}

	// Verify the ff-only flag was used (the default).
	found := false
	for _, call := range script.callLog {
		if strings.Contains(call, "chezmoi git pull -- --ff-only") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'chezmoi git pull -- --ff-only' call. Calls:\n%s",
			strings.Join(script.callLog, "\n"))
	}
}

func TestPull_RebaseFlag(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	script := &pushScript{
		t:         t,
		responses: map[string]string{},
		errors:    map[string]error{},
	}

	_, err := Pull(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, call := range script.callLog {
		if strings.Contains(call, "chezmoi git pull -- --rebase") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'chezmoi git pull -- --rebase' call. Calls:\n%s",
			strings.Join(script.callLog, "\n"))
	}
}

func TestPull_RejectedWhenDiverged(t *testing.T) {
	// ff-only refuses non-fast-forward; that's a real pull failure.
	tracker := withTempHome(t, "https://github.com/foo/bar")
	script := &pushScript{
		t:         t,
		responses: map[string]string{},
		errors: map[string]error{
			"chezmoi git pull -- --ff-only": errors.New("non-fast-forward"),
		},
	}

	_, err := Pull(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, false)
	if err == nil {
		t.Fatal("expected error on divergence")
	}
	if !strings.Contains(err.Error(), "pull failed") {
		t.Errorf("expected 'pull failed' error, got: %v", err)
	}
}

// ─── Sync ──────────────────────────────────────────────────────────────────

func TestSync_PullThenPush(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{"dot_zshrc": "x"})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}

	report, err := Sync(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "sync", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !report.Pulled {
		t.Error("expected Pulled=true")
	}
	if !report.Pushed {
		t.Error("expected Pushed=true")
	}
	if !report.Applied {
		t.Error("expected Applied=true")
	}
}

func TestSync_StopsAtPullFailure(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	script := &pushScript{
		t:         t,
		responses: map[string]string{},
		errors: map[string]error{
			"chezmoi git pull -- --ff-only": errors.New("network unreachable"),
		},
	}

	_, err := Sync(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "sync", false)
	if err == nil {
		t.Fatal("expected error when pull fails")
	}
	if !strings.Contains(err.Error(), "pull step failed") {
		t.Errorf("expected 'pull step failed' error, got: %v", err)
	}
}

func TestSync_StopsAtPushFailure(t *testing.T) {
	tracker := withTempHome(t, "https://github.com/foo/bar")
	withFakeChezmoiSource(t, map[string]string{"dot_zshrc": "x"})
	script := &pushScript{
		t:           t,
		responses:   map[string]string{},
		errors:      map[string]error{},
		stagedFiles: []string{"dot_zshrc"},
	}
	script.errors["chezmoi git push"] = errors.New("remote rejected")

	_, err := Sync(context.Background(),
		SyncDeps{ExecFn: script.run, State: tracker}, "sync", false)
	if err == nil {
		t.Fatal("expected error when push fails after successful pull")
	}
	if !strings.Contains(err.Error(), "push step failed") {
		t.Errorf("expected 'push step failed' error, got: %v", err)
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

func TestSummarizeMatches(t *testing.T) {
	matches := []Match{
		{Pattern: "AWS Access Key ID", Line: 5},
		{Pattern: "GitHub PAT", Line: 12},
	}
	got := summarizeMatches(matches)
	if !strings.Contains(got, "AWS Access Key ID") {
		t.Errorf("expected AWS pattern in summary, got %q", got)
	}
	if !strings.Contains(got, "line 5") {
		t.Errorf("expected line number in summary, got %q", got)
	}
}

func TestIsNothingToCommit(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{errors.New("nothing to commit, working tree clean"), true},
		{errors.New("no changes added to commit"), true},
		{errors.New("author identity unknown"), false},
		{nil, false},
	}
	for _, tc := range cases {
		name := "nil"
		if tc.err != nil {
			name = tc.err.Error()
		}
		t.Run(name, func(t *testing.T) {
			if got := isNothingToCommit(tc.err); got != tc.want {
				t.Errorf("isNothingToCommit = %v, want %v", got, tc.want)
			}
		})
	}
}
