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

package main

import (
	"encoding/json"
	"os"

	"github.com/Sumama-Jameel/nexus-engine/internal/dotfiles"
)

func jsonOutput(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}

func shortSHA(s string) string {
	if s == "" {
		return "<unknown>"
	}
	if len(s) <= 7 {
		return s
	}
	return s[:7]
}

func resolveSyncToken(flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	return os.Getenv("NEXUS_DOTFILES_TOKEN")
}

func reportMatchesForJSON(matches []dotfiles.Match) interface{} {
	if matches == nil {
		return nil
	}
	return matches
}
