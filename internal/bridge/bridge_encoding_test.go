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

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestDecodeWSLOutput(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		result := decodeWSLOutput([]byte{})
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("nil input", func(t *testing.T) {
		result := decodeWSLOutput(nil)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("UTF-8 passthrough", func(t *testing.T) {
		input := []byte("Hello, World!")
		result := decodeWSLOutput(input)
		if result != "Hello, World!" {
			t.Errorf("expected 'Hello, World!', got %q", result)
		}
	})

	t.Run("UTF-16 LE BOM with ASCII text", func(t *testing.T) {
		// UTF-16 LE encoding of "Hello" with BOM
		input := []byte{0xFF, 0xFE, 0x48, 0x00, 0x65, 0x00, 0x6C, 0x00, 0x6C, 0x00, 0x6F, 0x00}
		result := decodeWSLOutput(input)
		if result != "Hello" {
			t.Errorf("expected 'Hello', got %q", result)
		}
	})

	t.Run("UTF-16 LE without BOM but with null bytes", func(t *testing.T) {
		// UTF-16 LE encoding of "Hello World!" without BOM
		// Must be long enough for null byte detection (>=20 bytes, >=4 null bytes)
		wslText := "Hello World!"
		input := []byte{}
		for _, r := range wslText {
			input = append(input, byte(r), 0x00)
		}
		result := decodeWSLOutput(input)
		if result != wslText {
			t.Errorf("expected %q, got %q", wslText, result)
		}
	})

	t.Run("UTF-16 LE WSL distro list", func(t *testing.T) {
		// Simulated WSL --list --verbose output in UTF-16 LE with BOM
		// "  NAME              STATE           VERSION\n* Ubuntu            Running         2\n  Debian            Stopped         2"
		wslText := "  NAME              STATE           VERSION\n* Ubuntu            Running         2\n  Debian            Stopped         2"
		input := encodeUTF16LE(wslText)

		result := decodeWSLOutput(input)
		if !strings.Contains(result, "NAME") {
			t.Error("decoded output should contain 'NAME'")
		}
		if !strings.Contains(result, "Ubuntu") {
			t.Error("decoded output should contain 'Ubuntu'")
		}
		if !strings.Contains(result, "Running") {
			t.Error("decoded output should contain 'Running'")
		}
	})

	t.Run("UTF-16 LE WSL version output", func(t *testing.T) {
		wslText := "WSL version: 2.0.9.0\nKernel version: 5.15.133.1-1"
		input := encodeUTF16LE(wslText)

		result := decodeWSLOutput(input)
		if !strings.Contains(result, "WSL version:") {
			t.Error("decoded output should contain 'WSL version:'")
		}
		if !strings.Contains(result, "2.0.9.0") {
			t.Error("decoded output should contain '2.0.9.0'")
		}
	})
}

// encodeUTF16LE encodes a UTF-8 string to UTF-16 LE with BOM for testing.
func encodeUTF16LE(s string) []byte {
	// Add BOM
	result := []byte{0xFF, 0xFE}

	// Encode each rune as UTF-16 LE
	for _, r := range s {
		if r <= 0xFFFF {
			result = append(result, byte(r), byte(r>>8))
		} else {
			// Surrogate pair for characters above U+FFFF
			r1, r2 := utf16.EncodeRune(r)
			result = append(result, byte(r1), byte(r1>>8), byte(r2), byte(r2>>8))
		}
	}

	return result
}
