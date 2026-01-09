use orchard_service::witness::{StateRead, WitnessBundle};

#[test]
fn test_witness_bundle_serialization() {
    let witness = WitnessBundle {
        pre_state_root: [0x11; 32],
        post_state_root: [0x22; 32],
        reads: vec![
            StateRead {
                key: "read_key_1".to_string(),
                value: vec![0x44; 32],
                merkle_proof: vec![[0u8; 32]],
            },
            StateRead {
                key: "read_key_2".to_string(),
                value: vec![0x55; 32],
                merkle_proof: vec![[0u8; 32]],
            },
        ],
        writes: vec![],
    };

    let serialized = witness.serialize().expect("Serialization should succeed");
    assert!(!serialized.is_empty(), "Serialized data should not be empty");
}
