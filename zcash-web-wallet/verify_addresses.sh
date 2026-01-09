#!/bin/bash

cd /Users/michael/Desktop/jam/zcash-web-wallet

echo "=== Rust Implementation ==="
cargo run --release --example derive_address

echo ""
echo "=== Go Implementation ==="
go run derive_address.go

echo ""
echo "=== Verification ==="
echo "Comparing outputs..."

rust_output=$(cargo run --release --example derive_address 2>/dev/null)
go_output=$(go run derive_address.go 2>/dev/null)

if [ "$rust_output" == "$go_output" ]; then
    echo "✅ SUCCESS: Rust and Go implementations produce IDENTICAL results"
else
    echo "❌ FAILURE: Outputs differ"
    echo ""
    echo "Rust output:"
    echo "$rust_output"
    echo ""
    echo "Go output:"
    echo "$go_output"
fi
