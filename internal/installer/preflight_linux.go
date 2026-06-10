//go:build linux

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

package installer

import (
        "os"
        "syscall"
)

// checkDiskSpace verifies at least minMB megabytes are free on the root filesystem.
func checkDiskSpace(minMB uint64) bool {
        var stat syscall.Statfs_t
        if err := syscall.Statfs("/", &stat); err != nil {
                return false
        }
        freeMB := (stat.Bavail * uint64(stat.Bsize)) / 1024 / 1024 //nolint:gosec
        return freeMB >= minMB
}

// checkLock checks if the package manager's lock file is held by another process.
// This uses actual flock testing where possible, not just file existence.
func checkLock(family string) bool {
        lockFiles := map[string][]string{
                "debian": {"/var/lib/dpkg/lock-frontend", "/var/lib/apt/lists/lock"},
                "ubuntu": {"/var/lib/dpkg/lock-frontend", "/var/lib/apt/lists/lock"},
                "arch":   {"/var/lib/pacman/db.lck"},
                "fedora": {"/var/lib/rpm/.rpm.lock"},
                "alpine": {}, // Alpine uses no lock file
        }

        files, ok := lockFiles[family]
        if !ok {
                return true // Unknown family — assume no lock
        }

        return checkLockFiles(files)
}

// checkLockFiles tests whether any of the given lock files are held by another
// process. It attempts to acquire an exclusive, non-blocking flock on each file.
// If any flock call fails, the lock is considered held and the function returns false.
func checkLockFiles(files []string) bool {
        for _, f := range files {
                // Try to open the lock file exclusively (non-blocking)
                lockFile, err := os.OpenFile(f, os.O_RDONLY, 0)
                if err != nil {
                        if os.IsPermission(err) {
                                // Can't read the lock file — likely held by root
                                // This is normal on a system where we're not root
                                continue
                        }
                        if os.IsNotExist(err) {
                                // No lock file = no lock held
                                continue
                        }
                        // Other error — can't determine, assume no lock
                        continue
                }
                defer lockFile.Close()

                // Try to acquire an exclusive, non-blocking lock
                // If another process holds it, this will fail immediately
                err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
                if err != nil {
                        // Lock is held by another process
                        return false
                }
                // We got the lock — release it immediately
                syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
        }

        return true
}
