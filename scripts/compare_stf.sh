#!/bin/bash

# Script to compare two StateTransition JSON files
# Usage: ./scripts/compare_stf.sh <file1.json> <file2.json>

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_PATH="$REPO_ROOT/bin/compare_stf"

# Build the tool if it doesn't exist or if the source is newer
if [ ! -f "$BIN_PATH" ] || [ "$SCRIPT_DIR/compare_stf.go" -nt "$BIN_PATH" ]; then
    echo "Building compare_stf tool..."
    cd "$REPO_ROOT"
    go build -o "$BIN_PATH" scripts/compare_stf.go
fi

# Run the comparison
exec "$BIN_PATH" "$@"
