//! Orchard object kind taxonomy for WriteIntents
//!
//! Defines the different types of state writes that flow from refine to accumulate.

/// Object kind discriminator for Orchard WriteIntents
///
/// Used in WriteIntent.object_kind to distinguish different types of state updates:
/// - STATE_WRITE: Generic state writes (commitment_root, commitment_size, consensus params)
/// - DEPOSIT: Deposit extrinsic data (processed specially by accumulate)
/// - NULLIFIER: Nullifier spent marker
/// - COMMITMENT: Individual commitment to tree
/// - FEE_TALLY: Fee tally delta for epoch
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum ObjectKind {
    /// Generic state write (commitment_root, commitment_size, etc.)
    StateWrite = 0x00,

    /// Deposit extrinsic data for accumulate processing
    /// Payload: [commitment:32][value:8][sender_index:4]
    Deposit = 0x01,

    /// Nullifier spent marker
    /// Payload: [1u8; 32] (spent flag)
    Nullifier = 0x02,

    /// Individual commitment to be added to tree
    /// Payload: [commitment:32]
    Commitment = 0x03,

    /// Fee tally delta for epoch
    /// Payload: [fee_delta:16] (u128)
    FeeTally = 0x04,

    /// Delta write for supply tracking (Phase 7)
    /// Payload: [asset_id:4][component:str][amount:16]
    DeltaWrite = 0x05,

    /// Compact block for light client support
    /// Payload: serialized CompactBlock data
    Block = 0x06,
}

impl ObjectKind {
    /// Try to parse from byte value
    pub fn from_u8(value: u8) -> Option<Self> {
        match value {
            0x00 => Some(Self::StateWrite),
            0x01 => Some(Self::Deposit),
            0x02 => Some(Self::Nullifier),
            0x03 => Some(Self::Commitment),
            0x04 => Some(Self::FeeTally),
            0x05 => Some(Self::DeltaWrite),
            0x06 => Some(Self::Block),
            _ => None,
        }
    }
}
