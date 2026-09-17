#!/usr/bin/env bash
#
# Build a macOS installer package (.pkg) for magpie.
#
# It compiles a universal (Intel + Apple Silicon) binary, lays it out at
# /usr/local/bin/magpie, and wraps it in a double-clickable installer that
# shows the MIT license.
#
# Run from the repo root on macOS with Xcode Command Line Tools installed
# (provides lipo, pkgbuild, productbuild). If missing: xcode-select --install
#
#   ./scripts/build-pkg.sh
#   -> dist/magpie-<version>.pkg
#
# Optional signing + notarization (produces a package that installs with no
# Gatekeeper warning). Leave unset for a local/unsigned package.
#   APP_SIGN_ID        "Developer ID Application: Your Name (TEAMID)"   # signs the binary
#   INSTALLER_SIGN_ID  "Developer ID Installer: Your Name (TEAMID)"     # signs the .pkg
#   NOTARY_PROFILE     name of a stored `notarytool` keychain profile
#
# Override defaults if you like:
#   IDENTIFIER=io.github.cgast.magpie   VERSION=1.2.3   ./scripts/build-pkg.sh

set -euo pipefail

APP="magpie"
IDENTIFIER="${IDENTIFIER:-io.github.cgast.magpie}"
MODULE="github.com/cgast/magpie"

# Version: explicit $VERSION, else the git tag, else 0.0.0. Strip a leading "v".
VERSION="${VERSION:-$(git describe --tags --always 2>/dev/null || echo "")}"
VERSION="${VERSION#v}"
VERSION="${VERSION:-0.0.0}"

if [[ ! -f LICENSE || ! -f go.mod ]]; then
  echo "error: run this from the repo root (LICENSE and go.mod must be present)" >&2
  exit 1
fi
for tool in go lipo pkgbuild productbuild; do
  command -v "$tool" >/dev/null 2>&1 || { echo "error: '$tool' not found (need Xcode Command Line Tools)" >&2; exit 1; }
done

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ROOT="$WORK/root"
mkdir -p "$ROOT/usr/local/bin" "$WORK/resources"

LDFLAGS="-s -w -X ${MODULE}/cmd.Version=${VERSION}"

echo "==> building universal binary (v$VERSION)"
GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$WORK/${APP}_arm64" .
GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$WORK/${APP}_amd64" .
lipo -create -output "$ROOT/usr/local/bin/$APP" "$WORK/${APP}_arm64" "$WORK/${APP}_amd64"
chmod 755 "$ROOT/usr/local/bin/$APP"

# Optionally sign the binary itself (required before notarization).
if [[ -n "${APP_SIGN_ID:-}" ]]; then
  echo "==> codesigning binary"
  codesign --force --options runtime --timestamp --sign "$APP_SIGN_ID" "$ROOT/usr/local/bin/$APP"
fi

echo "==> building component package"
COMPONENT="$WORK/${APP}-component.pkg"
pkgbuild \
  --root "$ROOT" \
  --identifier "$IDENTIFIER" \
  --version "$VERSION" \
  --install-location / \
  "$COMPONENT"

# Distribution wrapper: nicer installer UI + license pane.
cp LICENSE "$WORK/resources/LICENSE.txt"
cat > "$WORK/distribution.xml" <<XML
<?xml version="1.0" encoding="utf-8"?>
<installer-gui-script minSpecVersion="1">
    <title>magpie</title>
    <license file="LICENSE.txt"/>
    <options customize="never" require-scripts="false" hostArchitectures="arm64,x86_64"/>
    <choices-outline>
        <line choice="default"/>
    </choices-outline>
    <choice id="default">
        <pkg-ref id="${IDENTIFIER}"/>
    </choice>
    <pkg-ref id="${IDENTIFIER}" version="${VERSION}">${APP}-component.pkg</pkg-ref>
</installer-gui-script>
XML

mkdir -p dist
OUT="dist/${APP}-${VERSION}.pkg"

echo "==> building installer package"
PRODUCT_ARGS=(
  --distribution "$WORK/distribution.xml"
  --package-path "$WORK"
  --resources "$WORK/resources"
)
if [[ -n "${INSTALLER_SIGN_ID:-}" ]]; then
  PRODUCT_ARGS+=(--sign "$INSTALLER_SIGN_ID")
fi
productbuild "${PRODUCT_ARGS[@]}" "$OUT"

# Notarize + staple when signed and a notary profile is provided.
if [[ -n "${INSTALLER_SIGN_ID:-}" && -n "${NOTARY_PROFILE:-}" ]]; then
  echo "==> notarizing (this can take a minute)"
  xcrun notarytool submit "$OUT" --keychain-profile "$NOTARY_PROFILE" --wait
  xcrun stapler staple "$OUT"
fi

echo ""
echo "built $OUT"
if [[ -z "${INSTALLER_SIGN_ID:-}" ]]; then
  echo "note: unsigned — on first open recipients right-click the .pkg > Open,"
  echo "      or install from the terminal: sudo installer -pkg \"$OUT\" -target /"
fi
