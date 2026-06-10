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
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- NexusError.Error() tests ---

func TestNexusError_Error_WithCause(t *testing.T) {
	cause := errors.New("disk full")
	err := &NexusError{
		Code:    "PREFLIGHT_FAIL",
		Message: "pre-flight check failed",
		Stage:   "PreFlight",
		Cause:   cause,
	}

	got := err.Error()
	want := "[PREFLIGHT_FAIL] pre-flight check failed (stage=PreFlight): disk full"

	if got != want {
		t.Errorf("Error() with cause = %q, want %q", got, want)
	}
}

func TestNexusError_Error_WithoutCause(t *testing.T) {
	err := &NexusError{
		Code:    "VERIFY_FAIL",
		Message: "verification failed",
		Stage:   "Verify",
		Cause:   nil,
	}

	got := err.Error()
	want := "[VERIFY_FAIL] verification failed (stage=Verify)"

	if got != want {
		t.Errorf("Error() without cause = %q, want %q", got, want)
	}
}

func TestNexusError_Error_AllStages(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		message string
		stage   string
		cause   error
		want    string
	}{
		{
			name:    "PreFlight stage",
			code:    "PREFLIGHT_FAIL",
			message: "check failed",
			stage:   "PreFlight",
			cause:   errors.New("root cause"),
			want:    "[PREFLIGHT_FAIL] check failed (stage=PreFlight): root cause",
		},
		{
			name:    "Install stage",
			code:    "INSTALL_FAIL",
			message: "install failed",
			stage:   "Install",
			cause:   nil,
			want:    "[INSTALL_FAIL] install failed (stage=Install)",
		},
		{
			name:    "Verify stage",
			code:    "VERIFY_FAIL",
			message: "verify failed",
			stage:   "Verify",
			cause:   nil,
			want:    "[VERIFY_FAIL] verify failed (stage=Verify)",
		},
		{
			name:    "Rollback stage",
			code:    "ROLLBACK_FAIL",
			message: "rollback failed",
			stage:   "Rollback",
			cause:   errors.New("partial undo"),
			want:    "[ROLLBACK_FAIL] rollback failed (stage=Rollback): partial undo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &NexusError{Code: tt.code, Message: tt.message, Stage: tt.stage, Cause: tt.cause}
			got := err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- NexusError.Unwrap() tests ---

func TestNexusError_Unwrap_WithCause(t *testing.T) {
	cause := errors.New("underlying failure")
	err := &NexusError{
		Code:    "FOUNDATION_FAIL",
		Message: "foundation",
		Stage:   "Install",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped == nil {
		t.Fatal("Unwrap() returned nil, expected non-nil cause")
	}
	if unwrapped.Error() != "underlying failure" {
		t.Errorf("Unwrap() = %q, want %q", unwrapped.Error(), "underlying failure")
	}
}

func TestNexusError_Unwrap_NilCause(t *testing.T) {
	err := &NexusError{
		Code:    "INSTALL_FAIL",
		Message: "install",
		Stage:   "Install",
		Cause:   nil,
	}

	unwrapped := err.Unwrap()
	if unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

// --- Constructor tests: individual ---

func TestNewVerifyError(t *testing.T) {
	cause := errors.New("checksum mismatch")
	err := NewVerifyError("package verification failed", cause)

	if err.Code != "VERIFY_FAIL" {
		t.Errorf("Code = %q, want %q", err.Code, "VERIFY_FAIL")
	}
	if err.Stage != "Verify" {
		t.Errorf("Stage = %q, want %q", err.Stage, "Verify")
	}
	if err.Message != "package verification failed" {
		t.Errorf("Message = %q, want %q", err.Message, "package verification failed")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewVerifyError_NilCause(t *testing.T) {
	err := NewVerifyError("verification timeout", nil)

	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
	// Ensure Error() doesn't panic and formats correctly without cause.
	got := err.Error()
	if !strings.Contains(got, "VERIFY_FAIL") || !strings.Contains(got, "stage=Verify") {
		t.Errorf("Error() = %q, missing expected components", got)
	}
	if strings.Contains(got, ": ") {
		t.Errorf("Error() should not contain cause separator when cause is nil, got %q", got)
	}
}

func TestNewRollbackError(t *testing.T) {
	cause := errors.New("state corruption")
	err := NewRollbackError("rollback incomplete", cause)

	if err.Code != "ROLLBACK_FAIL" {
		t.Errorf("Code = %q, want %q", err.Code, "ROLLBACK_FAIL")
	}
	if err.Stage != "Rollback" {
		t.Errorf("Stage = %q, want %q", err.Stage, "Rollback")
	}
	if err.Message != "rollback incomplete" {
		t.Errorf("Message = %q, want %q", err.Message, "rollback incomplete")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewRollbackError_NilCause(t *testing.T) {
	err := NewRollbackError("rollback partial", nil)

	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
	got := err.Error()
	if !strings.Contains(got, "ROLLBACK_FAIL") || !strings.Contains(got, "stage=Rollback") {
		t.Errorf("Error() = %q, missing expected components", got)
	}
}

func TestNewInstallError(t *testing.T) {
	cause := errors.New("permission denied")
	err := NewInstallError("install aborted", cause)

	if err.Code != "INSTALL_FAIL" {
		t.Errorf("Code = %q, want %q", err.Code, "INSTALL_FAIL")
	}
	if err.Stage != "Install" {
		t.Errorf("Stage = %q, want %q", err.Stage, "Install")
	}
	if err.Message != "install aborted" {
		t.Errorf("Message = %q, want %q", err.Message, "install aborted")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestNewInstallError_NilCause(t *testing.T) {
	err := NewInstallError("install failed", nil)

	if err.Cause != nil {
		t.Errorf("Cause = %v, want nil", err.Cause)
	}
	got := err.Error()
	if !strings.Contains(got, "INSTALL_FAIL") || !strings.Contains(got, "stage=Install") {
		t.Errorf("Error() = %q, missing expected components", got)
	}
}

// --- Table-driven constructor tests ---

func TestConstructors_TableDriven(t *testing.T) {
	cause := errors.New("root cause")

	tests := []struct {
		name    string
		builder func(string, error) *NexusError
		message string
		wantCode string
		wantStage string
	}{
		{
			name:      "NewPreflightError",
			builder:   NewPreflightError,
			message:   "preflight check",
			wantCode:  "PREFLIGHT_FAIL",
			wantStage: "PreFlight",
		},
		{
			name:      "NewFoundationError",
			builder:   NewFoundationError,
			message:   "foundation setup",
			wantCode:  "FOUNDATION_FAIL",
			wantStage: "Install",
		},
		{
			name:      "NewVerifyError",
			builder:   NewVerifyError,
			message:   "verify packages",
			wantCode:  "VERIFY_FAIL",
			wantStage: "Verify",
		},
		{
			name:      "NewRollbackError",
			builder:   NewRollbackError,
			message:   "rollback changes",
			wantCode:  "ROLLBACK_FAIL",
			wantStage: "Rollback",
		},
		{
			name:      "NewInstallError",
			builder:   NewInstallError,
			message:   "install packages",
			wantCode:  "INSTALL_FAIL",
			wantStage: "Install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.builder(tt.message, cause)

			if err.Code != tt.wantCode {
				t.Errorf("Code = %q, want %q", err.Code, tt.wantCode)
			}
			if err.Stage != tt.wantStage {
				t.Errorf("Stage = %q, want %q", err.Stage, tt.wantStage)
			}
			if err.Message != tt.message {
				t.Errorf("Message = %q, want %q", err.Message, tt.message)
			}
			if err.Cause != cause {
				t.Errorf("Cause = %v, want %v", err.Cause, cause)
			}

			// Verify Error() output contains all expected parts.
			errStr := err.Error()
			if !strings.Contains(errStr, "["+tt.wantCode+"]") {
				t.Errorf("Error() %q missing code [%s]", errStr, tt.wantCode)
			}
			if !strings.Contains(errStr, "stage="+tt.wantStage) {
				t.Errorf("Error() %q missing stage=%s", errStr, tt.wantStage)
			}
			if !strings.Contains(errStr, tt.message) {
				t.Errorf("Error() %q missing message %q", errStr, tt.message)
			}
			if !strings.Contains(errStr, "root cause") {
				t.Errorf("Error() %q missing cause text", errStr)
			}
		})
	}
}

func TestConstructors_TableDriven_NilCause(t *testing.T) {
	tests := []struct {
		name    string
		builder func(string, error) *NexusError
		message string
		wantCode string
		wantStage string
	}{
		{
			name:      "NewPreflightError nil cause",
			builder:   NewPreflightError,
			message:   "preflight",
			wantCode:  "PREFLIGHT_FAIL",
			wantStage: "PreFlight",
		},
		{
			name:      "NewFoundationError nil cause",
			builder:   NewFoundationError,
			message:   "foundation",
			wantCode:  "FOUNDATION_FAIL",
			wantStage: "Install",
		},
		{
			name:      "NewVerifyError nil cause",
			builder:   NewVerifyError,
			message:   "verify",
			wantCode:  "VERIFY_FAIL",
			wantStage: "Verify",
		},
		{
			name:      "NewRollbackError nil cause",
			builder:   NewRollbackError,
			message:   "rollback",
			wantCode:  "ROLLBACK_FAIL",
			wantStage: "Rollback",
		},
		{
			name:      "NewInstallError nil cause",
			builder:   NewInstallError,
			message:   "install",
			wantCode:  "INSTALL_FAIL",
			wantStage: "Install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.builder(tt.message, nil)

			if err.Cause != nil {
				t.Errorf("Cause = %v, want nil", err.Cause)
			}
			// Error() should not contain the ": cause" suffix when cause is nil.
			errStr := err.Error()
			if strings.Contains(errStr, ": ") {
				// Check if the ": " is part of the cause suffix (after stage=...).
				stageIdx := strings.Index(errStr, "(stage=")
				if stageIdx != -1 {
					afterStage := errStr[stageIdx:]
					if strings.Contains(afterStage, ": ") {
						t.Errorf("Error() %q should not have cause suffix with nil cause", errStr)
					}
				}
			}
		})
	}
}

// --- errors.Is / errors.As compatibility ---

func TestNexusError_ErrorsAs(t *testing.T) {
	tests := []struct {
		name    string
		err     *NexusError
	}{
		{
			name: "PreflightError as NexusError",
			err:  NewPreflightError("preflight", errors.New("cause")),
		},
		{
			name: "FoundationError as NexusError",
			err:  NewFoundationError("foundation", errors.New("cause")),
		},
		{
			name: "VerifyError as NexusError",
			err:  NewVerifyError("verify", errors.New("cause")),
		},
		{
			name: "RollbackError as NexusError",
			err:  NewRollbackError("rollback", errors.New("cause")),
		},
		{
			name: "InstallError as NexusError",
			err:  NewInstallError("install", errors.New("cause")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *NexusError
			if !errors.As(tt.err, &target) {
				t.Fatal("errors.As failed: expected NexusError target to match")
			}
			if target.Code != tt.err.Code {
				t.Errorf("errors.As recovered Code = %q, want %q", target.Code, tt.err.Code)
			}
			if target.Stage != tt.err.Stage {
				t.Errorf("errors.As recovered Stage = %q, want %q", target.Stage, tt.err.Stage)
			}
		})
	}
}

func TestNexusError_ErrorsIs_WithWrappedCause(t *testing.T) {
	baseErr := errors.New("base failure")

	tests := []struct {
		name string
		err  *NexusError
	}{
		{
			name: "PreflightError wraps base",
			err:  NewPreflightError("preflight", baseErr),
		},
		{
			name: "FoundationError wraps base",
			err:  NewFoundationError("foundation", baseErr),
		},
		{
			name: "VerifyError wraps base",
			err:  NewVerifyError("verify", baseErr),
		},
		{
			name: "RollbackError wraps base",
			err:  NewRollbackError("rollback", baseErr),
		},
		{
			name: "InstallError wraps base",
			err:  NewInstallError("install", baseErr),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.err, baseErr) {
				t.Error("errors.Is failed: expected wrapped cause to match baseErr")
			}
		})
	}
}

func TestNexusError_ErrorsIs_NilCauseNotMatching(t *testing.T) {
	err := NewVerifyError("verify", nil)
	if errors.Is(err, errors.New("something")) {
		t.Error("errors.Is should not match unrelated error when cause is nil")
	}
}

func TestNexusError_ErrorsAs_ThroughFmtErrorf(t *testing.T) {
	original := NewRollbackError("rollback", errors.New("snapshot missing"))
	wrapped := fmt.Errorf("installer aborted: %w", original)

	var target *NexusError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed through fmt.Errorf wrap: expected NexusError target to match")
	}
	if target.Code != "ROLLBACK_FAIL" {
		t.Errorf("unwrapped Code = %q, want %q", target.Code, "ROLLBACK_FAIL")
	}
	if target.Stage != "Rollback" {
		t.Errorf("unwrapped Stage = %q, want %q", target.Stage, "Rollback")
	}
}

func TestNexusError_ErrorsIs_ThroughFmtErrorf(t *testing.T) {
	cause := errors.New("network timeout")
	original := NewInstallError("install", cause)
	wrapped := fmt.Errorf("step failed: %w", original)

	if !errors.Is(wrapped, cause) {
		t.Error("errors.Is failed through double wrap: expected cause to be discoverable")
	}
}

// --- Edge case tests ---

func TestNexusError_Error_EmptyMessage(t *testing.T) {
	err := &NexusError{
		Code:    "INSTALL_FAIL",
		Message: "",
		Stage:   "Install",
		Cause:   nil,
	}

	got := err.Error()
	want := "[INSTALL_FAIL]  (stage=Install)"
	if got != want {
		t.Errorf("Error() with empty message = %q, want %q", got, want)
	}
}

func TestNexusError_NilReceiver_PanicSafe(t *testing.T) {
	// Calling methods on a nil *NexusError would panic in Go if not
	// guarded. This test documents the current behavior. If the
	// implementation adds nil-receiver guards, update accordingly.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("nil receiver caused panic (expected for current impl): %v", r)
		}
	}()

	var err *NexusError = nil
	_ = err.Error()
	_ = err.Unwrap()
}

// --- Unwrap chain test ---

func TestNexusError_UnwrapChain(t *testing.T) {
	inner := errors.New("innermost")
	middle := NewVerifyError("verify", inner)
	outer := NewRollbackError("rollback", middle)

	// Walk the chain via errors.Is.
	if !errors.Is(outer, inner) {
		t.Error("errors.Is failed: outer should reach innermost error through chain")
	}

	// Walk the chain via errors.As.
	var ne *NexusError
	if !errors.As(outer, &ne) {
		t.Fatal("errors.As failed: outer should match NexusError")
	}
	// The first NexusError found should be the outer one.
	if ne.Code != "ROLLBACK_FAIL" {
		t.Errorf("errors.As returned Code = %q, want %q (outermost NexusError)", ne.Code, "ROLLBACK_FAIL")
	}

	// Verify we can also reach the inner NexusError through Unwrap.
	unwrapped := errors.Unwrap(outer)
	if unwrapped == nil {
		t.Fatal("Unwrap(outer) returned nil")
	}
	var innerNE *NexusError
	if !errors.As(unwrapped, &innerNE) {
		t.Fatal("errors.As failed on unwrapped: expected NexusError")
	}
	if innerNE.Code != "VERIFY_FAIL" {
		t.Errorf("inner NexusError Code = %q, want %q", innerNE.Code, "VERIFY_FAIL")
	}
}
