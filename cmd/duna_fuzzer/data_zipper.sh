#!/bin/bash
set -e

# Define variables for the target folder and temporary directory.
DATA_DIR="data"
DATA_TEMP="${DATA_DIR}_temp"

echo "Cleaning up old ${DATA_DIR}.zip..."
rm -f "${DATA_DIR}.zip"

# If the temporary directory exists, remove it.
if [ -d "${DATA_TEMP}" ]; then
    echo "${DATA_TEMP} directory already exists. Removing it..."
    rm -rf "${DATA_TEMP}"
fi

# Create a temporary directory to hold the subdirectories.
mkdir "${DATA_TEMP}"

# Loop through each subdirectory in DATA_DIR and copy it to DATA_TEMP.
for d in "${DATA_DIR}"/*; do
    if [ -d "$d" ]; then
        dirName=$(basename "$d")
        echo "Copying $dirName into ${DATA_TEMP}..."
        cp -r "$d" "${DATA_TEMP}/"
    else
        echo "Warning: '$d' is not a directory. Skipping."
    fi
done

# Zip the entire temporary folder with maximum compression (-9)
cd "${DATA_TEMP}"
zip -r -9 "../${DATA_DIR}.zip" .
cd ..
# Remove the temporary directory.
rm -rf "${DATA_TEMP}"

echo "${DATA_DIR}.zip created successfully."