use evm_service::verkle::srs::generate_srs_points;
use hex::encode;

// Ensure our SRS generation matches go-ipa/go-verkle test vectors.
#[test]
fn srs_matches_go_ipa_vectors() {
    let srs = generate_srs_points(256);
    let first_hex = encode(srs[0].to_bytes());
    let last_hex = encode(srs[255].to_bytes());

    assert_eq!(
        first_hex,
        "01587ad1336675eb912550ec2a28eb8923b824b490dd2ba82e48f14590a298a0",
        "first SRS point must match go-ipa test vector"
    );
    assert_eq!(
        last_hex,
        "3de2be346b539395b0c0de56a5ccca54a317f1b5c80107b0802af9a62276a4d8",
        "256th SRS point must match go-ipa test vector"
    );
}
