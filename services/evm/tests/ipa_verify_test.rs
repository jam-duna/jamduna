use hex_literal::hex;

use ark_ff::{PrimeField, BigInteger};
use evm_service::verkle::{
    BandersnatchPoint, Fr, IPAConfig, IPAProof, Transcript, IPA_VECTOR_LENGTH, verify_ipa_proof,
};

const PROOF_HEX: &str = "273395a8febdaed38e94c3d874e99c911a47dd84616d54c55021d5c4131b507e46a4ec2c7e82b77ec2f533994c91ca7edaef212c666a1169b29c323eabb0cf690e0146638d0e2d543f81da4bd597bf3013e1663f340a8f87b845495598d0a3951590b6417f868edaeb3424ff174901d1185a53a3ee127fb7be0af42dda44bf992885bde279ef821a298087717ef3f2b78b2ede7f5d2ea1b60a4195de86a530eb247fd7e456012ae9a070c61635e55d1b7a340dfab8dae991d6273d099d9552815434cc1ba7bcdae341cf7928c6f25102370bdf4b26aad3af654d9dff4b3735661db3177342de5aad774a59d3e1b12754aee641d5f9cd1ecd2751471b308d2d8410add1c9fcc5a2b7371259f0538270832a98d18151f653efbc60895fab8be9650510449081626b5cd24671d1a3253487d44f589c2ff0da3557e307e520cf4e0054bbf8bdffaa24b7e4cce5092ccae5a08281ee24758374f4e65f126cacce64051905b5e2038060ad399c69ca6cb1d596d7c9cb5e161c7dcddc1a7ad62660dd4a5f69b31229b80e6b3df520714e4ea2b5896ebd48d14c7455e91c1ecf4acc5ffb36937c49413b7d1005dd6efbd526f5af5d61131ca3fcdae1218ce81c75e62b39100ec7f474b48a2bee6cef453fa1bc3db95c7c6575bc2d5927cbf7413181ac905766a4038a7b422a8ef2bf7b5059b5c546c19a33c1049482b9a9093f864913ca82290decf6e9a65bf3f66bc3ba4a8ed17b56d890a83bcbe74435a42499dec115";

fn test_poly() -> Vec<Fr> {
    let mut poly = Vec::with_capacity(IPA_VECTOR_LENGTH);
    // Eight chunks of 1..=32 (matches go-ipa TestIPAConsistencySimpleProof)
    for _ in 0..8 {
        for v in 1u64..=32 {
            poly.push(Fr::from(v));
        }
    }
    poly
}

#[test]
fn ipa_verification_matches_go_vector() {
    let config = IPAConfig::new();
    let poly = test_poly();

    // Commitment should match go-ipa test vector
    let commitment = BandersnatchPoint::msm(&config.srs, &poly);
    assert_eq!(
        commitment.to_bytes(),
        hex!("1b9dff8f5ebbac250d291dfe90e36283a227c64b113c37f1bfb9e7a743cdb128")
    );

    let eval_point = Fr::from(2101u64);
    let result = Fr::from_le_bytes_mod_order(&hex!(
        "4a353e70b03c89f161de002e8713beec0d740a5e20722fd5bd68b30540a33208"
    ));

    let proof_bytes = hex::decode(PROOF_HEX).expect("valid hex proof");
    let proof = IPAProof::deserialize(&proof_bytes).expect("proof deserializes");

    let mut transcript = Transcript::new(b"test");
    let ok = verify_ipa_proof(
        &mut transcript,
        &config,
        &commitment,
        &proof,
        eval_point,
        result,
    )
    .expect("verifier runs");
    assert!(ok, "IPA proof must verify");

    // Transcript state should match go-ipa (final squeeze with label \"state\")
    let challenge = transcript.challenge_scalar(b"state");
    assert_eq!(
        challenge.into_bigint().to_bytes_le(),
        hex!("0a81881cbfd7d7197a54ebd67ed6a68b5867f3c783706675b34ece43e85e7306")
            .to_vec()
    );
}
