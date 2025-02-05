#!/bin/bash
set -e

echo "Cleaning up old data.zip..."
rm -f data.zip

# Check if data_temp exists; if it does, remove it.
if [ -d "data_temp" ]; then
    echo "data_temp directory already exists. Removing it..."
    rm -rf data_temp
fi

# Create a temporary directory to hold the data directories.
mkdir data_temp

# Define the list of directories to include.
DATA_DIRS="chainspecs fallback safrole assurances orderedaccumulation"

# Loop over each directory in DATA_DIRS and copy it into data_temp.
for d in $DATA_DIRS; do
    if [ -d "$d" ]; then
        echo "Copying $d into data_temp..."
        cp -r "$d" data_temp/
    else
        echo "Warning: Directory '$d' not found. Skipping."
    fi
done

# Zip the entire temporary folder with maximum compression (-9)
cd data_temp
zip -r -9 ../data.zip .
cd ..
# Remove the temporary folder.
rm -rf data_temp

echo "data.zip created successfully."