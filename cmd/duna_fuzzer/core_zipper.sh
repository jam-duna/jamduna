#!/bin/bash
set -e

echo "Cleaning up old core.zip..."
rm -f core.zip

if [ -z "$JAM_PATH" ]; then
    echo "JAM_PATH is not set. Please export JAM_PATH before running this script."
    exit 1
fi

# Set local variable for the FFI directory.
JAM_FFI_DIR="${JAM_PATH}/ffi"

echo "Using JAM_PATH: $JAM_PATH"
echo "Using JAM_FFI_DIR: $JAM_FFI_DIR"

# Change to the JAM_PATH directory (the repository root).
cd "$JAM_PATH"

# Build/update FFI files.
make ffi_force

# Build the binary.
echo "Building the importblocks binary..."
cd "$JAM_PATH/cmd/importblocks"
make build

# Create a temporary build directory for packaging.
mkdir -p core_build

echo "Copying necessary files into core_build/..."
# Copy the main binary.
cp importblocks core_build/
# Copy the required FFI files from JAM_FFI_DIR.
cp "$JAM_FFI_DIR/zcash-srs-2-11-uncompressed.bin" core_build/
cp "$JAM_FFI_DIR/libbls.a" core_build/
cp "$JAM_FFI_DIR/libbandersnatch.a" core_build/

echo "Creating core.zip with maximum compression..."
cd core_build
zip -r -9 ../core.zip .
cd ..
rm -rf core_build

echo "core.zip created."