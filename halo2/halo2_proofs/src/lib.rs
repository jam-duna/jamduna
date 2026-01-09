//! # halo2_proofs

#![cfg_attr(docsrs, feature(doc_cfg))]
#![cfg_attr(not(feature = "std"), no_std)]
// The actual lints we want to disable.
#![allow(clippy::op_ref, clippy::many_single_char_names)]
#![deny(rustdoc::broken_intra_doc_links)]
#![deny(missing_debug_implementations)]
#![deny(missing_docs)]
#![deny(unsafe_code)]

extern crate alloc;
#[cfg(feature = "std")]
extern crate std;

pub(crate) mod collections {
    #[cfg(feature = "std")]
    pub use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
    #[cfg(not(feature = "std"))]
    pub use alloc::collections::{BTreeMap, BTreeSet};
    #[cfg(not(feature = "std"))]
    pub use hashbrown::{HashMap, HashSet};

    // Vec and String re-exports for no_std
    #[cfg(not(feature = "std"))]
    pub use alloc::{vec, vec::Vec, string::String, format};
    #[cfg(feature = "std")]
    pub use std::{vec, vec::Vec, string::String, format};
}

pub(crate) mod io {
    #[cfg(feature = "std")]
    pub use std::io::*;
    #[cfg(not(feature = "std"))]
    pub use core2::io::*;
}

pub(crate) mod sync {
    #[cfg(feature = "std")]
    pub use std::sync::Arc;
    #[cfg(not(feature = "std"))]
    pub use alloc::sync::Arc;
}

pub mod arithmetic;
pub mod circuit;
pub use pasta_curves as pasta;
mod multicore;
pub mod plonk;
pub mod poly;
pub mod transcript;

#[cfg(feature = "std")]
pub mod dev;
mod helpers;
