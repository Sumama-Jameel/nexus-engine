# ADR 009: Secrets Vault Strategy

**Status:** Accepted
**Date:** 2026-07-07
**Deciders:** Nexus Core
**Phase:** V9 — Identity & Security (The Soul)

## Context

V7 binds dotfiles, V8 syncs them, and now both are entering the cloud. The moment a user `--force`s an SSH key into their dotfile repo, that key is committed to Git and pushed to the remote — plaintext in a (potentially public) repo. V9 fixes this by encrypting sensitive files *before* they enter the chezmoi source dir.

## Decision

**Use Age (https://age-encryption.org/) for all vault operations.** Age is:
- X25519 + ChaCha20-Poly1305 (modern, audited, simple)
- A single, small binary (`age-keygen` + `age`) with no dependencies
- Designed for "encrypt files before storage in untrusted repos" — exactly our use case

## How it works

1. **`age-keygen -o private.key`** generates an X25519 key pair.
2. **`age -e -r <pubkey> -o output.age <file>`** encrypts file content with the recipient's public key.
3. **`age -d -i private.key -o output.txt <input>`** decrypts with the identity file.

The public key is stored in `~/.nexus/vault/public.key`. The private key is at `~/.nexus/vault/private.key` with `0600` permissions and is optionally mirrored to the OS keyring.

## Consequences

### Positive
- Zero new Go dependencies (age shelled out via `AllowedCommands`)
- Compatible with chezmoi's native `.age` support
- Pre-push secret scan skips `.age` files (ciphertext is safe to commit)
- Works with all three CLI flows: init, encrypt, decrypt, unlock

### Negative
- Key loss = data loss. User is warned at `vault init` time.
- Requires age binary on the system (installed via `nexus dotfiles vault install`)

## References
- ADR 007: Chezmoi Shell-Out Strategy
- ADR 004: Zero-Trust Command Execution
- `internal/dotfiles/vault.go` — all vault operations
- `internal/dotfiles/vault_keyring.go` — OS keyring mirroring