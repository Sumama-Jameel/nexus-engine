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

import "github.com/zalando/go-keyring"

// keyringUser is the "account" portion of the keyring tuple
// (service, user). We use a fixed user because we only store one
// value per service (the vault private key).
const keyringUser = "vault"

// keyringSet stores `value` in the OS keyring under (service, keyringUser).
// Wrapped to keep the zalando/go-keyring import isolated and to provide
// a single seam for future testing or alternate backends.
//
// On Linux without a Secret Service daemon (common in headless / CI
// environments), this returns an error. Callers must treat keyring
// failures as non-fatal — the canonical store is the file on disk.
func keyringSet(service, value string) error {
	return keyring.Set(service, keyringUser, value)
}

// keyringGet retrieves the value stored under (service, keyringUser).
// Returns an error if no entry exists or the keyring backend is unavailable.
func keyringGet(service string) (string, error) {
	return keyring.Get(service, keyringUser)
}

// keyringDelete removes the entry under (service, keyringUser). Used
// during vault re-init to avoid stale entries lingering after the key
// has been rotated.
func keyringDelete(service string) error {
	return keyring.Delete(service, keyringUser)
}
