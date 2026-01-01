// Orchard Builder - Pure Rust JAM Work Package Construction
//
// This crate provides a complete Rust implementation for building JAM work packages
// for the Orchard service, eliminating the need for complex FFI boundaries.

pub mod state;
pub mod witness;
pub mod workpackage;
pub mod bundle_codec;
pub mod merkle;
pub mod merkle_impl;
pub mod sequence;
pub mod witness_based;

pub use workpackage::{WorkPackageBuilder, WorkPackage, OrchardExtrinsic};
pub use state::{OrchardState, StateWitnesses, WriteIntents};
pub use witness::{build_witnesses, verify_witnesses};

// Re-export orchard types
pub use orchard::{
    builder::Builder as OrchardBundleBuilder,
    bundle::{Bundle, Flags, Authorized},
    keys::{SpendingKey, FullViewingKey, Scope},
    note::{Note, Nullifier},
    tree::MerkleHashOrchard,
    value::ValueCommitment,
    Address,
};

// Type alias for authorized Orchard bundles
pub type OrchardBundle = Bundle<Authorized, i64>;

pub type Result<T> = std::result::Result<T, Error>;

/// Compute the Orchard-only fee from a bundle's value balance.
///
/// For this service, we only accept Orchard bundles without transparent or Sapling
/// components, so the Zcash fee formula reduces to:
/// fee = valueBalanceOrchard.
pub fn compute_orchard_fee(bundle: &OrchardBundle) -> Result<u64> {
    let value_balance = *bundle.value_balance();
    if value_balance < 0 {
        return Err(Error::InvalidBundle(format!(
            "Negative value balance {} requires transparent inputs",
            value_balance
        )));
    }
    u64::try_from(value_balance).map_err(|_| {
        Error::InvalidBundle(format!(
            "Value balance {} does not fit in u64",
            value_balance
        ))
    })
}

#[derive(Debug)]
pub enum Error {
    InvalidBundle(String),
    InvalidWitness(String),
    InvalidState(String),
    InsufficientFee,
    NullifierExists,
    InvalidAnchor,
    SerializationError(String),
    OrchardError(String),
}

impl std::fmt::Display for Error {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Error::InvalidBundle(s) => write!(f, "Invalid bundle: {}", s),
            Error::InvalidWitness(s) => write!(f, "Invalid witness: {}", s),
            Error::InvalidState(s) => write!(f, "Invalid state: {}", s),
            Error::InsufficientFee => write!(f, "Insufficient fee"),
            Error::NullifierExists => write!(f, "Nullifier already exists"),
            Error::InvalidAnchor => write!(f, "Invalid anchor"),
            Error::SerializationError(s) => write!(f, "Serialization error: {}", s),
            Error::OrchardError(s) => write!(f, "Orchard error: {}", s),
        }
    }
}

impl std::error::Error for Error {}
