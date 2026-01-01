use orchard_service::crypto::{commitment, nullifier, pallas_from_bytes_reduced, pallas_to_bytes};
use orchard_halo2_circuits::poseidon::hash::core::{note_commitment_native, nullifier_native};

#[test]
fn test_nullifier_matches_circuits() {
    let sk_spend = [0x11u8; 32];
    let rho = [0x22u8; 32];
    let owner_pk = [0x33u8; 32];
    let note_rseed = [0x44u8; 32];
    let memo_hash = [0x55u8; 32];
    let asset_id = 1u32;
    let amount = 100u128;
    let unlock_height = 0u64;

    println!("computing service commitment");
    let commitment_service =
        commitment(asset_id, amount, &owner_pk, &rho, &note_rseed, unlock_height, &memo_hash)
            .expect("commitment should succeed");
    println!("service commitment computed");
    let nullifier_service =
        nullifier(&sk_spend, &rho, &commitment_service).expect("nullifier should succeed");
    println!("service nullifier computed");

    let rho_fp = pallas_from_bytes_reduced(&rho);
    let note_rseed_fp = pallas_from_bytes_reduced(&note_rseed);
    let memo_hash_fp = pallas_from_bytes_reduced(&memo_hash);
    println!("computing circuit commitment");
    let commitment_circuit_fp = note_commitment_native(
        asset_id,
        amount,
        owner_pk,
        rho_fp,
        note_rseed_fp,
        unlock_height,
        memo_hash_fp,
    );
    println!("circuit commitment computed");
    let commitment_circuit = pallas_to_bytes(&commitment_circuit_fp);
    assert_eq!(
        commitment_service, commitment_circuit,
        "commitment mismatch between service and circuits"
    );

    println!("computing circuit nullifier");
    let sk_fp = pallas_from_bytes_reduced(&sk_spend);
    let nullifier_circuit_fp = nullifier_native(sk_fp, rho_fp, commitment_circuit_fp);
    println!("circuit nullifier computed");
    let nullifier_circuit = pallas_to_bytes(&nullifier_circuit_fp);
    assert_eq!(
        nullifier_service, nullifier_circuit,
        "nullifier mismatch between service and circuits"
    );
}
