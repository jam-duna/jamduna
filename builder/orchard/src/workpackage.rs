// Orchard work package builder
//
// Produces JAM work packages for Orchard shielded transfers. This builder generates
// Orchard bundles and state witnesses. This eliminates FFI complexity by keeping
// all Orchard logic in Rust.

use crate::bundle_codec::{deserialize_bundle, serialize_bundle};
use crate::merkle_impl::IncrementalMerkleTree;
use crate::state::{OrchardState, StateWitnesses, WriteIntents};
use crate::witness::{
    apply_write_intents, build_witnesses, compute_write_intents, verify_witnesses, BuilderState,
};
use crate::{Error, Result, OrchardBundle};
use pasta_curves::arithmetic::CurveAffine;
use pasta_curves::group::ff::PrimeField;
use pasta_curves::group::{Curve, GroupEncoding};
use pasta_curves::pallas;
use serde::{Deserialize, Serialize};
#[cfg(feature = "circuit")]
use halo2_proofs::poly::commitment::Params;
#[cfg(feature = "circuit")]
use halo2_proofs::pasta::vesta;
#[cfg(feature = "circuit")]
use std::fs;
#[cfg(feature = "circuit")]
use std::path::PathBuf;
#[cfg(feature = "circuit")]
use std::sync::OnceLock;

/// Orchard work package with 3-4 extrinsics for JAM service
/// CompactBlock is derived from bundle_bytes during verification
/// 4th extrinsic (TransparentTxData) is optional for transparent transaction support
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrchardExtrinsic {
    /// Extrinsic 1: PreStateWitness (tag=1)
    #[serde(serialize_with = "hex_bytes", deserialize_with = "from_hex_bytes")]
    pub pre_state_witness_extrinsic: Vec<u8>,

    /// Extrinsic 2: PostStateWitness (tag=2)
    #[serde(serialize_with = "hex_bytes", deserialize_with = "from_hex_bytes")]
    pub post_state_witness_extrinsic: Vec<u8>,

    /// Extrinsic 3: BundleProof (tag=3) - includes bundle_bytes
    #[serde(serialize_with = "hex_bytes", deserialize_with = "from_hex_bytes", default, skip_serializing_if = "Vec::is_empty")]
    pub bundle_proof_extrinsic: Vec<u8>,

    /// Extrinsic 4: TransparentTxData (tag=4) - optional, for transparent transactions
    #[serde(serialize_with = "hex_bytes_opt", deserialize_with = "from_hex_bytes_opt", skip_serializing_if = "Vec::is_empty", default)]
    pub transparent_tx_data_extrinsic: Vec<u8>,

    /// Gas limit for execution
    pub gas_limit: u64,

    /// Original bundle bytes (for reference/debugging)
    #[serde(serialize_with = "hex_bytes_opt", deserialize_with = "from_hex_bytes_opt", skip_serializing_if = "Vec::is_empty", default)]
    pub bundle_bytes: Vec<u8>,

    /// State witnesses (for reference/debugging)
    pub witnesses: StateWitnesses,

    /// Pre-state payload (for Go conversion)
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub pre_state: Option<PreStatePayload>,

    /// Pre-state commitment leaves (for payload reconstruction)
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub pre_state_commitments: Vec<[u8; 32]>,

    /// Proofs for spent commitments (membership in pre-state tree)
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub spent_commitment_proofs: Vec<SpentCommitmentProof>,

    /// Post-state payload (for Go conversion)
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub post_state: Option<StateRootsPayload>,
}

/// Pre-state payload fields needed by the Go converter.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PreStatePayload {
    pub commitment_root: [u8; 32],
    pub commitment_size: u64,
    pub commitment_frontier: Vec<[u8; 32]>,
    pub nullifier_root: [u8; 32],
    pub nullifier_size: u64,

    // Transparent transaction state (added for transparent verification)
    #[serde(default, skip_serializing_if = "is_zero_hash")]
    pub transparent_merkle_root: [u8; 32],
    #[serde(default, skip_serializing_if = "is_zero_hash")]
    pub transparent_utxo_root: [u8; 32],
    #[serde(default)]
    pub transparent_utxo_size: u64,
}

/// State root fields used for post-state payloads.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StateRootsPayload {
    pub commitment_root: [u8; 32],
    pub commitment_size: u64,
    pub nullifier_root: [u8; 32],
    pub nullifier_size: u64,

    // Transparent transaction state (added for transparent verification)
    #[serde(default, skip_serializing_if = "is_zero_hash")]
    pub transparent_merkle_root: [u8; 32],
    #[serde(default, skip_serializing_if = "is_zero_hash")]
    pub transparent_utxo_root: [u8; 32],
    #[serde(default)]
    pub transparent_utxo_size: u64,
}

#[derive(Debug, Clone, Copy, Default)]
pub struct TransparentState {
    pub merkle_root: [u8; 32],
    pub utxo_root: [u8; 32],
    pub utxo_size: u64,
}

#[derive(Debug, Clone, Default)]
pub struct TransparentData {
    pub tx_data_extrinsic: Vec<u8>,
    pub pre_state: TransparentState,
    pub post_state: Option<TransparentState>,
}

/// Proof that a spent commitment exists in the pre-state commitment tree.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SpentCommitmentProof {
    /// Nullifier for the spent note
    pub nullifier: [u8; 32],
    /// Commitment value (leaf)
    pub commitment: [u8; 32],
    /// Position in commitment tree
    pub tree_position: u64,
    /// Merkle branch siblings (leaf -> root)
    pub branch_siblings: Vec<[u8; 32]>,
}

// Custom serializers for base64 encoding (standard for Go []byte)
fn hex_bytes<S>(bytes: &Vec<u8>, serializer: S) -> core::result::Result<S::Ok, S::Error>
where
    S: serde::Serializer,
{
    use base64::Engine;
    serializer.serialize_str(&base64::engine::general_purpose::STANDARD.encode(bytes))
}

fn from_hex_bytes<'de, D>(deserializer: D) -> core::result::Result<Vec<u8>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    use base64::Engine;
    let s: &str = serde::Deserialize::deserialize(deserializer)?;
    base64::engine::general_purpose::STANDARD
        .decode(s)
        .map_err(serde::de::Error::custom)
}

fn hex_bytes_opt<S>(bytes: &Vec<u8>, serializer: S) -> core::result::Result<S::Ok, S::Error>
where
    S: serde::Serializer,
{
    use base64::Engine;
    if bytes.is_empty() {
        serializer.serialize_str("")
    } else {
        serializer.serialize_str(&base64::engine::general_purpose::STANDARD.encode(bytes))
    }
}

fn from_hex_bytes_opt<'de, D>(deserializer: D) -> core::result::Result<Vec<u8>, D::Error>
where
    D: serde::Deserializer<'de>,
{
    use base64::Engine;
    let s: &str = serde::Deserialize::deserialize(deserializer)?;
    if s.is_empty() {
        Ok(Vec::new())
    } else {
        base64::engine::general_purpose::STANDARD
            .decode(s)
            .map_err(serde::de::Error::custom)
    }
}

// Helper for skip_serializing_if with zero hash check
fn is_zero_hash(hash: &[u8; 32]) -> bool {
    hash.iter().all(|&b| b == 0)
}

/// Complete JAM work package for Orchard service
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkPackage {
    /// Service ID (Orchard service index in JAM)
    pub service_id: u32,

    /// Work package payload
    pub payload: WorkPackagePayload,

    /// Authorization segment (empty for Orchard - auth is in bundle signatures)
    pub authorization: Vec<u8>,

    /// Gas limit
    pub gas_limit: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum WorkPackagePayload {
    /// Single Orchard bundle submission
    Submit(OrchardExtrinsic),

    /// Batch submission (multiple bundles)
    BatchSubmit(Vec<OrchardExtrinsic>),
}

/// Builder for constructing JAM work packages
pub struct WorkPackageBuilder {
    /// Builder's off-chain state (full trees and sets)
    builder_state: BuilderState,

    /// Service ID
    service_id: u32,
}

impl WorkPackageBuilder {
    /// Create new work package builder
    pub fn new(chain_state: OrchardState, service_id: u32) -> Self {
        Self {
            builder_state: BuilderState::new(chain_state),
            service_id,
        }
    }

    /// Create a builder with pre-populated trees (used for sequences).
    pub fn from_builder_state(builder_state: BuilderState, service_id: u32) -> Self {
        Self {
            builder_state,
            service_id,
        }
    }

    /// Sync builder state with latest on-chain state
    pub fn sync(&mut self, new_state: OrchardState) -> Result<()> {
        self.builder_state.sync(new_state)
    }

    /// Return the current chain state snapshot.
    pub fn chain_state(&self) -> &OrchardState {
        &self.builder_state.chain_state
    }

    /// Compute write intents using the current full builder state.
    pub fn compute_write_intents(&self, bundle: &OrchardBundle) -> Result<WriteIntents> {
        let mut temp_state = self.builder_state.clone();
        compute_write_intents(&mut temp_state, bundle)
    }

    /// Apply write intents to the internal builder state.
    pub fn apply_write_intents(&mut self, intents: &WriteIntents) -> Result<()> {
        apply_write_intents(&mut self.builder_state, intents)
    }

    /// Build work package from Orchard bundle
    pub fn build_work_package(
        &mut self,
        bundle: OrchardBundle,
        gas_limit: u64,
    ) -> Result<WorkPackage> {
        self.build_work_package_with_spent_positions_and_transparent(
            bundle,
            &[],
            gas_limit,
            None,
        )
    }

    /// Build work package with explicit spent note positions.
    pub fn build_work_package_with_spent_positions(
        &mut self,
        bundle: OrchardBundle,
        spent_positions: &[u64],
        gas_limit: u64,
    ) -> Result<WorkPackage> {
        self.build_work_package_with_spent_positions_and_transparent(
            bundle,
            spent_positions,
            gas_limit,
            None,
        )
    }

    pub fn build_work_package_with_transparent(
        &mut self,
        bundle: OrchardBundle,
        gas_limit: u64,
        transparent_data: Option<TransparentData>,
    ) -> Result<WorkPackage> {
        self.build_work_package_with_spent_positions_and_transparent(
            bundle,
            &[],
            gas_limit,
            transparent_data,
        )
    }

    pub fn build_work_package_with_spent_positions_and_transparent(
        &mut self,
        bundle: OrchardBundle,
        spent_positions: &[u64],
        gas_limit: u64,
        transparent_data: Option<TransparentData>,
    ) -> Result<WorkPackage> {
        let (transparent_tx_data_extrinsic, transparent_pre_state, transparent_post_state) =
            match transparent_data {
                Some(data) => (data.tx_data_extrinsic, data.pre_state, data.post_state),
                None => (Vec::new(), TransparentState::default(), None),
            };

        // 1. Serialize bundle to Zcash v5 format
        let bundle_bytes = serialize_bundle(&bundle)?;

        // 2. Generate state witnesses
        let witnesses = build_witnesses(&self.builder_state, &bundle)?;

        // 3. Verify witnesses are valid
        verify_witnesses(&self.builder_state.chain_state, &witnesses, &bundle)?;

        // 4. Build CompactBlock from bundle
        let compact_block_bytes = build_compact_block_from_bundle(&bundle, &self.builder_state)?;

        // 5. Compute post-state
        let write_intents = compute_write_intents(&mut self.builder_state.clone(), &bundle)?;
        let mut post_state_builder = self.builder_state.clone();
        apply_write_intents(&mut post_state_builder, &write_intents)?;
        let post_state = post_state_builder.chain_state.clone();
        let pre_nullifier_size = self.builder_state.nullifier_set.len() as u64;
        let post_nullifier_size = post_state_builder.nullifier_set.len() as u64;

        // 6. Serialize the 3 extrinsics (CompactBlock is derived from bundle_bytes)
        let pre_state_witness_extrinsic = serialize_pre_state_witness(&self.builder_state.chain_state, &witnesses, &bundle)?;
        let post_state_witness_extrinsic = serialize_post_state_witness(
            &post_state,
            &witnesses,
            &bundle,
            transparent_post_state.as_ref(),
        )?;
        let (vk_id, public_inputs, proof_bytes) =
            generate_bundle_proof(&bundle, &self.builder_state.chain_state)?;
        let bundle_proof_extrinsic = serialize_bundle_proof(vk_id, &public_inputs, &proof_bytes, &bundle_bytes);
        let pre_state_commitments = Vec::new();
        let commitment_frontier = self.builder_state.commitment_tree.frontier();
        let spent_nullifiers: Vec<[u8; 32]> = bundle
            .actions()
            .iter()
            .map(|action| action.nullifier().to_bytes())
            .collect();
        let spent_commitment_proofs = build_spent_commitment_proofs(
            &self.builder_state.commitment_tree,
            spent_positions,
            &spent_nullifiers,
        )?;

        // 7. Create extrinsic with 3 serialized forms (4th is optional for transparent txs)
        let post_state_transparent = transparent_post_state.unwrap_or_default();

        let extrinsic = OrchardExtrinsic {
            pre_state_witness_extrinsic,
            post_state_witness_extrinsic,
            bundle_proof_extrinsic,
            transparent_tx_data_extrinsic: transparent_tx_data_extrinsic,
            gas_limit,
            bundle_bytes, // Keep for debugging
            witnesses,    // Keep for debugging
            pre_state: Some(PreStatePayload {
                commitment_root: self.builder_state.chain_state.commitment_root,
                commitment_size: self.builder_state.chain_state.commitment_size,
                commitment_frontier: commitment_frontier.clone(),
                nullifier_root: self.builder_state.chain_state.nullifier_root,
                nullifier_size: pre_nullifier_size,
                transparent_merkle_root: transparent_pre_state.merkle_root,
                transparent_utxo_root: transparent_pre_state.utxo_root,
                transparent_utxo_size: transparent_pre_state.utxo_size,
            }),
            pre_state_commitments,
            spent_commitment_proofs,
            post_state: Some(StateRootsPayload {
                commitment_root: post_state.commitment_root,
                commitment_size: post_state.commitment_size,
                nullifier_root: post_state.nullifier_root,
                nullifier_size: post_nullifier_size,
                transparent_merkle_root: post_state_transparent.merkle_root,
                transparent_utxo_root: post_state_transparent.utxo_root,
                transparent_utxo_size: post_state_transparent.utxo_size,
            }),
        };

        // 7. Build work package
        Ok(WorkPackage {
            service_id: self.service_id,
            payload: WorkPackagePayload::Submit(extrinsic),
            authorization: vec![], // Orchard uses bundle signatures, no separate auth
            gas_limit,
        })
    }

    /// Build batch work package from multiple bundles
    pub fn build_batch_work_package(
        &mut self,
        bundles: Vec<(OrchardBundle, u64)>,
        total_gas: u64,
    ) -> Result<WorkPackage> {
        self.build_batch_work_package_with_transparent(bundles, total_gas, None)
    }

    /// Build batch work package from multiple bundles, with optional transparent state data.
    pub fn build_batch_work_package_with_transparent(
        &mut self,
        bundles: Vec<(OrchardBundle, u64)>,
        total_gas: u64,
        transparent_data: Option<TransparentData>,
    ) -> Result<WorkPackage> {
        let (transparent_tx_data_extrinsic, transparent_pre_state, transparent_post_state) =
            match transparent_data {
                Some(data) => (data.tx_data_extrinsic, data.pre_state, data.post_state),
                None => (Vec::new(), TransparentState::default(), None),
            };

        let mut extrinsics = Vec::new();
        let mut temp_state = self.builder_state.clone();
        let mut total_gas = 0u64;

        for (bundle, gas_limit) in bundles {
            let bundle_bytes = serialize_bundle(&bundle)?;
            let witnesses = build_witnesses(&temp_state, &bundle)?;
            verify_witnesses(&temp_state.chain_state, &witnesses, &bundle)?;

            let compact_block_bytes = build_compact_block_from_bundle(&bundle, &temp_state)?;

            let pre_nullifier_size = temp_state.nullifier_set.len() as u64;
            let pre_state = temp_state.chain_state.clone();
            let write_intents = compute_write_intents(&mut temp_state, &bundle)?;
            apply_write_intents(&mut temp_state, &write_intents)?;
            let post_state = temp_state.chain_state.clone();
            let post_nullifier_size = temp_state.nullifier_set.len() as u64;

            let pre_state_witness_extrinsic = serialize_pre_state_witness(&pre_state, &witnesses, &bundle)?;
            let post_state_witness_extrinsic = serialize_post_state_witness(
                &post_state,
                &witnesses,
                &bundle,
                transparent_post_state.as_ref(),
            )?;
            let (vk_id, public_inputs, proof_bytes) = generate_bundle_proof(&bundle, &pre_state)?;
            let bundle_proof_extrinsic = serialize_bundle_proof(vk_id, &public_inputs, &proof_bytes, &bundle_bytes);
            let pre_state_commitments = Vec::new();
            let commitment_frontier = temp_state.commitment_tree.frontier();
            let post_state_transparent = transparent_post_state.unwrap_or(transparent_pre_state);
            let transparent_extrinsic = if transparent_tx_data_extrinsic.is_empty() || !extrinsics.is_empty() {
                Vec::new()
            } else {
                transparent_tx_data_extrinsic.clone()
            };

            extrinsics.push(OrchardExtrinsic {
                pre_state_witness_extrinsic,
                post_state_witness_extrinsic,
                bundle_proof_extrinsic,
                transparent_tx_data_extrinsic: transparent_extrinsic,
                gas_limit,
                bundle_bytes,
                witnesses: witnesses.clone(),
                pre_state: Some(PreStatePayload {
                    commitment_root: pre_state.commitment_root,
                    commitment_size: pre_state.commitment_size,
                    commitment_frontier: commitment_frontier.clone(),
                    nullifier_root: pre_state.nullifier_root,
                    nullifier_size: pre_nullifier_size,
                    transparent_merkle_root: transparent_pre_state.merkle_root,
                    transparent_utxo_root: transparent_pre_state.utxo_root,
                    transparent_utxo_size: transparent_pre_state.utxo_size,
                }),
                pre_state_commitments,
                spent_commitment_proofs: Vec::new(),
                post_state: Some(StateRootsPayload {
                    commitment_root: post_state.commitment_root,
                    commitment_size: post_state.commitment_size,
                    nullifier_root: post_state.nullifier_root,
                    nullifier_size: post_nullifier_size,
                    transparent_merkle_root: post_state_transparent.merkle_root,
                    transparent_utxo_root: post_state_transparent.utxo_root,
                    transparent_utxo_size: post_state_transparent.utxo_size,
                }),
            });
            total_gas += gas_limit;
        }

        Ok(WorkPackage {
            service_id: self.service_id,
            payload: WorkPackagePayload::BatchSubmit(extrinsics),
            authorization: vec![],
            gas_limit: total_gas,
        })
    }

    /// Simulate execution and get updated state
    pub fn simulate_execution(&mut self, bundle: &OrchardBundle) -> Result<OrchardState> {
        let mut temp_state = self.builder_state.clone();
        let write_intents = compute_write_intents(&mut temp_state, bundle)?;
        apply_write_intents(&mut temp_state, &write_intents)?;
        Ok(temp_state.chain_state.clone())
    }
}

fn build_spent_commitment_proofs(
    commitment_tree: &IncrementalMerkleTree,
    spent_positions: &[u64],
    spent_nullifiers: &[[u8; 32]],
) -> Result<Vec<SpentCommitmentProof>> {
    if spent_positions.len() != spent_nullifiers.len() {
        return Err(Error::InvalidWitness(
            "Spent positions/nullifiers length mismatch".to_string()
        ));
    }
    let mut proofs = Vec::with_capacity(spent_positions.len());

    for (&position, nullifier) in spent_positions.iter().zip(spent_nullifiers.iter()) {
        let index = position as usize;
        let proof = commitment_tree
            .prove(index)
            .ok_or_else(|| Error::InvalidWitness("Missing commitment proof".to_string()))?;
        proofs.push(SpentCommitmentProof {
            nullifier: *nullifier,
            commitment: proof.leaf,
            tree_position: position,
            branch_siblings: proof.siblings,
        });
    }

    Ok(proofs)
}

#[derive(Clone, Copy, Debug, Default)]
struct Delta {
    asset_id: u32,
    in_public: u128,
    out_public: u128,
    burn_public: u128,
    mint_public: u128,
}

fn generate_bundle_proof(
    bundle: &OrchardBundle,
    chain_state: &OrchardState,
) -> Result<(u32, Vec<[u8; 32]>, Vec<u8>)> {
    // Generate Orchard NU5 public inputs (9 fields per action)
    let public_inputs = encode_orchard_nu5_public_inputs(
        &chain_state.commitment_root,
        bundle,
    )?;

    let proof_bytes = bundle.authorization().proof().as_ref().to_vec();
    Ok((1, public_inputs, proof_bytes))
}

/// Refine function - validates extrinsic and produces write intents
pub fn refine(
    extrinsic: &OrchardExtrinsic,
    chain_state: &OrchardState,
) -> Result<WriteIntents> {
    // 1. Deserialize bundle
    let bundle = deserialize_bundle(&extrinsic.bundle_bytes)?;

    // 2. Verify witnesses against chain state
    verify_witnesses(chain_state, &extrinsic.witnesses, &bundle)?;

    // 3. Compute state updates
    let mut builder_state = BuilderState::new(chain_state.clone());
    compute_write_intents(&mut builder_state, &bundle)
}

/// Accumulate function - applies write intents to state
pub fn accumulate(
    chain_state: &OrchardState,
    write_intents: &WriteIntents,
) -> Result<OrchardState> {
    let mut builder_state = BuilderState::new(chain_state.clone());
    apply_write_intents(&mut builder_state, write_intents)?;
    Ok(builder_state.chain_state.clone())
}

#[cfg(feature = "circuit")]
static PARAMS: OnceLock<Params<vesta::Affine>> = OnceLock::new();

#[cfg(feature = "circuit")]
fn get_or_load_params() -> &'static Params<vesta::Affine> {
    PARAMS.get_or_init(|| {
        let params_path = PathBuf::from("keys/orchard.params");
        if !params_path.exists() {
            panic!("Orchard params not found at {:?}. Run setup first.", params_path);
        }

        let params_bytes = fs::read(&params_path)
            .expect("Failed to read params file");

        Params::<vesta::Affine>::read(&mut &params_bytes[..])
            .expect("Failed to deserialize params")
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_builder_creation() {
        let state = OrchardState::default();
        let service_id = 42;

        let _builder = WorkPackageBuilder::new(state, service_id);

        // More tests would go here with actual bundle construction
    }

    #[test]
    fn test_refine_accumulate_roundtrip() {
        // Test that refine + accumulate produces consistent state
        let mut state = OrchardState::default();

        // Would construct actual bundle and extrinsic here
        // Then verify refine -> accumulate -> new state is consistent
    }
}

/// Build CompactBlock from Orchard bundle
fn build_compact_block_from_bundle(bundle: &OrchardBundle, builder_state: &BuilderState) -> Result<Vec<u8>> {
    // Extract data directly from bundle
    let nullifiers: Vec<Vec<u8>> = bundle.actions().iter()
        .map(|a| a.nullifier().to_bytes().to_vec())
        .collect();

    let commitments: Vec<Vec<u8>> = bundle.actions().iter()
        .map(|a| a.cmx().to_bytes().to_vec())
        .collect();

    // Create CompactBlock structure for CBOR serialization
    #[derive(Serialize)]
    struct CompactOutput {
        commitment: Vec<u8>,
        ephemeral_key: Vec<u8>,
        ciphertext: Vec<u8>,
    }

    #[derive(Serialize)]
    struct CompactTx {
        index: u64,
        txid: Vec<u8>,
        nullifiers: Vec<Vec<u8>>,
        commitments: Vec<CompactOutput>,
    }

    #[derive(Serialize)]
    struct ChainMetadata {
        orchard_commitment_tree_size: u32,
    }

    #[derive(Serialize)]
    struct CompactBlock {
        proto_version: u32,
        height: u64,
        hash: Vec<u8>,
        prev_hash: Vec<u8>,
        time: u32,
        header: Vec<u8>,
        vtx: Vec<CompactTx>,
        chain_metadata: ChainMetadata,
    }

    let compact_outputs: Vec<CompactOutput> = commitments.iter().map(|c| {
        CompactOutput {
            commitment: c.clone(),
            ephemeral_key: vec![0u8; 32],
            ciphertext: Vec::new(),
        }
    }).collect();

    let compact_tx = CompactTx {
        index: 0,
        txid: vec![0u8; 32],
        nullifiers,
        commitments: compact_outputs,
    };

    let compact_block = CompactBlock {
        proto_version: 1,
        height: builder_state.chain_state.commitment_size,
        hash: vec![0u8; 32],
        prev_hash: vec![0u8; 32],
        time: 0,
        header: Vec::new(),
        vtx: vec![compact_tx],
        chain_metadata: ChainMetadata {
            orchard_commitment_tree_size: (builder_state.chain_state.commitment_size + commitments.len() as u64) as u32,
        },
    };

    // Serialize to CBOR
    serde_cbor::to_vec(&compact_block)
        .map_err(|e| Error::SerializationError(format!("Failed to serialize CompactBlock: {}", e)))
}

/// Serialize PreStateWitness extrinsic (tag=1)
fn serialize_pre_state_witness(
    chain_state: &OrchardState,
    witnesses: &StateWitnesses,
    bundle: &OrchardBundle,
) -> Result<Vec<u8>> {
    const TAG_PRE_STATE_WITNESS: u8 = 1;

    // Extract nullifiers from bundle
    let nullifiers: Vec<[u8; 32]> = bundle
        .actions()
        .iter()
        .map(|action| action.nullifier().to_bytes())
        .collect();

    // Serialize WitnessBundle for pre-state
    // Format matches services/orchard/src/witness.rs WitnessBundle::serialize
    let mut witness_bytes = Vec::new();

    // Pre-state root (current state)
    witness_bytes.extend_from_slice(&chain_state.commitment_root);

    // Post-state root (same for pre-witness)
    witness_bytes.extend_from_slice(&chain_state.commitment_root);

    // Number of reads: nullifiers only (no JAM witnesses for state roots)
    let num_reads = nullifiers.len() as u32;
    witness_bytes.extend_from_slice(&num_reads.to_le_bytes());

    // StateRead 1+: Each nullifier absence proof
    for (i, nullifier) in nullifiers.iter().enumerate() {
        let key = format!("nullifier_{}", hex::encode(nullifier)); // Full 32 bytes as hex
        let value = &[0u8; 32]; // Absence proof: value is empty (0s)

        // Get the corresponding absence proof
        let proof = if i < witnesses.nullifier_absence_proofs.len() {
            &witnesses.nullifier_absence_proofs[i].siblings
        } else {
            &[] as &[[u8; 32]]
        };

        serialize_state_read(&mut witness_bytes, &key, value, proof)?;
    }

    // Number of writes (0 for pre-state)
    witness_bytes.extend_from_slice(&0u32.to_le_bytes());

    // Wrap in extrinsic format (tag + length + data)
    let mut extrinsic = Vec::with_capacity(1 + 4 + witness_bytes.len());
    extrinsic.push(TAG_PRE_STATE_WITNESS);
    extrinsic.extend_from_slice(&(witness_bytes.len() as u32).to_le_bytes());
    extrinsic.extend_from_slice(&witness_bytes);

    Ok(extrinsic)
}

/// Helper to serialize a StateRead
fn serialize_state_read(
    buf: &mut Vec<u8>,
    key: &str,
    value: &[u8],
    proof: &[[u8; 32]],
) -> Result<()> {
    // Key length + key
    buf.extend_from_slice(&(key.len() as u32).to_le_bytes());
    buf.extend_from_slice(key.as_bytes());

    // Value length + value
    buf.extend_from_slice(&(value.len() as u32).to_le_bytes());
    buf.extend_from_slice(value);

    // Proof length + proof
    buf.extend_from_slice(&(proof.len() as u32).to_le_bytes());
    for node in proof {
        buf.extend_from_slice(node);
    }

    Ok(())
}

/// Helper to serialize a StateWrite
fn serialize_state_write(
    buf: &mut Vec<u8>,
    key: &str,
    old_value: Option<&[u8]>,
    new_value: &[u8],
    proof: &[[u8; 32]],
) -> Result<()> {
    // Key length + key
    buf.extend_from_slice(&(key.len() as u32).to_le_bytes());
    buf.extend_from_slice(key.as_bytes());

    // Old value (optional with flag byte)
    if let Some(old_val) = old_value {
        buf.push(1); // Has old value
        buf.extend_from_slice(&(old_val.len() as u32).to_le_bytes());
        buf.extend_from_slice(old_val);
    } else {
        buf.push(0); // No old value
    }

    // New value length + new value
    buf.extend_from_slice(&(new_value.len() as u32).to_le_bytes());
    buf.extend_from_slice(new_value);

    // Proof length + proof
    buf.extend_from_slice(&(proof.len() as u32).to_le_bytes());
    for node in proof {
        buf.extend_from_slice(node);
    }

    Ok(())
}

/// Serialize PostStateWitness extrinsic (tag=2)
/// Expanded format now includes transparent state roots for validator verification
fn serialize_post_state_witness(
    post_state: &OrchardState,
    _witnesses: &StateWitnesses,
    _bundle: &OrchardBundle,
    transparent_state: Option<&TransparentState>,
) -> Result<Vec<u8>> {
    let (transparent_merkle_root, transparent_utxo_root, transparent_utxo_size) =
        match transparent_state {
            Some(state) => (state.merkle_root, state.utxo_root, state.utxo_size),
            None => ([0u8; 32], [0u8; 32], 0),
        };

    serialize_post_state_witness_with_transparent(
        post_state,
        &transparent_merkle_root,
        &transparent_utxo_root,
        transparent_utxo_size,
    )
}

/// Serialize PostStateWitness with transparent roots (expanded format matching Go)
fn serialize_post_state_witness_with_transparent(
    post_state: &OrchardState,
    transparent_merkle_root: &[u8; 32],
    transparent_utxo_root: &[u8; 32],
    transparent_utxo_size: u64,
) -> Result<Vec<u8>> {
    const TAG_POST_STATE_WITNESS: u8 = 2;

    // Serialize WitnessBundle for post-state
    // Format: pre_nullifier (32) + post_commitment (32) + reads (4) + writes (4)
    //       + transparent_merkle (32) + transparent_utxo (32) + transparent_size (8)
    let mut witness_bytes = Vec::new();

    // Shielded state (existing)
    witness_bytes.extend_from_slice(&[0u8; 32]); // pre-nullifier root (unused, zeros)
    witness_bytes.extend_from_slice(&post_state.commitment_root); // post-commitment root
    witness_bytes.extend_from_slice(&0u32.to_le_bytes()); // state read count
    witness_bytes.extend_from_slice(&0u32.to_le_bytes()); // state write count

    // Transparent state (NEW - matches Go format)
    witness_bytes.extend_from_slice(transparent_merkle_root);
    witness_bytes.extend_from_slice(transparent_utxo_root);
    witness_bytes.extend_from_slice(&transparent_utxo_size.to_le_bytes());

    // Wrap in extrinsic format (tag + length + data)
    let mut extrinsic = Vec::with_capacity(1 + 4 + witness_bytes.len());
    extrinsic.push(TAG_POST_STATE_WITNESS);
    extrinsic.extend_from_slice(&(witness_bytes.len() as u32).to_le_bytes());
    extrinsic.extend_from_slice(&witness_bytes);

    Ok(extrinsic)
}

/// Serialize BundleProof extrinsic (tag=3)
fn serialize_bundle_proof(
    vk_id: u32,
    public_inputs: &[[u8; 32]],
    proof_bytes: &[u8],
    bundle_bytes: &[u8],
) -> Vec<u8> {
    const TAG_BUNDLE_PROOF: u8 = 3;

    let mut extrinsic = Vec::new();
    extrinsic.push(TAG_BUNDLE_PROOF);
    extrinsic.extend_from_slice(&vk_id.to_le_bytes());
    extrinsic.extend_from_slice(&(public_inputs.len() as u32).to_le_bytes());
    for input in public_inputs {
        extrinsic.extend_from_slice(input);
    }
    extrinsic.extend_from_slice(&(proof_bytes.len() as u32).to_le_bytes());
    extrinsic.extend_from_slice(proof_bytes);
    extrinsic.extend_from_slice(&(bundle_bytes.len() as u32).to_le_bytes());
    extrinsic.extend_from_slice(bundle_bytes);
    extrinsic
}

/// Encode Orchard NU5 public inputs (9 fields per action)
///
/// Per Orchard circuit Instance layout:
/// 1. anchor: Commitment tree root
/// 2. cv_net_x: Net value commitment x-coordinate
/// 3. cv_net_y: Net value commitment y-coordinate
/// 4. nf_old: Nullifier of spent note
/// 5. rk_x: Randomized verification key x-coordinate
/// 6. rk_y: Randomized verification key y-coordinate
/// 7. cmx: New note commitment
/// 8. enable_spend: Spend enabled flag (0 or 1)
/// 9. enable_output: Output enabled flag (0 or 1)
fn encode_orchard_nu5_public_inputs(
    anchor: &[u8; 32],
    bundle: &OrchardBundle,
) -> Result<Vec<[u8; 32]>> {
    let mut inputs = Vec::with_capacity(bundle.actions().len() * 9);

    for action in bundle.actions() {
        // Field 1: anchor (commitment tree root)
        inputs.push(*anchor);

        // Fields 2-3: cv_net (net value commitment x/y)
        let cv_net_bytes = action.cv_net().to_bytes();
        let (cv_net_x, cv_net_y) =
            point_bytes_to_xy(&cv_net_bytes, true, "cv_net")?;
        inputs.push(cv_net_x);
        inputs.push(cv_net_y);

        // Field 4: nf_old (nullifier)
        inputs.push(action.nullifier().to_bytes());

        // Fields 5-6: rk (randomized verification key x/y)
        let rk_bytes: [u8; 32] = action.rk().into();
        let (rk_x, rk_y) = point_bytes_to_xy(&rk_bytes, false, "rk")?;
        inputs.push(rk_x);
        inputs.push(rk_y);

        // Field 7: cmx (new note commitment)
        inputs.push(action.cmx().to_bytes());

        // Field 8: enable_spend (1 = spend enabled)
        inputs.push(u32_to_field_bytes(1));

        // Field 9: enable_output (1 = output enabled)
        inputs.push(u32_to_field_bytes(1));
    }

    Ok(inputs)
}

fn u32_to_field_bytes(value: u32) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[0..4].copy_from_slice(&value.to_le_bytes());
    bytes
}

fn u64_to_field_bytes(value: u64) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[0..8].copy_from_slice(&value.to_le_bytes());
    bytes
}

fn u128_lo_to_field_bytes(value: u128) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[0..8].copy_from_slice(&(value as u64).to_le_bytes());
    bytes
}

fn u128_hi_to_field_bytes(value: u128) -> [u8; 32] {
    let mut bytes = [0u8; 32];
    bytes[0..8].copy_from_slice(&((value >> 64) as u64).to_le_bytes());
    bytes
}

fn point_bytes_to_xy(
    bytes: &[u8; 32],
    allow_identity: bool,
    label: &str,
) -> Result<([u8; 32], [u8; 32])> {
    let point = pallas::Point::from_bytes(bytes)
        .into_option()
        .ok_or_else(|| Error::InvalidBundle(format!("Invalid {} point bytes", label)))?;
    let coords = point.to_affine().coordinates().into_option();

    if let Some(coords) = coords {
        Ok((coords.x().to_repr(), coords.y().to_repr()))
    } else if allow_identity {
        let zero = pallas::Base::from(0u64).to_repr();
        Ok((zero, zero))
    } else {
        Err(Error::InvalidBundle(format!("{} point is identity", label)))
    }
}
