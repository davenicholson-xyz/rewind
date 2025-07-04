#!/usr/bin/env sh

set -e

REPO="davenicholson-xyz/rewind" 
BINARY_NAME="rewind"
VERSION=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Normalize architecture name
case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

EXT="tar.gz"

FILE="${BINARY_NAME}-${OS}-${ARCH}-${VERSION}.${EXT}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILE}"

# Download files
echo "Downloading $FILE..."
curl -LO "$URL"


# Extract and install
if [ "$EXT" = "zip" ]; then
    unzip -o "$FILE"
else
    tar -xzf "$FILE"
fi

chmod +x "$BINARY_NAME"

INSTALL_PATH="/usr/local/bin/$BINARY_NAME"
echo "Installing to $INSTALL_PATH"
sudo mv "$BINARY_NAME" "$INSTALL_PATH"
rm $FILE

echo "$BINARY_NAME installed successfully!"

echo ""
echo "Setting up rewind service..."
if rewind service install; then
    echo "💡 Initialize your first project with: rewind init"
else
    echo "⚠️  Service setup failed - you can set it up later with: rewind service install"
fi
