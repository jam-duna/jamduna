#!/usr/bin/env bash
set -euo pipefail

GREEN="\033[0;32m"
RED="\033[0;31m"
NC="\033[0m"  # Reset color

# 1) Check JAMTESTNET_DIR is set
if [ -z "${JAMTESTNET_DIR:-}" ]; then
  echo -e "${RED}Error:${NC} Environment variable JAMTESTNET_DIR is not set."
  echo "Please export JAMTESTNET_DIR=/path/to/jamtestnet (must be a git repo)."
  exit 1
fi

# 2) Verify it is a directory and a git repo
if [ ! -d "$JAMTESTNET_DIR" ]; then
  echo -e "${RED}Error:${NC} The directory '$JAMTESTNET_DIR' does not exist. Aborting."
  exit 1
fi

if [ ! -d "$JAMTESTNET_DIR/.git" ]; then
  echo -e "${RED}Error:${NC} '$JAMTESTNET_DIR' is not a git repository (no .git folder)."
  exit 1
fi

# 3) Move into the jamtestnet repo, sync to main, and create a new branch
pushd "$JAMTESTNET_DIR" > /dev/null

echo "Fetching latest changes from origin..."
git fetch origin

echo "Checking out 'main' branch..."
git checkout main

echo "Resetting 'main' to origin/main (hard)..."
git reset --hard origin/main

# Create a new branch (e.g., update-1676062690)
branch_name="update-$(date +%s)"
echo "Creating and switching to new branch '$branch_name'..."
git checkout -b "$branch_name"

# Return to the script directory to do the copying
popd > /dev/null

# 4) Remove existing items in jamtestnet for a clean copy
echo "Removing old data in '$JAMTESTNET_DIR' to ensure a clean copy..."
rm -rf "${JAMTESTNET_DIR}/data.zip"
rm -rf "${JAMTESTNET_DIR}/data" 
rm -rf "${JAMTESTNET_DIR}/fuzzed.zip"
rm -rf "${JAMTESTNET_DIR}/fuzzed"

# 5) Copy the new data over
echo "Copying new data.zip, data/ to '$JAMTESTNET_DIR'..."
cp -rv data.zip data "$JAMTESTNET_DIR" || {
  echo -e "${RED}Warning:${NC} One or more files/folders might be missing. Check manually if needed."
}

# 6) Show final status in the jamtestnet repo
pushd "$JAMTESTNET_DIR" > /dev/null
git status

echo ""
echo -e "${GREEN}Done!${NC} You are now on branch '$branch_name' with fresh data. "
echo "You can review changes, commit, and push them, for example:"
echo "  git add ."
echo "  git commit -m 'Update test vectors'"
echo "  git push --set-upstream origin $branch_name"
popd > /dev/null