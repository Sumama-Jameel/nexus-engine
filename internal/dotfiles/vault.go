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

// Package dotfiles — V9 Secrets Vault primitives.
//
// The vault encrypts sensitive files (SSH keys, GPG keys, cloud
// credentials) with age encryption before they enter the chezmoi
// source dir. Decryption is handled by chezmoi natively — we just
// ensure the right key file is in the right place when `chezmoi apply`
// runs.

package dotfiles

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// ─── Vault constants ───────────────────────────────────────────────────────

// VaultKeyringID is the OS keyring entry name where the private key is
// optionally mirrored. Same ID is used across Linux (Secret Service),
// macOS (Keychain), and Windows (Credential Manager) — go-keyring
// abstracts the backend.
const VaultKeyringID = "nexus-dotfiles-vault"

// MaxVaultFileSize caps the size of files accepted by VaultAdd. Age is
// designed for small secrets (SSH keys are ~5 KB, GPG keys ~50 KB,
// .netrc <1 KB). Larger files should go through a different mechanism.
// 10 MB is generous enough to cover any realistic secret file.
const MaxVaultFileSize = 10 * 1024 * 1024

// PrivateKeyName is the filename of the age private key inside the vault
// directory. Conventional name matching age-keygen's default.
const PrivateKeyName = "private.key"

// PublicKeyName is the filename of the age public key inside the vault
// directory. Conventional name matching the .pub convention.
const PublicKeyName = "public.key"

// ─── Vault dep types ───────────────────────────────────────────────────────

// PromptFunc is the dependency-injected prompt callback. Tests provide
// a fake that returns canned answers; the CLI provides one that reads
// from stdin. nil means "no interactive prompts available" — callers
// that need prompts must check before constructing this nil case.
type PromptFunc func(question string) (bool, error)

// VaultInitDeps holds dependencies for VaultInit.
type VaultInitDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
	// Prompt is called when overwriting an existing vault. nil means
	// "never overwrite" — caller must pass --force to skip the prompt.
	Prompt PromptFunc
	// Force skips the overwrite prompt and unconditionally regenerates.
	Force bool
}

// VaultAddDeps holds dependencies for VaultAdd.
type VaultAddDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
	// DryRun previews the encryption without writing the output file.
	DryRun bool
}

// VaultRemoveDeps holds dependencies for VaultRemove.
type VaultRemoveDeps struct {
	ExecFn ExecFunc
	State  *engine.StateTracker
	Audit  *engine.AuditLogger
	Prompt PromptFunc
	Force  bool
}

// VaultUnlockDeps holds dependencies for VaultUnlock.
type VaultUnlockDeps struct {
	ExecFn  ExecFunc
	State   *engine.StateTracker
	Audit   *engine.AuditLogger
	KeyFile string // --key-file flag; empty means "try keyring first"
	Force   bool
}

// ─── Vault report types ────────────────────────────────────────────────────

// VaultReport is the structured result of any vault operation. Fields
// are populated best-effort depending on the operation.
type VaultReport struct {
	Operation     string             `json:"operation"`
	PublicKey     string             `json:"public_key,omitempty"`
	PublicKeyShort string            `json:"public_key_short,omitempty"`
	KeyPath       string             `json:"key_path,omitempty"`
	KeyringID     string             `json:"keyring_id,omitempty"`
	EncryptedPath string             `json:"encrypted_path,omitempty"`
	OriginalPath  string             `json:"original_path,omitempty"`
	Files         []VaultFileEntry   `json:"files,omitempty"`
	Status        *VaultStatusReport `json:"status,omitempty"`
	StartedAt     time.Time          `json:"started_at"`
	CompletedAt   time.Time          `json:"completed_at"`
}

// VaultFileEntry maps an original filesystem path to its encrypted
// counterpart inside the chezmoi source dir.
type VaultFileEntry struct {
	Original  string `json:"original"`
	Encrypted string `json:"encrypted"`
	Size      int64  `json:"size"`
}

// VaultStatusReport is the structured status snapshot.
type VaultStatusReport struct {
	Initialized   bool   `json:"initialized"`
	PublicKeyShort string `json:"public_key_short,omitempty"`
	KeyPath       string `json:"key_path,omitempty"`
	KeyExists     bool   `json:"key_exists"`
	KeyPermOK     bool   `json:"key_perm_ok"`
	KeyringID     string `json:"keyring_id,omitempty"`
	KeyringOK     bool   `json:"keyring_ok"`
	FileCount     int    `json:"file_count"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// ─── VaultInit ─────────────────────────────────────────────────────────────

// VaultInit generates a new age key pair and stores it in the vault
// directory + OS keyring. If the vault is already initialized, prompts
// for confirmation (unless Force is true).
//
// Flow:
//  1. Check vault state — refuse to overwrite unless Force or prompt says yes
//  2. Create ~/.nexus/vault/ with 0700 permissions
//  3. Run `age-keygen -o <keyPath>` — captures public key from stdout
//  4. Write public key to <keyPath>.pub
//  5. Set private key permissions to 0600
//  6. Mirror private key to OS keyring (best-effort — keyring failure
//     is non-fatal; the file is the canonical source)
//  7. Record state
//
// SAFETY: the private key is NEVER logged. The public key IS logged
// (it's not secret). The keyring entry name is logged so audit trails
// can correlate.
func VaultInit(ctx context.Context, deps VaultInitDeps) (*VaultReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: VaultInitDeps.ExecFn must not be nil")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("dotfiles: VaultInitDeps.State must not be nil")
	}

	vaultDir := VaultDir()
	keyPath := filepath.Join(vaultDir, PrivateKeyName)
	pubKeyPath := filepath.Join(vaultDir, PublicKeyName)

	report := &VaultReport{
		Operation: "init",
		StartedAt: time.Now().UTC(),
	}

	// Step 1: confirm overwrite if already initialized.
	existing := deps.State.GetVaultState()
	if existing.Initialized {
		if !deps.Force {
			if deps.Prompt == nil {
				return nil, fmt.Errorf("vault already initialized at %s; pass --force to overwrite", keyPath)
			}
			ok, err := deps.Prompt(fmt.Sprintf("Vault already initialized at %s. Overwrite? [y/N]", keyPath))
			if err != nil {
				return nil, fmt.Errorf("prompt failed: %w", err)
			}
			if !ok {
				return nil, fmt.Errorf("vault init cancelled")
			}
		}
	}

	// Step 2: create vault directory with 0700.
	if err := os.MkdirAll(vaultDir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory %s: %w", vaultDir, err)
	}

	// Step 3: run age-keygen. Public key is printed to stdout.
	out, err := deps.ExecFn(ctx, "age-keygen", "-o", keyPath)
	if err != nil {
		return nil, fmt.Errorf("age-keygen failed: %w", err)
	}
	publicKey := strings.TrimSpace(out)
	if !strings.HasPrefix(publicKey, "age1") {
		return nil, fmt.Errorf("unexpected output from age-keygen (expected public key, got %q)", truncate(out, 80))
	}

	// Step 4: write public key file.
	if err := os.WriteFile(pubKeyPath, []byte(publicKey+"\n"), 0o644); err != nil {
		return nil, fmt.Errorf("failed to write public key: %w", err)
	}

	// Step 5: enforce 0600 on private key.
	if err := os.Chmod(keyPath, 0o600); err != nil {
		return nil, fmt.Errorf("failed to set private key permissions: %w", err)
	}

	// Step 6: mirror to OS keyring (best-effort).
	keyringID := ""
	privKeyData, err := os.ReadFile(keyPath)
	if err == nil {
		if err := keyringSet(VaultKeyringID, string(privKeyData)); err == nil {
			keyringID = VaultKeyringID
		}
		// Keyring failure is non-fatal — the file is the canonical store.
		// On Linux without a Secret Service daemon, the keyring call
		// fails silently and we fall back to file-only.
	}

	// Step 7: record state.
	if err := deps.State.RecordVaultInit(publicKey, keyPath, keyringID); err != nil {
		return nil, fmt.Errorf("failed to record vault state: %w", err)
	}

	// Audit (public key fingerprint only — never the private key).
		logAudit(deps.Audit, "DOTFILES_VAULT_INIT", "success", engine.ShortKeyFingerprint(publicKey))

		report.PublicKey = publicKey
		report.PublicKeyShort = engine.ShortKeyFingerprint(publicKey)
	report.KeyPath = keyPath
	report.KeyringID = keyringID
	report.CompletedAt = time.Now().UTC()
	return report, nil
}

// ─── VaultAdd ──────────────────────────────────────────────────────────────

// VaultAdd encrypts a sensitive file and stores the ciphertext in the
// chezmoi source dir. The original file is untouched.
//
// Flow:
//  1. Verify vault initialized (refuse if not — user must `vault init`)
//  2. Validate the input file: must exist, absolute, under $HOME,
//     no shell metacharacters, size <= 10 MB
//  3. Compute target: <basename>.age in the chezmoi source dir
//  4. Run `age -e -r <pubkey> -o <target> <file>` to encrypt
//  5. Record original → encrypted mapping in state
//
// SAFETY: the file content NEVER touches the audit log. Only the paths
// are logged.
func VaultAdd(ctx context.Context, filePath string, deps VaultAddDeps) (*VaultReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: VaultAddDeps.ExecFn must not be nil")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("dotfiles: VaultAddDeps.State must not be nil")
	}

	vaultState := deps.State.GetVaultState()
	if !vaultState.Initialized {
		return nil, fmt.Errorf("vault not initialized; run 'nexus dotfiles vault init' first")
	}

	report := &VaultReport{
		Operation:    "add",
		OriginalPath: filePath,
		StartedAt:    time.Now().UTC(),
	}

	// Step 2: validate the input file. Reuse V7's path validation but
	// allow sensitive paths (that's the whole point of the vault).
	if err := validateManagedPath(filePath, true /* allowSensitive */); err != nil {
		return nil, fmt.Errorf("invalid file path: %w", err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot stat file: %w", err)
	}
	if info.Size() > MaxVaultFileSize {
		return nil, fmt.Errorf("file is %d bytes; vault refuses files larger than %d MB (use a different mechanism for large files)",
			info.Size(), MaxVaultFileSize/(1024*1024))
	}

	// Step 3: compute target path inside chezmoi source dir.
	basename := filepath.Base(filePath)
	targetName := basename + ".age"
	targetPath := filepath.Join(chezmoiSourceDir(), targetName)

	// Step 4: encrypt.
	_, err = deps.ExecFn(ctx, "age", "-e", "-r", vaultState.PublicKey,
		"-o", targetPath, filePath)
	if err != nil {
		return nil, fmt.Errorf("age encryption failed: %w", err)
	}

	// Step 5: record state.
	if err := deps.State.RecordVaultAdd(filePath, targetPath); err != nil {
		// Try to clean up the orphaned encrypted file so we don't leave
		// state divergence (file exists but not tracked).
		_ = os.Remove(targetPath)
		return nil, fmt.Errorf("failed to record vault state: %w", err)
	}

	logAudit(deps.Audit, "DOTFILES_VAULT_ENCRYPT", "success",
		fmt.Sprintf("%s -> %s", filepath.Base(filePath), targetName))

	report.EncryptedPath = targetPath
	report.CompletedAt = time.Now().UTC()
	return report, nil
}

// ─── VaultList ─────────────────────────────────────────────────────────────

// VaultList returns all encrypted files tracked by the vault. Returns
// nil report (no error) when vault is uninitialized.
func VaultList(deps struct {
	State *engine.StateTracker
}) (*VaultReport, error) {
	if deps.State == nil {
		return nil, fmt.Errorf("dotfiles: State must not be nil")
	}

	vs := deps.State.GetVaultState()
	report := &VaultReport{
		Operation: "list",
	}
	if !vs.Initialized {
		return report, nil
	}

	for orig, enc := range vs.EncryptedFiles {
		size := int64(0)
		if info, err := os.Stat(enc); err == nil {
			size = info.Size()
		}
		report.Files = append(report.Files, VaultFileEntry{
			Original:  orig,
			Encrypted: enc,
			Size:      size,
		})
	}
	return report, nil
}

// ─── VaultStatus ───────────────────────────────────────────────────────────

// VaultStatus returns a structured snapshot of the vault's health:
// initialized? key file present? permissions correct? keyring entry
// accessible? file count?
func VaultStatus(deps struct {
	State *engine.StateTracker
}) (*VaultReport, error) {
	if deps.State == nil {
		return nil, fmt.Errorf("dotfiles: State must not be nil")
	}

	vs := deps.State.GetVaultState()
	status := &VaultStatusReport{
		Initialized:   vs.Initialized,
		PublicKeyShort: vs.PublicKeyShort,
		KeyPath:       vs.KeyPath,
		KeyringID:     vs.KeyringID,
		FileCount:     len(vs.EncryptedFiles),
	}

	if vs.Initialized {
		status.CreatedAt = vs.CreatedAt.Format("2006-01-02T15:04:05Z07:00")

		if vs.KeyPath != "" {
			info, err := os.Stat(vs.KeyPath)
			if err == nil {
				status.KeyExists = true
				status.KeyPermOK = info.Mode().Perm() == 0o600
			}
		}

		if vs.KeyringID != "" {
			_, err := keyringGet(vs.KeyringID)
			status.KeyringOK = err == nil
		}
	}

	return &VaultReport{
		Operation: "status",
		Status:    status,
	}, nil
}

// ─── VaultUnlock ───────────────────────────────────────────────────────────

// VaultUnlock installs a private key on this machine so chezmoi can
// decrypt the vault's encrypted files during `chezmoi apply`. This is
// the command a user runs on a fresh machine after copying their
// private key (via secure channel) or after restoring from keyring.
//
// Flow:
//  1. Load the key — from --key-file if provided, otherwise from keyring
//  2. Validate the key (header must start with AGE-SECRET-KEY-1)
//  3. Verify the key works: encrypt a known plaintext with the public
//     key, decrypt with the new private key. If roundtrip fails or
//     produces wrong output, reject the key.
//  4. Write the verified key to ~/.nexus/vault/private.key with 0600
//  5. Patch chezmoi config (chezmoi.toml) to point at our key file
//
// The roundtrip verify is the critical safety step. Without it, a user
// could paste the wrong file (corrupted, wrong recipient, truncated)
// and silently lose access to all their encrypted dotfiles.
func VaultUnlock(ctx context.Context, deps VaultUnlockDeps) (*VaultReport, error) {
	if deps.ExecFn == nil {
		return nil, fmt.Errorf("dotfiles: VaultUnlockDeps.ExecFn must not be nil")
	}
	if deps.State == nil {
		return nil, fmt.Errorf("dotfiles: VaultUnlockDeps.State must not be nil")
	}

	vs := deps.State.GetVaultState()
	if !vs.Initialized {
		return nil, fmt.Errorf("vault not initialized; run 'nexus dotfiles vault init' first")
	}

	report := &VaultReport{
		Operation:  "unlock",
		KeyPath:    filepath.Join(VaultDir(), PrivateKeyName),
		KeyringID:  vs.KeyringID,
		StartedAt:  time.Now().UTC(),
	}

	// Step 1: load the key.
	keyData, source, err := loadUnlockKey(deps.KeyFile, vs.KeyringID)
	if err != nil {
		return nil, err
	}

	// Step 2: header check.
	keyData = strings.TrimSpace(keyData)
	if !strings.HasPrefix(keyData, "AGE-SECRET-KEY-1") {
		return nil, fmt.Errorf("key file does not appear to be an age private key (missing AGE-SECRET-KEY-1 header)")
	}

	// Step 3: roundtrip verify. Encrypt "nexus-vault-verify" with the
	// public key, then decrypt with the provided private key. If
	// decryption fails or produces wrong output, reject the key.
	verifyPlaintext := fmt.Sprintf("nexus-vault-verify-%d", time.Now().UnixNano())
	ciphertext, err := deps.ExecFn(ctx, "age", "-e", "-r", vs.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("verification: failed to encrypt test plaintext: %w", err)
	}
	// Write ciphertext to temp file for age to decrypt (age -d reads from file).
	tmpCipher, err := os.CreateTemp("", "nexus-vault-verify-*.age")
	if err != nil {
		return nil, fmt.Errorf("verification: failed to create temp file: %w", err)
	}
	defer os.Remove(tmpCipher.Name())
	if _, err := tmpCipher.WriteString(ciphertext); err != nil {
		return nil, fmt.Errorf("verification: failed to write temp file: %w", err)
	}
	tmpCipher.Close()

	decrypted, err := deps.ExecFn(ctx, "age", "-d", "-i", "/dev/stdin", tmpCipher.Name())
	if err != nil {
		return nil, fmt.Errorf("verification: failed to decrypt with provided key (wrong key for this vault?): %w", err)
	}
	decrypted = strings.TrimSpace(decrypted)
	if decrypted != verifyPlaintext {
		return nil, fmt.Errorf("verification: decrypted output mismatch (key may be corrupted or for a different vault)")
	}

	// Step 4: write the verified key to the canonical location.
	if err := os.MkdirAll(VaultDir(), 0o700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory: %w", err)
	}
	if err := os.WriteFile(report.KeyPath, []byte(keyData+"\n"), 0o600); err != nil {
		return nil, fmt.Errorf("failed to write private key: %w", err)
	}

	// Step 5: patch chezmoi config (best-effort — chezmoi may not be installed).
	if err := patchChezmoiAgeConfig(report.KeyPath); err != nil {
		// Non-fatal: user can configure chezmoi manually if needed.
		// The key is installed; chezmoi just won't auto-find it.
		report.Status = &VaultStatusReport{
			Initialized: true,
			KeyPath:     report.KeyPath,
			KeyExists:   true,
			KeyPermOK:   true,
			FileCount:   len(vs.EncryptedFiles),
		}
	} else {
		report.Status = &VaultStatusReport{
			Initialized: true,
			KeyPath:     report.KeyPath,
			KeyExists:   true,
			KeyPermOK:   true,
			KeyringID:   vs.KeyringID,
			KeyringOK:   vs.KeyringID != "",
			FileCount:   len(vs.EncryptedFiles),
		}
	}

	logAudit(deps.Audit, "DOTFILES_VAULT_UNLOCK", "success", source)

	report.CompletedAt = time.Now().UTC()
	return report, nil
}

// loadUnlockKey loads a private key from --key-file or, if empty, from
// the OS keyring. Returns the key data, a human-readable source label,
// and any error.
func loadUnlockKey(keyFile, keyringID string) (string, string, error) {
	if keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return "", "", fmt.Errorf("failed to read key file %s: %w", keyFile, err)
		}
		return string(data), fmt.Sprintf("file:%s", keyFile), nil
	}
	if keyringID != "" {
		data, err := keyringGet(keyringID)
		if err != nil {
			return "", "", fmt.Errorf("failed to read key from keyring (try --key-file): %w", err)
		}
		return data, fmt.Sprintf("keyring:%s", keyringID), nil
	}
	return "", "", fmt.Errorf("no key source: provide --key-file or initialize the vault with keyring support")
}

// patchChezmoiAgeConfig writes/updates the [age] section of
// ~/.config/chezmoi/chezmoi.toml to point chezmoi at our key file.
// Idempotent — only modifies if the file exists.
func patchChezmoiAgeConfig(keyPath string) error {
	configPath := filepath.Join(chezmoiConfigDir(), "chezmoi.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err // file doesn't exist or unreadable — non-fatal
	}
	content := string(data)

	// Idempotent patch: if [age] section already points at our key, no-op.
	needle := fmt.Sprintf("identity = %q", keyPath)
	if strings.Contains(content, needle) {
		return nil
	}

	// Find [age] section and replace the identity line, or append a section.
	var patched string
	if strings.Contains(content, "[age]") {
		// Replace existing identity line within the [age] section.
		lines := strings.Split(content, "\n")
		inAgeSection := false
		replaced := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "[age]" {
				inAgeSection = true
				continue
			}
			if inAgeSection && strings.HasPrefix(trimmed, "[") {
				// Exited the [age] section without finding identity.
				break
			}
			if inAgeSection && strings.HasPrefix(trimmed, "identity") {
				lines[i] = fmt.Sprintf("identity = %q", keyPath)
				replaced = true
				break
			}
		}
		if !replaced {
			// Append identity to the [age] section.
			patched = strings.Join(lines, "\n")
			if !strings.HasSuffix(patched, "\n") {
				patched += "\n"
			}
			patched += fmt.Sprintf("identity = %q\n", keyPath)
		} else {
			patched = strings.Join(lines, "\n")
		}
	} else {
		// No [age] section — append one.
		patched = content
		if !strings.HasSuffix(patched, "\n") {
			patched += "\n"
		}
		patched += fmt.Sprintf("\n[age]\nidentity = %q\n", keyPath)
	}

	return os.WriteFile(configPath, []byte(patched), 0o644)
}

// ─── VaultRemove ───────────────────────────────────────────────────────────

// VaultRemove deletes an encrypted file from the chezmoi source dir and
// removes its tracking entry from state. The ORIGINAL file at the
// source path is NOT touched — only the encrypted copy is removed.
//
// If the original argument matches multiple tracked entries (e.g.,
// basename collision), the user must disambiguate. For now, we match
// by full original path. If no exact match, we match by basename.
func VaultRemove(ctx context.Context, target string, deps VaultRemoveDeps) (*VaultReport, error) {
	if deps.State == nil {
		return nil, fmt.Errorf("dotfiles: VaultRemoveDeps.State must not be nil")
	}

	vs := deps.State.GetVaultState()
	if !vs.Initialized {
		return nil, fmt.Errorf("vault not initialized")
	}

	// Find the entry. Exact path match first, then basename match.
	encryptedPath, ok := vs.EncryptedFiles[target]
	if !ok {
		// Fallback: match by basename. The user might pass "id_rsa" instead of
		// the full path "~/.ssh/id_rsa".
		for orig, enc := range vs.EncryptedFiles {
			if filepath.Base(orig) == target || filepath.Base(orig) == filepath.Base(target) {
				encryptedPath = enc
				target = orig
				ok = true
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("file %q is not in the vault; use 'nexus dotfiles vault list' to see tracked files", target)
	}

	report := &VaultReport{
		Operation:    "remove",
		OriginalPath: target,
		EncryptedPath: encryptedPath,
		StartedAt:    time.Now().UTC(),
	}

	// Prompt for confirmation unless --force.
	if !deps.Force {
		if deps.Prompt == nil {
			return nil, fmt.Errorf("removing encrypted file is destructive; pass --force to skip the confirmation prompt")
		}
		ok, err := deps.Prompt(fmt.Sprintf("Remove %s from the vault? The original file at %s will NOT be touched. [y/N]",
			filepath.Base(encryptedPath), target))
		if err != nil {
			return nil, fmt.Errorf("prompt failed: %w", err)
		}
		if !ok {
			return nil, fmt.Errorf("vault remove cancelled")
		}
	}

	// Delete the encrypted file.
	if err := os.Remove(encryptedPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to delete encrypted file: %w", err)
	}

	// Update state.
	if err := deps.State.RecordVaultRemove(target); err != nil {
		return nil, fmt.Errorf("failed to update vault state: %w", err)
	}

	logAudit(deps.Audit, "DOTFILES_VAULT_REMOVE", "success", filepath.Base(encryptedPath))

	report.CompletedAt = time.Now().UTC()
	return report, nil
}

// ─── Utility ───────────────────────────────────────────────────────────────

// truncate returns the first n bytes of s, appending "..." if truncated.
// Used to safely log potentially large command output.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
