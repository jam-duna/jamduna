#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# --- Configuration ---
# Set the data source and project path environment variables
# Note: JAM_PATH should be set to the root of your 'jam' project checkout.
export DATADIR="/tmp/michael/jam"
export JAM_PATH="/Users/michael/Github/jam"

# --- Interactive JOBID Input ---
# Prompt the user to enter the JOBID
read -p "Please enter the JOBID: " JOBID

# Validate that the user entered a JOBID
if [ -z "${JOBID}" ]; then
  echo "Error: No JOBID entered. Exiting."
  exit 1
fi
# Export the JOBID so it's available to sub-processes if needed
export JOBID
echo "Using JOBID: ${JOBID}"
# --- End Configuration ---

# --- Script ---

# Define the full destination paths using the JAM_PATH variable
STATE_TRANSITIONS_DEST_DIR="${JAM_PATH}/cmd/importblocks/rawdata/assurances/state_transitions"
BUNDLE_SNAPSHOTS_DEST_DIR="${JAM_PATH}/cmd/importblocks/rawdata/assurances/bundle_snapshots"

# Create an array of all destination directories that need to be prepared.
# This makes it easy to add more directories in the future.
DEST_DIRS=(
  "${STATE_TRANSITIONS_DEST_DIR}"
  "${BUNDLE_SNAPSHOTS_DEST_DIR}"
)

# Loop through and prepare each destination directory
for dir in "${DEST_DIRS[@]}"; do
  echo "--- Preparing Destination: ${dir} ---"

  # Check if the destination directory exists. If not, create it.
  if [ ! -d "${dir}" ]; then
    echo "Destination directory does not exist. Creating it now..."
    mkdir -p "${dir}"
    echo "Directory created."
  else
    # If the directory already exists, clean its contents.
    echo "Cleaning destination directory..."
    # Using 'find' is a safe way to delete all contents (files and subdirectories)
    # without deleting the directory itself.
    find "${dir}" -mindepth 1 -delete
    echo "Directory cleaned."
  fi
done

# Loop through the nodes and copy files to their respective destinations
echo "--- Starting File Copy Process ---"
for i in {0..5}; do
  echo "--- Processing Node ${i} ---"

  # --- Handle state_transitions copy ---
  SOURCE_DIR_ST="${DATADIR}/${JOBID}/node${i}/data/state_transitions/"
  
  # Check if the source directory for state_transitions exists before trying to copy
  if [ -d "${SOURCE_DIR_ST}" ]; then
    # Check if the source directory is not empty to avoid 'cp' errors with globbing
    if [ -n "$(ls -A "${SOURCE_DIR_ST}" 2>/dev/null)" ]; then
        echo "Copying state_transitions files from node${i}..."
        # Copy all files from the source to the destination
        cp -f "${SOURCE_DIR_ST}"* "${STATE_TRANSITIONS_DEST_DIR}"
    else
        echo "Warning: Source directory ${SOURCE_DIR_ST} is empty. Skipping."
    fi
  else
    echo "Warning: Source directory ${SOURCE_DIR_ST} not found. Skipping."
  fi

  # --- Handle bundle_snapshots copy ---
  SOURCE_DIR_BS="${DATADIR}/${JOBID}/node${i}/data/bundle_snapshots/"

  # Check if the source directory for bundle_snapshots exists before trying to copy
  if [ -d "${SOURCE_DIR_BS}" ]; then
    # Check if the source directory is not empty
    if [ -n "$(ls -A "${SOURCE_DIR_BS}" 2>/dev/null)" ]; then
        echo "Copying bundle_snapshots files from node${i}..."
        # Copy all files from the source to the destination
        cp -f "${SOURCE_DIR_BS}"* "${BUNDLE_SNAPSHOTS_DEST_DIR}"
    else
        echo "Warning: Source directory ${SOURCE_DIR_BS} is empty. Skipping."
    fi
  else
    echo "Warning: Source directory ${SOURCE_DIR_BS} not found. Skipping."
  fi
done

echo "Script finished successfully."
echo ""
echo "--- Data Locations ---"
echo "State transitions data is in: ${STATE_TRANSITIONS_DEST_DIR}"
echo "Bundle snapshots data is in:  ${BUNDLE_SNAPSHOTS_DEST_DIR}"
