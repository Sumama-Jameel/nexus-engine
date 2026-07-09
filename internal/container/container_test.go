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

package container

import (
	"context"
	"errors"
	"testing"
)

// fakeExec is a programmable ExecFn for container tests.
type fakeExec struct {
	err error
}

func (f *fakeExec) run(ctx context.Context, cmd string, args ...string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "", nil
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"my-container", true},
		{"my_container", true},
		{"MyContainer", true},
		{"", false},
		{"-bad", false},
		{"bad..name", false},
		{"../path", false},
		{"a", true},
		{"a-b", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateName(tc.name)
			if tc.valid && err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("expected invalid, got nil")
			}
		})
	}
}

func TestValidateImageRef(t *testing.T) {
	tests := []struct {
		ref   string
		valid bool
	}{
		{"fedora:39", true},
		{"ubuntu:22.04", true},
		{"", false},
		{"a", true},
		{"a:", true},
		{":tag", false},
		{"../path", false},
		{"--inject", false},
		{"a:b:c", false},
	}
	for _, tc := range tests {
		t.Run(tc.ref, func(t *testing.T) {
			err := validateImageRef(tc.ref)
			if tc.valid && err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("expected invalid, got nil")
			}
		})
	}
}

func TestDetectFamily(t *testing.T) {
	tests := []struct {
		img    string
		family string
	}{
		{"fedora:39", "fedora"},
		{"ubuntu:22.04", "debian"},
		{"debian:12", "debian"},
		{"archlinux:latest", "arch"},
		{"alpine:3.19", "alpine"},
		{"opensuse/tumbleweed", "suse"},
		{"centos:9", "fedora"},
		{"unknown:latest", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.img, func(t *testing.T) {
			f := detectFamily(tc.img)
			if f != tc.family {
				t.Errorf("detectFamily(%q) = %q, want %q", tc.img, f, tc.family)
			}
		})
	}
}

func TestCreate(t *testing.T) {
	exec := &fakeExec{}
	ctx := context.Background()
	_, err := Create(ctx, CreateDeps{ExecFn: exec.run}, "test", CreateOpts{Image: "fedora:39"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func TestCreateBadName(t *testing.T) {
	_, err := Create(context.Background(), CreateDeps{}, "", CreateOpts{Image: "fedora:39"})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestCreateBadImage(t *testing.T) {
	_, err := Create(context.Background(), CreateDeps{}, "test", CreateOpts{})
	if err == nil {
		t.Fatal("expected error for empty image")
	}
}

func TestCreateExecError(t *testing.T) {
	exec := &fakeExec{err: errors.New("distrobox failed")}
	ctx := context.Background()
	_, err := Create(ctx, CreateDeps{ExecFn: exec.run}, "test", CreateOpts{Image: "fedora:39"})
	if err == nil {
		t.Fatal("expected error from distrobox")
	}
}
