#!/usr/bin/env bash
set -euo pipefail

GREEN="\033[0;32m"
NC="\033[0m" # No Color

# ---------------------------------------
# Argument Parsing
# ---------------------------------------
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "Usage: $0 /path/to/external/data [rawDataVersion]"
  exit 1
fi

# Assign positional parameters.
rawDataDir="$1"
rawDataVersion="${2:-}"

# ---------------------------------------
# STEP 1: Copy Raw Data (with optional version)
# ---------------------------------------
echo -e "${GREEN}STEP 1: Copy Raw Data${NC}"
if [ -n "$rawDataVersion" ]; then
  echo "Using rawDataVersion: ${rawDataVersion}"
  ./copy_rawdata.sh "$rawDataDir" "$rawDataVersion"
else
  ./copy_rawdata.sh "$rawDataDir"
fi

# ---------------------------------------
# STEP 2: Fuzzing
# ---------------------------------------
echo -e "${GREEN}STEP 2: Fuzzing${NC}"
make fuzz_only

# ---------------------------------------
# STEP 3: Zipping 'Data' & 'Fuzzed' folders
# ---------------------------------------
echo -e "${GREEN}STEP 3: Zipping 'Data' & 'Fuzzed' folders${NC}"
./data_fuzz_zipper.sh

# ---------------------------------------
# STEP 4: Copying over to jamtestnet using external script
# ---------------------------------------
if [ -n "$rawDataVersion" ]; then
  echo -e "${GREEN}STEP 4: Copying over to jamtestnet using external script with version '${rawDataVersion}'${NC}"
  ./moving_to_jamtestnet_external.sh "$rawDataVersion"
else
  echo -e "${GREEN}STEP 4: Copying over to jamtestnet using external script (no version provided)${NC}"
  ./moving_to_jamtestnet_external.sh
fi