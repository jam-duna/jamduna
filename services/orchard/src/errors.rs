#![allow(dead_code)]
use alloc::string::String;

/// Orchard service error types (matching services/orchard/docs/ORCHARD.md specification)
#[derive(Debug, Clone)]
pub enum OrchardError {
    // ZK proof errors
    InvalidProof,
    InvalidPublicInputs,

    // VK registry errors
    InvalidVkId { vk_id: u32 },
    VkHashMismatch { vk_id: u32, expected: [u8; 32], computed: [u8; 32] },
    PublicInputMismatch { vk_id: u32, expected_fields: usize, provided_fields: usize },

    // State errors
    AnchorMismatch { expected: [u8; 32], got: [u8; 32] },
    NullifierAlreadySpent { nullifier: [u8; 32] },
    InvalidVerkleProof { key: [u8; 32] },
    TreeCapacityExceeded { current_size: u64, requested_additions: u64, capacity: u64 },
    WitnessMissing,

    // Validation errors
    InvalidMemoVersion { version: u8 },
    ValueBindingFailure { expected: [u8; 32], got: [u8; 32] },
    GasLimitOutOfBounds { provided: u64, min: u64, max: u64 },
    InvalidGasLimit { provided: u64, minimum: u64, maximum: u64 },  // Alias for GasLimitOutOfBounds
    FeeUnderflow { provided: u128, minimum: u128 },
    EncryptedPayloadNotAllowed { length: usize },  // MVP: encrypted payloads disabled
    EncryptedPayloadNotSupported,  // Legacy alias
    InvalidEpoch(u64, u64),  // (provided_epoch, current_epoch)
    InvalidFieldElement,

    // Replay errors
    NullifierReusedInWorkItem { nullifier: [u8; 32] },

    // Multi-asset errors
    DeltasNotSorted,
    ReservedAssetUsed,
    InvalidAssetId { asset_id: u32 },

    // Compliance errors
    ComplianceRootMissing,
    ComplianceRootMismatch { expected: [u8; 32], got: [u8; 32] },

    // Exercise/swap errors
    TermsHashRequired,
    TermsHashUnused,
    ExerciseRatioMismatch { expected: u128, got: u128 },
    SwapPricingMismatch { expected: u128, got: u128 },

    // Supply errors
    SupplyInvariantViolation {
        asset_id: u32,
        total: u128,
        transparent: u128,
        shielded: u128,
    },

    // Generic errors
    ParseError(String),
    SerializationError(String),
    StateError(String),
}

impl core::fmt::Display for OrchardError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            OrchardError::InvalidProof => write!(f, "Invalid ZK proof"),
            OrchardError::InvalidPublicInputs => write!(f, "Invalid public inputs"),
            OrchardError::InvalidVkId { vk_id } => {
                write!(f, "Invalid VK ID: {}", vk_id)
            }
            OrchardError::VkHashMismatch { vk_id, expected, computed } => {
                write!(f, "VK hash mismatch for ID {}: expected {:?}, computed {:?}",
                       vk_id, &expected[..8], &computed[..8])
            }
            OrchardError::PublicInputMismatch { vk_id, expected_fields, provided_fields } => {
                write!(f, "Public input count mismatch for VK {}: expected {} fields, got {}",
                       vk_id, expected_fields, provided_fields)
            }
            OrchardError::AnchorMismatch { expected, got } => {
                write!(f, "Anchor mismatch: expected {:?}, got {:?}", expected, got)
            }
            OrchardError::NullifierAlreadySpent { nullifier } => {
                write!(f, "Nullifier already spent: {:?}", nullifier)
            }
            OrchardError::InvalidVerkleProof { key } => {
                write!(f, "Invalid Verkle proof for key: {:?}", key)
            }
            OrchardError::TreeCapacityExceeded { current_size, requested_additions, capacity } => {
                write!(f, "Tree capacity exceeded: {} + {} > {}", current_size, requested_additions, capacity)
            }
            OrchardError::WitnessMissing => {
                write!(f, "State witness missing for key")
            }
            OrchardError::InvalidMemoVersion { version } => {
                write!(f, "Invalid memo version: {}", version)
            }
            OrchardError::ValueBindingFailure { expected, got } => {
                write!(f, "Value binding failure: expected {:?}, got {:?}", expected, got)
            }
            OrchardError::GasLimitOutOfBounds { provided, min, max } => {
                write!(f, "Gas limit {} out of bounds [{}, {}]", provided, min, max)
            }
            OrchardError::InvalidGasLimit { provided, minimum, maximum } => {
                write!(f, "Gas limit {} out of bounds [{}, {}]", provided, minimum, maximum)
            }
            OrchardError::FeeUnderflow { provided, minimum } => {
                write!(f, "Fee {} below minimum {}", provided, minimum)
            }
            OrchardError::EncryptedPayloadNotAllowed { length } => {
                write!(f, "Encrypted payload not allowed in MVP (length: {})", length)
            }
            OrchardError::EncryptedPayloadNotSupported => {
                write!(f, "Encrypted payload not supported")
            }
            OrchardError::InvalidEpoch(provided, current) => {
                write!(f, "Invalid epoch: provided {}, current {}", provided, current)
            }
            OrchardError::InvalidFieldElement => {
                write!(f, "Invalid field element encoding")
            }
            OrchardError::NullifierReusedInWorkItem { nullifier } => {
                write!(f, "Nullifier reused within work item: {:?}", nullifier)
            }
            OrchardError::DeltasNotSorted => {
                write!(f, "Deltas not sorted by asset_id")
            }
            OrchardError::ReservedAssetUsed => {
                write!(f, "Reserved asset_id=0 used in non-zero delta")
            }
            OrchardError::InvalidAssetId { asset_id } => {
                write!(f, "Invalid asset_id: {}", asset_id)
            }
            OrchardError::ComplianceRootMissing => {
                write!(f, "Compliance root missing for equity output")
            }
            OrchardError::ComplianceRootMismatch { expected, got } => {
                write!(f, "Compliance root mismatch: expected {:?}, got {:?}", expected, got)
            }
            OrchardError::TermsHashRequired => {
                write!(f, "Terms hash required for burn/mint deltas")
            }
            OrchardError::TermsHashUnused => {
                write!(f, "Terms hash provided but no burn/mint deltas")
            }
            OrchardError::ExerciseRatioMismatch { expected, got } => {
                write!(f, "Exercise ratio mismatch: expected {}, got {}", expected, got)
            }
            OrchardError::SwapPricingMismatch { expected, got } => {
                write!(f, "Swap pricing mismatch: expected {}, got {}", expected, got)
            }
            OrchardError::SupplyInvariantViolation { asset_id, total, transparent, shielded } => {
                write!(f, "Supply invariant violated for asset {}: total={}, transparent={}, shielded={}",
                       asset_id, total, transparent, shielded)
            }
            OrchardError::ParseError(msg) => write!(f, "Parse error: {}", msg),
            OrchardError::SerializationError(msg) => write!(f, "Serialization error: {}", msg),
            OrchardError::StateError(msg) => write!(f, "State error: {}", msg),
        }
    }
}

pub type Result<T> = core::result::Result<T, OrchardError>;
