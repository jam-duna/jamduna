//! Structs related to swap bundles.

use crate::{
    bundle::commitments::hash_swap_bundle,
    bundle::{derive_bvk, Authorization, Bundle, BundleCommitment},
    circuit::VerifyingKey,
    orchard_flavor::OrchardZSA,
    primitives::redpallas::{self, Binding},
    value::ValueCommitTrapdoor,
    Proof,
};
use alloc::vec::Vec;

use k256::elliptic_curve::rand_core::{CryptoRng, RngCore};
use nonempty::NonEmpty;
use crate::orchard_sighash_versioning::{VerBindingSig, VerSpendAuthSig};
use crate::primitives::OrchardPrimitives;

/// A swap bundle to be applied to the ledger.
#[derive(Debug, Clone)]
pub struct SwapBundle<V> {
    /// The list of action groups that make up this swap bundle.
    action_groups: Vec<Bundle<ActionGroupAuthorized, V, OrchardZSA>>,
    /// The net value moved out of this swap.
    ///
    /// This is the sum of Orchard spends minus the sum of Orchard outputs.
    value_balance: V,
    /// The binding signature for this swap.
    binding_signature: VerBindingSig,
}

impl<V> SwapBundle<V> {
    /// Constructs a `SwapBundle` from its constituent parts.
    pub fn from_parts(
        action_groups: Vec<Bundle<ActionGroupAuthorized, V, OrchardZSA>>,
        value_balance: V,
        binding_signature: VerBindingSig,
    ) -> Self {
        SwapBundle {
            action_groups,
            value_balance,
            binding_signature,
        }
    }
}

impl<V: Copy + Into<i64> + std::iter::Sum> SwapBundle<V> {
    /// Constructs a `SwapBundle` from its action groups and respective binding signature keys.
    /// Keys should go in the same order as the action groups.
    pub fn new<R: RngCore + CryptoRng>(
        rng: R,
        action_groups: Vec<Bundle<ActionGroupAuthorized, V, OrchardZSA>>,
        bsks: Vec<redpallas::SigningKey<Binding>>,
    ) -> Self {
        assert_eq!(action_groups.len(), bsks.len());
        // Evaluate the swap value balance by summing the value balance of each action group.
        let value_balance = action_groups.iter().map(|a| *a.value_balance()).sum();
        // Evaluate the swap bsk by summing the bsk of each action group.
        let bsk = bsks
            .into_iter()
            .map(ValueCommitTrapdoor::from_bsk)
            .sum::<ValueCommitTrapdoor>()
            .into_bsk();
        // Evaluate the swap sighash
        let sighash: [u8; 32] = BundleCommitment(hash_swap_bundle(
            action_groups.iter().collect(),
            value_balance,
        ))
        .into();
        // Evaluate the swap binding signature which is equal to the signature of the swap sigash
        // with the swap binding signature key bsk.
        let binding_signature = VerBindingSig::new(
            OrchardZSA::default_sighash_version(),
            bsk.sign(rng, &sighash),
        );
        // Create the swap bundle
        SwapBundle {
            action_groups,
            value_balance,
            binding_signature,
        }
    }
}

/// Authorizing data for an action group, ready to be sent to the matcher.
#[derive(Debug, Clone)]
pub struct ActionGroupAuthorized {
    proof: Proof,
}

impl Authorization for ActionGroupAuthorized {
    type SpendAuth = VerSpendAuthSig;

    /// Return the proof component of the authorizing data.
    fn proof(&self) -> Option<&Proof> {
        Some(&self.proof)
    }
}

impl ActionGroupAuthorized {
    /// Constructs the authorizing data for an action group from its proof.
    pub fn from_parts(proof: Proof) -> Self {
        ActionGroupAuthorized { proof }
    }
}

impl<V, D: OrchardPrimitives> Bundle<ActionGroupAuthorized, V, D> {
    /// Verifies the proof for this bundle.
    pub fn verify_proof(&self, vk: &VerifyingKey) -> Result<(), halo2_proofs::plonk::Error> {
        self.authorization()
            .proof()
            .unwrap()
            .verify(vk, &self.to_instances())
    }
}

impl<V> SwapBundle<V> {
    /// Returns the list of action groups that make up this swap bundle.
    pub fn action_groups(&self) -> &Vec<Bundle<ActionGroupAuthorized, V, OrchardZSA>> {
        &self.action_groups
    }

    /// Returns the binding signature of this swap bundle.
    pub fn binding_signature(&self) -> &VerBindingSig {
        &self.binding_signature
    }

    /// The net value moved out of this swap.
    ///
    /// This is the sum of Orchard spends minus the sum of Orchard outputs.
    pub fn value_balance(&self) -> &V {
        &self.value_balance
    }
}

impl<V: Copy + Into<i64>> SwapBundle<V> {
    /// Computes a commitment to the effects of this swap bundle, suitable for inclusion
    /// within a transaction ID.
    pub fn commitment(&self) -> BundleCommitment {
        BundleCommitment(hash_swap_bundle(
            self.action_groups.iter().collect(),
            self.value_balance,
        ))
    }

    /// Returns the transaction binding validating key for this swap bundle.
    pub fn binding_validating_key(&self) -> redpallas::VerificationKey<Binding> {
        let actions = self
            .action_groups
            .iter()
            .flat_map(|ag| ag.actions().iter().cloned())
            .collect::<Vec<_>>();

        let burn = self
            .action_groups
            .iter()
            .flat_map(|ag| ag.burn().iter().cloned())
            .collect::<Vec<_>>();

        derive_bvk(
            &NonEmpty::from_vec(actions).expect("SwapBundle must have at least one action"),
            self.value_balance,
            &burn,
        )
    }
}
