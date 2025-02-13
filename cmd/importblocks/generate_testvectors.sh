#!/usr/bin/env bash
set -euo pipefail

GREEN="\033[0;32m"
NC="\033[0m" # No Color

# Usage: run_single_test <make_target_in_node> <cpnode_mode>
run_single_test() {
  local node_target="$1"
  local mode="$2"

  local pipe_file
  pipe_file=$(mktemp -u)
  mkfifo "$pipe_file"

  cleanup() {
    rm -f "$pipe_file"
  }
  trap cleanup EXIT

  echo "----------------------------------------------------------"
  echo "Starting node target: $node_target"
  echo "Will pass mode=$mode to cpnode later."
  echo "----------------------------------------------------------"

  # Change to node directory
  pushd ../../node/ > /dev/null || {
    echo "Error: node directory not found."
    exit 1
  }

  # Start "make $node_target" in the background, piping to tee
  (
    make "$node_target" 2>&1 | tee /dev/tty > "$pipe_file"
  ) &
  local make_pid=$!
  local found_jamdir=""
  local found_jobid=""
  local line

  # Read pipe line by line
  while IFS= read -r line; do
    # Regex to match dataDir=.../jam/1234_abcdef1234/node0
    if [[ "$line" =~ dataDir=(/[^[:space:]]*/jam)/([0-9]+_[a-z0-9]+)/node[0-9]+ ]]; then
      # Capture the first occurrence only
      if [ -z "$found_jamdir" ] || [ -z "$found_jobid" ]; then
        found_jamdir="${BASH_REMATCH[1]}"
        found_jobid="${BASH_REMATCH[2]}"

        # Strip everything before the underscore so "1234_abcdef1234" → "abcdef1234"
        found_jobid="${found_jobid#*_}"

        echo "!!!Captured JAMDIR: $found_jamdir"
        echo "!!!Captured JOBID:  $found_jobid"
      fi
    fi
  done < "$pipe_file"

  wait "$make_pid"
  trap - EXIT  # Clear trap before removing the pipe
  rm -f "$pipe_file"

  # Check we found values
  if [ -z "$found_jamdir" ] || [ -z "$found_jobid" ]; then
    echo "No JAMDIR/JOBID found in output for target '$node_target'."
    exit 1
  fi

  # Return to original directory
  popd > /dev/null || {
    echo "Error: Could not return to original directory."
    exit 1
  }
  echo "Calling cpnode with MODE=$mode, JAMDIR=$found_jamdir, JOBID=$found_jobid..."
  make cpnode JAMDIR="$found_jamdir" MODE="$mode" JOBID="$found_jobid"
}

# List of pairs: "<node_make_target> <cpnode_mode>"
test_pairs=(
  "fib assurances"
  "fallback fallback"
  "safrole safrole"
  "megatron_short orderedaccumulation"
)

processed_modes=""

for pair in "${test_pairs[@]}"; do
  set -- $pair
  node_target=$1
  cpnode_mode=$2

  run_single_test "$node_target" "$cpnode_mode"
  processed_modes="$processed_modes $cpnode_mode"
done

echo "All test vectors generated successfully!"
echo "Processed modes: $processed_modes"