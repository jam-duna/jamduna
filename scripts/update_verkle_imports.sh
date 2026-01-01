#!/bin/bash
# Script to update import paths from storage to builder/evm/verkle

echo "Updating Verkle import paths..."

# Find and replace import statements
find . -name "*.go" -type f -exec sed -i.bak \
    's|github.com/colorfulnotion/jam/storage|github.com/colorfulnotion/jam/builder/evm/verkle|g' {} \;

# Find and replace package references (in case there are direct references)
find . -name "*.go" -type f -exec sed -i.bak \
    's|import ".*storage"|import evmverkle "github.com/colorfulnotion/jam/builder/evm/verkle"|g' {} \;

# Clean up backup files
find . -name "*.go.bak" -delete

echo "Import path updates completed!"
echo "Files updated:"
grep -r "builder/evm/verkle" . --include="*.go" | wc -l