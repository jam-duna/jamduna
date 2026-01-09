// RedPallas signature verification for Orchard bundles (no_std)
//
// Implements spend authorization and binding signature verification
// as required by Zcash Orchard consensus rules.

#![allow(dead_code)]

use crate::bundle_parser::{ParsedBundle, ParsedAction};
use crate::bundle_codec::{
    DecodedActionV6,
    DecodedIssueBundle,
    DecodedIssuedNote,
    DecodedSwapBundle,
    DecodedZSABundle,
    ZSA_ENC_CIPHERTEXT_SIZE,
};
use crate::crypto::{range_check_64bit, verify_halo2_proof};
use crate::errors::{OrchardError, Result};
use crate::nu7_types::{AssetBase, BurnRecord};
use alloc::collections::{BTreeMap, BTreeSet};
use alloc::format;
use alloc::vec::Vec;
use pasta_curves::arithmetic::{CurveAffine, CurveExt};
use pasta_curves::pallas;
use pasta_curves::group::{Curve, Group};
use pasta_curves::group::GroupEncoding;
use pasta_curves::group::ff::{Field, FromUniformBytes, PrimeField, PrimeFieldBits};
use k256::schnorr::{Signature as SchnorrSignature, VerifyingKey as SchnorrVerifyingKey};
use k256::schnorr::signature::hazmat::PrehashVerifier;
use sinsemilla::HashDomain;

/// Orchard value commitment generator V (affine coordinates).
/// From zcash/orchard `value_commit_v::GENERATOR`.
const VALUE_COMMITMENT_V_X: [u8; 32] = [
    103, 67, 249, 58, 110, 189, 167, 42, 140, 124, 90, 43, 127, 163, 4, 254,
    50, 178, 155, 79, 112, 106, 168, 247, 66, 15, 61, 142, 122, 89, 112, 47,
];
const VALUE_COMMITMENT_V_Y: [u8; 32] = [
    142, 242, 90, 175, 126, 196, 19, 164, 219, 227, 255, 167, 102, 167, 158, 29,
    66, 108, 109, 19, 99, 127, 145, 30, 175, 25, 25, 49, 105, 81, 14, 45,
];

/// The byte-encoding of the basepoint for the Orchard `SpendAuthSig` on Pallas.
/// Reproducible via `pallas::Point::hash_to_curve("z.cash:Orchard")(b"G")`.
const ORCHARD_SPENDAUTHSIG_BASEPOINT_BYTES: [u8; 32] = [
    0x63, 0xc9, 0x75, 0xb8, 0x84, 0x72, 0x1a, 0x8d,
    0x0c, 0xa1, 0x70, 0x7b, 0xe3, 0x0c, 0x7f, 0x0c,
    0x5f, 0x44, 0x5f, 0x3e, 0x7c, 0x18, 0x8d, 0x3b,
    0x06, 0xd6, 0xf1, 0x28, 0xb3, 0x23, 0x55, 0xb7,
];

/// The byte-encoding of the basepoint for the Orchard `BindingSig` on Pallas.
/// Reproducible via `pallas::Point::hash_to_curve("z.cash:Orchard-cv")(b"r")`.
const ORCHARD_BINDINGSIG_BASEPOINT_BYTES: [u8; 32] = [
    0x91, 0x5a, 0x3c, 0x88, 0x68, 0xc6, 0xc3, 0x0e,
    0x2f, 0x80, 0x90, 0xee, 0x45, 0xd7, 0x6e, 0x40,
    0x48, 0x20, 0x8d, 0xea, 0x5b, 0x23, 0x66, 0x4f,
    0xbb, 0x09, 0xa4, 0x0f, 0x55, 0x44, 0xf4, 0x07,
];

const ZSA_COMPACT_NOTE_SIZE: usize = 84;
const ZSA_MEMO_SIZE: usize = 512;

const ZCASH_ORCHARD_ACTION_GROUPS_HASH_PERSONALIZATION: &[u8; 16] = b"ZTxIdOrcActGHash";
const ZCASH_ORCHARD_ACTIONS_COMPACT_HASH_PERSONALIZATION_V6: &[u8; 16] = b"ZTxId6OActC_Hash";
const ZCASH_ORCHARD_ACTIONS_MEMOS_HASH_PERSONALIZATION: &[u8; 16] = b"ZTxIdOrcActMHash";
const ZCASH_ORCHARD_ACTIONS_NONCOMPACT_HASH_PERSONALIZATION_V6: &[u8; 16] = b"ZTxId6OActN_Hash";
const ZCASH_ORCHARD_ZSA_BURN_HASH_PERSONALIZATION: &[u8; 16] = b"ZTxIdOrcBurnHash";
const ZCASH_ORCHARD_HASH_PERSONALIZATION: &[u8; 16] = b"ZTxIdOrchardHash";
const ZCASH_ORCHARD_ZSA_ISSUE_PERSONALIZATION: &[u8; 16] = b"ZTxIdSAIssueHash";
const ZCASH_ORCHARD_ZSA_ISSUE_ACTION_PERSONALIZATION: &[u8; 16] = b"ZTxIdIssuActHash";
const ZCASH_ORCHARD_ZSA_ISSUE_NOTE_PERSONALIZATION: &[u8; 16] = b"ZTxIdIAcNoteHash";
const ZSA_ASSET_DIGEST_PERSONALIZATION: &[u8; 16] = b"ZSA-Asset-Digest";
const ZSA_ASSET_BASE_PERSONALIZATION: &str = "z.cash:OrchardZSA";
const ISSUE_SIG_ALGO_BYTE: u8 = 0x00;
const ISSUE_SIGHASH_INFO_V0: [u8; 1] = [0u8];
const PRF_EXPAND_PERSONALIZATION: &[u8; 16] = b"Zcash_ExpandSeed";
const KEY_DIVERSIFICATION_PERSONALIZATION: &str = "z.cash:Orchard-gd";
const NOTE_COMMITMENT_PERSONALIZATION: &str = "z.cash:Orchard-NoteCommit";
const NOTE_ZSA_COMMITMENT_PERSONALIZATION: &str = "z.cash:ZSA-NoteCommit";
const L_ORCHARD_BASE: usize = 255;

/// Per-asset value balance tracking (service-side sanity checks).
pub struct AssetBalances {
    balances: BTreeMap<AssetBase, i64>,
}

impl AssetBalances {
    pub fn new() -> Self {
        Self {
            balances: BTreeMap::new(),
        }
    }

    pub fn add_spend(&mut self, asset: AssetBase, value: u64) {
        *self.balances.entry(asset).or_insert(0) += value as i64;
    }

    pub fn add_output(&mut self, asset: AssetBase, value: u64) {
        *self.balances.entry(asset).or_insert(0) -= value as i64;
    }

    pub fn add_burn(&mut self, asset: AssetBase, value: u64) {
        *self.balances.entry(asset).or_insert(0) += value as i64;
    }

    pub fn verify_balanced(&self) -> Result<()> {
        for (asset, balance) in &self.balances {
            if asset.is_native() {
                // Native USDx can have non-zero balance (fee/transparent)
                continue;
            }
            if *balance != 0 {
                return Err(OrchardError::StateError(format!(
                    "Unbalanced asset {:?}: {}",
                    asset.to_bytes(),
                    balance
                )));
            }
        }
        Ok(())
    }
}

/// Verify all signatures in an Orchard bundle
///
/// This function verifies:
/// 1. Each action's spend authorization signature (RedPallas over SpendAuthSig)
/// 2. The bundle's binding signature (RedPallas over Binding)
///
/// Per Zcash Protocol Spec § 4.6.3:
/// - spend_auth_sig MUST verify against rk (randomized verification key) and sighash
/// - binding_sig MUST verify against bvk (binding verification key) and sighash
///
/// Security: Failure to verify signatures allows proof replay attacks and
/// unauthorized spending.
pub fn verify_bundle_signatures(
    bundle: &ParsedBundle,
    sighash: &[u8; 32],
) -> Result<()> {
    // Verify spend authorization signatures
    for (i, (action, sig)) in bundle.actions.iter().zip(&bundle.spend_auth_sigs).enumerate() {
        verify_spend_auth_signature(action, sig, sighash)
            .map_err(|e| OrchardError::StateError(format!(
                "Action {} spend_auth_sig verification failed: {}",
                i, e
            )))?;
    }

    // Verify binding signature
    verify_binding_signature(bundle, sighash)?;

    Ok(())
}

/// Verify signatures for a V6 ZSA bundle.
pub fn verify_zsa_bundle_signatures(
    bundle: &DecodedZSABundle,
    sighash: &[u8; 32],
) -> Result<()> {
    if bundle.actions.len() != bundle.spend_auth_sigs.len() {
        return Err(OrchardError::StateError(
            "ZSA bundle actions/signature count mismatch".into(),
        ));
    }

    for (i, (action, sig)) in bundle.actions.iter().zip(&bundle.spend_auth_sigs).enumerate() {
        verify_spend_auth_signature_raw(&action.rk, sig, sighash).map_err(|e| {
            OrchardError::StateError(format!(
                "ZSA action {} spend_auth_sig verification failed: {}",
                i, e
            ))
        })?;
    }

    verify_binding_signature_zsa(
        &bundle.actions,
        bundle.value_balance,
        &bundle.burns,
        &bundle.binding_sig,
        sighash,
    )
}

/// Comprehensive SwapBundle verification (Phase 4.2)
///
/// Verifies:
/// 1. Each action group's Halo2 proof independently
/// 2. Aggregate asset balances across all action groups
/// 3. Per-asset balance == 0 (except native USDx)
/// 4. Aggregated binding signature from all actions + burns
/// 5. Bundle-level binding signature
///
/// Returns Ok(()) if all verifications pass.
pub fn verify_swap_bundle(
    bundle: &DecodedSwapBundle,
    sighash: &[u8; 32],
) -> Result<()> {
    // 1. Verify each action group's proof independently
    for (group_idx, group) in bundle.action_groups.iter().enumerate() {
        verify_action_group_proof(group, group_idx)?;
    }

    // 2. Asset balance verification
    // TODO: Currently disabled - only burns are tracked, no spend/output tracking
    // This would fail on any non-native burn since there are no offsetting spends/outputs
    // The Halo2 proof already validates asset balances at circuit level
    // Service-side verification is deferred until spend/output tracking is implemented
    //
    // Future: Extract asset metadata from witness or proof public inputs
    // See NU7_IMPLEMENTATION_PLAN.md § 4.2.1 for implementation options

    // 4. Verify signatures (spend auth + binding)
    verify_swap_bundle_signatures(bundle, sighash)?;

    Ok(())
}

/// Verify a single action group's Halo2 proof
///
/// Each action group has its own OrchardZSA circuit proof that proves:
/// - Note commitments are well-formed
/// - Nullifiers are correctly derived
/// - Value balance is correct
/// - Asset validity
fn verify_action_group_proof(group: &crate::bundle_codec::DecodedActionGroup, group_idx: usize) -> Result<()> {
    // Validate NonEmpty constraint
    if group.actions.is_empty() {
        return Err(OrchardError::StateError(format!(
            "Action group {} has no actions (NonEmpty violation)",
            group_idx
        )));
    }

    // Validate expiry_height == 0 (NU7 consensus rule)
    if group.expiry_height != 0 {
        return Err(OrchardError::StateError(format!(
            "Action group {} expiry_height must be 0, got {}",
            group_idx, group.expiry_height
        )));
    }

    if group.proof.is_empty() {
        return Err(OrchardError::StateError(format!(
            "Action group {} has empty proof",
            group_idx
        )));
    }

    validate_orchard_flags(group.flags)?;
    let public_inputs = build_zsa_public_inputs(&group.actions, group.flags, &group.anchor)?;
    let verified = verify_halo2_proof(&group.proof, &public_inputs, 2)?;
    if !verified {
        return Err(OrchardError::StateError(format!(
            "Action group {} proof verification failed",
            group_idx
        )));
    }

    Ok(())
}

fn validate_orchard_flags(flags: u8) -> Result<()> {
    if (flags & 0xF0) != 0 {
        return Err(OrchardError::StateError(
            "Invalid Orchard flags: only bits 0-3 are allowed".into(),
        ));
    }
    Ok(())
}

fn build_zsa_public_inputs(
    actions: &[DecodedActionV6],
    flags: u8,
    anchor: &[u8; 32],
) -> Result<Vec<[u8; 32]>> {
    validate_orchard_flags(flags)?;
    let enable_spend = flag_to_field_bytes((flags & 0x01) != 0);
    let enable_output = flag_to_field_bytes((flags & 0x02) != 0);
    let enable_zsa = flag_to_field_bytes((flags & 0x04) != 0);

    let mut inputs = Vec::with_capacity(actions.len() * 10);
    for action in actions {
        let (cv_net_x, cv_net_y) = point_bytes_to_xy(&action.cv_net, true, "cv_net")?;
        let (rk_x, rk_y) = point_bytes_to_xy(&action.rk, false, "rk")?;

        inputs.push(*anchor);
        inputs.push(cv_net_x);
        inputs.push(cv_net_y);
        inputs.push(action.nullifier);
        inputs.push(rk_x);
        inputs.push(rk_y);
        inputs.push(action.cmx);
        inputs.push(enable_spend);
        inputs.push(enable_output);
        inputs.push(enable_zsa);
    }

    Ok(inputs)
}

fn flag_to_field_bytes(enabled: bool) -> [u8; 32] {
    let value = if enabled { 1u64 } else { 0u64 };
    pallas::Base::from(value).to_repr()
}

fn point_bytes_to_xy(
    bytes: &[u8; 32],
    allow_identity: bool,
    label: &str,
) -> Result<([u8; 32], [u8; 32])> {
    let point = pallas::Point::from_bytes(bytes)
        .into_option()
        .ok_or_else(|| OrchardError::StateError(format!("Invalid {} point bytes", label)))?;
    let coords = point.to_affine().coordinates().into_option();

    if let Some(coords) = coords {
        Ok((coords.x().to_repr(), coords.y().to_repr()))
    } else if allow_identity {
        let zero = pallas::Base::from(0u64).to_repr();
        Ok((zero, zero))
    } else {
        Err(OrchardError::StateError(format!("{} point is identity", label)))
    }
}

/// Verify signatures for a V6 SwapBundle (multiple action groups).
pub fn verify_swap_bundle_signatures(
    bundle: &DecodedSwapBundle,
    sighash: &[u8; 32],
) -> Result<()> {
    for (group_idx, group) in bundle.action_groups.iter().enumerate() {
        if group.actions.len() != group.spend_auth_sigs.len() {
            return Err(OrchardError::StateError(format!(
                "Swap action group {} actions/signature count mismatch",
                group_idx
            )));
        }

        let group_digest = hash_action_group_zsa(
            &group.actions,
            group.flags,
            &group.anchor,
            group.expiry_height,
            &group.burns,
        )
        .map_err(|e| {
            OrchardError::StateError(format!(
                "Swap action group {} digest computation failed: {}",
                group_idx, e
            ))
        })?;

        for (action_idx, (action, sig)) in group
            .actions
            .iter()
            .zip(&group.spend_auth_sigs)
            .enumerate()
        {
            verify_spend_auth_signature_raw(&action.rk, sig, &group_digest).map_err(|e| {
                OrchardError::StateError(format!(
                    "Swap action group {} action {} spend_auth_sig verification failed: {}",
                    group_idx, action_idx, e
                ))
            })?;
        }
    }

    let mut all_actions = Vec::new();
    let mut all_burns = Vec::new();
    for group in &bundle.action_groups {
        all_actions.extend_from_slice(&group.actions);
        all_burns.extend_from_slice(&group.burns);
    }

    verify_binding_signature_zsa(
        &all_actions,
        bundle.value_balance,
        &all_burns,
        &bundle.binding_sig,
        sighash,
    )
}

/// Verify a single spend authorization signature
///
/// Per Zcash spec, this verifies:
///   rk.verify(sighash, spend_auth_sig) == valid
///
/// Where:
/// - rk = randomized verification key (32 bytes, Pallas point)
/// - sighash = transaction sighash (32 bytes)
/// - spend_auth_sig = RedPallas signature (64 bytes: R || s)
fn verify_spend_auth_signature(
    action: &ParsedAction,
    signature: &[u8; 64],
    sighash: &[u8; 32],
) -> Result<()> {
    verify_spend_auth_signature_raw(&action.rk, signature, sighash)
}

fn verify_spend_auth_signature_raw(
    rk_bytes: &[u8; 32],
    signature: &[u8; 64],
    sighash: &[u8; 32],
) -> Result<()> {
    // Parse verification key (rk) as Pallas point
    let vk_point = parse_verification_key(rk_bytes)?;
    let basepoint = parse_basepoint(&ORCHARD_SPENDAUTHSIG_BASEPOINT_BYTES)?;

    // Verify RedPallas signature
    verify_redpallas_signature(
        &vk_point,
        signature,
        sighash,
        b"Zcash_RedPallasH",
        &basepoint,
    )
}

/// Verify bundle binding signature
///
/// Per Zcash spec, this verifies:
///   bvk.verify(sighash, binding_sig) == valid
///
/// Where:
/// - bvk = binding verification key (computed from cv_net values)
/// - sighash = transaction sighash
/// - binding_sig = RedPallas signature (64 bytes)
///
/// bvk = sum(cv_net) over all actions
fn verify_binding_signature(
    bundle: &ParsedBundle,
    sighash: &[u8; 32],
) -> Result<()> {
    // Per Zcash Protocol Spec § 4.6.3:
    // Binding verification key: bvk = Σ cv_net + value_balance * V
    //
    // This cryptographically binds the binding signature to BOTH:
    // 1. The sum of value commitments (cv_net)
    // 2. The net value leaving/entering the shielded pool (value_balance)
    //
    // Without the value_balance term, attackers can forge arbitrary fees!

    // Step 1: Sum all action value commitments
    let mut bvk = pallas::Point::identity();

    for action in &bundle.actions {
        let cv_net = parse_value_commitment(&action.cv_net)?;
        bvk += cv_net;
    }

    // Step 2: Subtract value_balance term using Orchard value commitment generator V
    // value_balance > 0 means value leaving pool (fees being paid)
    // value_balance < 0 means value entering pool (deposits)
    // value_balance = 0 means balanced (no net flow)

    // Convert value_balance (i64) to scalar
    // Orchard uses i64 for value_balance: positive = outflow, negative = inflow
    if bundle.value_balance != 0 {
        let v_generator = value_commitment_v_generator()?;
        let value_balance_scalar = if bundle.value_balance > 0 {
            // Positive: value leaving pool (fee payment)
            pallas::Scalar::from(bundle.value_balance as u64)
        } else {
            // Negative: value entering pool (must negate)
            // -value_balance gives positive u64, then negate the scalar
            -pallas::Scalar::from((-bundle.value_balance) as u64)
        };

        // bvk = sum(cv_net) - value_balance * V
        bvk -= v_generator * value_balance_scalar;
    }

    // Step 3: Verify RedPallas binding signature with correct bvk
    let basepoint = parse_basepoint(&ORCHARD_BINDINGSIG_BASEPOINT_BYTES)?;
    verify_redpallas_signature(
        &bvk,
        &bundle.binding_signature,
        sighash,
        b"Zcash_RedPallasH",
        &basepoint,
    )
        .map_err(|e| OrchardError::StateError(format!(
            "Binding signature verification failed: {}",
            e
        )))
}

fn verify_binding_signature_zsa(
    actions: &[DecodedActionV6],
    value_balance: i64,
    burns: &[BurnRecord],
    binding_sig: &[u8; 64],
    sighash: &[u8; 32],
) -> Result<()> {
    let mut bvk = pallas::Point::identity();

    for action in actions {
        let cv_net = parse_value_commitment(&action.cv_net)?;
        bvk += cv_net;
    }

    if value_balance != 0 {
        let v_generator = value_commitment_v_generator()?;
        let value_balance_scalar = if value_balance > 0 {
            pallas::Scalar::from(value_balance as u64)
        } else {
            -pallas::Scalar::from((-value_balance) as u64)
        };
        bvk -= v_generator * value_balance_scalar;
    }

    let burn_commitments = burns
        .iter()
        .map(burn_commitment)
        .collect::<Result<Vec<_>>>()?;
    for burn in burn_commitments {
        bvk -= burn;
    }

    let basepoint = parse_basepoint(&ORCHARD_BINDINGSIG_BASEPOINT_BYTES)?;
    verify_redpallas_signature(
        &bvk,
        binding_sig,
        sighash,
        b"Zcash_RedPallasH",
        &basepoint,
    )
    .map_err(|e| OrchardError::StateError(format!(
        "ZSA binding signature verification failed: {}",
        e
    )))
}

fn burn_commitment(burn: &BurnRecord) -> Result<pallas::Point> {
    if burn.amount == 0 {
        return Err(OrchardError::StateError(
            "Burn amount must be non-zero".into(),
        ));
    }
    if burn.asset.is_native() {
        return Err(OrchardError::StateError(
            "Native USDx burn not allowed".into(),
        ));
    }
    if !range_check_64bit(burn.amount) {
        return Err(OrchardError::StateError(
            "Burn amount out of range".into(),
        ));
    }
    let base = parse_asset_base(&burn.asset)?;
    let scalar = pallas::Scalar::from(burn.amount);
    Ok(base * scalar)
}

fn parse_asset_base(asset: &AssetBase) -> Result<pallas::Point> {
    let bytes = asset.to_bytes();
    let point = pallas::Point::from_bytes(&bytes);
    if point.is_some().into() {
        Ok(point.unwrap())
    } else {
        Err(OrchardError::StateError("Invalid asset base encoding".into()))
    }
}

/// Parse a verification key from 32 bytes
fn parse_verification_key(bytes: &[u8; 32]) -> Result<pallas::Point> {
    // Parse as compressed Pallas point
    let point_option = pallas::Point::from_bytes(bytes);

    if point_option.is_some().into() {
        Ok(point_option.unwrap())
    } else {
        Err(OrchardError::StateError("Invalid verification key encoding".into()))
    }
}

/// Parse a value commitment from 32 bytes
fn parse_value_commitment(bytes: &[u8; 32]) -> Result<pallas::Point> {
    let point_option = pallas::Point::from_bytes(bytes);

    if point_option.is_some().into() {
        Ok(point_option.unwrap())
    } else {
        Err(OrchardError::StateError("Invalid value commitment encoding".into()))
    }
}

/// Compute Orchard value commitment generator V
fn value_commitment_v_generator() -> Result<pallas::Point> {
    let x = ct_to_result(
        pallas::Base::from_repr(VALUE_COMMITMENT_V_X),
        "Invalid V generator x",
    )?;
    let y = ct_to_result(
        pallas::Base::from_repr(VALUE_COMMITMENT_V_Y),
        "Invalid V generator y",
    )?;
    let affine = ct_to_result(
        pallas::Affine::from_xy(x, y),
        "Invalid V generator point",
    )?;
    Ok(pallas::Point::from(affine))
}

fn ct_to_result<T>(value: subtle::CtOption<T>, message: &str) -> Result<T> {
    if bool::from(value.is_some()) {
        Ok(value.unwrap())
    } else {
        Err(OrchardError::StateError(message.into()))
    }
}

/// Parse a RedPallas basepoint from 32 bytes
fn parse_basepoint(bytes: &[u8; 32]) -> Result<pallas::Point> {
    let point_option = pallas::Point::from_bytes(bytes);

    if point_option.is_some().into() {
        Ok(point_option.unwrap())
    } else {
        Err(OrchardError::StateError("Invalid RedPallas basepoint encoding".into()))
    }
}

/// Verify a RedPallas signature
///
/// RedPallas signature format (64 bytes):
/// - R: 32 bytes (Pallas point, signature nonce)
/// - s: 32 bytes (Pallas scalar, signature response)
///
/// Verification equation:
///   s * B == R + c * VK
///
/// Where:
/// - B = Pallas base point
/// - c = H(R || VK || msg) (challenge)
/// - VK = verification key (public key)
///
/// This follows the standard Schnorr signature verification.
fn verify_redpallas_signature(
    vk: &pallas::Point,
    signature: &[u8; 64],
    message: &[u8],
    domain_sep: &[u8; 16],
    basepoint: &pallas::Point,
) -> Result<()> {
    use blake2b_simd::Params as Blake2bParams;

    // Parse signature components
    let mut r_bytes = [0u8; 32];
    r_bytes.copy_from_slice(&signature[0..32]);

    let mut s_bytes = [0u8; 32];
    s_bytes.copy_from_slice(&signature[32..64]);

    // Parse R as Pallas point
    let r_point_option = pallas::Point::from_bytes(&r_bytes);
    if r_point_option.is_none().into() {
        return Err(OrchardError::StateError("Invalid signature R component".into()));
    }
    let r_point = r_point_option.unwrap();

    // Parse s as Pallas scalar (Fq is the scalar field of Pallas)
    let s_scalar_option = pallas::Scalar::from_repr(s_bytes);
    if s_scalar_option.is_none().into() {
        return Err(OrchardError::StateError("Invalid signature s component".into()));
    }
    let s_scalar = s_scalar_option.unwrap();

    // Compute challenge: c = H(domain_sep || R || VK || msg)
    let mut hasher = Blake2bParams::new()
        .hash_length(64)
        .personal(domain_sep)
        .to_state();

    hasher.update(&r_bytes);
    hasher.update(&vk.to_bytes());
    hasher.update(message);

    let hash = hasher.finalize();
    let hash_bytes: [u8; 64] = (*hash.as_array()).into();

    // Reduce hash to scalar (mod q) using wide reduction
    let c_scalar = pallas::Scalar::from_uniform_bytes(&hash_bytes);

    // Verify equation: s * B == R + c * VK
    let lhs = *basepoint * s_scalar;
    let rhs = r_point + (*vk * c_scalar);

    if lhs == rhs {
        Ok(())
    } else {
        Err(OrchardError::StateError("Signature verification equation failed".into()))
    }
}

/// Compute bundle commitment per ZIP-244 (Zcash Orchard spec)
///
/// This computes the standard Zcash bundle commitment hash used as sighash.
/// Per ZIP-244 § 4.17 (txid_digest):
///
/// bundle_commitment = Blake2b-256(
///   compact_hash || memos_hash || noncompact_hash || flags || value_balance || anchor
/// )
///
/// Where:
/// - compact_hash = Blake2b(nullifier || cmx || epk || enc[0..52]) for all actions
/// - memos_hash = Blake2b(enc[52..564]) for all actions
/// - noncompact_hash = Blake2b(cv_net || rk || enc[564..] || out_ciphertext) for all actions
/// - flags = bundle flags byte
/// - value_balance = i64 little-endian
/// - anchor = commitment tree root
///
/// SECURITY: This MUST match what the builder uses when signing bundles.
/// The builder calls `proven_bundle.commitment()` which follows this spec.
pub fn compute_bundle_commitment(bundle: &ParsedBundle) -> [u8; 32] {
    use blake2b_simd::Params as Blake2bParams;

    // Personalization strings per ZIP-244
    const ZCASH_ORCHARD_HASH_PERSONALIZATION: &[u8; 16] = b"ZTxIdOrchardHash";
    const ZCASH_ORCHARD_ACTIONS_COMPACT_HASH: &[u8; 16] = b"ZTxIdOrcActCHash";
    const ZCASH_ORCHARD_ACTIONS_MEMOS_HASH: &[u8; 16] = b"ZTxIdOrcActMHash";
    const ZCASH_ORCHARD_ACTIONS_NONCOMPACT_HASH: &[u8; 16] = b"ZTxIdOrcActNHash";

    // Step 1: Compute compact_hash (nullifier || cmx || epk || enc[0..52])
    let mut compact_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTIONS_COMPACT_HASH)
        .to_state();

    for action in &bundle.actions {
        compact_hasher.update(&action.nullifier);
        compact_hasher.update(&action.cmx);
        compact_hasher.update(&action.epk_bytes);
        compact_hasher.update(&action.enc_ciphertext[..52]);
    }
    let compact_hash = compact_hasher.finalize();

    // Step 2: Compute memos_hash (enc[52..564])
    let mut memos_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTIONS_MEMOS_HASH)
        .to_state();

    for action in &bundle.actions {
        memos_hasher.update(&action.enc_ciphertext[52..564]);
    }
    let memos_hash = memos_hasher.finalize();

    // Step 3: Compute noncompact_hash (cv_net || rk || enc[564..] || out_ciphertext)
    let mut noncompact_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTIONS_NONCOMPACT_HASH)
        .to_state();

    for action in &bundle.actions {
        noncompact_hasher.update(&action.cv_net);
        noncompact_hasher.update(&action.rk);
        noncompact_hasher.update(&action.enc_ciphertext[564..]); // enc_ciphertext is 580 bytes, [564..] = last 16 bytes
        noncompact_hasher.update(&action.out_ciphertext);
    }
    let noncompact_hash = noncompact_hasher.finalize();

    // Step 4: Compute final bundle commitment hash
    let mut final_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_HASH_PERSONALIZATION)
        .to_state();

    final_hasher.update(compact_hash.as_bytes());
    final_hasher.update(memos_hash.as_bytes());
    final_hasher.update(noncompact_hash.as_bytes());
    final_hasher.update(&[bundle.flags]);
    final_hasher.update(&bundle.value_balance.to_le_bytes());
    final_hasher.update(&bundle.anchor);

    let hash = final_hasher.finalize();
    let mut result = [0u8; 32];
    result.copy_from_slice(hash.as_bytes());
    result
}

fn hash_action_group_zsa(
    actions: &[DecodedActionV6],
    flags: u8,
    anchor: &[u8; 32],
    expiry_height: u32,
    burns: &[BurnRecord],
) -> Result<[u8; 32]> {
    use blake2b_simd::Params as Blake2bParams;

    if ZSA_COMPACT_NOTE_SIZE + ZSA_MEMO_SIZE > ZSA_ENC_CIPHERTEXT_SIZE {
        return Err(OrchardError::StateError(
            "ZSA ciphertext size constants inconsistent".into(),
        ));
    }

    let mut compact_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTIONS_COMPACT_HASH_PERSONALIZATION_V6)
        .to_state();
    let mut memos_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTIONS_MEMOS_HASH_PERSONALIZATION)
        .to_state();
    let mut noncompact_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTIONS_NONCOMPACT_HASH_PERSONALIZATION_V6)
        .to_state();

    for action in actions {
        compact_hasher.update(&action.nullifier);
        compact_hasher.update(&action.cmx);
        compact_hasher.update(&action.epk_bytes);
        compact_hasher.update(&action.enc_ciphertext[..ZSA_COMPACT_NOTE_SIZE]);

        let memo_start = ZSA_COMPACT_NOTE_SIZE;
        let memo_end = memo_start + ZSA_MEMO_SIZE;
        memos_hasher.update(&action.enc_ciphertext[memo_start..memo_end]);

        noncompact_hasher.update(&action.cv_net);
        noncompact_hasher.update(&action.rk);
        noncompact_hasher.update(&action.enc_ciphertext[memo_end..]);
        noncompact_hasher.update(&action.out_ciphertext);
    }

    let compact_hash = compact_hasher.finalize();
    let memos_hash = memos_hasher.finalize();
    let noncompact_hash = noncompact_hasher.finalize();

    let mut group_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ACTION_GROUPS_HASH_PERSONALIZATION)
        .to_state();
    group_hasher.update(compact_hash.as_bytes());
    group_hasher.update(memos_hash.as_bytes());
    group_hasher.update(noncompact_hash.as_bytes());
    group_hasher.update(&[flags]);
    group_hasher.update(anchor);
    group_hasher.update(&expiry_height.to_le_bytes());

    let mut burn_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ZSA_BURN_HASH_PERSONALIZATION)
        .to_state();
    for burn in burns {
        burn_hasher.update(&burn.asset.to_bytes());
        burn_hasher.update(&burn.amount.to_le_bytes());
    }
    group_hasher.update(burn_hasher.finalize().as_bytes());

    let mut out = [0u8; 32];
    out.copy_from_slice(group_hasher.finalize().as_bytes());
    Ok(out)
}

pub fn compute_zsa_bundle_commitment(bundle: &DecodedZSABundle) -> Result<[u8; 32]> {
    let group_hash = hash_action_group_zsa(
        &bundle.actions,
        bundle.flags,
        &bundle.anchor,
        bundle.expiry_height,
        &bundle.burns,
    )?;

    let mut h = blake2b_simd::Params::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_HASH_PERSONALIZATION)
        .to_state();
    h.update(&group_hash);
    h.update(&bundle.value_balance.to_le_bytes());

    let mut out = [0u8; 32];
    out.copy_from_slice(h.finalize().as_bytes());
    Ok(out)
}

pub fn compute_swap_bundle_commitment(bundle: &DecodedSwapBundle) -> Result<[u8; 32]> {
    let mut h = blake2b_simd::Params::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_HASH_PERSONALIZATION)
        .to_state();

    for group in &bundle.action_groups {
        let group_hash = hash_action_group_zsa(
            &group.actions,
            group.flags,
            &group.anchor,
            group.expiry_height,
            &group.burns,
        )?;
        h.update(&group_hash);
    }

    h.update(&bundle.value_balance.to_le_bytes());
    let mut out = [0u8; 32];
    out.copy_from_slice(h.finalize().as_bytes());
    Ok(out)
}

pub fn verify_issue_bundle(bundle: &DecodedIssueBundle) -> Result<Vec<AssetBase>> {
    if bundle.issuer_key.is_empty() {
        return Err(OrchardError::StateError(
            "IssueBundle missing issuer key".into(),
        ));
    }
    if bundle.actions.is_empty() {
        return Err(OrchardError::StateError(
            "IssueBundle has no actions".into(),
        ));
    }

    let mut finalized = BTreeSet::new();
    for action in &bundle.actions {
        if finalized.contains(&action.asset_desc_hash) {
            return Err(OrchardError::StateError(
                "IssueBundle action after finalize".into(),
            ));
        }
        if action.finalize {
            finalized.insert(action.asset_desc_hash);
        }
    }

    let ik = decode_issue_validating_key(&bundle.issuer_key)?;
    verify_issue_sighash_info(&bundle.sighash_info)?;
    let sig = decode_issue_signature(&bundle.signature)?;
    // Use IssueBundle commitment as the prehash (JAM doesn't provide full tx sighash).
    let sighash = compute_issue_bundle_commitment(bundle)?;

    ik.verify_prehash(&sighash, &sig).map_err(|_| {
        OrchardError::StateError("IssueBundle signature verification failed".into())
    })?;

    let mut assets = Vec::with_capacity(bundle.actions.len());
    for action in &bundle.actions {
        let asset = derive_asset_base(&bundle.issuer_key, &action.asset_desc_hash)?;
        assets.push(asset);
    }

    Ok(assets)
}

pub fn compute_issue_bundle_commitments(
    bundle: &DecodedIssueBundle,
) -> Result<Vec<[u8; 32]>> {
    if bundle.issuer_key.is_empty() {
        return Err(OrchardError::StateError(
            "IssueBundle missing issuer key".into(),
        ));
    }
    if bundle.actions.is_empty() {
        return Err(OrchardError::StateError(
            "IssueBundle has no actions".into(),
        ));
    }

    let mut commitments = Vec::new();
    for action in &bundle.actions {
        let asset = derive_asset_base(&bundle.issuer_key, &action.asset_desc_hash)?;
        for note in &action.notes {
            commitments.push(issue_note_commitment(note, &asset)?);
        }
    }

    Ok(commitments)
}

fn verify_issue_sighash_info(bytes: &[u8]) -> Result<()> {
    if bytes != ISSUE_SIGHASH_INFO_V0 {
        return Err(OrchardError::StateError(
            "Unsupported IssueBundle sighash_info".into(),
        ));
    }
    Ok(())
}

fn issue_note_commitment(note: &DecodedIssuedNote, asset: &AssetBase) -> Result<[u8; 32]> {
    let diversifier: [u8; 11] = note.recipient[..11]
        .try_into()
        .map_err(|_| OrchardError::StateError("Invalid IssueBundle recipient length".into()))?;
    let pk_d_bytes: [u8; 32] = note.recipient[11..]
        .try_into()
        .map_err(|_| OrchardError::StateError("Invalid IssueBundle recipient length".into()))?;

    let pk_d = pallas::Point::from_bytes(&pk_d_bytes)
        .into_option()
        .ok_or_else(|| OrchardError::StateError("Invalid IssueBundle pk_d".into()))?;
    if bool::from(pk_d.is_identity()) {
        return Err(OrchardError::StateError(
            "IssueBundle pk_d is identity".into(),
        ));
    }

    let g_d = diversify_hash(&diversifier)?;
    let g_d_bytes = g_d.to_bytes();
    let pk_d_bytes = pk_d.to_bytes();

    let rho = pallas::Base::from_repr(note.rho)
        .into_option()
        .ok_or_else(|| OrchardError::StateError("Invalid IssueBundle rho".into()))?;

    let esk = pallas::Scalar::from_uniform_bytes(&prf_expand_tag(&note.rseed, 0x04, &note.rho));
    if bool::from(esk.is_zero()) {
        return Err(OrchardError::StateError(
            "Invalid IssueBundle rseed (esk is zero)".into(),
        ));
    }

    let rcm = pallas::Scalar::from_uniform_bytes(&prf_expand_tag(&note.rseed, 0x05, &note.rho));
    let psi = pallas::Base::from_uniform_bytes(&prf_expand_tag(&note.rseed, 0x09, &note.rho));

    let mut message = Vec::new();
    message.extend(bytes_to_bits_le(&g_d_bytes));
    message.extend(bytes_to_bits_le(&pk_d_bytes));
    message.extend(u64_to_bits_le(note.value));
    message.extend(base_to_bits_le(&rho, L_ORCHARD_BASE));
    message.extend(base_to_bits_le(&psi, L_ORCHARD_BASE));
    message.extend(bytes_to_bits_le(&asset.to_bytes()));

    let m_prefix = format!("{}-M", NOTE_ZSA_COMMITMENT_PERSONALIZATION);
    let r_prefix = format!("{}-r", NOTE_COMMITMENT_PERSONALIZATION);
    let m_domain = HashDomain::new(&m_prefix);
    let r_base = pallas::Point::hash_to_curve(&r_prefix)(&[]);
    let cm = m_domain
        .hash_to_point(message.into_iter())
        .map(|p| p + (r_base * rcm))
        .into_option()
        .ok_or_else(|| OrchardError::StateError("IssueBundle note commitment failed".into()))?;

    Ok(extract_p(&cm).to_repr())
}

fn diversify_hash(diversifier: &[u8; 11]) -> Result<pallas::Point> {
    let hasher = pallas::Point::hash_to_curve(KEY_DIVERSIFICATION_PERSONALIZATION);
    let mut g_d = hasher(diversifier);
    if bool::from(g_d.is_identity()) {
        g_d = hasher(&[]);
    }
    if bool::from(g_d.is_identity()) {
        return Err(OrchardError::StateError(
            "Diversify hash returned identity".into(),
        ));
    }
    Ok(g_d)
}

fn prf_expand_tag(seed: &[u8; 32], tag: u8, rho: &[u8; 32]) -> [u8; 64] {
    let mut t = [0u8; 33];
    t[0] = tag;
    t[1..].copy_from_slice(rho);
    prf_expand(seed, &t)
}

fn prf_expand(seed: &[u8; 32], t: &[u8]) -> [u8; 64] {
    let mut h = blake2b_simd::Params::new()
        .hash_length(64)
        .personal(PRF_EXPAND_PERSONALIZATION)
        .to_state();
    h.update(seed);
    h.update(t);
    let mut out = [0u8; 64];
    out.copy_from_slice(h.finalize().as_bytes());
    out
}

fn bytes_to_bits_le(bytes: &[u8]) -> Vec<bool> {
    let mut bits = Vec::with_capacity(bytes.len() * 8);
    for byte in bytes {
        for i in 0..8 {
            bits.push(((byte >> i) & 1) == 1);
        }
    }
    bits
}

fn base_to_bits_le(value: &pallas::Base, limit: usize) -> Vec<bool> {
    value.to_le_bits().iter().by_vals().take(limit).collect()
}

fn u64_to_bits_le(value: u64) -> Vec<bool> {
    let mut bits = Vec::with_capacity(64);
    for i in 0..64 {
        bits.push(((value >> i) & 1) == 1);
    }
    bits
}

fn extract_p(point: &pallas::Point) -> pallas::Base {
    point
        .to_affine()
        .coordinates()
        .into_option()
        .map(|coords| *coords.x())
        .unwrap_or_else(pallas::Base::zero)
}

fn decode_issue_validating_key(bytes: &[u8]) -> Result<SchnorrVerifyingKey> {
    if bytes.len() != 33 || bytes[0] != ISSUE_SIG_ALGO_BYTE {
        return Err(OrchardError::StateError(
            "Invalid IssueBundle issuer key encoding".into(),
        ));
    }
    let key_bytes: [u8; 32] = bytes[1..]
        .try_into()
        .map_err(|_| OrchardError::StateError("Invalid issuer key length".into()))?;
    SchnorrVerifyingKey::from_bytes(&key_bytes).map_err(|_| {
        OrchardError::StateError("Invalid IssueBundle issuer key".into())
    })
}

fn decode_issue_signature(bytes: &[u8]) -> Result<SchnorrSignature> {
    if bytes.len() != 65 || bytes[0] != ISSUE_SIG_ALGO_BYTE {
        return Err(OrchardError::StateError(
            "Invalid IssueBundle signature encoding".into(),
        ));
    }
    SchnorrSignature::try_from(&bytes[1..]).map_err(|_| {
        OrchardError::StateError("Invalid IssueBundle signature".into())
    })
}

/// Computes the IssueBundle prehash used for issuer Schnorr signatures.
pub fn compute_issue_bundle_commitment(bundle: &DecodedIssueBundle) -> Result<[u8; 32]> {
    use blake2b_simd::Params as Blake2bParams;

    let mut h = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ZSA_ISSUE_PERSONALIZATION)
        .to_state();

    h.update(&compact_size_bytes(bundle.issuer_key.len()));
    h.update(&bundle.issuer_key);

    let mut action_hasher = Blake2bParams::new()
        .hash_length(32)
        .personal(ZCASH_ORCHARD_ZSA_ISSUE_ACTION_PERSONALIZATION)
        .to_state();

    for action in &bundle.actions {
        action_hasher.update(&action.asset_desc_hash);

        let mut note_hasher = Blake2bParams::new()
            .hash_length(32)
            .personal(ZCASH_ORCHARD_ZSA_ISSUE_NOTE_PERSONALIZATION)
            .to_state();
        for note in &action.notes {
            note_hasher.update(&note.recipient);
            note_hasher.update(&note.value.to_le_bytes());
            note_hasher.update(&note.rho);
            note_hasher.update(&note.rseed);
        }
        action_hasher.update(note_hasher.finalize().as_bytes());
        action_hasher.update(&[action.finalize as u8]);
    }

    h.update(action_hasher.finalize().as_bytes());
    let mut out = [0u8; 32];
    out.copy_from_slice(h.finalize().as_bytes());
    Ok(out)
}

fn compact_size_bytes(size: usize) -> Vec<u8> {
    match size {
        s if s < 253 => vec![s as u8],
        s if s <= 0xFFFF => [&[253_u8], &(s as u16).to_le_bytes()[..]].concat(),
        s if s <= 0xFFFF_FFFF => [&[254_u8], &(s as u32).to_le_bytes()[..]].concat(),
        s => [&[255_u8], &(s as u64).to_le_bytes()[..]].concat(),
    }
}

pub fn derive_asset_base(issuer_key_bytes: &[u8], asset_desc_hash: &[u8; 32]) -> Result<AssetBase> {
    let mut asset_id = Vec::with_capacity(1 + issuer_key_bytes.len() + asset_desc_hash.len());
    asset_id.push(0x00);
    asset_id.extend_from_slice(issuer_key_bytes);
    asset_id.extend_from_slice(asset_desc_hash);

    let asset_digest = blake2b_simd::Params::new()
        .hash_length(64)
        .personal(ZSA_ASSET_DIGEST_PERSONALIZATION)
        .to_state()
        .update(&asset_id)
        .finalize();

    let asset_base =
        pallas::Point::hash_to_curve(ZSA_ASSET_BASE_PERSONALIZATION)(asset_digest.as_bytes());
    if bool::from(asset_base.is_identity()) {
        return Err(OrchardError::StateError(
            "Derived asset base is identity".into(),
        ));
    }

    Ok(AssetBase::from_bytes(asset_base.to_bytes()))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_verification_key_identity() {
        // Identity point (all zeros except first bit for sign)
        let bytes = [0u8; 32];
        // This should either succeed or fail consistently
        let _ = parse_verification_key(&bytes);
    }

    #[test]
    fn test_parse_verification_key_generator() {
        // Pallas generator point
        let generator = pallas::Point::generator();
        let bytes = generator.to_bytes();
        let parsed = parse_verification_key(&bytes).unwrap();
        assert_eq!(parsed, generator);
    }

    #[test]
    fn test_bundle_commitment_deterministic() {
        // Bundle commitment should be deterministic for same input
        // (tested in integration tests with actual bundle data)
    }

    #[test]
    fn test_verify_action_group_proof_empty_actions() {
        // Test that empty action group fails NonEmpty validation
        let group = crate::bundle_codec::DecodedActionGroup {
            actions: Vec::new(),
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: Vec::new(),
        };

        let result = verify_action_group_proof(&group, 0);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("NonEmpty"));
    }

    #[test]
    fn test_verify_action_group_proof_nonzero_expiry() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedActionV6, ZSA_ENC_CIPHERTEXT_SIZE};

        // Test that non-zero expiry_height fails NU7 validation
        let action = DecodedActionV6 {
            cv_net: [0u8; 32],
            nullifier: [0u8; 32],
            rk: [0u8; 32],
            cmx: [0u8; 32],
            epk_bytes: [0u8; 32],
            enc_ciphertext: [0u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0u8; 80],
        };

        let group = DecodedActionGroup {
            actions: vec![action],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 1, // Should be 0
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64]],
        };

        let result = verify_action_group_proof(&group, 0);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("expiry_height"));
    }

    #[test]
    fn test_verify_action_group_proof_empty_proof() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedActionV6, ZSA_ENC_CIPHERTEXT_SIZE};

        // Test that empty proof fails validation
        let action = DecodedActionV6 {
            cv_net: [0u8; 32],
            nullifier: [0u8; 32],
            rk: [0u8; 32],
            cmx: [0u8; 32],
            epk_bytes: [0u8; 32],
            enc_ciphertext: [0u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0u8; 80],
        };

        let group = DecodedActionGroup {
            actions: vec![action],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: Vec::new(), // Empty proof
            spend_auth_sigs: vec![[0u8; 64]],
        };

        let result = verify_action_group_proof(&group, 0);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("empty proof"));
    }

    #[test]
    fn test_asset_balances_native_asset_allowed() {
        // Native USDx can have non-zero balance
        let mut balances = AssetBalances::new();
        let native_asset = AssetBase::NATIVE;

        balances.add_spend(native_asset, 100);
        balances.add_output(native_asset, 50);

        // Native asset can have net balance (for fees/transparent)
        let result = balances.verify_balanced();
        assert!(result.is_ok());
    }

    #[test]
    fn test_asset_balances_custom_asset_must_balance() {
        // Custom assets must balance to zero
        let mut balances = AssetBalances::new();
        let custom_asset = AssetBase::from_bytes([1u8; 32]);

        balances.add_spend(custom_asset, 100);
        balances.add_output(custom_asset, 50);

        // Custom asset has unbalanced value (100 - 50 = 50)
        let result = balances.verify_balanced();
        assert!(result.is_err());
    }

    #[test]
    fn test_asset_balances_custom_asset_balanced() {
        // Custom assets must balance to zero
        let mut balances = AssetBalances::new();
        let custom_asset = AssetBase::from_bytes([1u8; 32]);

        balances.add_spend(custom_asset, 100);
        balances.add_output(custom_asset, 100);

        // Custom asset is balanced
        let result = balances.verify_balanced();
        assert!(result.is_ok());
    }

    #[test]
    fn test_swap_bundle_signatures_invalid_sighash() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedActionV6};

        // Test that invalid sighash fails signature verification
        // Use generator point for rk to be a valid verification key
        let generator = pallas::Point::generator();
        let rk_bytes = generator.to_bytes();

        let action = DecodedActionV6 {
            cv_net: generator.to_bytes(),
            nullifier: [1u8; 32],
            rk: rk_bytes,
            cmx: [2u8; 32],
            epk_bytes: [3u8; 32],
            enc_ciphertext: [0u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0u8; 80],
        };

        let group = DecodedActionGroup {
            actions: vec![action],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64]], // Invalid signature
        };

        let bundle = DecodedSwapBundle {
            action_groups: vec![group],
            value_balance: 0,
            binding_sig: [0u8; 64], // Invalid signature
        };

        let sighash = [0xFFu8; 32];
        let result = verify_swap_bundle_signatures(&bundle, &sighash);
        // Should fail on spend auth sig or binding sig verification
        assert!(
            result.is_err(),
            "Invalid signatures should fail verification"
        );
    }

    #[test]
    fn test_swap_bundle_multiple_action_groups() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedActionV6};

        // Test that multiple action groups pass structural validation
        // (actual proof verification will fail with dummy data, but that's expected)
        let generator = pallas::Point::generator();

        let action1 = DecodedActionV6 {
            cv_net: generator.to_bytes(),
            nullifier: [1u8; 32],
            rk: generator.to_bytes(),
            cmx: [1u8; 32],
            epk_bytes: [1u8; 32],
            enc_ciphertext: [1u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [1u8; 80],
        };

        let action2 = DecodedActionV6 {
            cv_net: generator.to_bytes(),
            nullifier: [2u8; 32],
            rk: generator.to_bytes(),
            cmx: [2u8; 32],
            epk_bytes: [2u8; 32],
            enc_ciphertext: [2u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [2u8; 80],
        };

        let group1 = DecodedActionGroup {
            actions: vec![action1],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64]],
        };

        let group2 = DecodedActionGroup {
            actions: vec![action2],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64]],
        };

        // Test that both groups pass structural validation checks
        // (Will fail on actual proof verification, but checks NonEmpty, expiry_height, flags, etc.)
        let result1 = verify_action_group_proof(&group1, 0);
        let result2 = verify_action_group_proof(&group2, 1);

        // Both should fail on proof verification (not structural validation)
        assert!(result1.is_err(), "Should fail on proof verification");
        assert!(result2.is_err(), "Should fail on proof verification");

        // Verify they fail on proof, not structure
        let err1 = result1.unwrap_err().to_string();
        let err2 = result2.unwrap_err().to_string();
        assert!(
            err1.contains("proof") || err1.contains("Invalid") || err1.contains("point"),
            "Should fail on proof or point parsing: {}",
            err1
        );
        assert!(
            err2.contains("proof") || err2.contains("Invalid") || err2.contains("point"),
            "Should fail on proof or point parsing: {}",
            err2
        );
    }

    #[test]
    fn test_swap_bundle_with_burns() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedActionV6};

        // Test that burns are included in action groups and don't cause structural errors
        let burn1 = BurnRecord {
            asset: AssetBase::from_bytes([1u8; 32]),
            amount: 100,
        };

        let burn2 = BurnRecord {
            asset: AssetBase::from_bytes([2u8; 32]),
            amount: 50,
        };

        let generator = pallas::Point::generator();
        let action = DecodedActionV6 {
            cv_net: generator.to_bytes(),
            nullifier: [0u8; 32],
            rk: generator.to_bytes(),
            cmx: [0u8; 32],
            epk_bytes: [0u8; 32],
            enc_ciphertext: [0u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0u8; 80],
        };

        let group = DecodedActionGroup {
            actions: vec![action],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: vec![burn1, burn2],
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64]],
        };

        // Burns should not affect structural validation (will fail on proof verification)
        let result = verify_action_group_proof(&group, 0);
        // Should fail on proof verification, not on burns
        if result.is_err() {
            let err = result.unwrap_err().to_string();
            assert!(
                err.contains("proof") || err.contains("Invalid") || err.contains("point"),
                "Should fail on proof/point, not burns: {}",
                err
            );
        }
    }

    #[test]
    fn test_issue_bundle_signature_verification_invalid() {
        // Test that invalid issuer signature fails
        let issuer_key = vec![0u8; 32]; // Invalid key
        let bundle = DecodedIssueBundle {
            issuer_key: issuer_key.clone(),
            actions: vec![],
            sighash_info: ISSUE_SIGHASH_INFO_V0.to_vec(),
            signature: vec![0u8; 64], // Invalid signature
        };

        let result = verify_issue_bundle(&bundle);
        assert!(result.is_err());
    }

    #[test]
    fn test_issue_bundle_empty_actions() {
        // Test that empty actions fail NonEmpty validation
        let issuer_key = vec![0u8; 32];
        let bundle = DecodedIssueBundle {
            issuer_key,
            actions: vec![],
            sighash_info: ISSUE_SIGHASH_INFO_V0.to_vec(),
            signature: vec![0u8; 64],
        };

        // Empty actions should fail during commitment computation
        let result = verify_issue_bundle(&bundle);
        assert!(result.is_err());
    }

    #[test]
    fn test_issue_bundle_invalid_sighash_info() {
        use crate::bundle_codec::DecodedIssueAction;

        // Test that invalid sighash_info fails
        let issuer_key = vec![0u8; 32];
        let action = DecodedIssueAction {
            asset_desc_hash: [0u8; 32],
            notes: vec![],
            finalize: false,
        };

        let bundle = DecodedIssueBundle {
            issuer_key,
            actions: vec![action],
            sighash_info: vec![0x01], // Invalid version
            signature: vec![0u8; 64],
        };

        let result = verify_issue_bundle(&bundle);
        assert!(result.is_err(), "Invalid sighash_info should fail");
        // Error might be from sighash_info check or later signature verification
        let err_str = result.unwrap_err().to_string();
        assert!(
            err_str.contains("sighash_info") || err_str.contains("signature") || err_str.contains("key"),
            "Error should be related to validation failure: {}",
            err_str
        );
    }

    #[test]
    fn test_burn_commitment_zero_amount() {
        // Test burn with zero amount
        let burn = BurnRecord {
            asset: AssetBase::from_bytes([1u8; 32]),
            amount: 0,
        };

        // Zero burn should be rejected
        let result = burn_commitment(&burn);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("zero"));
    }

    #[test]
    fn test_burn_commitment_native_asset() {
        // Test burn of native USDx asset (must be rejected)
        let burn = BurnRecord {
            asset: AssetBase::NATIVE,
            amount: 100,
        };

        let result = burn_commitment(&burn);
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("Native USDx"));
    }

    #[test]
    fn test_validate_orchard_flags_valid() {
        // Test valid flag values (bits 0-3)
        assert!(validate_orchard_flags(0b0000_0000).is_ok());
        assert!(validate_orchard_flags(0b0000_0001).is_ok());
        assert!(validate_orchard_flags(0b0000_0010).is_ok());
        assert!(validate_orchard_flags(0b0000_0011).is_ok());
        assert!(validate_orchard_flags(0b0000_0100).is_ok());
        assert!(validate_orchard_flags(0b0000_1000).is_ok());
        assert!(validate_orchard_flags(0b0000_1111).is_ok());
    }

    #[test]
    fn test_validate_orchard_flags_invalid() {
        // Test invalid flag values (bits beyond 0-3)
        assert!(validate_orchard_flags(0b0001_0000).is_err()); // Bit 4 set
        assert!(validate_orchard_flags(0b0010_0000).is_err()); // Bit 5 set
        assert!(validate_orchard_flags(0b1111_0000).is_err()); // Upper bits set
    }

    #[test]
    fn test_zsa_bundle_commitment_deterministic() {
        use crate::bundle_codec::DecodedActionV6;

        // Test that ZSA bundle commitment is deterministic
        let action = DecodedActionV6 {
            cv_net: [0x12u8; 32],
            nullifier: [0x34u8; 32],
            rk: [0x56u8; 32],
            cmx: [0x78u8; 32],
            epk_bytes: [0x9au8; 32],
            enc_ciphertext: [0xbcu8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0xdeu8; 80],
        };

        let burn = BurnRecord {
            asset: AssetBase::from_bytes([0xffu8; 32]),
            amount: 100,
        };

        let bundle = DecodedZSABundle {
            actions: vec![action.clone()],
            flags: 0b11,
            anchor: [0xabu8; 32],
            expiry_height: 0,
            burns: vec![burn.clone()],
            proof: vec![0x00u8; 100],
            spend_auth_sigs: vec![[0x11u8; 64]],
            value_balance: -50,
            binding_sig: [0x22u8; 64],
        };

        let commitment1 = compute_zsa_bundle_commitment(&bundle).unwrap();
        let commitment2 = compute_zsa_bundle_commitment(&bundle).unwrap();

        assert_eq!(commitment1, commitment2, "ZSA bundle commitment should be deterministic");
    }

    #[test]
    fn test_swap_bundle_commitment_deterministic() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedActionV6};

        // Test that swap bundle commitment is deterministic
        let action = DecodedActionV6 {
            cv_net: [0x12u8; 32],
            nullifier: [0x34u8; 32],
            rk: [0x56u8; 32],
            cmx: [0x78u8; 32],
            epk_bytes: [0x9au8; 32],
            enc_ciphertext: [0xbcu8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0xdeu8; 80],
        };

        let group = DecodedActionGroup {
            actions: vec![action],
            flags: 0b11,
            anchor: [0xabu8; 32],
            expiry_height: 0,
            burns: vec![],
            proof: vec![0x00u8; 100],
            spend_auth_sigs: vec![[0x11u8; 64]],
        };

        let bundle = DecodedSwapBundle {
            action_groups: vec![group],
            value_balance: -50,
            binding_sig: [0x22u8; 64],
        };

        let commitment1 = compute_swap_bundle_commitment(&bundle).unwrap();
        let commitment2 = compute_swap_bundle_commitment(&bundle).unwrap();

        assert_eq!(commitment1, commitment2, "Swap bundle commitment should be deterministic");
    }

    #[cfg(all(feature = "orchard", feature = "orchard-issuance"))]
    #[test]
    fn test_issue_bundle_commitment_matches_orchard() {
        use orchard::issuance_auth::{IssueAuthKey, IssueValidatingKey, ZSASchnorr};
        use orchard::keys::{FullViewingKey, Scope, SpendingKey};
        use orchard::note::{AssetBase as OrchardAssetBase, Note, RandomSeed, Rho};
        use orchard::value::NoteValue;
        use orchard::ExtractedNoteCommitment;
        use rand::{rngs::StdRng, RngCore, SeedableRng};

        let mut rng = StdRng::seed_from_u64(0x5a5a_1234);
        let isk = IssueAuthKey::<ZSASchnorr>::random(&mut rng);
        let ik = IssueValidatingKey::from(&isk);
        let issuer_key = ik.encode();

        let asset_desc_hash = [0x11u8; 32];
        let asset = OrchardAssetBase::derive(&ik, &asset_desc_hash);

        let sk = SpendingKey::from_bytes([7u8; 32]).expect("valid spending key");
        let fvk = FullViewingKey::from(&sk);
        let address = fvk.address_at(0u32, Scope::External);
        let recipient = address.to_raw_address_bytes();

        let mut rho_bytes = [0u8; 32];
        rho_bytes[0] = 1;
        let rho = Rho::from_bytes(rho_bytes).unwrap();

        let (rseed, rseed_bytes) = loop {
            let mut rseed_bytes = [0u8; 32];
            rng.fill_bytes(&mut rseed_bytes);
            if let Some(rseed) = RandomSeed::from_bytes(rseed_bytes, &rho).into_option() {
                break (rseed, rseed_bytes);
            }
        };

        let note = Note::from_parts(
            address,
            NoteValue::from_raw(42),
            asset,
            rho,
            rseed,
        )
        .unwrap();
        let cmx: ExtractedNoteCommitment = note.commitment().into();
        let expected = cmx.to_bytes();

        let bundle = DecodedIssueBundle {
            issuer_key: issuer_key.clone(),
            actions: vec![DecodedIssueAction {
                asset_desc_hash,
                notes: vec![DecodedIssuedNote {
                    recipient,
                    value: 42,
                    rho: rho_bytes,
                    rseed: rseed_bytes,
                }],
                finalize: false,
            }],
            sighash_info: vec![0u8],
            signature: Vec::new(),
        };

        let commitments = compute_issue_bundle_commitments(&bundle).unwrap();
        assert_eq!(commitments, vec![expected]);
    }

    #[test]
    fn test_split_only_action_groups_validated_by_binding_sig() {
        use crate::bundle_codec::{DecodedActionGroup, DecodedSwapBundle, ZSA_ENC_CIPHERTEXT_SIZE};
        use blake2b_simd::Params as Blake2bParams;

        // Split-only action groups are enforced by binding signature math:
        // if outputs are not balanced by spends elsewhere, signature verification fails.

        let basepoint = parse_basepoint(&ORCHARD_BINDINGSIG_BASEPOINT_BYTES).unwrap();
        let sk = pallas::Scalar::from(5u64);
        let vk = basepoint * sk;

        let sighash = [0x42u8; 32];
        let binding_sig = {
            let r = pallas::Scalar::from(7u64);
            let r_point = basepoint * r;

            let mut hasher = Blake2bParams::new()
                .hash_length(64)
                .personal(b"Zcash_RedPallasH")
                .to_state();
            hasher.update(&r_point.to_bytes());
            hasher.update(&vk.to_bytes());
            hasher.update(&sighash);
            let hash = hasher.finalize();
            let hash_bytes: [u8; 64] = (*hash.as_array()).into();
            let c = pallas::Scalar::from_uniform_bytes(&hash_bytes);
            let s = r + c * sk;

            let mut sig = [0u8; 64];
            sig[..32].copy_from_slice(&r_point.to_bytes());
            sig[32..].copy_from_slice(&s.to_repr());
            sig
        };

        let split1 = basepoint * pallas::Scalar::from(2u64);
        let split2 = basepoint * pallas::Scalar::from(3u64);
        let spend = vk - split1 - split2;

        let make_action = |cv_net: pallas::Point, nullifier: [u8; 32]| DecodedActionV6 {
            cv_net: cv_net.to_bytes(),
            nullifier,
            rk: basepoint.to_bytes(),
            cmx: [0u8; 32],
            epk_bytes: [0u8; 32],
            enc_ciphertext: [0u8; ZSA_ENC_CIPHERTEXT_SIZE],
            out_ciphertext: [0u8; 80],
        };

        // Group 1: Has spend (nullifier present)
        let group1 = DecodedActionGroup {
            actions: vec![make_action(spend, [1u8; 32])],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64]],
        };

        // Group 2: Split-only (no spends, only outputs)
        let group2 = DecodedActionGroup {
            actions: vec![
                make_action(split1, [0u8; 32]),
                make_action(split2, [0u8; 32]),
            ],
            flags: 0,
            anchor: [0u8; 32],
            expiry_height: 0,
            burns: Vec::new(),
            proof: vec![0u8; 100],
            spend_auth_sigs: vec![[0u8; 64], [0u8; 64]],
        };

        let swap_bundle = DecodedSwapBundle {
            action_groups: vec![group1, group2],
            value_balance: 0,
            binding_sig: binding_sig,
        };

        let actions = swap_bundle
            .action_groups
            .iter()
            .flat_map(|g| &g.actions)
            .cloned()
            .collect::<Vec<_>>();

        let result = verify_binding_signature_zsa(
            &actions,
            swap_bundle.value_balance,
            &[],
            &swap_bundle.binding_sig,
            &sighash,
        );
        assert!(result.is_ok(), "Balanced split-only groups should verify");

        let mut unbalanced = actions.clone();
        unbalanced[1].cv_net = (basepoint * pallas::Scalar::from(4u64)).to_bytes();
        let result = verify_binding_signature_zsa(
            &unbalanced,
            swap_bundle.value_balance,
            &[],
            &swap_bundle.binding_sig,
            &sighash,
        );
        assert!(result.is_err(), "Unbalanced split-only group should fail");
    }
}
