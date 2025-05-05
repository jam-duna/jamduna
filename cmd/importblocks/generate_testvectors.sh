#!/usr/bin/env bash
set -euo pipefail

GREEN="\033[0;32m"
NC="\033[0m"

data_dir="rawdata"
#remove old data
if [ -d "$data_dir" ]; then
  echo -e "${GREEN}Removing old data directory...${NC}"
  rm -rf "$data_dir"
fi
mkdir -p "$data_dir"
echo -e "${GREEN}Data directory created: $data_dir${NC}"
echo -e "${GREEN}Generating test vectors...${NC}"
echo -e "${GREEN}----------------------------------------------------------${NC}"
echo -e "${GREEN}Starting test vector generation...${NC}"
echo -e "${GREEN}----------------------------------------------------------${NC}"

create_temp_log_file() {
  local tmpfile
  if [[ "$(uname)" == "Darwin" ]]; then
    tmpfile=$(mktemp -t output)
  else
    tmpfile=$(mktemp /tmp/output.XXXXXX.txt)
  fi
  echo "Temporary log file created: ${tmpfile}" >&2
  echo "$tmpfile"
}

run_single_test() {
  local node_target="$1"
  local mode="$2"

  local pipe_file
  pipe_file=$(mktemp -u)
  mkfifo "$pipe_file"
  
  local log_file
  log_file=$(create_temp_log_file)
  local filtered_log="${log_file}.filtered"

  cleanup() {
    rm -r rawdata/*
    rm -f "$pipe_file"
    [ -f "$log_file" ] && rm -f "$log_file"
    [ -f "$filtered_log" ] && rm -f "$filtered_log"
  }
  trap cleanup EXIT

  echo "----------------------------------------------------------"
  echo "Starting node target: $node_target"
  echo "Will pass mode=$mode to cpnode later."
  echo "----------------------------------------------------------"

  pushd ../../node/ > /dev/null || { echo "Error: node directory not found."; return 1; }

  ( make "$node_target" 2>&1 | tee "$log_file" | tee /dev/tty > "$pipe_file" ) &
  local make_pid=$!
  local found_jamdir=""
  local full_jobid=""
  local found_jobid=""
  local line

  while IFS= read -r line; do
    if [[ "$line" =~ dataDir=(/[^[:space:]]*/jam)/([0-9]+_[a-z0-9]+)/node[0-9]+ ]]; then
      if [ -z "$found_jamdir" ] || [ -z "$full_jobid" ]; then
        found_jamdir="${BASH_REMATCH[1]}"
        full_jobid="${BASH_REMATCH[2]}"
        found_jobid="${full_jobid#*_}"
        echo "!!!Captured JAMDIR: $found_jamdir"
        echo "!!!Captured JOBID:  $found_jobid"
      fi
    fi
  done < "$pipe_file"

  wait "$make_pid"

  sed -n '/JAMTEST/,$p' "$log_file" | sed -n '/^--- PASS:/q;p' | grep -v "dataDir" > "$filtered_log"

  local dest_dir="${found_jamdir}/${full_jobid}"
  mkdir -p "$dest_dir"
  mv "$filtered_log" "${dest_dir}/${found_jobid}.log"
  echo "!!!Log file saved as: ${dest_dir}/${found_jobid}.log"

  trap - EXIT
  rm -f "$pipe_file"

  if [ -z "$found_jamdir" ] || [ -z "$found_jobid" ]; then
    echo "No JAMDIR/JOBID found in output for target '$node_target'."
    return 1
  fi

  popd > /dev/null || { echo "Error: Could not return to original directory."; return 1; }
  echo "Calling cpnode with MODE=$mode, JAMDIR=$found_jamdir, JOBID=$found_jobid..."
  make cpnode JAMDIR="$found_jamdir" MODE="$mode" JOBID="$found_jobid"
}

test_pairs=(
  "megatron orderedaccumulation"
  "fib2 assurances"
  "game_of_life generic"
)

processed_modes=""

for pair in "${test_pairs[@]}"; do
  set -- $pair
  node_target=$1
  cpnode_mode=$2

  if ! run_single_test "$node_target" "$cpnode_mode"; then
    echo "Test for target '$node_target' failed, continuing to next."
    continue
  fi
  processed_modes="$processed_modes $cpnode_mode"
done
pushd ../make_dispute > /dev/null || { echo "Error: make_dispute directory not found."; exit 1; }
make
popd > /dev/null || { echo "Error: Could not return to original directory."; exit 1; }


echo "All test vectors generated successfully!"
echo "Processed modes: $processed_modes"
