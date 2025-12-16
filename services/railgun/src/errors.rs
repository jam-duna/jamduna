use alloc::string::String;

/// Railgun service error types (matching RAILGUN.md specification)
#[derive(Debug, Clone)]
pub enum RailgunError {
    // ZK proof errors
    InvalidProof,
    InvalidPublicInputs,

    // State errors
    AnchorMismatch { expected: [u8; 32], got: [u8; 32] },
    NullifierAlreadySpent { nullifier: [u8; 32] },
    InvalidVerkleProof { key: [u8; 32] },
    TreeCapacityExceeded { current_size: u64, requested_additions: u64, capacity: u64 },

    // Validation errors
    InvalidMemoVersion { version: u8 },
    ValueBindingFailure { expected: [u8; 32], got: [u8; 32] },
    GasLimitOutOfBounds { provided: u64, min: u64, max: u64 },
    FeeUnderflow { provided: u64, minimum: u64 },
    EncryptedPayloadNotAllowed { length: usize },  // MVP: encrypted payloads disabled

    // Replay errors
    NullifierReusedInWorkItem { nullifier: [u8; 32] },

    // Generic errors
    ParseError(String),
    SerializationError(String),
    StateError(String),
}

impl core::fmt::Display for RailgunError {
    fn fmt(&self, f: &mut core::fmt::Formatter<'_>) -> core::fmt::Result {
        match self {
            RailgunError::InvalidProof => write!(f, "Invalid ZK proof"),
            RailgunError::InvalidPublicInputs => write!(f, "Invalid public inputs"),
            RailgunError::AnchorMismatch { expected, got } => {
                write!(f, "Anchor mismatch: expected {:?}, got {:?}", expected, got)
            }
            RailgunError::NullifierAlreadySpent { nullifier } => {
                write!(f, "Nullifier already spent: {:?}", nullifier)
            }
            RailgunError::InvalidVerkleProof { key } => {
                write!(f, "Invalid Verkle proof for key: {:?}", key)
            }
            RailgunError::TreeCapacityExceeded { current_size, requested_additions, capacity } => {
                write!(f, "Tree capacity exceeded: {} + {} > {}", current_size, requested_additions, capacity)
            }
            RailgunError::InvalidMemoVersion { version } => {
                write!(f, "Invalid memo version: {}", version)
            }
            RailgunError::ValueBindingFailure { expected, got } => {
                write!(f, "Value binding failure: expected {:?}, got {:?}", expected, got)
            }
            RailgunError::GasLimitOutOfBounds { provided, min, max } => {
                write!(f, "Gas limit {} out of bounds [{}, {}]", provided, min, max)
            }
            RailgunError::FeeUnderflow { provided, minimum } => {
                write!(f, "Fee {} below minimum {}", provided, minimum)
            }
            RailgunError::EncryptedPayloadNotAllowed { length } => {
                write!(f, "Encrypted payload not allowed in MVP (length: {})", length)
            }
            RailgunError::NullifierReusedInWorkItem { nullifier } => {
                write!(f, "Nullifier reused within work item: {:?}", nullifier)
            }
            RailgunError::ParseError(msg) => write!(f, "Parse error: {}", msg),
            RailgunError::SerializationError(msg) => write!(f, "Serialization error: {}", msg),
            RailgunError::StateError(msg) => write!(f, "State error: {}", msg),
        }
    }
}

pub type Result<T> = core::result::Result<T, RailgunError>;
