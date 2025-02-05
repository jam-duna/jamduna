#!/bin/bash
set -e

echo "Cleaning up old core.zip..."
rm -f core.zip

if [ -z "$JAM_PATH" ]; then
    echo "JAM_PATH is not set. Please export JAM_PATH before running this script."
    exit 1
fi

echo "Using JAM_PATH: $JAM_PATH"

# Create a temporary directory to collect only the necessary FFI files.
mkdir -p tmp_ffi

echo "Copying FFI files from JAM_PATH..."
cp "$JAM_PATH/zcash-srs-2-11-uncompressed.bin" tmp_ffi/
cp "$JAM_PATH/bls/target/release/libbls.a" tmp_ffi/
cp "$JAM_PATH/bandersnatch/target/release/libbandersnatch.a" tmp_ffi/

# Rename the temporary directory to "ffi" so that the archive will have a folder named "ffi".
mv tmp_ffi ffi

echo "Creating core.zip with maximum compression..."
zip -r -9 core.zip ffi

# Clean up the temporary folder.
rm -rf ffi

echo "core.zip created."