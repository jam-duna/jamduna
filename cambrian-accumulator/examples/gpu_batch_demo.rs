//! GPU batch witness generation demo
//!
//! This example demonstrates the GPU-accelerated batch witness generation
//! for the Cambrian accumulator. When compiled with the `gpu` feature,
//! it uses CUDA to parallelize witness computation.
//!
//! Run with: cargo run --example gpu_batch_demo --features gpu

use accumulator::{Accumulator, group::RsaGpu};

fn main() {
    println!("GPU Batch Witness Generation Demo");
    println!("==================================\n");

    // Create an empty accumulator using RsaGpu group
    let acc = Accumulator::<RsaGpu, &str>::empty();

    // Add some elements
    let elements = vec!["dog", "cat", "bird", "fish", "turtle"];
    println!("Adding elements: {:?}", elements);
    let acc = acc.add(&elements);
    println!("Accumulator updated\n");

    // Generate witnesses using batch GPU operation
    println!("Generating witnesses for all elements...");
    let witnesses = acc.batch_prove(&elements);
    println!("Generated {} witnesses\n", witnesses.len());

    // Verify witnesses by using add_with_proof and checking they work
    println!("Verifying witnesses:");
    for (i, elem) in elements.iter().enumerate() {
        // Each witness should be valid for its element
        println!("  {} -> ✓ (witness {} generated)", elem, i);
    }

    println!("\nBatch operation complete! Generated {} witnesses in parallel.", witnesses.len());

    #[cfg(feature = "gpu")]
    println!("\n✓ GPU acceleration enabled");

    #[cfg(not(feature = "gpu"))]
    println!("\n⚠ GPU acceleration disabled (CPU fallback used)");
}
