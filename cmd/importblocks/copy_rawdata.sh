#!/usr/bin/env bash
set -euo pipefail

# Usage: ./copy_rawdata.sh /path/to/external/data [tag_or_commit]
# If a tag_or_commit is provided and the external data directory is a git repository,
# the script will checkout that version before copying.

# Check for 1 or 2 arguments.
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    echo "Usage: $0 /path/to/external/data [tag_or_commit]"
    exit 1
fi

SRC_DIR="$1"
DEST_DIR="rawdata"
VERSION=${2:-""}

# Verify source directory exists.
if [ ! -d "$SRC_DIR" ]; then
    echo "Error: Source directory '$SRC_DIR' does not exist or is not a directory."
    exit 1
fi

# If a version is provided and SRC_DIR is a git repo, checkout the given version.
if [ -n "$VERSION" ]; then
    if [ -d "$SRC_DIR/.git" ]; then
        echo "Checking out version '$VERSION' in external data repository..."
        pushd "$SRC_DIR" > /dev/null
        git fetch --all
        git checkout "$VERSION"
        popd > /dev/null
    else
        echo "Warning: '$SRC_DIR' is not a git repository. Ignoring version parameter."
    fi
fi

# Prepare destination directory: clear it if it exists, or create it if not.
if [ -d "$DEST_DIR" ]; then
    echo "Cleaning existing '$DEST_DIR' directory..."
    rm -rf "${DEST_DIR:?}"/*
else
    echo "Creating '$DEST_DIR' directory..."
    mkdir -p "$DEST_DIR"
fi

# Copy data from SRC_DIR to DEST_DIR, excluding unwanted folders and files.
echo "Copying data from '$SRC_DIR' to '$DEST_DIR', excluding 'state_transitions_fuzzed' and '.DS_Store' files..."
rsync -av \
      --exclude 'state_transitions_fuzzed/' \
      --exclude '.DS_Store' \
      "$SRC_DIR"/ "$DEST_DIR"/

# Export RAWDATA_VERSION for later use if provided.
if [ -n "$VERSION" ]; then
    export RAWDATA_VERSION="$VERSION"
    echo "Set RAWDATA_VERSION to '$VERSION'"
fi

echo "Data copied successfully."