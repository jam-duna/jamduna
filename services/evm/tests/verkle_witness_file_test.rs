use std::env;
use std::fs;

use evm_service::verkle_proof::{verify_verkle_witness_section, WitnessSection};

#[test]
fn verify_witness_section_from_file() {
    let path = match env::var("JAM_VERKLE_WITNESS_FILE") {
        Ok(value) => value,
        Err(_) => return,
    };
    let section = env::var("JAM_VERKLE_WITNESS_SECTION").unwrap_or_else(|_| "pre".to_string());
    let witness_data = fs::read(&path).expect("failed to read witness file");

    match section.as_str() {
        "pre" => {
            verify_verkle_witness_section(&witness_data, WitnessSection::Pre)
                .expect("pre-state verification failed");
        }
        "post" => {
            verify_verkle_witness_section(&witness_data, WitnessSection::Post)
                .expect("post-state verification failed");
        }
        "both" => {
            verify_verkle_witness_section(&witness_data, WitnessSection::Pre)
                .expect("pre-state verification failed");
            verify_verkle_witness_section(&witness_data, WitnessSection::Post)
                .expect("post-state verification failed");
        }
        _ => panic!("invalid JAM_VERKLE_WITNESS_SECTION (use pre|post|both)"),
    }
}
