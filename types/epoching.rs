#![cfg_attr(not(feature = "std"), no_std)]

use frame_support::{decl_event, decl_module, dispatch::DispatchResult};
use frame_system::ensure_signed;

/// MsgUpdateParams represents a message for updating parameters.
pub struct MsgUpdateParams<T: Trait> {
    // `authority` is the address of the governance account.
    // just FYI: cosmos.AddressString marks that this field should use type alias
    // for AddressString instead of string, but the functionality is not yet implemented
    // in cosmos-proto
    authority: T::AccountId,

    // `params` defines the epoching parameters to update.
    //
    // NOTE: All parameters must be supplied.
    params: EpochParams,
}

// EpochParams defines the parameters for the module.
struct EpochParams {
    // epoch_interval is the number of consecutive blocks to form an epoch
    epoch_interval: u64,
}

// InjectedCheckpoint wraps the checkpoint and the extended votes
pub struct InjectedCheckpoint<T: Trait> {
    // RawCheckpointWithMeta is the checkpoint with additional metadata
    ckpt: RawCheckpointWithMeta,
    // extended_commit_info is the commit info including the vote extensions
    // from the previous proposal
    extended_commit_info: tendermint::abci::ExtendedCommitInfo,
}

/// VoteExtension defines the structure used to create a BLS vote extension.
pub struct VoteExtension<T: Trait> {
    /// signer is the address of the vote extension signer
    signer: T::AccountId,
    /// validator_address is the address of the validator
    validator_address: T::AccountId,
    /// block_hash is the hash of the block that the vote extension is signed over
    block_hash: Vec<u8>,
    /// epoch_num is the epoch number of the vote extension
    epoch_num: u64,
    /// height is the height of the vote extension
    height: u64,
    /// bls_sig is the BLS signature
    bls_sig: Vec<u8>,
}

/// RawCheckpoint wraps the BLS multi sig with metadata
struct RawCheckpoint {
    /// epoch_num defines the epoch number the raw checkpoint is for
    epoch_num: u64,
    /// block_hash defines the 'BlockID.Hash', which is the hash of
    /// the block that individual BLS sigs are signed on
    block_hash: Vec<u8>, // BlockHash - Consider using a fixed-size array if block_hash has a fixed length
    /// bitmap defines the bitmap that indicates the signers of the BLS multi sig
    bitmap: Vec<u8>, // Consider using a fixed-size array if bitmap has a fixed length
    /// bls_multi_sig defines the multi sig that is aggregated from individual BLS sigs
    bls_multi_sig: Vec<u8>, // "github.com/babylonchain/babylon/crypto/bls12381.Signature"
    // Consider using a fixed-size array if bls_multi_sig has a fixed length
}

// RawCheckpointWithMeta wraps the raw checkpoint with metadata.
struct RawCheckpointWithMeta {
    // RawCheckpoint ckpt = 1;
    ckpt: RawCheckpoint,
    // status defines the status of the checkpoint
    status: CheckpointStatus,
    // bls_aggr_pk defines the aggregated BLS public key
    bls_aggr_pk: Vec<u8>, // This would be replaced with the appropriate type for BLS public key
    // power_sum defines the accumulated voting power for the checkpoint
    power_sum: u64,
    // lifecycle defines the lifecycle of this checkpoint, i.e., each state
    // transition and the time (in both timestamp and block height) of this
    // transition.
    lifecycle: Vec<CheckpointStateUpdate>,
}



// ValidatorWithBLSSet defines a set of validators with their BLS public keys
struct ValidatorWithBlsKeySet {
    // val_set is a list of validators with their BLS keys
    val_set: Vec<ValidatorWithBlsKey>,
}

// ValidatorWithBlsKey couples validator address, voting power, and its bls public key
struct ValidatorWithBlsKey {
    // validator_address is the address of the validator
    validator_address: String,
    // bls_pub_key is the BLS public key of the validator
    bls_pub_key: Vec<u8>,
    // voting_power is the voting power of the validator at the given epoch
    voting_power: u64,
}

// GenesisState defines the checkpointing module's genesis state.
struct GenesisState {
    // genesis_keys defines the public keys for the genesis validators
    genesis_keys: Vec<GenesisKey>,
}

// GenesisKey defines public key information about the genesis validators
struct GenesisKey {
    // validator_address is the address corresponding to a validator
    validator_address: String,
    // bls_key defines the BLS key of the validator at genesis
    bls_key: BlsKey,
    // val_pubkey defines the ed25519 public key of the validator at genesis
    val_pubkey: cosmos_crypto_ed25519_PubKey,
}

// BlsKey defines the BLS key type.
struct BlsKey {
  // TODO
}

// cosmos_crypto_ed25519_PubKey defines the ed25519 public key type.
struct cosmos_crypto_ed25519_PubKey {
    // TODO
}

// Define the CheckpointStatus enum separately
enum CheckpointStatus {
    // Your CheckpointStatus enum variants go here
}

// Define the CheckpointStateUpdate struct separately
struct CheckpointStateUpdate {
    // Your CheckpointStateUpdate struct definition goes here
}

/// Epoch is a structure that contains the metadata of an epoch
pub struct Epoch {
    /// epoch_number is the number of this epoch
    pub epoch_number: u64,
    /// current_epoch_interval is the epoch interval at the time of this epoch
    pub current_epoch_interval: u64,
    /// first_block_height is the height of the first block in this epoch
    pub first_block_height: u64,
    /// last_block_time is the time of the last block in this epoch.
    /// Babylon needs to remember the last header's time of each epoch to complete unbonding validators/delegations when a previous epoch's checkpoint is finalised.
    /// The last_block_time field is nil in the epoch's beginning, and is set upon the end of this epoch.
    pub last_block_time: Timestamp,
    /// app_hash_root is the Merkle root of all AppHashs in this epoch.  It will be used for proving a block is in an epoch
    pub app_hash_root: Vec<u8>,
    /// sealer is the last block of the sealed epoch
    /// sealer_app_hash points to the sealer but stored in the 1st header of the next epoch
    pub sealer_app_hash: Vec<u8>,
    /// sealer_block_hash is the hash of the sealer the validator set has generated a BLS multisig on the hash, i.e., hash of the last block in the epoch
    pub sealer_block_hash: Vec<u8>,
}

decl_storage! {
    trait Store for Module<T: Trait> as EpochStorage {
        Epochs get(fn epochs): map hasher(blake2_128_concat) u64 => Option<Epoch>;
        Checkpoints: map hasher(blake2_128_concat) u64 => Option<RawCheckpointWithMeta>;
        ValidatorSets: map hasher(blake2_128_concat) u64 => ValidatorWithBlsKeySet;

        // RegistrationState
        // Define the storage map to store the mapping between validator addresses and BLS public keys
        AddrToBlsKeys get(fn addr_to_bls_keys): map hasher(blake2_128_concat) T::AccountId => Vec<u8>;

        // Define the storage map to store the mapping between BLS public keys and validator addresses
        BlsKeysToAddr get(fn bls_keys_to_addr): map hasher(blake2_128_concat) Vec<u8> => T::AccountId;
    }
}



decl_event!(
    pub enum Event<T>
    where
        AccountId = <T as frame_system::Trait>::AccountId,
        RawCheckpoint = RawCheckpoint,
        RawCheckpointWithMeta = RawCheckpointWithMeta,
    {
        EventBeginEpoch(u64),
        EventEndEpoch(u64),
        // Event emitted when a checkpoint reaches the `Accumulating` state.
        EventCheckpointAccumulating(RawCheckpointWithMeta),
        // Event emitted when a checkpoint reaches the `Sealed` state.
        EventCheckpointSealed(RawCheckpointWithMeta),
        // Event emitted when a checkpoint reaches the `Submitted` state.
        EventCheckpointSubmitted(RawCheckpointWithMeta),
        // Event emitted when a checkpoint reaches the `Confirmed` state.
        EventCheckpointConfirmed(RawCheckpointWithMeta),
        // Event emitted when a checkpoint reaches the `Finalized` state.
        EventCheckpointFinalized(RawCheckpointWithMeta),
        // Event emitted when a checkpoint switches to a `Forgotten` state.
        EventCheckpointForgotten(RawCheckpointWithMeta),
        // Event emitted when two conflicting checkpoints are found.
        EventConflictingCheckpoint(RawCheckpoint, RawCheckpointWithMeta),
    }
);


impl<T: Trait> EpochingHooks<BlockNumber> for Module<T> {
    fn after_epoch_begins(epoch: u64) {
        // Implement logic for after_epoch_begins hook.
    }

    fn after_epoch_ends(epoch: u64) {
        // Implement logic for after_epoch_ends hook.
    }

    fn before_slash_threshold(val_set: ValidatorSet) {
        // Implement logic for before_slash_threshold hook.
    }
}

// Implement the module
decl_module! {
    pub struct Module<T: Trait> for enum Call where origin: T::Origin {
        // Function to add a checkpoint
        pub fn add_checkpoint(origin, epoch_num: u64, block_hash: Vec<u8>, bitmap: Vec<u8>, bls_multi_sig: Vec<u8>, status: CheckpointStatus, bls_aggr_pk: Vec<u8>, power_sum: u64, lifecycle: Vec<CheckpointStateUpdate>) -> DispatchResult {
            // Ensure the transaction is signed
            let _ = ensure_signed(origin)?;

            // Create a new RawCheckpointWithMeta object
            let new_checkpoint = RawCheckpointWithMeta {
                ckpt: RawCheckpoint { epoch_num, block_hash, bitmap, bls_multi_sig },
                status,
                bls_aggr_pk,
                power_sum,
                lifecycle,
            };

            // Insert the new checkpoint into the storage map
            <Checkpoints<T>>::insert(epoch_num, new_checkpoint);

            // Return Ok(()) indicating success
            Ok(())
        }

        // Function for updating epoch parameters.
        #[weight = 10_000]
        pub fn update_epoch(origin, msg: MsgUpdateParams<T>) -> DispatchResult {
            let _sender = ensure_signed(origin)?;

            // Implement the logic for updating epoch parameters here.

            Ok(())
        }


        pub fn register_bls_key(origin, bls_public_key: Vec<u8>) -> Result<(), &'static str> {
                let who = ensure_signed(origin)?;

                // Ensure the BLS public key is not already registered
                ensure!(!<BlsKeysToAddr>::contains_key(&bls_public_key), "BLS public key is already registered");

                // Store the mapping between the validator address and BLS public key
                <AddrToBlsKeys>::insert(&who, &bls_public_key);

                // Store the mapping between the BLS public key and validator address
                <BlsKeysToAddr>::insert(&bls_public_key, &who);

                Ok(())
        }

        pub fn unregister_bls_key(origin) -> Result<(), &'static str> {
                let who = ensure_signed(origin)?;

                // Get the BLS public key associated with the validator address
                let bls_public_key = <AddrToBlsKeys>::get(&who).ok_or("BLS public key not found for the given address")?;

                // Remove the mappings
                <AddrToBlsKeys>::remove(&who);
                <BlsKeysToAddr>::remove(&bls_public_key);

                Ok(())
        }

        // Add validator set for an epoch
        pub fn add_validator_set(origin, epoch: u64, validators: Vec<(String, Vec<u8>, u64)>) -> dispatch::DispatchResult {
            let _who = ensure_signed(origin)?;

            let mut validator_set = ValidatorWithBlsKeySet { val_set: Vec::new() };
            for (validator_address, bls_pub_key, voting_power) in validators {
                let validator = ValidatorWithBlsKey {
                    validator_address,
                    bls_pub_key,
                    voting_power,
                };
                validator_set.val_set.push(validator);
            }
            <ValidatorSets<T>>::insert(epoch, validator_set);
            Ok(())
        }

    }
}
