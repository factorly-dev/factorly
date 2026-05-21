#!/usr/bin/env bash
# Rebuild Formula/factorly.rb in factorly-dev/homebrew-tap from the
# template and the per-platform sha256s in build/checksums.txt, then
# commit and push.
#
# Required env:
#   BREW_TAP_TOKEN   PAT with contents:write on factorly-dev/homebrew-tap
#   VERSION          Plain semver, e.g. "0.13.0" (no leading v)
#
# Assumes the release workflow has already produced build/checksums.txt
# (via `make release`) before this step runs.
set -euo pipefail

: "${BREW_TAP_TOKEN:?BREW_TAP_TOKEN env var is required}"
: "${VERSION:?VERSION env var is required}"

CHECKSUMS="build/checksums.txt"
TEMPLATE="brew/factorly.rb.tmpl"
TAP_REPO="factorly-dev/homebrew-tap"

if [ ! -f "$CHECKSUMS" ]; then
    echo "error: $CHECKSUMS not found — run 'make release' first" >&2
    exit 1
fi
if [ ! -f "$TEMPLATE" ]; then
    echo "error: $TEMPLATE not found" >&2
    exit 1
fi

# Pull each sha out of checksums.txt. Layout is:
#   <sha256>  factorly-<version>-<platform>-<arch>[.exe]
sha_for() {
    local suffix="$1"
    local line
    line=$(grep " factorly-${VERSION}-${suffix}$" "$CHECKSUMS" || true)
    if [ -z "$line" ]; then
        echo "error: no checksum for factorly-${VERSION}-${suffix} in $CHECKSUMS" >&2
        exit 1
    fi
    awk '{print $1}' <<< "$line"
}

SHA_DARWIN_ARM64=$(sha_for "darwin-arm64")
SHA_DARWIN_AMD64=$(sha_for "darwin-amd64")
SHA_LINUX_ARM64=$(sha_for "linux-arm64")
SHA_LINUX_AMD64=$(sha_for "linux-amd64")

# Render the formula. Plain sed for portability; no Jinja-style deps.
WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

git clone --depth 1 \
    "https://x-access-token:${BREW_TAP_TOKEN}@github.com/${TAP_REPO}.git" \
    "$WORKDIR/tap"

mkdir -p "$WORKDIR/tap/Formula"

sed \
    -e "s|__VERSION__|${VERSION}|g" \
    -e "s|__SHA_DARWIN_ARM64__|${SHA_DARWIN_ARM64}|g" \
    -e "s|__SHA_DARWIN_AMD64__|${SHA_DARWIN_AMD64}|g" \
    -e "s|__SHA_LINUX_ARM64__|${SHA_LINUX_ARM64}|g" \
    -e "s|__SHA_LINUX_AMD64__|${SHA_LINUX_AMD64}|g" \
    "$TEMPLATE" > "$WORKDIR/tap/Formula/factorly.rb"

cd "$WORKDIR/tap"

# No-op if nothing changed (e.g., re-run for the same version).
if git diff --quiet; then
    echo "Homebrew tap already up to date for v${VERSION}, nothing to push"
    exit 0
fi

git config user.name "factorly-release-bot"
git config user.email "release-bot@factorly-dev.users.noreply.github.com"
git add Formula/factorly.rb
git commit -m "factorly ${VERSION}"
git push origin HEAD

echo "Pushed factorly ${VERSION} formula to ${TAP_REPO}"
