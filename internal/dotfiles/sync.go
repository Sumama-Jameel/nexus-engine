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
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// SyncDeps holds the dependencies for push/pull/sync operations.
//
// Mirrors SourceDeps so the CLI wiring in main.go stays consistent. The
// Token field is optional: when set (and the source is HTTPS), it is
// injected into the remote URL for this exec call only — never persisted.
type SyncDeps struct {
	ExecFn     ExecFunc
	State      *engine.StateTracker
	Audit      *engine.AuditLogger
	Token      string // PAT for HTTPS auth; empty = use system default (SSH key or git creds)
	SkipSecretScan bool // when true, skip pre-push secret scan (used by --force)
}

// SyncReport is the structured result of a push, pull, or sync operation.
//
// Fields are populated best-effort. Callers should check Error before
// trusting the other fields.
type SyncReport struct {
	Operation   string    `json:"operation"`              // "push" | "pull" | "sync"
	DryRun      bool      `json:"dry_run,omitempty"`
	Source      string    `json:"source"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	FilesScanned int      `json:"files_scanned,omitempty"`
	SecretsFound []Match  `json:"secrets_found,omitempty"` // empty when none / scan skipped
	Pushed      bool      `json:"pushed,omitempty"`
	Pulled      bool      `json:"pulled,omitempty"`
	Applied     bool      `json:"applied,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}

// Push stages all local chezmoi changes, scans them for secrets, commits,
// and pushes to the remote.
//
// Safety order (matches the V8 plan):
//  1. Validate preconditions (source bound, URL passes SSRF guard)
//  2. Stage all changes (`chezmoi git add -A`)
//  3. Scan staged file CONTENTS for known secret patterns
//  4. If secrets found AND not --force: abort with report
//  5. Commit (`chezmoi git commit -m <msg>`) with default or user message
//  6. Push (`chezmoi git push <url>`) with optional PAT-injected URL
//  7. Record SHA + audit
//
// Dry-run short-circuits after step 1 and returns the planned operation.
func Push(ctx context.Context, deps SyncDeps, message string) (*SyncReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: SyncDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	report := &SyncReport{
		Operation: "push",
		StartedAt: time.Now().UTC(),
	}

	source, err := requireBoundSource(deps.State)
	if err != nil {
		return nil, err
	}
	report.Source = source

	// Re-validate URL on every operation (defense in depth: in case
	// AllowedSourceHosts is tightened between binds).
	if err := validateSourceForSync(source); err != nil {
		return nil, err
	}

	// Step 1: Stage everything.
	// The `--` separator tells chezmoi to pass `-A` to git instead of
	// parsing it as a chezmoi flag.
	if _, err := deps.ExecFn(ctx, "chezmoi", "git", "add", "--", "-A"); err != nil {
		return nil, fmt.Errorf("chezmoi git add failed: %w", err)
	}

	// Step 2: List staged files.
	staged, err := listStagedFiles(ctx, deps.ExecFn)
	if err != nil {
		return nil, err
	}

	// Step 3: Scan staged content.
	matches := scanStagedFiles(staged)
	report.FilesScanned = len(staged)
	report.SecretsFound = matches

	if len(matches) > 0 && !deps.SkipSecretScan {
		return report, fmt.Errorf(
			"refusing to push: %d potential secret(s) detected (run with --force to override): %s",
			len(matches), summarizeMatches(matches))
	}

	// Step 4: Commit only if there's something to commit.
	if len(staged) > 0 {
		if message == "" {
			message = fmt.Sprintf("nexus: sync at %s",
				time.Now().UTC().Format(time.RFC3339))
		}
		if _, err := deps.ExecFn(ctx, "chezmoi", "git", "commit", "-m", message); err != nil {
			// "nothing to commit" exits non-zero in some git versions; treat
			// that as a no-op rather than a failure.
			if !isNothingToCommit(err) {
				return report, fmt.Errorf("chezmoi git commit failed: %w", err)
			}
		}
	}

	// Step 5: Push (with optional PAT-injected URL).
	// The `--` separator isn't strictly required for push (the URL is
	// positional, not a flag), but we include it for consistency and to
	// future-proof against URLs that might start with `-`.
	pushArgs := []string{"git", "push"}
	if pushURL, ok := injectToken(source, deps.Token); ok {
		pushArgs = append(pushArgs, "--", pushURL)
	}
	if _, err := deps.ExecFn(ctx, "chezmoi", pushArgs...); err != nil {
		return report, fmt.Errorf("chezmoi git push failed: %w", err)
	}

	// Step 6: Capture HEAD SHA + record state.
	sha, _ := deps.ExecFn(ctx, "chezmoi", "git", "rev-parse", "HEAD")
	report.CommitSHA = strings.TrimSpace(sha)
	report.Pushed = true
	report.CompletedAt = time.Now().UTC()

	if deps.State != nil {
		_ = deps.State.RecordDotfilesPush(report.CommitSHA)
	}
	logAudit(deps.Audit, "DOTFILES_PUSH", "success", report.CommitSHA)

	return report, nil
}

// Pull fetches from the remote and applies the result to the live system.
//
// By default uses `--ff-only`: refuses to merge if the local working copy
// has diverged from the remote (no auto-merge — force the user to inspect).
// Pass rebase=true to use --rebase instead (useful for the "single user,
// multiple machines" case where replaying local commits is safe).
//
// After a successful fetch, Pull invokes Apply (V7) to materialize the
// new state on disk.
func Pull(ctx context.Context, deps SyncDeps, rebase bool) (*SyncReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: SyncDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	report := &SyncReport{
		Operation: "pull",
		StartedAt: time.Now().UTC(),
	}

	source, err := requireBoundSource(deps.State)
	if err != nil {
		return nil, err
	}
	report.Source = source

	if err := validateSourceForSync(source); err != nil {
		return nil, err
	}

	// Fetch + merge (or rebase) via chezmoi git.
	// Note: chezmoi's `git` subcommand uses pflag, which tries to parse
	// args starting with `-` as chezmoi flags. The `--` separator tells
	// pflag to stop parsing and pass everything after to git as-is.
	// Verified against chezmoi 2.50.0: without `--`, `chezmoi git pull --ff-only`
	// errors with "chezmoi: unknown flag: --ff-only".
	pullArgs := []string{"git", "pull", "--"}
	if rebase {
		pullArgs = append(pullArgs, "--rebase")
	} else {
		pullArgs = append(pullArgs, "--ff-only")
	}
	if pullURL, ok := injectToken(source, deps.Token); ok {
		pullArgs = append(pullArgs, pullURL)
	}

	if _, err := deps.ExecFn(ctx, "chezmoi", pullArgs...); err != nil {
		return report, fmt.Errorf("chezmoi git pull failed: %w", err)
	}

	// Apply the newly-pulled state (delegate to V7's Apply).
	applyDeps := ApplyDeps{
		ExecFn: deps.ExecFn,
		State:  deps.State,
		Audit:  deps.Audit,
	}
	if _, err := Apply(ctx, applyDeps); err != nil {
		return report, fmt.Errorf("apply after pull failed: %w", err)
	}

	sha, _ := deps.ExecFn(ctx, "chezmoi", "git", "rev-parse", "HEAD")
	report.CommitSHA = strings.TrimSpace(sha)
	report.Pulled = true
	report.Applied = true
	report.CompletedAt = time.Now().UTC()

	if deps.State != nil {
		_ = deps.State.RecordDotfilesPull(report.CommitSHA)
	}
	logAudit(deps.Audit, "DOTFILES_PULL", "success", report.CommitSHA)

	return report, nil
}

// Sync is pull + apply + push in sequence. Stops at the first failure so
// the user gets a clear "where did it stop" report.
//
// This is the convenience command for the "I just sat down at my desk,
// make my machine match the cloud" workflow. power users can compose
// Pull and Push themselves.
func Sync(ctx context.Context, deps SyncDeps, message string, rebase bool) (*SyncReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: SyncDeps.ExecFn must not be nil (Zero-Trust boundary)")
	}

	report := &SyncReport{
		Operation: "sync",
		StartedAt: time.Now().UTC(),
	}

	source, err := requireBoundSource(deps.State)
	if err != nil {
		return nil, err
	}
	report.Source = source

	if err := validateSourceForSync(source); err != nil {
		return nil, err
	}

	// Step 1: Pull first (so push has the latest remote HEAD).
	pullReport, err := Pull(ctx, deps, rebase)
	if err != nil {
		return report, fmt.Errorf("sync: pull step failed: %w", err)
	}
	report.Pulled = pullReport.Pulled
	report.Applied = pullReport.Applied
	report.CommitSHA = pullReport.CommitSHA

	// Step 2: Push (only if pull succeeded).
	pushReport, err := Push(ctx, deps, message)
	if err != nil {
		return report, fmt.Errorf("sync: push step failed: %w", err)
	}
	report.Pushed = pushReport.Pushed
	report.CommitSHA = pushReport.CommitSHA
	report.FilesScanned = pushReport.FilesScanned
	report.SecretsFound = pushReport.SecretsFound

	report.CompletedAt = time.Now().UTC()
	return report, nil
}

// ─── helpers ───────────────────────────────────────────────────────────────

// requireBoundSource returns the source URL or an error if no source is bound.
func requireBoundSource(st *engine.StateTracker) (string, error) {
	if st == nil {
		return "", fmt.Errorf("no dotfile source bound — run 'nexus dotfiles init <url>' first")
	}
	src := st.GetDotfilesState().Source
	if src == "" {
		return "", fmt.Errorf("no dotfile source bound — run 'nexus dotfiles init <url>' first")
	}
	return src, nil
}

// validateSourceForSync re-checks the source URL against the current
// AllowedSourceHosts whitelist. This is the V8 "re-validate on every
// operation" defense: even if the URL was valid at bind time, the
// whitelist might have been tightened since.
func validateSourceForSync(source string) error {
	if err := engine.ValidateURLAgainstHosts(source, AllowedSourceHosts); err != nil {
		return fmt.Errorf("source URL no longer permitted: %w", err)
	}
	return nil
}

// listStagedFiles runs `chezmoi git diff --cached --name-only` and parses
// the output (one path per line). Returns the absolute paths within the
// chezmoi source dir.
func listStagedFiles(ctx context.Context, execFn ExecFunc) ([]string, error) {
	out, err := execFn(ctx, "chezmoi", "git", "diff", "--cached", "--name-only")
	if err != nil {
		return nil, fmt.Errorf("chezmoi git diff --cached failed: %w", err)
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	sourceDir := chezmoiSourceDir()
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.Join(sourceDir, line))
	}
	return files, nil
}

// scanStagedFiles reads each file's content and runs the secret scanner.
// Files that no longer exist on disk (deletions) are skipped — they can't
// leak secrets. Errors reading individual files are swallowed with the
// file simply excluded from the scan count.
//
// V9 update: uses ScanFile (not Scan) so `.age` ciphertext files are
// skipped automatically. This is how the vault works with the push flow
// — users encrypt sensitive files with `vault add`, the encrypted blobs
// sit in the chezmoi source dir, and `nexus dotfiles push` skips them.
func scanStagedFiles(paths []string) []Match {
	if len(paths) == 0 {
		return nil
	}
	var all []Match
	for _, p := range paths {
		all = append(all, ScanFile(p)...)
	}
	return all
}

// summarizeMatches formats a short, safe-to-print summary of secret matches.
// The secret values are already redacted in each Match.Snippet — we only
// print the file:line:pattern tuple here.
func summarizeMatches(matches []Match) string {
	if len(matches) == 0 {
		return ""
	}
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, fmt.Sprintf("[line %d: %s]", m.Line, m.Pattern))
	}
	// Cap at 5 entries to keep error messages readable.
	if len(parts) > 5 {
		parts = append(parts[:5], fmt.Sprintf("... and %d more", len(matches)-5))
	}
	return strings.Join(parts, ", ")
}

// injectToken returns the URL with the PAT baked in (HTTPS only) and true,
// or the original URL and false when no token injection applies (SSH, or
// no token provided).
//
// The token is included in the URL only for the duration of the single
// exec call. It is never written to state, never logged, never persisted
// to the chezmoi source dir's .git/config.
func injectToken(source, token string) (string, bool) {
	if token == "" {
		return source, false
	}
	if !strings.HasPrefix(source, "https://") {
		// SSH or other scheme: token injection doesn't apply.
		return source, false
	}
	// Build https://x-access-token:<token>@<rest> — git's convention for HTTPS PAT auth.
	rest := strings.TrimPrefix(source, "https://")
	return "https://x-access-token:" + token + "@" + rest, true
}

// isNothingToCommit inspects an exec error to detect the benign case where
// `git commit` found nothing staged. Returns true when the error message
// contains the canonical "nothing to commit" string.
func isNothingToCommit(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "nothing to commit") ||
		strings.Contains(msg, "no changes added to commit")
}

// logAudit writes an audit entry when an AuditLogger is provided.
// Failures here are best-effort and don't propagate.
func logAudit(a *engine.AuditLogger, action, result, detail string) {
	if a == nil {
		return
	}
	_ = a.Log(engine.AuditEntry{
		Action:  action,
		Result:  result,
		Package: detail, // store commit SHA / URL in Package field for traceability
	})
}
