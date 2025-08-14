
#!/usr/bin/env bash
set -euo pipefail

GREEN="\033[0;32m"
NC="\033[0m" # No Color

# ---------------------------------------
# STEP 1: Generate TestVectors
# ---------------------------------------
echo -e "${GREEN}STEP 1: Generate TestVectors${NC}"
./generate_testvectors.sh

# ---------------------------------------
# STEP 2: Fuzzing
# ---------------------------------------
#echo -e "${GREEN}STEP 2: Fuzzing${NC}"
#make fuzz_only

# ---------------------------------------
# STEP 3: Zipping Data folders & Fuzzed folders
# ---------------------------------------
#echo -e "${GREEN}STEP 3: Zipping 'Data' & 'Fuzzed' folders${NC}"
./data_fuzz_zipper.sh

# ---------------------------------------
# STEP 4: Copying over to jamtestnet
# ---------------------------------------
#echo -e "${GREEN}STEP 4: Copying over to jamtestnet${NC}"
./moving_to_jamtestnet.sh
