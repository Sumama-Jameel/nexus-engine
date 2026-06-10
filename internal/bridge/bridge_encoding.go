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

package bridge

// decodeWSLOutput converts Windows WSL command output from UTF-16 LE to UTF-8.
//
// Windows `wsl.exe` commands (wsl --list --verbose, wsl --version, wsl --status)
// output text encoded in UTF-16 LE with a Byte Order Mark (BOM) of 0xFF 0xFE.
// When this output is naively interpreted as UTF-8 via `string(output)`, the
// result contains garbage characters (null bytes interspersed, BOM at start).
//
// This function:
//  1. Detects and strips the UTF-16 LE BOM (0xFF 0xFE) if present
//  2. Detects UTF-16 LE encoding by checking for null bytes in ASCII range
//  3. Decodes UTF-16 LE byte pairs into valid UTF-8 runes
//
// If the input is already valid UTF-8 (no BOM, no null bytes), it is returned
// unchanged. This function is safe to call on all platforms.
func decodeWSLOutput(b []byte) string {
	if len(b) == 0 {
		return ""
	}

	// Check for UTF-16 LE BOM (0xFF 0xFE)
	hasBOM := len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE

	// Detect UTF-16 LE by checking for null bytes in ASCII range
	// In UTF-16 LE, ASCII characters have a null byte as the high byte
	// (e.g., 'A' = 0x41 0x00). We check the first 20 bytes for this pattern.
	isUTF16LE := hasBOM
	if !isUTF16LE && len(b) >= 20 {
		// Check for null byte pattern: even byte is printable ASCII, odd byte is 0
		nullCount := 0
		for i := 1; i < 20 && i < len(b); i += 2 {
			if b[i] == 0 && b[i-1] >= 0x20 && b[i-1] <= 0x7E {
				nullCount++
			}
		}
		// If more than half the checked bytes are null, it's likely UTF-16 LE
		isUTF16LE = nullCount >= 4
	}

	if !isUTF16LE {
		// Already UTF-8, return as-is
		return string(b)
	}

	// Strip BOM if present
	if hasBOM {
		b = b[2:]
	}

	// Decode UTF-16 LE byte pairs into runes
	runes := make([]rune, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(b[i]) | rune(b[i+1])<<8
		runes = append(runes, r)
	}

	// Encode as UTF-8 string
	return string(runes)
}
