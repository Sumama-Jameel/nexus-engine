#!/usr/bin/env bash
#
# check-license-headers.sh — Verify all Go source files have the Apache 2.0 license header.
#
# Usage:
#   ./scripts/check-license-headers.sh          # Check all files
#   ./scripts/check-license-headers.sh --fix    # Add missing headers automatically
#
# Exit codes:
#   0 — All files have the correct header
#   1 — One or more files are missing the header

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# The license header pattern — matches the first line of the header block
HEADER_PATTERN="Copyright 2024-2026 Nexus Protocol Contributors"

# The full header block to prepend (used with --fix)
read -r -d '' HEADER_BLOCK << 'EOF' || true
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
EOF

FIX_MODE=false
if [[ "${1:-}" == "--fix" ]]; then
    FIX_MODE=true
fi

missing_count=0
fixed_count=0

cd "${PROJECT_ROOT}"

while IFS= read -r -d '' file; do
    # Check if the file contains the copyright line in the first 15 lines
    # (accounting for potential build tags before the header)
    if head -15 "${file}" | grep -q "${HEADER_PATTERN}"; then
        continue
    fi

    missing_count=$((missing_count + 1))

    if ${FIX_MODE}; then
        # Check if file starts with a build tag
        first_line=$(head -1 "${file}")
        if [[ "${first_line}" =~ ^//go:build ]] || [[ "${first_line}" =~ ^//\ \+build ]]; then
            # Extract build tags (all consecutive comment lines at the top)
            build_tags=""
            in_tags=true
            while IFS= read -r line; do
                if ${in_tags}; then
                    if [[ "${line}" =~ ^//go:build ]] || [[ "${line}" =~ ^//\ \+build ]] || [[ "${line}" =~ ^//[^g] ]]; then
                        build_tags="${build_tags}${line}"$'\n'
                    elif [[ -z "${line}" ]]; then
                        # Blank line after build tags — skip it, we'll add our own
                        :
                    else
                        in_tags=false
                        break
                    fi
                fi
            done < "${file}"

            # Create temp file: build tags + license + blank + rest of file
            {
                printf '%s' "${build_tags}"
                echo ""
                printf '%s\n' "${HEADER_BLOCK}"
                echo ""
                # Skip the build tags at the beginning of the original file
                in_tags=true
                first_content=true
                while IFS= read -r line; do
                    if ${in_tags}; then
                        if [[ "${line}" =~ ^//go:build ]] || [[ "${line}" =~ ^//\ \+build ]]; then
                            continue
                        elif [[ -z "${line}" ]]; then
                            continue
                        else
                            in_tags=false
                        fi
                    fi
                    if ${first_content} && [[ -z "${line}" ]]; then
                        continue
                    fi
                    first_content=false
                    echo "${line}"
                done < "${file}"
            } > "${file}.tmp"
            mv "${file}.tmp" "${file}"
        else
            # No build tag — prepend header directly
            {
                printf '%s\n' "${HEADER_BLOCK}"
                echo ""
                cat "${file}"
            } > "${file}.tmp"
            mv "${file}.tmp" "${file}"
        fi

        fixed_count=$((fixed_count + 1))
        echo "FIXED: ${file}"
    else
        echo "MISSING: ${file}"
    fi
done < <(find . -name "*.go" -not -path "*/vendor/*" -print0)

echo ""
if ${FIX_MODE}; then
    echo "Result: Fixed ${fixed_count} files, ${missing_count} were missing headers."
else
    echo "Result: ${missing_count} file(s) missing Apache 2.0 license header."
    echo "Run './scripts/check-license-headers.sh --fix' to add missing headers."
fi

if [[ ${missing_count} -gt 0 ]] && ! ${FIX_MODE}; then
    exit 1
fi

exit 0
