// JAM Work Package Builder for Orchard Service
//
// Pure Rust implementation for building complete JAM work packages containing
// Orchard bundles and state witnesses. This eliminates FFI complexity by keeping
// all Orchard logic in Rust.

use crate::bundle_codec::{deserialize_bundle, serialize_bundle};
use crate::state::{OrchardState, StateWitnesses, WriteIntents};
use crate::witness::{
    apply_write_intents, build_witnesses, compute_write_intents, verify_witnesses, BuilderState,
};
use crate::{Error, Result, OrchardBundle};
use serde::{Deserialize, Serialize};
#[cfg(feature = "circuit")]
use halo2_proofs::poly::commitment::Params;
#[cfg(feature = "circuit")]
use pasta_curves::vesta::Affine as VestaAffine;
#[cfg(feature = "circuit")]
use std::io::BufReader;
#[cfg(feature = "circuit")]
use std::path::PathBuf;
#[cfg(feature = "circuit")]
use std::sync::OnceLock;

/// Orchard extrinsic submitted to JAM service
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OrchardExtrinsic {
    /// Zcash v5 Orchard bundle encoding (byte-for-byte compatible)
    pub bundle_bytes: Vec<u8>,

    /// Gas limit for execution
    pub gas_limit: u64,

    /// State witnesses (pre-execution Merkle proofs)
    pub witnesses: StateWitnesses,

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

    /// Sync builder state with latest on-chain state
    pub fn sync(&mut self, new_state: OrchardState) -> Result<()> {
        self.builder_state.sync(new_state)
    }

    /// Build work package from Orchard bundle
    pub fn build_work_package(
        &mut self,
        bundle: OrchardBundle,
        gas_limit: u64,
    ) -> Result<WorkPackage> {
        // 1. Serialize bundle to Zcash v5 format
        let bundle_bytes = serialize_bundle(&bundle)?;

        // 2. Generate state witnesses
        let witnesses = build_witnesses(&self.builder_state, &bundle)?;

        // 3. Verify witnesses are valid
        verify_witnesses(&self.builder_state.chain_state, &witnesses, &bundle)?;

        // 4. Create extrinsic
        let extrinsic = OrchardExtrinsic {
            bundle_bytes,
            gas_limit,
            witnesses,
        };

        // 5. Validate extrinsic
        self.validate_extrinsic(&self.builder_state.chain_state, &extrinsic, &bundle)?;

        // 6. Build work package
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
        bundles: Vec<(OrchardBundle, u64)>, // (bundle, gas_limit)
    ) -> Result<WorkPackage> {
        if bundles.is_empty() {
            return Err(Error::InvalidBundle("Empty batch payload".to_string()));
        }
        let mut temp_state = self.builder_state.clone();
        let mut extrinsics = Vec::new();
        let mut total_gas = 0u64;

        for (bundle, gas_limit) in bundles {
            let bundle_bytes = serialize_bundle(&bundle)?;
            let witnesses = build_witnesses(&temp_state, &bundle)?;
            verify_witnesses(&temp_state.chain_state, &witnesses, &bundle)?;

            let extrinsic = OrchardExtrinsic {
                bundle_bytes,
                gas_limit,
                witnesses,
            };

            self.validate_extrinsic(&temp_state.chain_state, &extrinsic, &bundle)?;
            extrinsics.push(extrinsic);
            total_gas = total_gas
                .checked_add(gas_limit)
                .ok_or_else(|| Error::InvalidBundle("Batch gas_limit overflow".to_string()))?;

            let intents = compute_write_intents(&mut temp_state, &bundle)?;
            apply_write_intents(&mut temp_state, &intents)?;
        }

        Ok(WorkPackage {
            service_id: self.service_id,
            payload: WorkPackagePayload::BatchSubmit(extrinsics),
            authorization: vec![],
            gas_limit: total_gas,
        })
    }

    /// Validate extrinsic before packaging
    fn validate_extrinsic(
        &self,
        state: &OrchardState,
        extrinsic: &OrchardExtrinsic,
        bundle: &OrchardBundle,
    ) -> Result<()> {
        // Verify gas limit in range
        if extrinsic.gas_limit < state.gas_min || extrinsic.gas_limit > state.gas_max {
            return Err(Error::InvalidBundle("Gas limit out of range".to_string()));
        }

        // Verify zero-sum value balance
        verify_zero_sum_value_balance(bundle)?;

        Ok(())
    }

    /// Get current builder state
    pub fn state(&self) -> &BuilderState {
        &self.builder_state
    }
}

/// Refine a full work package, including batch payloads.
pub fn refine_work_package(
    work_package: &WorkPackage,
    chain_state: &OrchardState,
) -> Result<WriteIntents> {
    match &work_package.payload {
        WorkPackagePayload::Submit(extrinsic) => {
            if work_package.gas_limit != extrinsic.gas_limit {
                return Err(Error::InvalidBundle(
                    "Work package gas_limit mismatch".to_string(),
                ));
            }
            refine_orchard(extrinsic, chain_state)
        }
        WorkPackagePayload::BatchSubmit(extrinsics) => {
            if extrinsics.is_empty() {
                return Err(Error::InvalidBundle("Empty batch payload".to_string()));
            }
            let total_gas = extrinsics
                .iter()
                .map(|ext| ext.gas_limit)
                .try_fold(0u64, |acc, gas| acc.checked_add(gas))
                .ok_or_else(|| Error::InvalidBundle("Batch gas_limit overflow".to_string()))?;
            if total_gas != work_package.gas_limit {
                return Err(Error::InvalidBundle(
                    "Work package gas_limit mismatch".to_string(),
                ));
            }
            refine_orchard_batch(extrinsics, chain_state)
        }
    }
}

fn refine_orchard_batch(
    extrinsics: &[OrchardExtrinsic],
    chain_state: &OrchardState,
) -> Result<WriteIntents> {
    let mut state = chain_state.clone();
    let mut new_commitments = Vec::new();
    let mut new_nullifiers = Vec::new();

    for extrinsic in extrinsics {
        let intents = refine_orchard(extrinsic, &state)?;
        new_commitments.extend(intents.new_commitments.iter().copied());
        new_nullifiers.extend(intents.new_nullifiers.iter().copied());
        accumulate_orchard(&mut state, &intents)?;
    }

    Ok(WriteIntents {
        new_commitments,
        new_nullifiers,
        expected_commitment_root: state.commitment_root,
        expected_nullifier_root: state.nullifier_root,
    })
}

/// Refine function - stateless verification of Orchard extrinsic
pub fn refine_orchard(
    extrinsic: &OrchardExtrinsic,
    chain_state: &OrchardState,
) -> Result<WriteIntents> {
    // 1. Deserialize bundle
    let bundle = deserialize_bundle(&extrinsic.bundle_bytes)?;

    // 2. Verify witnesses against chain state
    verify_witnesses(chain_state, &extrinsic.witnesses, &bundle)?;

    // 3. Verify gas policy
    verify_gas(extrinsic, &extrinsic.witnesses)?;

    // 4. Verify Orchard bundle proof
    verify_orchard_bundle(&bundle)?;

    // 5. Verify zero-sum value balance
    verify_zero_sum_value_balance(&bundle)?;

    // 6. Compute state transition write intents
    let mut temp_builder_state = BuilderState::new(chain_state.clone());
    compute_write_intents(
        &mut temp_builder_state,
        &bundle,
    )
}

/// Accumulate function - apply write intents to state
pub fn accumulate_orchard(
    state: &mut OrchardState,
    intents: &WriteIntents,
) -> Result<()> {
    use crate::state::compute_post_state;

    let updates = compute_post_state(state, intents)?;

    state.commitment_root = updates.new_commitment_root;
    state.commitment_size = updates.new_commitment_size;
    state.nullifier_root = updates.new_nullifier_root;

    Ok(())
}

// Helper functions

fn verify_gas(extrinsic: &OrchardExtrinsic, witnesses: &StateWitnesses) -> Result<()> {
    if extrinsic.gas_limit < witnesses.gas_min || extrinsic.gas_limit > witnesses.gas_max {
        return Err(Error::InvalidBundle("Gas limit out of range".to_string()));
    }

    Ok(())
}

#[cfg(feature = "circuit")]
pub(crate) fn verify_orchard_bundle(bundle: &OrchardBundle) -> Result<()> {
    static ORCHARD_VK: OnceLock<orchard::circuit::VerifyingKey> = OnceLock::new();
    let vk = ORCHARD_VK.get_or_init(|| {
        load_or_build_verifying_key().unwrap_or_else(|_| orchard::circuit::VerifyingKey::build())
    });

    bundle
        .verify_proof(vk)
        .map_err(|e| Error::InvalidBundle(format!("Proof verification failed: {:?}", e)))?;

    verify_bundle_signatures(bundle)?;

    Ok(())
}

#[cfg(not(feature = "circuit"))]
pub(crate) fn verify_orchard_bundle(_bundle: &OrchardBundle) -> Result<()> {
    Err(Error::InvalidBundle(
        "Circuit feature not enabled - cannot verify Halo2 proofs".to_string(),
    ))
}

fn verify_bundle_signatures(bundle: &OrchardBundle) -> Result<()> {
    let sighash: [u8; 32] = bundle.commitment().into();

    for (idx, action) in bundle.actions().iter().enumerate() {
        action
            .rk()
            .verify(&sighash, action.authorization())
            .map_err(|_| {
                Error::InvalidBundle(format!(
                    "Spend authorization signature invalid at action {}",
                    idx
                ))
            })?;
    }

    bundle
        .binding_validating_key()
        .verify(&sighash, bundle.authorization().binding_signature())
        .map_err(|_| Error::InvalidBundle("Binding signature invalid".to_string()))?;

    Ok(())
}

#[cfg(feature = "circuit")]
fn load_or_build_verifying_key() -> std::io::Result<orchard::circuit::VerifyingKey> {
    const ORCHARD_K: u32 = 11;
    const DEFAULT_KEYS_DIR: &str = "keys/orchard";
    const PARAMS_FILE: &str = "orchard.params";
    const VK_FILE: &str = "orchard.vk";

    let keys_dir = PathBuf::from(DEFAULT_KEYS_DIR);
    let params_path = keys_dir.join(PARAMS_FILE);
    let vk_path = keys_dir.join(VK_FILE);

    let params = if params_path.exists() {
        let file = std::fs::File::open(&params_path)?;
        let params = Params::<VestaAffine>::read(&mut BufReader::new(file))?;
        if params.k() != ORCHARD_K {
            return Err(std::io::Error::new(
                std::io::ErrorKind::Other,
                "params k does not match Orchard circuit",
            ));
        }
        params
    } else {
        Params::new(ORCHARD_K)
    };

    if vk_path.exists() {
        let file = std::fs::File::open(&vk_path)?;
        orchard::circuit::VerifyingKey::read(params, BufReader::new(file))
    } else {
        Ok(orchard::circuit::VerifyingKey::build_with_params(params))
    }
}

fn verify_zero_sum_value_balance(bundle: &OrchardBundle) -> Result<()> {
    let value_balance = *bundle.value_balance();

    if value_balance != 0 {
        return Err(Error::InvalidBundle(
            format!("Value balance {} != 0", value_balance)
        ));
    }

    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_work_package_builder() {
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
