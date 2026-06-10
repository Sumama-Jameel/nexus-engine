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

package engine

import (
        "syscall"
        "unsafe"
)

// probeDisk uses GetDiskFreeSpaceEx to get disk information on Windows.
func probeDisk(info *SystemInfo) error {
        var freeBytesAvailable int64
        var totalNumberOfBytes int64
        var totalNumberOfFreeBytes int64

        kernel32 := syscall.NewLazyDLL("kernel32.dll")
        getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

        rootPathName, _ := syscall.UTF16PtrFromString("C:\\")

        ret, _, err := getDiskFreeSpaceEx.Call(
                uintptr(unsafe.Pointer(rootPathName)),
                uintptr(unsafe.Pointer(&freeBytesAvailable)),
                uintptr(unsafe.Pointer(&totalNumberOfBytes)),
                uintptr(unsafe.Pointer(&totalNumberOfFreeBytes)),
        )
        if ret == 0 {
                return err
        }

        usedBytes := totalNumberOfBytes - freeBytesAvailable
        info.DiskTotalGB = float64(totalNumberOfBytes) / 1024 / 1024 / 1024
        info.DiskUsedGB = float64(usedBytes) / 1024 / 1024 / 1024
        return nil
}
