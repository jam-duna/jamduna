//! NU7 integration test scaffolds (compile-only).

#[test]
#[ignore = "integration test scaffold"]
fn test_issue_and_transfer_lifecycle() {
    todo!("builder → refiner → accumulator integration test");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_two_party_swap() {
    todo!("two-party swap (AAA ↔ BBB) with binding signature validation");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_three_party_swap() {
    todo!("three-party circular swap (AAA→BBB→CCC→AAA)");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_custom_asset_burn() {
    todo!("burn custom asset and validate value balance");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_issue_bundle_only_flow() {
    todo!("IssueBundle-only transaction (no OrchardBundle)");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_mixed_swap_and_issuance() {
    todo!("mixed swap + issuance in single work package");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_reject_action_after_finalize() {
    todo!("reject action after finalize in same IssueBundle");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_split_notes_surplus() {
    todo!("split notes with surplus routing");
}

#[test]
#[ignore = "integration test scaffold"]
fn test_mixed_usdx_and_custom_assets() {
    todo!("mixed USDx + custom assets in same transaction");
}
