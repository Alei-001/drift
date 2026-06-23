#!/bin/sh
set -e

# Drift installer for macOS/Linux
# Builds and installs drift to /usr/local/bin (or ~/.local/bin as fallback)

VERSION=0.1.0
INSTALL_DIR=/usr/local/bin

echo "Building drift $VERSION..."

go build -ldflags "-X github.com/drift/drift/internal/cli.version=$VERSION" -o drift ./cmd/drift/

# Try /usr/local/bin first, fall back to ~/.local/bin
if [ -w "$INSTALL_DIR" ]; then
    mv drift "$INSTALL_DIR/drift"
else
    echo "No write access to $INSTALL_DIR, using ~/.local/bin instead"
    INSTALL_DIR="$HOME/.local/bin"
    mkdir -p "$INSTALL_DIR"
    mv drift "$INSTALL_DIR/drift"

    # Add to PATH if not already there
    case ":$PATH:" in
        *":$INSTALL_DIR:"*) ;;
        *)
            echo "Add $INSTALL_DIR to your PATH:"
            echo "  export PATH=\"$INSTALL_DIR:\$PATH\"" >> "$HOME/.profile"
            echo "Added to ~/.profile (restart terminal or run: source ~/.profile)"
            ;;
    esac
fi

echo ""
echo "drift $VERSION installed to $INSTALL_DIR/drift"
echo "Run \"drift version\" to verify."
