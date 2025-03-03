#!/bin/bash
set -e

# Define directories.
RAW_DATA_DIR="rawdata"
FUZZED_DATA_DIR="fuzzed"
DATA_DIR="data"
DATA_TEMP="${DATA_DIR}"

echo "Cleaning up old ${DATA_DIR}.zip..."
rm -f "${DATA_DIR}.zip"

# Remove the temporary directory if it exists.
if [ -d "${DATA_TEMP}" ]; then
    echo "${DATA_TEMP} directory already exists. Removing it..."
    rm -rf "${DATA_TEMP}"
fi

if [ -d "${DATA_DIR}" ]; then
    echo "${DATA_DIR} directory already exists. Removing it..."
    rm -rf "${DATA_DIR}"
fi

# Create a temporary directory to hold the copied subdirectories.
mkdir "${DATA_TEMP}"

# Copy each subdirectory from RAW_DATA_DIR into DATA_TEMP.
for d in "${RAW_DATA_DIR}"/*; do
    if [ -d "$d" ]; then
        dirName=$(basename "$d")
        echo "Copying $dirName into ${DATA_TEMP}..."
        cp -r "$d" "${DATA_TEMP}/"
    else
        echo "Warning: '$d' is not a directory. Skipping."
    fi
done

# Merge fuzzed data: For each subdirectory in FUZZED_DATA_DIR,
# if a matching subdirectory exists in DATA_TEMP, copy its contents into a new folder named "state_transitions_fuzzed".
if [ -d "${FUZZED_DATA_DIR}" ]; then
    for f in "${FUZZED_DATA_DIR}"/*; do
        if [ -d "$f" ]; then
            fuzzName=$(basename "$f")
            if [ -d "${DATA_TEMP}/${fuzzName}" ]; then
                echo "Merging fuzzed data from $fuzzName into ${DATA_TEMP}/${fuzzName}/state_transitions_fuzzed..."
                mkdir -p "${DATA_TEMP}/${fuzzName}/state_transitions_fuzzed"
                cp -r "$f"/* "${DATA_TEMP}/${fuzzName}/state_transitions_fuzzed/"
            else
                echo "No matching directory for fuzzed data: $fuzzName in ${DATA_TEMP}. Skipping."
            fi
        else
            echo "Warning: '$f' is not a directory in ${FUZZED_DATA_DIR}. Skipping."
        fi
    done
else
    echo "Fuzzed directory '${FUZZED_DATA_DIR}' does not exist. Skipping fuzzed data merge."
fi

# Generate plain text versions of .log files (stripping ANSI escape sequences)
echo "Generating plain text versions for grepping..."
find "${DATA_TEMP}" -type f -name "*.log" | while read logfile; do
    txtfile="${logfile%.log}.txt"
    echo "Creating plain text version for $logfile -> $txtfile..."
    sed -r 's/\x1B\[[0-9;]*[mK]//g' "$logfile" > "$txtfile"
done

# Zip the entire temporary folder into data.zip with maximum compression.
cd "${DATA_TEMP}"
zip -r -9 "../${DATA_DIR}.zip" .
cd ..

echo "${DATA_DIR}.zip created successfully."