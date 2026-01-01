use evm_service::verkle::*;

#[test]
fn test_print_ipa_config() {
    let config = IPAConfig::new();

    // Expected go-ipa values
    let go_q = "4a2c7486fd924882bf02c6908de395122843e3e05264d7991e18e7985dad51e9";
    let go_srs0 = "01587ad1336675eb912550ec2a28eb8923b824b490dd2ba82e48f14590a298a0";
    let go_srs255 = "3de2be346b539395b0c0de56a5ccca54a317f1b5c80107b0802af9a62276a4d8";

    assert_eq!(
        hex::encode(config.q.to_bytes()),
        go_q,
        "Generator Q must match go-ipa"
    );
    assert_eq!(
        hex::encode(config.srs[0].to_bytes()),
        go_srs0,
        "SRS[0] must match go-ipa"
    );
    assert_eq!(
        hex::encode(config.srs[255].to_bytes()),
        go_srs255,
        "SRS[255] must match go-ipa"
    );
    assert_eq!(config.srs.len(), 256, "SRS length must be 256");
    assert_eq!(config.num_rounds(), 8, "num_rounds must be 8 for 256-length SRS");
}
