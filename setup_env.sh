#!/bin/bash

# Change directory to bandersnatch
cd bandersnatch

# Run cargo build --release
cargo build --release

cd ..

# Set environment variables with relative paths
export CARGO_MANIFEST_DIR=$(pwd)/bandersnatch
export LD_LIBRARY_PATH=$(pwd)/bandersnatch/target/release:$LD_LIBRARY_PATH

# Get the absolute path to the current script's directory
SCRIPT_DIR=$(dirname $(realpath $0))

# Set environment variables with absolute paths
export CARGO_MANIFEST_DIR="$SCRIPT_DIR/bandersnatch"
export LD_LIBRARY_PATH="$SCRIPT_DIR/bandersnatch/target/release:$LD_LIBRARY_PATH"

# Optional: Display the set environment variables
echo "Environment variables set:"
echo "CARGO_MANIFEST_DIR=$CARGO_MANIFEST_DIR"
echo "LD_LIBRARY_PATH=$LD_LIBRARY_PATH"