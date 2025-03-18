#!/usr/bin/env bash
set -euo pipefail

GREEN="\033[0;32m"
RED="\033[0;31m"
NC="\033[0m"  # No Color

# ---------------------------------------
# Argument Parsing
# ---------------------------------------
if [ "$#" -gt 1 ]; then
  echo "Usage: $0 [optional_tag_or_commit]"
  exit 1
fi

optional_tag=""
if [ "$#" -eq 1 ]; then
  optional_tag="$1"
fi

# ---------------------------------------
# 1) Validate JAMTESTNET_DIR
# ---------------------------------------
if [ -z "${JAMTESTNET_DIR:-}" ]; then
  echo -e "${RED}Error:${NC} JAMTESTNET_DIR environment variable is not set."
  echo "Please export JAMTESTNET_DIR=/path/to/jamtestnet (must be a git repo)."
  exit 1
fi

if [ ! -d "$JAMTESTNET_DIR" ]; then
  echo -e "${RED}Error:${NC} The directory '$JAMTESTNET_DIR' does not exist. Aborting."
  exit 1
fi

if [ ! -d "$JAMTESTNET_DIR/.git" ]; then
  echo -e "${RED}Error:${NC} '$JAMTESTNET_DIR' is not a git repository (no .git folder)."
  exit 1
fi

# ---------------------------------------
# 2) Create a new branch using optional tag/commit
# ---------------------------------------
pushd "$JAMTESTNET_DIR" > /dev/null

timestamp=$(date +%s)
if [ -n "$optional_tag" ]; then
  branch_name="update-${timestamp}-${optional_tag}f"
else
  branch_name="update-${timestamp}"
fi

echo "Creating and switching to new branch '$branch_name'..."
git checkout -b "$branch_name"

popd > /dev/null

# ---------------------------------------
# 3) Clean old data and copy new data
# ---------------------------------------
echo "Removing old data in '$JAMTESTNET_DIR' to ensure a clean copy..."
rm -rf "${JAMTESTNET_DIR}/data.zip" "${JAMTESTNET_DIR}/data" "${JAMTESTNET_DIR}/fuzzed.zip" "${JAMTESTNET_DIR}/fuzzed"

echo "Copying new data (data.zip and data/) to '$JAMTESTNET_DIR'..."
cp -rv data.zip data "$JAMTESTNET_DIR" || {
  echo -e "${RED}Warning:${NC} One or more files/folders might be missing. Check manually if needed."
}

# ---------------------------------------
# 4) Display final status
# ---------------------------------------
pushd "$JAMTESTNET_DIR" > /dev/null
git status
popd > /dev/null

echo -e "${GREEN}Done!${NC} You are now on branch '$branch_name' with updated data."