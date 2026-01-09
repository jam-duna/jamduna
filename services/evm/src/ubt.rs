//! UBT witness parsing + multiproof verification (JAM profile hashing).

use alloc::collections::{BTreeMap, BTreeSet};
use alloc::format;
use alloc::string::String;
use alloc::vec;
use alloc::vec::Vec;
use primitive_types::{H160, H256, U256};
use utils::functions::{format_segment, log_info};
use utils::hash_functions::keccak256;

const ZERO32: [u8; 32] = [0u8; 32];

#[derive(Debug)]
pub enum UBTError {
    ParseError,
    LengthMismatch,
    InvalidProof,
    RootMismatch,
    InvalidKeyType(()),
    WitnessMismatch,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum WitnessValueKind {
    Pre,
    Post,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum UBTKeyType {
    BasicData = 0,
    CodeHash = 1,
    CodeChunk = 2,
    Storage = 3,
}

#[derive(Debug, Clone)]
pub struct UBTKeyMetadata {
    pub key_type: UBTKeyType,
    pub address: H160,
    pub extra: u64,
    pub storage_key: H256,
    pub tx_index: u32,
}

#[derive(Debug, Clone)]
pub struct UBTWitnessEntry {
    pub tree_key: [u8; 32],
    pub pre_value: [u8; 32],
    pub post_value: [u8; 32],
    pub metadata: UBTKeyMetadata,
}

#[derive(Debug, Clone)]
pub struct UBTWitnessSection {
    pub root: [u8; 32],
    pub entries: Vec<UBTWitnessEntry>,
    pub proof: MultiProof,
    pub extensions: Vec<ExtensionProof>,
}

#[derive(Debug, Clone)]
pub struct ExtensionProof {
    pub missing_stem: Stem,  // Changed from key: [u8; 32]
    pub stem: Stem,
    pub stem_hash: [u8; 32],
    pub divergence_depth: u16,
    // Siblings removed - verified inline during deserialization to save memory
}

impl UBTWitnessSection {
    pub fn deserialize(data: &[u8]) -> Result<Self, UBTError> {
        if data.len() < 32 + 4 + 4 {
            return Err(UBTError::ParseError);
        }

        // Debug: Log raw witness bytes
        let head_len = core::cmp::min(128, data.len());
        let tail_start = if data.len() > 128 { data.len() - 128 } else { 0 };
        log_info(&format!(
            "Rust UBTWitnessSection::deserialize: len={}, head128={}, tail128={}",
            data.len(),
            format_segment(&data[..head_len]),
            format_segment(&data[tail_start..])
        ));

        let mut offset = 0usize;
        let root = read_array_32(data, &mut offset)?;
        let count = usize::try_from(read_u32_be(data, &mut offset)?)
            .map_err(|_| UBTError::ParseError)?;
        if data.len() < offset + 4 {
            return Err(UBTError::ParseError);
        }
        let remaining_for_entries = data.len() - offset - 4;
        let max_entries = remaining_for_entries / 161;
        if count > max_entries {
            return Err(UBTError::ParseError);
        }

        let mut entries = Vec::with_capacity(count);
        for _ in 0..count {
            if data.len() < offset + 161 {
                return Err(UBTError::ParseError);
            }
            let entry = parse_entry(data, &mut offset)?;
            entries.push(entry);
        }

        let proof_len = usize::try_from(read_u32_be(data, &mut offset)?)
            .map_err(|_| UBTError::ParseError)?;
        if data.len() < offset + proof_len {
            return Err(UBTError::ParseError);
        }
        let proof_data = &data[offset..offset + proof_len];
        offset += proof_len;

        let proof = parse_multiproof(proof_data)?;
        let mut extensions = Vec::new();

        if offset < data.len() {
            let ext_count = usize::try_from(read_u32_be(data, &mut offset)?)
                .map_err(|_| UBTError::ParseError)?;

            // Guard: cap extension count to prevent memory exhaustion
            const MAX_EXTENSIONS: usize = 256;
            if ext_count > MAX_EXTENSIONS {
                return Err(UBTError::ParseError);
            }

            extensions = Vec::with_capacity(ext_count);
            let mut seen_missing_stems = BTreeSet::new();
            for _ext_idx in 0..ext_count {
                let missing_stem = read_array_31(data, &mut offset)?;
                if !seen_missing_stems.insert(missing_stem) {
                    return Err(UBTError::InvalidProof);
                }
                let stem = read_array_31(data, &mut offset)?;
                let stem_hash = read_array_32(data, &mut offset)?;
                let divergence_depth = read_u16_be(data, &mut offset)?;

                // Sparse encoding: read bitmap (31 bytes) + non-zero siblings
                let bitmap = read_array_31(data, &mut offset)?;

                // Verify extension proof inline to avoid storing 248×32 bytes per extension
                // Validate prefix match
                if divergence_depth >= 248 {
                    return Err(UBTError::InvalidProof);
                }
                if !extension_prefix_matches(&missing_stem, &stem, divergence_depth) {
                    return Err(UBTError::InvalidProof);
                }

                // Verify root reconstruction from bitmap-compressed siblings
                let mut current = stem_hash;
                let mut depth = 247u16;
                for i in 0..248 {
                    let byte_idx = i / 8;
                    let bit_idx = 7 - (i % 8);
                    let bit_set = (bitmap[byte_idx] & (1 << bit_idx)) != 0;

                    let sibling = if bit_set {
                        read_array_32(data, &mut offset)?
                    } else {
                        ZERO32
                    };

                    // Hash with sibling based on stem bit at this depth
                    if stem_bit(&stem, depth) == 0 {
                        current = combine_trie_hashes(current, sibling);
                    } else {
                        current = combine_trie_hashes(sibling, current);
                    }

                    if depth == 0 {
                        break;
                    }
                    depth -= 1;
                }

                // Verify reconstructed hash matches root
                if current != root {
                    return Err(UBTError::RootMismatch);
                }

                // Store only metadata, not siblings (saves ~8KB per extension)
                extensions.push(ExtensionProof {
                    missing_stem,
                    stem,
                    stem_hash,
                    divergence_depth,
                });
            }
        }

        if offset != data.len() {
            return Err(UBTError::LengthMismatch);
        }

        Ok(Self {
            root,
            entries,
            proof,
            extensions,
        })
    }
}

pub fn verify_witness_section(
    data: &[u8],
    expected_root: Option<[u8; 32]>,
    value_kind: WitnessValueKind,
) -> Result<UBTWitnessSection, UBTError> {
    let section = UBTWitnessSection::deserialize(data)?;
    let (head, tail) = witness_head_tail(data);
    let expected_root_str = match expected_root {
        Some(root) => format_segment(&root),
        None => String::from("none"),
    };
    log_info(&format!(
        "🔍 UBT verify start: kind={:?}, len={}, root={}, expected_root={}, head64={}, tail64={}",
        value_kind,
        data.len(),
        format_segment(&section.root),
        expected_root_str,
        head,
        tail
    ));

    if let Some(root) = expected_root {
        if section.root != root {
            return Err(UBTError::RootMismatch);
        }
    }

    verify_multi_proof(&section.proof, section.root)?;
    // Extension proofs already verified inline during deserialization
    verify_entries_against_proof(&section, value_kind)?;

    Ok(section)
}

fn witness_head_tail(data: &[u8]) -> (String, String) {
    if data.is_empty() {
        return (String::new(), String::new());
    }
    let head_len = core::cmp::min(64, data.len());
    let tail_len = core::cmp::min(64, data.len());
    let head = format_segment(&data[..head_len]);
    let tail = format_segment(&data[data.len() - tail_len..]);
    (head, tail)
}

pub fn entries_to_caches(
    entries: &[UBTWitnessEntry],
) -> (
    BTreeMap<H160, U256>,
    BTreeMap<H160, U256>,
    BTreeMap<H160, Vec<u8>>,
    BTreeMap<H160, H256>,
    BTreeMap<(H160, H256), H256>,
) {
    let mut balances = BTreeMap::new();
    let mut nonces = BTreeMap::new();
    let mut code_sizes = BTreeMap::new();
    let mut code_chunks: BTreeMap<H160, BTreeMap<u64, Vec<u8>>> = BTreeMap::new();
    let mut code = BTreeMap::new();
    let mut code_hashes = BTreeMap::new();
    let mut storage = BTreeMap::new();

    for entry in entries {
        let metadata = &entry.metadata;
        let pre_value = &entry.pre_value;
        let address = metadata.address;

        match metadata.key_type {
            UBTKeyType::BasicData => {
                let code_size =
                    ((pre_value[5] as u32) << 16) | ((pre_value[6] as u32) << 8) | pre_value[7] as u32;
                code_sizes.insert(address, code_size);

                let nonce_bytes = &pre_value[8..16];
                let nonce = U256::from_big_endian(nonce_bytes);
                nonces.insert(address, nonce);

                let mut balance_bytes = [0u8; 32];
                balance_bytes[16..32].copy_from_slice(&pre_value[16..32]);
                let balance = U256::from_big_endian(&balance_bytes);
                balances.insert(address, balance);
            }
            UBTKeyType::CodeHash => {
                let code_hash = if *pre_value == ZERO32 {
                    H256::from(keccak256(&[]))
                } else {
                    H256::from(*pre_value)
                };
                code_hashes.insert(address, code_hash);
            }
            UBTKeyType::CodeChunk => {
                let chunk_id = metadata.extra;
                let chunk_data = pre_value.to_vec();
                code_chunks
                    .entry(address)
                    .or_insert_with(BTreeMap::new)
                    .insert(chunk_id, chunk_data);
            }
            UBTKeyType::Storage => {
                let storage_key = metadata.storage_key;
                let storage_value = H256::from(*pre_value);
                storage.insert((address, storage_key), storage_value);
            }
        }
    }

    for (address, chunks) in code_chunks {
        let mut bytecode = Vec::new();
        for (_chunk_id, chunk_data) in chunks {
            bytecode.extend_from_slice(&chunk_data);
        }
        if let Some(code_size) = code_sizes.get(&address) {
            let size = match usize::try_from(*code_size) {
                Ok(size) => size,
                Err(_) => bytecode.len(),
            };
            if size < bytecode.len() {
                bytecode.truncate(size);
            }
        }
        code.insert(address, bytecode);
    }

    (balances, nonces, code, code_hashes, storage)
}

#[derive(Debug, Clone, Copy)]
pub struct CompactTreeKey {
    pub stem_index: u32,
    pub subindex: u8,
}

#[derive(Debug, Clone)]
pub struct MultiProof {
    pub keys: Vec<CompactTreeKey>,
    pub values: Vec<Option<[u8; 32]>>,
    pub nodes: Vec<[u8; 32]>,
    pub node_refs: Vec<u32>,
    pub stems: Vec<Stem>,
}

impl MultiProof {
    pub fn expand_key(&self, idx: usize) -> Result<TreeKey, UBTError> {
        if idx >= self.keys.len() {
            return Err(UBTError::InvalidProof);
        }
        let key = self.keys[idx];
        let stem_index =
            usize::try_from(key.stem_index).map_err(|_| UBTError::InvalidProof)?;
        if stem_index >= self.stems.len() {
            return Err(UBTError::InvalidProof);
        }
        Ok(TreeKey {
            stem: self.stems[stem_index],
            subindex: key.subindex,
        })
    }

    pub fn expand_key_bytes(&self, idx: usize) -> Result<[u8; 32], UBTError> {
        Ok(self.expand_key(idx)?.to_bytes())
    }
}

pub type Stem = [u8; 31];

#[derive(Debug, Clone, Copy)]
pub struct TreeKey {
    pub stem: Stem,
    pub subindex: u8,
}

impl TreeKey {
    pub fn to_bytes(self) -> [u8; 32] {
        let mut out = [0u8; 32];
        out[..31].copy_from_slice(&self.stem);
        out[31] = self.subindex;
        out
    }
}

fn parse_entry(data: &[u8], offset: &mut usize) -> Result<UBTWitnessEntry, UBTError> {
    let tree_key = read_array_32(data, offset)?;
    let key_type_byte = read_u8(data, offset)?;
    let key_type = match key_type_byte {
        0 => UBTKeyType::BasicData,
        1 => UBTKeyType::CodeHash,
        2 => UBTKeyType::CodeChunk,
        3 => UBTKeyType::Storage,
        _value => return Err(UBTError::InvalidKeyType(())),
    };

    let address_bytes = read_array_20(data, offset)?;
    let address = H160::from(address_bytes);

    let extra = read_u64_be(data, offset)?;
    let storage_key = H256::from(read_array_32(data, offset)?);
    let pre_value = read_array_32(data, offset)?;
    let post_value = read_array_32(data, offset)?;
    let tx_index = read_u32_be(data, offset)?;

    Ok(UBTWitnessEntry {
        tree_key,
        pre_value,
        post_value,
        metadata: UBTKeyMetadata {
            key_type,
            address,
            extra,
            storage_key,
            tx_index,
        },
    })
}

fn parse_multiproof(data: &[u8]) -> Result<MultiProof, UBTError> {
    let mut offset = 0usize;
    let keys_len = usize::try_from(read_u32_le(data, &mut offset)?)
        .map_err(|_| UBTError::ParseError)?;
    let values_len = usize::try_from(read_u32_le(data, &mut offset)?)
        .map_err(|_| UBTError::ParseError)?;
    let nodes_len = usize::try_from(read_u32_le(data, &mut offset)?)
        .map_err(|_| UBTError::ParseError)?;
    let node_refs_len = usize::try_from(read_u32_le(data, &mut offset)?)
        .map_err(|_| UBTError::ParseError)?;
    let stems_len = usize::try_from(read_u32_le(data, &mut offset)?)
        .map_err(|_| UBTError::ParseError)?;

    let min_bytes = keys_len
        .checked_mul(5)
        .and_then(|v| v.checked_add(values_len))
        .and_then(|v| v.checked_add(nodes_len.checked_mul(32)?))
        .and_then(|v| v.checked_add(node_refs_len.checked_mul(4)?))
        .and_then(|v| v.checked_add(stems_len.checked_mul(31)?))
        .ok_or(UBTError::ParseError)?;
    if min_bytes > data.len().saturating_sub(offset) {
        return Err(UBTError::ParseError);
    }

    let mut keys = Vec::with_capacity(keys_len);
    for _ in 0..keys_len {
        let stem_index = read_u32_le(data, &mut offset)?;
        let subindex = read_u8(data, &mut offset)?;
        keys.push(CompactTreeKey { stem_index, subindex });
    }

    let mut values = Vec::with_capacity(values_len);
    for _ in 0..values_len {
        let tag = read_u8(data, &mut offset)?;
        if tag == 0 {
            values.push(None);
        } else if tag == 1 {
            let value = read_array_32(data, &mut offset)?;
            values.push(Some(value));
        } else {
            return Err(UBTError::ParseError);
        }
    }

    let mut nodes = Vec::with_capacity(nodes_len);
    for _ in 0..nodes_len {
        nodes.push(read_array_32(data, &mut offset)?);
    }

    let mut node_refs = Vec::with_capacity(node_refs_len);
    for _ in 0..node_refs_len {
        node_refs.push(read_u32_le(data, &mut offset)?);
    }

    let mut stems = Vec::with_capacity(stems_len);
    for _ in 0..stems_len {
        stems.push(read_array_31(data, &mut offset)?);
    }

    if offset != data.len() {
        return Err(UBTError::LengthMismatch);
    }

    Ok(MultiProof {
        keys,
        values,
        nodes,
        node_refs,
        stems,
    })
}

fn verify_entries_against_proof(
    section: &UBTWitnessSection,
    value_kind: WitnessValueKind,
) -> Result<(), UBTError> {
    let mut proof_values: BTreeMap<[u8; 32], Option<[u8; 32]>> = BTreeMap::new();
    for idx in 0..section.proof.keys.len() {
        let expected_key = section.proof.expand_key_bytes(idx)?;
        proof_values.insert(expected_key, section.proof.values[idx]);
    }

    let mut extension_map: BTreeMap<Stem, &ExtensionProof> = BTreeMap::new();
    for ext in &section.extensions {
        extension_map.insert(ext.missing_stem, ext);
    }

    let mut seen_proof: BTreeMap<[u8; 32], ()> = BTreeMap::new();
    let mut seen_extension: BTreeMap<Stem, ()> = BTreeMap::new();

    for entry in &section.entries {
        let entry_value = match value_kind {
            WitnessValueKind::Pre => entry.pre_value,
            WitnessValueKind::Post => entry.post_value,
        };

        if let Some(proof_value) = proof_values.get(&entry.tree_key) {
            if !value_matches_proof(entry_value, *proof_value) {
                return Err(UBTError::WitnessMismatch);
            }
            seen_proof.insert(entry.tree_key, ());
        } else if let Some(_ext) = extension_map.get(&tree_key_from_bytes(entry.tree_key).stem) {
            if entry_value != ZERO32 {
                return Err(UBTError::WitnessMismatch);
            }
            seen_extension.insert(tree_key_from_bytes(entry.tree_key).stem, ());
        } else if section.root == ZERO32
            && section.proof.keys.is_empty()
            && section.extensions.is_empty()
        {
            if entry_value != ZERO32 {
                return Err(UBTError::WitnessMismatch);
            }
        } else {
            return Err(UBTError::WitnessMismatch);
        }

        if matches!(value_kind, WitnessValueKind::Pre) && entry.pre_value != entry.post_value {
            return Err(UBTError::WitnessMismatch);
        }
    }

    if seen_proof.len() != proof_values.len() {
        return Err(UBTError::WitnessMismatch);
    }
    if seen_extension.len() != extension_map.len() {
        return Err(UBTError::WitnessMismatch);
    }

    Ok(())
}

fn value_matches_proof(entry_value: [u8; 32], proof_value: Option<[u8; 32]>) -> bool {
    match proof_value {
        Some(val) => val == entry_value,
        None => entry_value == ZERO32,
    }
}

fn verify_multi_proof(mp: &MultiProof, expected_root: [u8; 32]) -> Result<(), UBTError> {
    if mp.keys.len() != mp.values.len() {
        return Err(UBTError::InvalidProof);
    }
    if !stems_sorted(&mp.stems) {
        return Err(UBTError::InvalidProof);
    }
    validate_multi_proof_canonical(mp)?;

    let mut value_map: BTreeMap<Stem, BTreeMap<u8, Option<[u8; 32]>>> = BTreeMap::new();
    for (idx, key) in mp.keys.iter().enumerate() {
        let stem_idx =
            usize::try_from(key.stem_index).map_err(|_| UBTError::InvalidProof)?;
        if stem_idx >= mp.stems.len() {
            return Err(UBTError::InvalidProof);
        }
        let stem = mp.stems[stem_idx];
        value_map
            .entry(stem)
            .or_insert_with(BTreeMap::new)
            .insert(key.subindex, mp.values[idx]);
    }

    let stems: Vec<Stem> = value_map.keys().copied().collect();
    let mut stream = NodeStream::new(&mp.nodes, &mp.node_refs);
    let root = rebuild_stem_trie(&stems, &value_map, 0, [0u8; 31], &mut stream)?;
    if stream.idx != stream.expected_count() {
        return Err(UBTError::InvalidProof);
    }
    if root != expected_root {
        return Err(UBTError::RootMismatch);
    }
    Ok(())
}

// Extension proofs are now verified inline during deserialization to save memory.
// This function is no longer used but kept for reference.
#[allow(dead_code)]
fn verify_extension_proofs_old(
    _proofs: &[ExtensionProof],
    _expected_root: [u8; 32],
) -> Result<(), UBTError> {
    // Moved to inline verification in deserialize() to avoid storing
    // 248×32 bytes of sibling hashes per extension proof.
    Ok(())
}

fn extension_prefix_matches(query: &Stem, extension: &Stem, depth: u16) -> bool {
    for i in 0..depth {
        if stem_bit(query, i) != stem_bit(extension, i) {
            return false;
        }
    }
    stem_bit(query, depth) != stem_bit(extension, depth)
}

fn tree_key_from_bytes(bytes: [u8; 32]) -> TreeKey {
    let mut stem = [0u8; 31];
    stem.copy_from_slice(&bytes[..31]);
    TreeKey {
        stem,
        subindex: bytes[31],
    }
}

fn validate_multi_proof_canonical(mp: &MultiProof) -> Result<(), UBTError> {
    validate_multi_proof_keys(mp)?;

    if mp.keys.is_empty() {
        if !mp.stems.is_empty() {
            return Err(UBTError::InvalidProof);
        }
    } else {
        let mut referenced = vec![false; mp.stems.len()];
        for key in &mp.keys {
            let idx = usize::try_from(key.stem_index).map_err(|_| UBTError::InvalidProof)?;
            if idx >= referenced.len() {
                return Err(UBTError::InvalidProof);
            }
            referenced[idx] = true;
        }
        if referenced.iter().any(|used| !*used) {
            return Err(UBTError::InvalidProof);
        }
    }

    if !mp.node_refs.is_empty() {
        validate_node_refs(mp)?;
    } else if !mp.nodes.is_empty() && mp.node_refs.is_empty() {
        validate_nodes_unique(mp)?;
    }

    Ok(())
}

fn validate_multi_proof_keys(mp: &MultiProof) -> Result<(), UBTError> {
    if mp.keys.is_empty() {
        return Ok(());
    }

    let mut prev: Option<TreeKey> = None;
    for key in &mp.keys {
        let stem_idx =
            usize::try_from(key.stem_index).map_err(|_| UBTError::InvalidProof)?;
        if stem_idx >= mp.stems.len() {
            return Err(UBTError::InvalidProof);
        }
        let current = TreeKey {
            stem: mp.stems[stem_idx],
            subindex: key.subindex,
        };

        if let Some(prev_key) = prev {
            if stem_less(&current.stem, &prev_key.stem) {
                return Err(UBTError::InvalidProof);
            }
            if stem_less(&prev_key.stem, &current.stem) {
                // ok
            } else if current.subindex <= prev_key.subindex {
                return Err(UBTError::InvalidProof);
            }
        }

        prev = Some(current);
    }

    Ok(())
}

fn validate_node_refs(mp: &MultiProof) -> Result<(), UBTError> {
    if mp.nodes.is_empty() {
        return Err(UBTError::InvalidProof);
    }

    let mut seen = BTreeMap::new();
    for node in &mp.nodes {
        if seen.insert(*node, ()).is_some() {
            return Err(UBTError::InvalidProof);
        }
    }

    let mut referenced = vec![false; mp.nodes.len()];
    for ref_idx in &mp.node_refs {
        let idx = usize::try_from(*ref_idx).map_err(|_| UBTError::InvalidProof)?;
        if idx >= mp.nodes.len() {
            return Err(UBTError::InvalidProof);
        }
        referenced[idx] = true;
    }
    if referenced.iter().any(|used| !*used) {
        return Err(UBTError::InvalidProof);
    }

    Ok(())
}

fn validate_nodes_unique(mp: &MultiProof) -> Result<(), UBTError> {
    let mut seen = BTreeMap::new();
    for node in &mp.nodes {
        if seen.insert(*node, ()).is_some() {
            return Err(UBTError::InvalidProof);
        }
    }
    Ok(())
}

struct NodeStream<'a> {
    nodes: &'a [[u8; 32]],
    refs: &'a [u32],
    idx: usize,
}

impl<'a> NodeStream<'a> {
    fn new(nodes: &'a [[u8; 32]], refs: &'a [u32]) -> Self {
        Self { nodes, refs, idx: 0 }
    }

    fn consume(&mut self) -> Result<[u8; 32], UBTError> {
        if !self.refs.is_empty() {
            if self.idx >= self.refs.len() {
                return Err(UBTError::InvalidProof);
            }
            let ref_idx = usize::try_from(self.refs[self.idx]).map_err(|_| UBTError::InvalidProof)?;
            self.idx += 1;
            if ref_idx >= self.nodes.len() {
                return Err(UBTError::InvalidProof);
            }
            return Ok(self.nodes[ref_idx]);
        }

        if self.idx >= self.nodes.len() {
            return Err(UBTError::InvalidProof);
        }
        let node = self.nodes[self.idx];
        self.idx += 1;
        Ok(node)
    }

    fn expected_count(&self) -> usize {
        if !self.refs.is_empty() {
            self.refs.len()
        } else {
            self.nodes.len()
        }
    }
}

fn rebuild_stem_trie(
    stems: &[Stem],
    value_map: &BTreeMap<Stem, BTreeMap<u8, Option<[u8; 32]>>>,
    depth: u16,
    prefix: Stem,
    stream: &mut NodeStream<'_>,
) -> Result<[u8; 32], UBTError> {
    if stems.is_empty() {
        return Ok(ZERO32);
    }
    if depth >= 248 {
        if stems.len() != 1 {
            return Err(UBTError::InvalidProof);
        }
        let stem = stems[0];
        let values = value_map.get(&stem).ok_or(UBTError::InvalidProof)?;
        let root = rebuild_stem_subtree(values, 0, 0, stream)?;
        return Ok(hash_stem_node(&stem, &root));
    }

    let mut left = Vec::new();
    let mut right = Vec::new();
    for stem in stems {
        if stem_bit(stem, depth) == 0 {
            left.push(*stem);
        } else {
            right.push(*stem);
        }
    }

    let (left_prefix, right_prefix) = split_prefix(prefix, depth);

    if !left.is_empty() && !right.is_empty() {
        let left_hash = rebuild_stem_trie(&left, value_map, depth + 1, left_prefix, stream)?;
        let right_hash = rebuild_stem_trie(&right, value_map, depth + 1, right_prefix, stream)?;
        return Ok(combine_trie_hashes(left_hash, right_hash));
    }

    if !left.is_empty() {
        let left_hash = rebuild_stem_trie(&left, value_map, depth + 1, left_prefix, stream)?;
        let sibling = stream.consume()?;
        return Ok(combine_trie_hashes(left_hash, sibling));
    }

    let sibling = stream.consume()?;
    let right_hash = rebuild_stem_trie(&right, value_map, depth + 1, right_prefix, stream)?;
    Ok(combine_trie_hashes(sibling, right_hash))
}

fn rebuild_stem_subtree(
    values: &BTreeMap<u8, Option<[u8; 32]>>,
    depth: u8,
    prefix: u8,
    stream: &mut NodeStream<'_>,
) -> Result<[u8; 32], UBTError> {
    if depth >= 8 {
        return Ok(match values.get(&prefix) {
            Some(Some(val)) => hash32(val),
            _ => ZERO32,
        });
    }

    let bit_pos = 7 - depth;
    let mut left_map = BTreeMap::new();
    let mut right_map = BTreeMap::new();
    for (key, val) in values {
        if (key >> bit_pos) & 1 == 0 {
            left_map.insert(*key, *val);
        } else {
            right_map.insert(*key, *val);
        }
    }

    let (left_prefix, right_prefix) = split_subindex_prefix(prefix, bit_pos);

    if !left_map.is_empty() && !right_map.is_empty() {
        let left_hash = rebuild_stem_subtree(&left_map, depth + 1, left_prefix, stream)?;
        let right_hash = rebuild_stem_subtree(&right_map, depth + 1, right_prefix, stream)?;
        return Ok(hash64(&left_hash, &right_hash));
    }

    if !left_map.is_empty() {
        let left_hash = rebuild_stem_subtree(&left_map, depth + 1, left_prefix, stream)?;
        let sibling = stream.consume()?;
        return Ok(hash64(&left_hash, &sibling));
    }

    let sibling = stream.consume()?;
    let right_hash = rebuild_stem_subtree(&right_map, depth + 1, right_prefix, stream)?;
    Ok(hash64(&sibling, &right_hash))
}

fn stem_bit(stem: &Stem, idx: u16) -> u8 {
    let i = usize::from(idx);
    (stem[i / 8] >> (7 - i % 8)) & 1
}

fn split_prefix(prefix: Stem, depth: u16) -> (Stem, Stem) {
    let mut left = prefix;
    let mut right = prefix;
    let byte_index = usize::from(depth / 8);
    let bit_index = 7 - (depth % 8) as u8;
    left[byte_index] &= !(1 << bit_index);
    right[byte_index] |= 1 << bit_index;
    (left, right)
}

fn split_subindex_prefix(prefix: u8, bit_pos: u8) -> (u8, u8) {
    let left = prefix & !(1 << bit_pos);
    let right = prefix | (1 << bit_pos);
    (left, right)
}

fn stems_sorted(stems: &[Stem]) -> bool {
    for i in 1..stems.len() {
        if !stem_less(&stems[i - 1], &stems[i]) {
            return false;
        }
    }
    true
}

fn stem_less(a: &Stem, b: &Stem) -> bool {
    for i in 0..a.len() {
        if a[i] < b[i] {
            return true;
        }
        if a[i] > b[i] {
            return false;
        }
    }
    false
}

fn combine_trie_hashes(left: [u8; 32], right: [u8; 32]) -> [u8; 32] {
    if left == ZERO32 {
        return right;
    }
    if right == ZERO32 {
        return left;
    }
    hash64(&left, &right)
}

fn hash32(value: &[u8; 32]) -> [u8; 32] {
    let mut buf = [0u8; 33];
    buf[0] = 0x00;
    buf[1..].copy_from_slice(value);
    *blake3::hash(&buf).as_bytes()
}

fn hash64(left: &[u8; 32], right: &[u8; 32]) -> [u8; 32] {
    if *left == ZERO32 && *right == ZERO32 {
        return ZERO32;
    }
    let mut buf = [0u8; 65];
    buf[0] = 0x01;
    buf[1..33].copy_from_slice(left);
    buf[33..].copy_from_slice(right);
    *blake3::hash(&buf).as_bytes()
}

fn hash_stem_node(stem: &Stem, subtree_root: &[u8; 32]) -> [u8; 32] {
    let mut buf = [0u8; 65];
    buf[0] = 0x02;
    buf[1..32].copy_from_slice(stem);
    buf[32] = 0x00;
    buf[33..].copy_from_slice(subtree_root);
    *blake3::hash(&buf).as_bytes()
}

fn read_u8(data: &[u8], offset: &mut usize) -> Result<u8, UBTError> {
    if *offset >= data.len() {
        return Err(UBTError::ParseError);
    }
    let out = data[*offset];
    *offset += 1;
    Ok(out)
}

fn read_u32_be(data: &[u8], offset: &mut usize) -> Result<u32, UBTError> {
    let block = read_block(data, offset, 4)?;
    Ok(u32::from_be_bytes([block[0], block[1], block[2], block[3]]))
}

fn read_u16_be(data: &[u8], offset: &mut usize) -> Result<u16, UBTError> {
    let block = read_block(data, offset, 2)?;
    Ok(u16::from_be_bytes([block[0], block[1]]))
}

fn read_u32_le(data: &[u8], offset: &mut usize) -> Result<u32, UBTError> {
    let block = read_block(data, offset, 4)?;
    Ok(u32::from_le_bytes([block[0], block[1], block[2], block[3]]))
}

fn read_u64_be(data: &[u8], offset: &mut usize) -> Result<u64, UBTError> {
    let block = read_block(data, offset, 8)?;
    Ok(u64::from_be_bytes([
        block[0], block[1], block[2], block[3], block[4], block[5], block[6], block[7],
    ]))
}

fn read_array_20(data: &[u8], offset: &mut usize) -> Result<[u8; 20], UBTError> {
    let block = read_block(data, offset, 20)?;
    let mut out = [0u8; 20];
    out.copy_from_slice(block);
    Ok(out)
}

fn read_array_31(data: &[u8], offset: &mut usize) -> Result<[u8; 31], UBTError> {
    let block = read_block(data, offset, 31)?;
    let mut out = [0u8; 31];
    out.copy_from_slice(block);
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;
    use alloc::vec;
    use alloc::vec::Vec;

    fn subtree_root_single(subindex: u8, value: [u8; 32]) -> [u8; 32] {
        let mut current = hash32(&value);
        for bit_pos in 0..8 {
            if (subindex >> bit_pos) & 1 == 0 {
                current = hash64(&current, &ZERO32);
            } else {
                current = hash64(&ZERO32, &current);
            }
        }
        current
    }

    fn single_key_multiproof(
        stem: Stem,
        subindex: u8,
        value: [u8; 32],
    ) -> (MultiProof, [u8; 32]) {
        let nodes = vec![ZERO32; 256];
        let proof = MultiProof {
            keys: vec![CompactTreeKey {
                stem_index: 0,
                subindex,
            }],
            values: vec![Some(value)],
            nodes,
            node_refs: Vec::new(),
            stems: vec![stem],
        };
        let subtree_root = subtree_root_single(subindex, value);
        let root = hash_stem_node(&stem, &subtree_root);
        (proof, root)
    }

    fn serialize_multiproof(mp: &MultiProof) -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&(mp.keys.len() as u32).to_le_bytes());
        out.extend_from_slice(&(mp.values.len() as u32).to_le_bytes());
        out.extend_from_slice(&(mp.nodes.len() as u32).to_le_bytes());
        out.extend_from_slice(&(mp.node_refs.len() as u32).to_le_bytes());
        out.extend_from_slice(&(mp.stems.len() as u32).to_le_bytes());

        for key in &mp.keys {
            out.extend_from_slice(&key.stem_index.to_le_bytes());
            out.push(key.subindex);
        }

        for value in &mp.values {
            match value {
                None => out.push(0),
                Some(val) => {
                    out.push(1);
                    out.extend_from_slice(val);
                }
            }
        }

        for node in &mp.nodes {
            out.extend_from_slice(node);
        }

        for ref_idx in &mp.node_refs {
            out.extend_from_slice(&ref_idx.to_le_bytes());
        }

        for stem in &mp.stems {
            out.extend_from_slice(stem);
        }

        out
    }

    fn serialize_witness_section(root: [u8; 32], entries: &[UBTWitnessEntry], proof: &MultiProof) -> Vec<u8> {
        let mut out = Vec::new();
        out.extend_from_slice(&root);
        out.extend_from_slice(&(entries.len() as u32).to_be_bytes());

        for entry in entries {
            out.extend_from_slice(&entry.tree_key);
            out.push(entry.metadata.key_type as u8);
            out.extend_from_slice(entry.metadata.address.as_bytes());
            out.extend_from_slice(&entry.metadata.extra.to_be_bytes());
            out.extend_from_slice(entry.metadata.storage_key.as_bytes());
            out.extend_from_slice(&entry.pre_value);
            out.extend_from_slice(&entry.post_value);
            out.extend_from_slice(&entry.metadata.tx_index.to_be_bytes());
        }

        let proof_bytes = serialize_multiproof(proof);
        out.extend_from_slice(&(proof_bytes.len() as u32).to_be_bytes());
        out.extend_from_slice(&proof_bytes);
        out
    }

    #[test]
    fn test_parse_multiproof_round_trip() {
        let mut stem = [0u8; 31];
        stem[0] = 0xAB;
        let value = [0x11u8; 32];
        let (proof, _) = single_key_multiproof(stem, 0x10, value);
        let encoded = serialize_multiproof(&proof);
        let decoded = parse_multiproof(&encoded).expect("parse multiproof");

        assert_eq!(decoded.keys.len(), 1);
        assert_eq!(decoded.values.len(), 1);
        assert_eq!(decoded.nodes.len(), 256);
        assert!(decoded.node_refs.is_empty());
        assert_eq!(decoded.stems, vec![stem]);
        assert_eq!(decoded.keys[0].stem_index, 0);
        assert_eq!(decoded.keys[0].subindex, 0x10);
        assert_eq!(decoded.values[0], Some(value));
    }

    #[test]
    fn test_verify_witness_section_pre_post_single_key() {
        let mut stem = [0u8; 31];
        stem[0] = 0x42;
        let subindex = 0x22;

        let pre_value = [0x01u8; 32];
        let post_value = [0x02u8; 32];

        let (pre_proof, pre_root) = single_key_multiproof(stem, subindex, pre_value);
        let (post_proof, post_root) = single_key_multiproof(stem, subindex, post_value);

        let mut tree_key = [0u8; 32];
        tree_key[..31].copy_from_slice(&stem);
        tree_key[31] = subindex;

        let address = H160::from([0u8; 20]);
        let storage_key = H256::from([0u8; 32]);
        let entry_pre = UBTWitnessEntry {
            tree_key,
            pre_value,
            post_value: pre_value,
            metadata: UBTKeyMetadata {
                key_type: UBTKeyType::BasicData,
                address,
                extra: 0,
                storage_key,
                tx_index: 0,
            },
        };

        let entry_post = UBTWitnessEntry {
            tree_key,
            pre_value,
            post_value,
            metadata: UBTKeyMetadata {
                key_type: UBTKeyType::BasicData,
                address,
                extra: 0,
                storage_key,
                tx_index: 0,
            },
        };

        let pre_bytes = serialize_witness_section(pre_root, &[entry_pre.clone()], &pre_proof);
        let post_bytes = serialize_witness_section(post_root, &[entry_post.clone()], &post_proof);

        let pre_section =
            verify_witness_section(&pre_bytes, Some(pre_root), WitnessValueKind::Pre).expect("pre");
        assert_eq!(pre_section.entries.len(), 1);

        let post_section =
            verify_witness_section(&post_bytes, Some(post_root), WitnessValueKind::Post).expect("post");
        assert_eq!(post_section.entries.len(), 1);
    }

    #[test]
    fn test_entries_to_caches_with_basic_data_and_storage() {
        let mut address_bytes = [0u8; 20];
        address_bytes[0] = 0x11;
        let address = H160::from(address_bytes);

        let mut basic = [0u8; 32];
        basic[7] = 0x03; // code size = 3
        basic[15] = 0x2A; // nonce = 42
        basic[31] = 0x7F; // balance = 127

        let storage_key = H256::from([0x55u8; 32]);
        let storage_value = [0x99u8; 32];

        let entries = vec![
            UBTWitnessEntry {
                tree_key: [0u8; 32],
                pre_value: basic,
                post_value: basic,
                metadata: UBTKeyMetadata {
                    key_type: UBTKeyType::BasicData,
                    address,
                    extra: 0,
                    storage_key: H256::from([0u8; 32]),
                    tx_index: 0,
                },
            },
            UBTWitnessEntry {
                tree_key: [0u8; 32],
                pre_value: [0xAAu8; 32],
                post_value: [0u8; 32],
                metadata: UBTKeyMetadata {
                    key_type: UBTKeyType::CodeChunk,
                    address,
                    extra: 0,
                    storage_key: H256::from([0u8; 32]),
                    tx_index: 0,
                },
            },
            UBTWitnessEntry {
                tree_key: [0u8; 32],
                pre_value: [0x11u8; 32],
                post_value: [0u8; 32],
                metadata: UBTKeyMetadata {
                    key_type: UBTKeyType::CodeHash,
                    address,
                    extra: 0,
                    storage_key: H256::from([0u8; 32]),
                    tx_index: 0,
                },
            },
            UBTWitnessEntry {
                tree_key: [0u8; 32],
                pre_value: storage_value,
                post_value: [0u8; 32],
                metadata: UBTKeyMetadata {
                    key_type: UBTKeyType::Storage,
                    address,
                    extra: 0,
                    storage_key,
                    tx_index: 0,
                },
            },
        ];

        let (balances, nonces, code, code_hashes, storage) = entries_to_caches(&entries);
        assert_eq!(balances.get(&address), Some(&U256::from(127u64)));
        assert_eq!(nonces.get(&address), Some(&U256::from(42u64)));
        assert_eq!(code.get(&address).map(|v| v.len()), Some(3));
        assert_eq!(code_hashes.get(&address), Some(&H256::from([0x11u8; 32])));
        assert_eq!(
            storage.get(&(address, storage_key)),
            Some(&H256::from(storage_value))
        );
    }
}

fn read_array_32(data: &[u8], offset: &mut usize) -> Result<[u8; 32], UBTError> {
    let block = read_block(data, offset, 32)?;
    let mut out = [0u8; 32];
    out.copy_from_slice(block);
    Ok(out)
}

fn read_block<'a>(
    data: &'a [u8],
    offset: &mut usize,
    len: usize,
) -> Result<&'a [u8], UBTError> {
    if data.len() < *offset + len {
        return Err(UBTError::ParseError);
    }
    let out = &data[*offset..*offset + len];
    *offset += len;
    Ok(out)
}
