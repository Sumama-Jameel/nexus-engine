# Contributing to Nexus Protocol

First off, thank you for considering a contribution to Nexus Protocol. This project thrives because of people like you.

This document provides guidelines for contributing to the Nexus Engine. Please read it carefully before submitting issues or pull requests.

---

## Code of Conduct

This project and everyone participating in it is governed by the [Contributor Covenant v2.1 Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to [sumamajamil2005@gmail.com](mailto:sumamajamil2005@gmail.com).

---

## Table of Contents

- [License](#license)
- [Bug Reports](#bug-reports)
- [Feature Suggestions](#feature-suggestions)
- [Development Setup](#development-setup)
- [Code Style](#code-style)
- [Pull Request Process](#pull-request-process)
- [Commit Format](#commit-format)
- [Developer Certificate of Origin](#developer-certificate-of-origin)
- [AI-Assisted Development](#ai-assisted-development)
- [Security Issues](#security-issues)

---

## License

Nexus Protocol Engine is licensed under the [Apache License 2.0](LICENSE). By contributing to this project, you agree that your contributions will be licensed under the same license.

All source files must include the Apache 2.0 license header at the top:

```go
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
```

Run `./scripts/check-license-headers.sh` to verify all files have the correct header.

---

## Bug Reports

We use GitHub Issues to track bugs. Before opening a new issue:

1. **Search existing issues** to avoid duplicates.
2. **Use the bug report issue template** — it ensures we get the information we need.
3. **Include the following** in your report:
   - **Nexus version**: Output of `nexus version`
   - **Operating system**: Name, version, and architecture (e.g., Ubuntu 24.04 amd64, Windows 11 arm64)
   - **Package manager**: apt, pacman, dnf, or apk (if applicable)
   - **Steps to reproduce**: Exact commands you ran
   - **Expected behavior**: What you expected to happen
   - **Actual behavior**: What actually happened, including full error output
   - **Configuration**: Contents of `~/.nexus/state.json` or relevant profile YAML (redact any sensitive values)

When possible, include `--json` output for structured error information:

```bash
nexus <command> --json
```

---

## Feature Suggestions

We welcome feature ideas, but please follow this process:

1. **Start a discussion** — Open a GitHub Discussion in the "Ideas" category before filing an issue. This lets us gauge interest and align on approach before you invest time in implementation.
2. **Write a clear proposal** — Describe the problem, the proposed solution, and any alternatives considered.
3. **File an issue** — After discussion reaches consensus, create a feature request issue referencing the discussion.

This process prevents duplicated effort and ensures features align with the project's architecture and security model.

---

## Development Setup

### Prerequisites

- **Go 1.23+** — [Download Go](https://go.dev/dl/)
- **Make** — For build automation (optional but recommended)
- **golangci-lint** — For advanced linting (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)

### Clone and Build

```bash
git clone https://github.com/Sumama-Jameel/nexus-engine.git
cd nexus-engine

# Build for current platform
make build

# Verify it works
./nexus version
```

### Run Tests

```bash
# Run all tests
make test

# Run with race detector
make test-race

# Generate coverage report
make test-coverage
```

### Cross-Compile

```bash
# All platforms
make build-all

# Specific platform
make build-linux          # Linux amd64
make build-linux-arm64    # Linux arm64
make build-windows        # Windows amd64
make build-windows-arm64  # Windows arm64
```

### Project Structure

Key packages to understand:

| Package | Purpose |
|---|---|
| `cmd/nexus/` | CLI entry point — Cobra commands (thin wiring layer) |
| `cmd/nexus/runner/` | Business logic — Humble Object pattern, 8 command methods with DI |
| `internal/engine/` | Core engine — probe, execute (Zero-Trust gate), configure, state, audit |
| `internal/bridge/` | OS detection — Windows/Linux environment detection, WSL2 readiness |
| `internal/installer/` | Package management — PackageManager interface, 4 implementations, Orchestrator |
| `internal/wsl/` | WSL2 import — rootfs registry, SSRF-safe downloader, 7-step importer |
| `pkg/manifest/` | Declarative profiles — YAML parsing, JSON Schema validation, profile store |

> **Important**: `internal/` packages are not importable by external projects. If you need to expose functionality, place it in `pkg/`.

---

## Code Style

### Mandatory

- **`go fmt`** — All code must be formatted with `go fmt`. No exceptions.
- **`go vet`** — All code must pass `go vet ./...` with zero warnings.
- **GoDoc comments** — Every exported function, type, constant, and variable must have a doc comment beginning with its name.

### Recommended

- **`golangci-lint`** — Run `golangci-lint run` before submitting. The project uses its recommended config.
- **`goimports`** — Use `goimports -w .` to manage import ordering.
- **Error wrapping** — Use `fmt.Errorf("context: %w", err)` for error chains. Never discard errors silently.
- **No `panic`** — Return errors. Panics are only acceptable in truly unrecoverable situations (and those are rare).

### Security-Aware Coding

Nexus operates on a Zero-Trust execution model. When contributing code:

- **Never use `exec.Command` directly** — Route all command execution through `SanitizeAndExecute` or the injected `ExecFunc` parameter.
- **Validate all user input** — Profile names, distro names, file paths, and URLs must pass validation before use.
- **No shell metacharacters** — Never pass user-controlled strings to shell commands without sanitization.
- **Atomic file writes** — Write to a temp file, then rename. Use the pattern established in `state.go` and `store.go`.
- **Constant-time comparison** — Use `crypto/subtle.ConstantTimeCompare` for integrity checks (see `downloader.go`).

---

## Pull Request Process

### Workflow

1. **Fork** the repository
2. **Create a branch** from `main`:
   ```bash
   git checkout -b feat/my-feature
   # or: fix/issue-123, docs/update-readme, test/add-coverage
   ```
3. **Make your changes** — Write code and tests
4. **Test** — Run `make test` (all tests must pass)
5. **Lint** — Run `make check` (format, vet, and test)
6. **Commit** — Use [Conventional Commits](#commit-format)
7. **Push** — Push your branch to your fork
8. **Submit** — Open a Pull Request against `main`

### PR Requirements

- [ ] All tests pass (`make test`)
- [ ] Code is formatted (`go fmt`) and vetted (`go vet`)
- [ ] New code has GoDoc comments on all exported symbols
- [ ] New functionality includes tests
- [ ] No `exec.Command` calls (use `SanitizeAndExecute` or `ExecFunc`)
- [ ] Commit messages follow [Conventional Commits](#commit-format)
- [ ] PR description references any related issues (`Fixes #123`, `Closes #456`)
- [ ] All new source files include the [Apache 2.0 license header](#license)
- [ ] All commits are signed off per the [DCO](#developer-certificate-of-origin)

### Review Process

- A maintainer will review your PR within 5 business days.
- Address review feedback by pushing additional commits (do not force-push during review).
- Once approved, a maintainer will merge your PR.

---

## Commit Format

This project uses [Conventional Commits](https://www.conventionalcommits.org/). Each commit message must follow this format:

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

| Type | Description |
|---|---|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `test` | Adding or updating tests |
| `refactor` | Code refactoring (no feature or fix) |
| `perf` | Performance improvement |
| `chore` | Build, CI, tooling, dependencies |
| `style` | Formatting, whitespace (no logic change) |
| `ci` | CI/CD configuration |
| `revert` | Revert a previous commit |

### Scopes

| Scope | Package |
|---|---|
| `engine` | `internal/engine/` |
| `bridge` | `internal/bridge/` |
| `installer` | `internal/installer/` |
| `wsl` | `internal/wsl/` |
| `manifest` | `pkg/manifest/` |
| `cli` | `cmd/nexus/` |
| `build` | Makefile, GoReleaser, CI |

### Examples

```
feat(installer): add zypper package manager support
fix(wsl): resolve Alpine adduser compatibility in setup script
docs(cli): add examples to nexus profile validate help text
test(engine): add coverage for SanitizeAndExecute edge cases
chore(build): update GoReleaser to v2 configuration
```

---

## Developer Certificate of Origin

This project uses the [Developer Certificate of Origin (DCO)](https://developercertificate.org/) to ensure that contributors have the right to submit their work under the Apache License 2.0.

By making a contribution to this project, you certify that:

> Developer Certificate of Origin
> Version 1.1
>
> Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
> 1 Letterman Drive
> Suite D4700
> San Francisco, CA, 94129
>
> Everyone is permitted to copy and distribute verbatim copies of this
> license document, but changing it is not allowed.
>
> Developer's Certificate of Origin 1.1
>
> By making a contribution to this project, I certify that:
>
> (a) The contribution was created in whole or in part by me and I
>     have the right to submit it under the open source license
>     indicated in the file; or
>
> (b) The contribution is based upon previous work that, to the best
>     of my knowledge, is covered under an appropriate open source
>     license and I have the right under that license to submit that
>     work with modifications, whether created in whole or in part
>     by me, under the same open source license (unless I am
>     permitted to submit under a different license), as indicated
>     in the file; or
>
> (c) The contribution was provided directly to me by some other
>     person who certified (a), (b) or (c) and I have not modified
>     it.
>
> (d) I understand and agree that this project and the contribution
>     are public and that a record of the contribution (including all
>     personal information I submit with it, including my sign-off) is
>     maintained indefinitely and may be redistributed consistent with
>     this project or the open source license(s) involved.

### How to Sign Off

Add a `Signed-off-by` line to every commit:

```bash
git commit -s -m "feat(installer): add zypper package manager support"
```

This automatically adds:

```
Signed-off-by: Your Name <your.email@example.com>
```

You can also add it manually to the commit message footer.

> **Important**: Your sign-off must use your real name (not a pseudonym) and a valid email address.

---

## Security Issues

**Do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability, please report it privately via [SECURITY.md](SECURITY.md). The project maintains a responsible disclosure process with a 48-hour acknowledgment target and a 90-day disclosure timeline.

Security issues include (but are not limited to):
- Command injection bypasses in `SanitizeAndExecute`
- SSRF protection bypasses
- Path traversal vulnerabilities
- Privilege escalation vectors

See [SECURITY.md](SECURITY.md) for the complete security policy and reporting instructions.

---

## AI-Assisted Development

This project was built with AI assistance. All AI-generated code was reviewed, tested, and approved by human maintainers. Contributors are welcome to use AI tools — just ensure you review and understand every line you submit, per the DCO sign-off requirement.

---

Thank you for contributing to Nexus Protocol. Every issue report, bug fix, feature, and documentation improvement makes this project better for everyone.
