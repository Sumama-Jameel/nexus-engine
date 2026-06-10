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

import "fmt"

// NexusError is the base error type for all Nexus installer errors.
// It provides a machine-readable Code, a human-readable Message,
// the Stage where the error occurred, and the underlying Cause.
type NexusError struct {
	Cause   error  // Underlying error
	Code    string // Machine-readable: PREFLIGHT_FAIL, FOUNDATION_FAIL, VERIFY_FAIL, ROLLBACK_FAIL, INSTALL_FAIL
	Message string // Human-readable description
	Stage   string // PreFlight, Install, Verify, Rollback
}

func (e *NexusError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s (stage=%s): %v", e.Code, e.Message, e.Stage, e.Cause)
	}
	return fmt.Sprintf("[%s] %s (stage=%s)", e.Code, e.Message, e.Stage)
}

func (e *NexusError) Unwrap() error { return e.Cause }

// Convenience constructors for each error category.
// Each constructor sets the appropriate Code and Stage so callers
// can distinguish error types using errors.As without string matching.

// NewPreflightError creates an error for pre-flight check failures.
// Stage: PreFlight, Code: PREFLIGHT_FAIL.
func NewPreflightError(message string, cause error) *NexusError {
	return &NexusError{
		Code:    "PREFLIGHT_FAIL",
		Message: message,
		Stage:   "PreFlight",
		Cause:   cause,
	}
}

// NewFoundationError creates an error for foundation package failures
// that trigger an abort and rollback.
// Stage: Install, Code: FOUNDATION_FAIL.
func NewFoundationError(message string, cause error) *NexusError {
	return &NexusError{
		Code:    "FOUNDATION_FAIL",
		Message: message,
		Stage:   "Install",
		Cause:   cause,
	}
}

// NewVerifyError creates an error for post-installation verification failures.
// Stage: Verify, Code: VERIFY_FAIL.
func NewVerifyError(message string, cause error) *NexusError {
	return &NexusError{
		Code:    "VERIFY_FAIL",
		Message: message,
		Stage:   "Verify",
		Cause:   cause,
	}
}

// NewRollbackError creates an error for rollback operation failures.
// Stage: Rollback, Code: ROLLBACK_FAIL.
func NewRollbackError(message string, cause error) *NexusError {
	return &NexusError{
		Code:    "ROLLBACK_FAIL",
		Message: message,
		Stage:   "Rollback",
		Cause:   cause,
	}
}

// NewInstallError creates an error for general installation failures
// that are not specific to foundation, preflight, verify, or rollback.
// Stage: Install, Code: INSTALL_FAIL.
func NewInstallError(message string, cause error) *NexusError {
	return &NexusError{
		Code:    "INSTALL_FAIL",
		Message: message,
		Stage:   "Install",
		Cause:   cause,
	}
}
