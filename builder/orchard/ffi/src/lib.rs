use std::ptr;
use std::slice;

use incrementalmerkletree::{Hashable, Level};
use orchard::tree::MerkleHashOrchard;

const TREE_DEPTH: usize = 32;

/// Error codes for FFI functions
pub const FFI_SUCCESS: i32 = 0;
pub const FFI_ERROR_NULL_POINTER: i32 = -1;
pub const FFI_ERROR_INVALID_INPUT: i32 = -2;
pub const FFI_ERROR_COMPUTATION_FAILED: i32 = -3;

fn parse_hash(bytes: &[u8; 32]) -> Result<MerkleHashOrchard, Box<dyn std::error::Error>> {
    MerkleHashOrchard::from_bytes(bytes).into_option().ok_or_else(|| "Invalid Merkle node bytes".into())
}

fn parse_hashes(
    bytes: &[u8],
    count: usize,
) -> Result<Vec<MerkleHashOrchard>, Box<dyn std::error::Error>> {
    let mut out = Vec::with_capacity(count);
    for i in 0..count {
        let start = i * 32;
        let end = start + 32;
        let mut arr = [0u8; 32];
        arr.copy_from_slice(&bytes[start..end]);
        out.push(parse_hash(&arr)?);
    }
    Ok(out)
}

fn empty_root(level: usize) -> MerkleHashOrchard {
    MerkleHashOrchard::empty_root(Level::from(level as u8))
}

fn write_hash(out: *mut u8, hash: MerkleHashOrchard) {
    let bytes = hash.to_bytes();
    unsafe {
        ptr::copy_nonoverlapping(bytes.as_ptr(), out, 32);
    }
}

fn write_frontier(out: *mut u8, frontier: &[MerkleHashOrchard]) {
    for (i, node) in frontier.iter().enumerate() {
        let bytes = node.to_bytes();
        unsafe {
            ptr::copy_nonoverlapping(bytes.as_ptr(), out.add(i * 32), 32);
        }
    }
}

/// Compute Sinsemilla Merkle tree root from leaves
///
/// # Safety
/// Caller must ensure:
/// - `leaves` points to valid memory of size `leaves_len * 32` (if `leaves_len > 0`)
/// - `root_out` points to valid writable memory of size 32
/// - `leaves_len` is the actual count of 32-byte leaves
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_compute_root(
    leaves: *const u8,
    leaves_len: usize,
    root_out: *mut u8,
) -> i32 {
    if root_out.is_null() || (leaves_len > 0 && leaves.is_null()) {
        return FFI_ERROR_NULL_POINTER;
    }

    if leaves_len == 0 {
        write_hash(root_out, empty_root(TREE_DEPTH));
        return FFI_SUCCESS;
    }

    let leaves_bytes = slice::from_raw_parts(leaves, leaves_len * 32);
    let leaf_hashes = match parse_hashes(leaves_bytes, leaves_len) {
        Ok(hashes) => hashes,
        Err(_) => return FFI_ERROR_INVALID_INPUT,
    };

    match compute_merkle_root_impl(&leaf_hashes) {
        Ok(root) => {
            write_hash(root_out, root);
            FFI_SUCCESS
        }
        Err(_) => FFI_ERROR_COMPUTATION_FAILED,
    }
}

/// Compute Sinsemilla Merkle tree root and frontier from leaves.
///
/// # Safety
/// Caller must ensure:
/// - `leaves` points to valid memory of size `leaves_len * 32` (if `leaves_len > 0`)
/// - `root_out` points to valid writable memory of size 32
/// - `frontier_out` points to valid writable memory of size `TREE_DEPTH * 32`
/// - `frontier_len_out` points to valid usize for output length
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_compute_root_and_frontier(
    leaves: *const u8,
    leaves_len: usize,
    root_out: *mut u8,
    frontier_out: *mut u8,
    frontier_len_out: *mut usize,
) -> i32 {
    if root_out.is_null()
        || frontier_out.is_null()
        || frontier_len_out.is_null()
        || (leaves_len > 0 && leaves.is_null())
    {
        return FFI_ERROR_NULL_POINTER;
    }

    let leaf_hashes = if leaves_len == 0 {
        Vec::new()
    } else {
        let leaves_bytes = slice::from_raw_parts(leaves, leaves_len * 32);
        match parse_hashes(leaves_bytes, leaves_len) {
            Ok(hashes) => hashes,
            Err(_) => return FFI_ERROR_INVALID_INPUT,
        }
    };

    let root = if leaf_hashes.is_empty() {
        empty_root(TREE_DEPTH)
    } else {
        match compute_merkle_root_impl(&leaf_hashes) {
            Ok(root) => root,
            Err(_) => return FFI_ERROR_COMPUTATION_FAILED,
        }
    };

    let frontier = if leaf_hashes.is_empty() {
        empty_frontier(TREE_DEPTH)
    } else {
        compute_frontier_from_leaves(&leaf_hashes, TREE_DEPTH)
    };

    write_hash(root_out, root);
    write_frontier(frontier_out, &frontier);
    *frontier_len_out = frontier.len();
    FFI_SUCCESS
}

/// Append new leaves to existing tree using frontier
///
/// # Safety
/// Caller must ensure:
/// - `old_root` points to valid 32-byte root (ignored when `old_size == 0`)
/// - `frontier` points to valid memory of size `frontier_len * 32`
/// - `new_leaves` points to valid memory of size `new_leaves_len * 32`
/// - `new_root_out` points to valid writable memory of size 32
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_append_with_frontier(
    old_root: *const u8,
    old_size: u64,
    frontier: *const u8,
    frontier_len: usize,
    new_leaves: *const u8,
    new_leaves_len: usize,
    new_root_out: *mut u8,
) -> i32 {
    if old_root.is_null()
        || new_root_out.is_null()
        || (new_leaves_len > 0 && new_leaves.is_null())
        || (frontier_len > 0 && frontier.is_null())
    {
        return FFI_ERROR_NULL_POINTER;
    }

    if new_leaves_len == 0 {
        return FFI_ERROR_INVALID_INPUT;
    }

    let old_root_hash = if old_size == 0 {
        empty_root(TREE_DEPTH)
    } else {
        let old_root_bytes = slice::from_raw_parts(old_root, 32);
        let mut arr = [0u8; 32];
        arr.copy_from_slice(old_root_bytes);
        match parse_hash(&arr) {
            Ok(hash) => hash,
            Err(_) => return FFI_ERROR_INVALID_INPUT,
        }
    };

    let frontier_hashes = if frontier_len == 0 {
        Vec::new()
    } else {
        let frontier_bytes = slice::from_raw_parts(frontier, frontier_len * 32);
        match parse_hashes(frontier_bytes, frontier_len) {
            Ok(hashes) => hashes,
            Err(_) => return FFI_ERROR_INVALID_INPUT,
        }
    };

    let new_leaves_bytes = slice::from_raw_parts(new_leaves, new_leaves_len * 32);
    let new_leaf_hashes = match parse_hashes(new_leaves_bytes, new_leaves_len) {
        Ok(hashes) => hashes,
        Err(_) => return FFI_ERROR_INVALID_INPUT,
    };

    match merkle_append_with_frontier_impl(old_root_hash, old_size, &frontier_hashes, &new_leaf_hashes) {
        Ok((new_root, _)) => {
            write_hash(new_root_out, new_root);
            FFI_SUCCESS
        }
        Err(_) => FFI_ERROR_COMPUTATION_FAILED,
    }
}

/// Append new leaves and return updated frontier.
///
/// # Safety
/// Caller must ensure:
/// - `old_root` points to valid 32-byte root (ignored when `old_size == 0`)
/// - `frontier` points to valid memory of size `frontier_len * 32`
/// - `new_leaves` points to valid memory of size `new_leaves_len * 32`
/// - `new_root_out` points to valid writable memory of size 32
/// - `new_frontier_out` points to valid writable memory of size `TREE_DEPTH * 32`
/// - `new_frontier_len_out` points to valid usize for output length
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_append_with_frontier_update(
    old_root: *const u8,
    old_size: u64,
    frontier: *const u8,
    frontier_len: usize,
    new_leaves: *const u8,
    new_leaves_len: usize,
    new_root_out: *mut u8,
    new_frontier_out: *mut u8,
    new_frontier_len_out: *mut usize,
) -> i32 {
    if new_root_out.is_null()
        || new_frontier_out.is_null()
        || new_frontier_len_out.is_null()
        || old_root.is_null()
        || (new_leaves_len > 0 && new_leaves.is_null())
        || (frontier_len > 0 && frontier.is_null())
    {
        return FFI_ERROR_NULL_POINTER;
    }

    if new_leaves_len == 0 {
        return FFI_ERROR_INVALID_INPUT;
    }

    let old_root_hash = if old_size == 0 {
        empty_root(TREE_DEPTH)
    } else {
        let old_root_bytes = slice::from_raw_parts(old_root, 32);
        let mut arr = [0u8; 32];
        arr.copy_from_slice(old_root_bytes);
        match parse_hash(&arr) {
            Ok(hash) => hash,
            Err(_) => return FFI_ERROR_INVALID_INPUT,
        }
    };

    let frontier_hashes = if frontier_len == 0 {
        Vec::new()
    } else {
        let frontier_bytes = slice::from_raw_parts(frontier, frontier_len * 32);
        match parse_hashes(frontier_bytes, frontier_len) {
            Ok(hashes) => hashes,
            Err(_) => return FFI_ERROR_INVALID_INPUT,
        }
    };

    let new_leaves_bytes = slice::from_raw_parts(new_leaves, new_leaves_len * 32);
    let new_leaf_hashes = match parse_hashes(new_leaves_bytes, new_leaves_len) {
        Ok(hashes) => hashes,
        Err(_) => return FFI_ERROR_INVALID_INPUT,
    };

    match merkle_append_with_frontier_impl(old_root_hash, old_size, &frontier_hashes, &new_leaf_hashes) {
        Ok((new_root, new_frontier)) => {
            write_hash(new_root_out, new_root);
            write_frontier(new_frontier_out, &new_frontier);
            *new_frontier_len_out = new_frontier.len();
            FFI_SUCCESS
        }
        Err(_) => FFI_ERROR_COMPUTATION_FAILED,
    }
}

/// Generate Merkle proof for commitment at given position
///
/// # Safety
/// Caller must ensure:
/// - `commitment` points to valid 32-byte commitment
/// - `all_commitments` points to valid memory of size `commitments_len * 32`
/// - `proof_out` points to valid writable memory of size `proof_len_out * 32`
/// - `proof_len_out` points to valid usize for output length
#[no_mangle]
pub unsafe extern "C" fn sinsemilla_generate_proof(
    commitment: *const u8,
    tree_position: u64,
    all_commitments: *const u8,
    commitments_len: usize,
    proof_out: *mut u8,
    proof_len_out: *mut usize,
) -> i32 {
    if commitment.is_null()
        || proof_out.is_null()
        || proof_len_out.is_null()
        || (commitments_len > 0 && all_commitments.is_null())
    {
        return FFI_ERROR_NULL_POINTER;
    }

    if tree_position >= commitments_len as u64 {
        return FFI_ERROR_INVALID_INPUT;
    }

    let commitment_bytes = slice::from_raw_parts(commitment, 32);
    let mut commitment_arr = [0u8; 32];
    commitment_arr.copy_from_slice(commitment_bytes);
    let commitment_hash = match parse_hash(&commitment_arr) {
        Ok(hash) => hash,
        Err(_) => return FFI_ERROR_INVALID_INPUT,
    };

    let commitments_bytes = slice::from_raw_parts(all_commitments, commitments_len * 32);
    let commitment_hashes = match parse_hashes(commitments_bytes, commitments_len) {
        Ok(hashes) => hashes,
        Err(_) => return FFI_ERROR_INVALID_INPUT,
    };

    match generate_merkle_proof_impl(commitment_hash, tree_position, &commitment_hashes) {
        Ok(proof_path) => {
            *proof_len_out = proof_path.len();
            for (i, sibling) in proof_path.iter().enumerate() {
                let bytes = sibling.to_bytes();
                let offset = i * 32;
                ptr::copy_nonoverlapping(bytes.as_ptr(), proof_out.add(offset), 32);
            }
            FFI_SUCCESS
        }
        Err(_) => FFI_ERROR_COMPUTATION_FAILED,
    }
}

fn compute_merkle_root_impl(
    leaves: &[MerkleHashOrchard],
) -> Result<MerkleHashOrchard, Box<dyn std::error::Error>> {
    compute_merkle_root_impl_with_depth(leaves, TREE_DEPTH)
}

fn compute_merkle_root_impl_with_depth(
    leaves: &[MerkleHashOrchard],
    depth: usize,
) -> Result<MerkleHashOrchard, Box<dyn std::error::Error>> {
    if leaves.is_empty() {
        return Ok(empty_root(depth));
    }

    if leaves.len() > (1usize << depth) {
        return Err("Too many leaves for tree depth".into());
    }

    let mut current_level = leaves.to_vec();

    for level in 0..depth {
        if current_level.len() == 1 {
            let mut root = current_level[0];
            for height in level..depth {
                let empty = empty_root(height);
                root = MerkleHashOrchard::combine(Level::from(height as u8), &root, &empty);
            }
            return Ok(root);
        }

        let empty = empty_root(level);
        let mut next_level = Vec::with_capacity((current_level.len() + 1) / 2);
        for pair in current_level.chunks(2) {
            let left = pair[0];
            let right = pair.get(1).copied().unwrap_or(empty);
            next_level.push(MerkleHashOrchard::combine(Level::from(level as u8), &left, &right));
        }
        current_level = next_level;
    }

    current_level
        .first()
        .copied()
        .ok_or_else(|| "Missing root after tree construction".into())
}

fn empty_frontier(depth: usize) -> Vec<MerkleHashOrchard> {
    (0..depth).map(empty_root).collect()
}

fn compute_frontier_from_leaves(
    leaves: &[MerkleHashOrchard],
    depth: usize,
) -> Vec<MerkleHashOrchard> {
    let mut frontier: Vec<Option<MerkleHashOrchard>> = vec![None; depth];
    let mut size: u64 = 0;

    for leaf in leaves {
        let mut node = *leaf;
        let mut level = 0usize;
        let mut cursor = size;

        while (cursor & 1) == 1 {
            let left = frontier[level]
                .ok_or("frontier missing node for occupied subtree")
                .unwrap();
            node = MerkleHashOrchard::combine(Level::from(level as u8), &left, &node);
            frontier[level] = None;
            level += 1;
            cursor >>= 1;
        }

        frontier[level] = Some(node);
        size += 1;
    }

    frontier
        .into_iter()
        .enumerate()
        .map(|(level, node)| node.unwrap_or_else(|| empty_root(level)))
        .collect()
}

fn merkle_root_from_frontier(
    frontier: &[MerkleHashOrchard],
    size: u64,
) -> Result<MerkleHashOrchard, Box<dyn std::error::Error>> {
    if size == 0 {
        return Ok(empty_root(TREE_DEPTH));
    }

    if frontier.len() != TREE_DEPTH {
        return Err("frontier length mismatch".into());
    }

    let mut digest: Option<MerkleHashOrchard> = None;
    let mut current_level = 0usize;
    let mut cursor = size;

    for level in 0..TREE_DEPTH {
        if (cursor & 1) == 1 {
            let node = frontier[level];
            if let Some(existing) = digest {
                let mut acc = existing;
                for l in current_level..level {
                    let empty = empty_root(l);
                    acc = MerkleHashOrchard::combine(Level::from(l as u8), &acc, &empty);
                }
                digest = Some(MerkleHashOrchard::combine(Level::from(level as u8), &node, &acc));
                current_level = level + 1;
            } else {
                digest = Some(node);
                current_level = level;
            }
        }
        cursor >>= 1;
    }

    let mut acc = digest.ok_or_else(|| -> Box<dyn std::error::Error> { "frontier root missing".into() })?;
    for l in current_level..TREE_DEPTH {
        let empty = empty_root(l);
        acc = MerkleHashOrchard::combine(Level::from(l as u8), &acc, &empty);
    }

    Ok(acc)
}

fn merkle_append_with_frontier_impl(
    old_root: MerkleHashOrchard,
    old_size: u64,
    frontier: &[MerkleHashOrchard],
    new_leaves: &[MerkleHashOrchard],
) -> Result<(MerkleHashOrchard, Vec<MerkleHashOrchard>), Box<dyn std::error::Error>> {
    if new_leaves.is_empty() {
        return Err("No new leaves to append".into());
    }

    if old_size == 0 {
        let new_frontier = compute_frontier_from_leaves(new_leaves, TREE_DEPTH);
        let new_root = compute_merkle_root_impl_with_depth(new_leaves, TREE_DEPTH)?;
        return Ok((new_root, new_frontier));
    }

    if frontier.len() != TREE_DEPTH {
        return Err("frontier length mismatch".into());
    }

    let computed_root = merkle_root_from_frontier(frontier, old_size)?;
    if computed_root != old_root {
        return Err("frontier does not match old_root".into());
    }

    let mut frontier_nodes = frontier.to_vec();
    let mut size = old_size;

    for leaf in new_leaves {
        let mut node = *leaf;
        let mut level = 0usize;
        let mut cursor = size;

        while (cursor & 1) == 1 {
            let left = frontier_nodes[level];
            node = MerkleHashOrchard::combine(Level::from(level as u8), &left, &node);
            frontier_nodes[level] = empty_root(level);
            level += 1;
            cursor >>= 1;
        }

        frontier_nodes[level] = node;
        size += 1;
    }

    let new_root = merkle_root_from_frontier(&frontier_nodes, size)?;
    Ok((new_root, frontier_nodes))
}

fn generate_merkle_proof_impl(
    commitment: MerkleHashOrchard,
    position: u64,
    all_commitments: &[MerkleHashOrchard],
) -> Result<Vec<MerkleHashOrchard>, Box<dyn std::error::Error>> {
    if position >= all_commitments.len() as u64 {
        return Err("Position out of bounds".into());
    }

    if all_commitments[position as usize] != commitment {
        return Err("Commitment mismatch at specified position".into());
    }

    let mut proof_path = Vec::with_capacity(TREE_DEPTH);
    let mut current_level = all_commitments.to_vec();
    let mut current_pos = position as usize;

    for level in 0..TREE_DEPTH {
        let sibling_index = current_pos ^ 1;
        let empty = empty_root(level);
        let sibling = current_level.get(sibling_index).copied().unwrap_or(empty);
        proof_path.push(sibling);

        let mut next_level = Vec::with_capacity((current_level.len() + 1) / 2);
        for pair in current_level.chunks(2) {
            let left = pair[0];
            let right = pair.get(1).copied().unwrap_or(empty);
            next_level.push(MerkleHashOrchard::combine(Level::from(level as u8), &left, &right));
        }

        current_level = next_level;
        current_pos >>= 1;
    }

    Ok(proof_path)
}
