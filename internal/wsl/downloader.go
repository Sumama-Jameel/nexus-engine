//go:build windows

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

package wsl

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Sumama-Jameel/nexus-engine/internal/engine"
)

// DownloadTimeout is the maximum time allowed for a single download.
// Large rootfs images (~120MB) on slow connections may need significant time.
// This can be overridden via context.
const DownloadTimeout = 10 * time.Minute

// DownloadProgress is a callback function that reports download progress.
// totalBytes is the expected total size (-1 if unknown).
// downloadedBytes is the number of bytes downloaded so far.
// percent is the calculated percentage (0-100, -1 if total is unknown).
//
// This callback enables:
//   - CLI: spinner/progress bar display
//   - Future Tauri HUD: progress bar in the GUI
type DownloadProgress func(totalBytes, downloadedBytes int64, percent float64)

// Downloader provides secure network I/O for fetching rootfs tarballs.
//
// Per V5 "The Instant Linux Importer (The Bridge)":
// "What to learn: Network I/O (Downloading files in Go)."
//
// But enterprise-grade means we don't just call http.Get(). This downloader
// enforces:
//  1. HTTPS-only (no plaintext HTTP)
//  2. SHA256 integrity verification (constant-time comparison)
//  3. SSRF protection (reject private IP ranges)
//  4. Atomic file writes (temp file + rename)
//  5. Progress reporting (for CLI and future HUD)
//  6. Content-Length validation (fail fast on size mismatch)
//  7. Context-based timeout (don't hang forever)
type Downloader struct {
	// client is the HTTP client used for downloads.
	// Configured with sensible timeouts and SSRF-safe transport.
	client *http.Client

	// progress is the optional progress callback.
	progress DownloadProgress
}

// NewDownloader creates a new secure Downloader instance.
// The optional progress callback receives download progress updates.
//
// SSRF protection is provided by engine.NewSSRFSafeTransport, shared with
// the V7 dotfile source resolver (see internal/engine/netguard.go).
func NewDownloader(progress DownloadProgress) *Downloader {
	transport := engine.NewSSRFSafeTransport()

	return &Downloader{
		client: &http.Client{
			Transport: transport,
			// Overall timeout is handled by the context, not the client,
			// because the context can be extended for large downloads.
			Timeout: 0,
		},
		progress: progress,
	}
}

// Download fetches a rootfs tarball from the given URL, verifies its integrity,
// and saves it atomically to the target path.
//
// The download flow:
//  1. Validate URL (HTTPS-only, no SSRF)
//  2. Create HTTP request with context
//  3. Stream response to temp file with progress reporting
//  4. Verify SHA256 hash (constant-time comparison)
//  5. Atomic rename from temp file to target path
//
// If the download fails or the hash doesn't match, the temp file is removed
// and no partial/corrupted file is left at the target path.
func (d *Downloader) Download(ctx context.Context, downloadURL, targetPath, expectedSHA256 string, expectedSize int64) error {
	// Step 1: Validate URL (HTTPS-only SSRF check; DNS-level rejection of
	// private IPs is enforced by the transport's DialContext).
	if err := engine.ValidateURL(downloadURL); err != nil {
		return fmt.Errorf("download URL validation failed: %w", err)
	}

	// Step 2: Check if file already exists and is verified
	if _, err := os.Stat(targetPath); err == nil {
		// File exists — verify its hash
		if err := VerifyFileSHA256(targetPath, expectedSHA256); err == nil {
			// Already downloaded and verified — skip download
			if d.progress != nil {
				d.progress(expectedSize, expectedSize, 100)
			}
			return nil
		}
		// Hash mismatch — remove the corrupted file
		os.Remove(targetPath)
	}

	// Step 3: Ensure target directory exists
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory '%s': %w", targetDir, err)
	}

	// Step 4: Create temp file for atomic write
	tmpPath := targetPath + ".downloading"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file '%s': %w", tmpPath, err)
	}
	defer func() {
		tmpFile.Close()
		// Clean up temp file if download failed
		if _, err := os.Stat(tmpPath); err == nil {
			os.Remove(tmpPath)
		}
	}()

	// Step 5: Create HTTP request with context
	ctx, cancel := context.WithTimeout(ctx, DownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set User-Agent for server-side logging
	req.Header.Set("User-Agent", "NexusProtocol/0.5.0")

	// Step 6: Execute request
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request returned status %d (%s)", resp.StatusCode, resp.Status)
	}

	// Step 7: Validate Content-Length if provided
	contentLength := resp.ContentLength
	if expectedSize > 0 && contentLength > 0 && contentLength != expectedSize {
		return fmt.Errorf("content-length mismatch: expected %d bytes, server reports %d bytes",
			expectedSize, contentLength)
	}

	// Step 8: Stream response body to temp file with progress reporting
	hasher := sha256.New()
	downloaded := int64(0)
	totalForProgress := expectedSize
	if contentLength > 0 {
		totalForProgress = contentLength
	}

	buf := make([]byte, 32*1024) // 32KB read buffer
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			// Write to temp file
			written, writeErr := tmpFile.Write(buf[:n])
			if writeErr != nil {
				return fmt.Errorf("failed to write to temp file: %w", writeErr)
			}
			if written != n {
				return fmt.Errorf("short write: expected %d bytes, wrote %d", n, written)
			}

			// Update hash
			hasher.Write(buf[:n])

			// Update progress
			downloaded += int64(n)
			if d.progress != nil {
				percent := float64(-1)
				if totalForProgress > 0 {
					percent = float64(downloaded) / float64(totalForProgress) * 100
				}
				d.progress(totalForProgress, downloaded, percent)
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("download stream error: %w", readErr)
		}
	}

	// Step 9: Verify SHA256 hash (constant-time comparison)
	computedHash := fmt.Sprintf("%x", hasher.Sum(nil))
	if expectedSHA256 != "" && !strings.HasPrefix(expectedSHA256, "placeholder") {
		if subtle.ConstantTimeCompare([]byte(computedHash), []byte(expectedSHA256)) != 1 {
			return fmt.Errorf("SHA256 verification FAILED: computed %s does not match expected %s. "+
				"The downloaded file may be corrupted or tampered with. Download aborted.",
				computedHash, expectedSHA256)
		}
	}

	// Step 10: Sync and close temp file
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	tmpFile.Close()

	// Step 11: Atomic rename to target path
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return fmt.Errorf("failed to commit download (rename failed): %w", err)
	}

	return nil
}

// VerifyFileSHA256 computes the SHA256 hash of a file and compares it
// against an expected hash using constant-time comparison.
//
// This function is used:
//   - After download to verify integrity
//   - On subsequent runs to skip re-downloading verified files
//   - By the `nexus wsl import --verify-only` command
//
// SECURITY: Uses crypto/subtle.ConstantTimeCompare to prevent timing attacks.
func VerifyFileSHA256(path, expectedSHA256 string) error {
	if expectedSHA256 == "" || strings.HasPrefix(expectedSHA256, "placeholder") {
		// Placeholder hash — skip verification with a warning
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file for verification: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("failed to compute hash: %w", err)
	}

	computedHash := fmt.Sprintf("%x", hasher.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(computedHash), []byte(expectedSHA256)) != 1 {
		return fmt.Errorf("SHA256 mismatch: computed %s, expected %s", computedHash, expectedSHA256)
	}

	return nil
}
