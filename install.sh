#!/usr/bin/env sh
#
# install.sh — one-command installer for clyde.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Systemartis/clyde/main/install.sh | sh
#
# Or with options:
#   curl -fsSL ... | INSTALL_DIR=/usr/local/bin VERSION=v0.1.0 sh
#
# What it does:
#   1. Detects your OS + arch.
#   2. Resolves the requested release tag (default: latest).
#   3. Downloads the matching archive, the checksum manifest, and (if cosign
#      is on PATH) the signature + Fulcio cert.
#   4. Verifies cosign signature (if cosign is installed) — refuses to install
#      a release whose signature can't be authenticated to the Systemartis
#      release workflow.
#   5. Verifies sha256 against the manifest.
#   6. Extracts and copies the binary into INSTALL_DIR.
#
# Environment overrides:
#   VERSION       Tag to install. Default: the GitHub "latest" release.
#   INSTALL_DIR   Where to drop the binary. Default: $HOME/.local/bin.
#   GITHUB_REPO   Source repo. Default: Systemartis/clyde.
#   SKIP_COSIGN   "1" disables cosign verify even if cosign is present.
#
# Tested with sh / bash / dash / zsh. POSIX-only — no [[ ]] or $(()).

set -eu

# --- Config -----------------------------------------------------------------

GITHUB_REPO="${GITHUB_REPO:-Systemartis/clyde}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-}"
SKIP_COSIGN="${SKIP_COSIGN:-}"

# --- Helpers ----------------------------------------------------------------

err() { printf >&2 "install.sh: %s\n" "$*"; }
die() { err "$*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"; }

# --- Pre-flight -------------------------------------------------------------

need curl
need tar
need uname
need install
# sha256 verifier — different name on Linux vs macOS.
SHA256=""
if command -v sha256sum >/dev/null 2>&1; then
    SHA256="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHA256="shasum -a 256"
else
    die "missing sha256sum or shasum"
fi

# --- Detect OS / arch -------------------------------------------------------

uname_s="$(uname -s)"
case "$uname_s" in
    Linux)  os="linux" ;;
    Darwin) os="darwin" ;;
    *)      die "unsupported OS: $uname_s (clyde ships linux + darwin only)" ;;
esac

uname_m="$(uname -m)"
case "$uname_m" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) die "unsupported arch: $uname_m" ;;
esac

# --- Resolve version --------------------------------------------------------

if [ -z "$VERSION" ]; then
    # GitHub redirects /releases/latest to /releases/tag/<latest>; -L follows
    # the redirect and -I gives us the headers, where Location: holds the tag.
    VERSION="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
        "https://github.com/${GITHUB_REPO}/releases/latest" \
        | sed 's|.*/tag/||')"
    [ -n "$VERSION" ] || die "could not resolve latest version"
fi
case "$VERSION" in
    v*) ;;
    *)  VERSION="v$VERSION" ;;  # accept "0.1.0" or "v0.1.0"
esac
# strip the leading v for the archive name (`clyde_0.1.0_...`).
ver="${VERSION#v}"

archive="clyde_${ver}_${os}_${arch}.tar.gz"
base="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}"

printf '%s\n' "Installing clyde ${VERSION} (${os}/${arch}) -> ${INSTALL_DIR}"

# --- Download ---------------------------------------------------------------

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

(
    cd "$tmp"
    curl -fsSLO "${base}/${archive}"        || die "could not download ${archive}"
    curl -fsSLO "${base}/checksums.txt"     || die "could not download checksums.txt"

    # Cosign artifacts — best-effort; skip cleanly if they 404 (lets the
    # script work for ad-hoc releases or self-hosted forks without cosign).
    cosign_present=0
    if [ -z "$SKIP_COSIGN" ] && command -v cosign >/dev/null 2>&1; then
        if curl -fsSLO "${base}/checksums.txt.sig" 2>/dev/null \
        && curl -fsSLO "${base}/checksums.txt.pem" 2>/dev/null; then
            cosign_present=1
        else
            err "cosign signatures not found for ${VERSION} — continuing with sha256 only"
        fi
    fi

    # --- Verify -------------------------------------------------------------

    if [ "$cosign_present" = "1" ]; then
        printf '%s\n' "Verifying cosign signature..."
        cosign verify-blob \
            --certificate checksums.txt.pem \
            --signature checksums.txt.sig \
            --certificate-identity-regexp \
                "https://github.com/${GITHUB_REPO}/.github/workflows/release.yml@refs/tags/v.*" \
            --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
            checksums.txt >/dev/null \
            || die "cosign verify failed — refusing to install"
    else
        err "cosign not used — install cosign and re-run for cryptographic verification"
    fi

    printf '%s\n' "Verifying sha256..."
    $SHA256 -c checksums.txt --ignore-missing >/dev/null || die "sha256 verify failed"

    # --- Extract + install --------------------------------------------------

    tar -xzf "$archive" clyde
    [ -x ./clyde ] || die "archive did not contain a 'clyde' binary"

    mkdir -p "$INSTALL_DIR"
    install -m 0755 ./clyde "$INSTALL_DIR/clyde"
)

# --- Smoke + PATH hint ------------------------------------------------------

if [ -x "$INSTALL_DIR/clyde" ]; then
    printf '%s\n' "Installed: $("$INSTALL_DIR/clyde" --version | head -1)"
fi

case ":$PATH:" in
    *:"$INSTALL_DIR":*) ;;
    *)
        printf '\n%s\n' "$INSTALL_DIR is not on \$PATH. Add it:"
        printf '%s\n'   "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
        ;;
esac
