#!/usr/bin/env bash
# Copyright 2026 Jordan Sherer <hi@jordansherer.com>
# SPDX-License-Identifier: gpl

set -euo pipefail

# Generate a changelog entry for a version range using the Anthropic API.
# Prepends the entry to CHANGELOG.md.
#
# Usage:
#   ./scripts/changelog.sh v0.6.0..v0.6.1
#   ./scripts/changelog.sh                  # prompts for range

RANGE="${1:-}"
if [ -z "$RANGE" ]; then
    printf "Range (e.g. v0.6.0..v0.6.1): "
    read -r RANGE
    if [ -z "$RANGE" ]; then
        echo "Aborted." >&2
        exit 1
    fi
fi

# Extract the target version from the range (part after ..)
VERSION="${RANGE##*..}"
if [ -z "$VERSION" ]; then
    echo "Could not determine version from range: $RANGE" >&2
    exit 1
fi

DATE=$(date +%Y-%m-%d)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Generating changelog for $VERSION..."

# Get raw commit messages
make changelog RANGE="$RANGE" > "$TMPDIR/changes.txt" 2>/dev/null

if [ ! -s "$TMPDIR/changes.txt" ]; then
    echo "No changes found for range $RANGE" >&2
    exit 1
fi

# Summarize with LLM
factorly -v call anthropic.ask \
    --max_tokens 10240 \
    --system 'You convert git commit messages into a Keep a Changelog formatted entry.

Rules:
- Output ONLY the markdown sections, nothing else
- Group changes under: ### Added, ### Changed, ### Fixed, ### Removed, ### Security, ### Deprecated
- Omit sections that have no entries
- Use bullet points (- ) for each change
- One line per change, concise
- No commit hashes, no commentary, no preamble, no closing remarks

Example output:

### Added
- Per-tool output filter engine with 27 built-in filters
- Hash-chained audit log for tamper-evident logging

### Fixed
- Rate limiter now fails closed on store errors
- Vault password zeroed on all error paths' \
    --prompt @"$TMPDIR/changes.txt" \
    > "$TMPDIR/summary.txt"

if [ ! -s "$TMPDIR/summary.txt" ]; then
    echo "Failed to generate summary" >&2
    exit 1
fi

# Build the new entry
{
    echo "## [$VERSION] - $DATE"
    echo ""
    cat "$TMPDIR/summary.txt"
    echo ""
    echo ""
} > "$TMPDIR/entry.txt"

# Prepend to CHANGELOG.md (or create it)
if [ -f CHANGELOG.md ]; then
    cat "$TMPDIR/entry.txt" CHANGELOG.md > "$TMPDIR/merged.txt"
    mv "$TMPDIR/merged.txt" CHANGELOG.md
else
    {
        echo "# Changelog"
        echo ""
        echo "All notable changes to this project will be documented in this file."
        echo ""
        echo "The format is based on [Keep a Changelog](https://keepachangelog.com/)."
        echo ""
        cat "$TMPDIR/entry.txt"
    } > CHANGELOG.md
fi

echo "Updated CHANGELOG.md with $VERSION"
