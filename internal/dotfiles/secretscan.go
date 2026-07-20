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
	"os"
	"regexp"
	"strings"
)

// Match represents a single secret-pattern hit in a scanned file.
//
// Snippet contains the matched line with the secret value replaced by
// "***REDACTED***" so the value never reaches logs, audit entries, or
// user-facing output.
type Match struct {
	// Pattern is the human-readable name of the pattern that matched
	// (e.g., "AWS Access Key ID"). Never contains the secret value.
	Pattern string `json:"pattern"`
	// Line is the 1-based line number where the match was found.
	Line int `json:"line"`
	// Snippet is the line content with the secret replaced by a redacted
	// placeholder. Safe to log.
	Snippet string `json:"snippet"`
}

// secretPatterns is the ordered list of regexes we scan for.
//
// Each pattern is intentionally high-confidence (fixed prefix + length) to
// keep false-positive rate low. False positives train users to ignore
// real warnings — a worse outcome than a missed novel secret format.
//
// Patterns are intentionally narrow. This is the LAST line of defense
// before bytes leave the machine via `nexus dotfiles push`. V7 already
// blocks sensitive paths by default; this catches secrets that slipped
// into non-sensitive files (e.g., a .zshrc with an exported API key).
var secretPatterns = []struct {
	Name  string
	Regex *regexp.Regexp
}{
	{
		Name:  "AWS Access Key ID",
		Regex: regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	},
	{
		Name:  "GitHub Personal Access Token (classic)",
		Regex: regexp.MustCompile(`ghp_[0-9a-zA-Z]{36}`),
	},
	{
		Name:  "GitHub Fine-Grained Personal Access Token",
		Regex: regexp.MustCompile(`github_pat_[0-9a-zA-Z_]{82}`),
	},
	{
		Name:  "Private Key (PEM block)",
		Regex: regexp.MustCompile(`-----BEGIN (RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----`),
	},
	{
		Name:  "Slack Token",
		Regex: regexp.MustCompile(`xox[abposr]-[0-9a-zA-Z\-]{10,}`),
	},
	{
		Name: "Generic API key assignment",
		// Matches lines like:  api_key = "abcdefghijklmnop1234"
		//                     APItoken: 'xyz123abc456def789ghi'
		// 16+ char alphanumeric value, quoted.
		Regex: regexp.MustCompile(`(?i)(api[_-]?key|apikey|auth[_-]?token)\s*[:=]\s*["'][A-Za-z0-9_\-]{16,}["']`),
	},
}

// Scan inspects the given content for known secret patterns and returns
// all matches. The returned Match.Snippet values are safe to log — the
// secret is replaced with "***REDACTED***".
//
// Returns nil (not an empty slice) when no matches are found, so callers
// can use `len(matches) == 0` or `matches == nil` interchangeably.
//
// Per V8: this runs on every staged file before `chezmoi git push`. It is
// defense-in-depth — V7 already blocks sensitive paths by default, and
// `--force` can override the V7 block. The scanner is the safety net for
// secrets that ended up in non-sensitive files.
func Scan(content []byte) []Match {
	if len(content) == 0 {
		return nil
	}

	var matches []Match
	for _, line := range strings.Split(string(content), "\n") {
		for _, p := range secretPatterns {
			if loc := p.Regex.FindStringIndex(line); loc != nil {
				matches = append(matches, Match{
					Pattern: p.Name,
					Line:    lineNumber(content, line),
					Snippet: redact(line, loc[0], loc[1]),
				})
			}
		}
	}
	return matches
}

// ScanFile inspects the file at path for known secret patterns. V9 update:
// files with the `.age` extension are skipped entirely — their content is
// age-encrypted ciphertext (X25519 + ChaCha20-Poly1305), which contains no
// plaintext secrets and is safe to commit to any repo. This is what lets
// users manage encrypted SSH keys and cloud credentials in (potentially
// public) dotfile repos.
//
// Files that cannot be read (deleted between staging and scan, permission
// errors) return nil matches — they're skipped, same as Scan over an
// empty buffer.
func ScanFile(path string) []Match {
	if strings.HasSuffix(path, ".age") {
		return nil // ciphertext is safe
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return Scan(content)
}

// lineNumber returns the 1-based line number of `line` within `content`.
// We re-split here (rather than threading the index through) because
// strings.Split already walked the content — this just finds the offset.
func lineNumber(content []byte, line string) int {
	idx := strings.Index(string(content), line)
	if idx < 0 {
		return 1
	}
	return strings.Count(string(content[:idx]), "\n") + 1
}

// redact returns line[start:end] with the matched span replaced by
// "***REDACTED***". Preserves surrounding context for usability.
func redact(line string, start, end int) string {
	if start < 0 || end > len(line) || start > end {
		return "***REDACTED***"
	}
	// Cap the visible prefix/suffix so the redacted snippet stays short.
	const contextChars = 20
	prefixStart := start - contextChars
	if prefixStart < 0 {
		prefixStart = 0
	}
	suffixEnd := end + contextChars
	if suffixEnd > len(line) {
		suffixEnd = len(line)
	}
	return line[prefixStart:start] + "***REDACTED***" + line[end:suffixEnd]
}
