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

package engine

import "syscall"

// probeDisk uses syscall.Statfs to get disk information on Linux.
func probeDisk(info *SystemInfo) error {
        var stat syscall.Statfs_t
        if err := syscall.Statfs("/", &stat); err != nil {
                return err
        }
        // Total and used space in bytes
        totalBytes := stat.Blocks * uint64(stat.Bsize) //nolint:gosec
        availBytes := stat.Bavail * uint64(stat.Bsize) //nolint:gosec
        usedBytes := totalBytes - availBytes

        info.DiskTotalGB = float64(totalBytes) / 1024 / 1024 / 1024
        info.DiskUsedGB = float64(usedBytes) / 1024 / 1024 / 1024
        return nil
}
