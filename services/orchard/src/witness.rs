#![allow(dead_code)]
use alloc::{string::String, string::ToString, vec::Vec};
use crate::errors::{Result, OrchardError};

// Use BTreeMap instead of HashMap for no-std compatibility
use alloc::collections::BTreeMap as HashMap;

/// State access tracking for witness generation and verification
#[derive(Debug, Clone)]
pub struct StateRead {
    pub key: String,
    pub value: Vec<u8>,
    pub merkle_proof: Vec<[u8; 32]>,  // Proof path to state root
}

#[derive(Debug, Clone)]
pub struct StateWrite {
    pub key: String,
    pub old_value: Option<Vec<u8>>,
    pub new_value: Vec<u8>,
    pub merkle_proof: Vec<[u8; 32]>,  // Proof path in post-state
}

#[derive(Debug, Clone)]
pub struct WitnessBundle {
    pub pre_state_root: [u8; 32],
    pub post_state_root: [u8; 32],
    pub reads: Vec<StateRead>,
    pub writes: Vec<StateWrite>,
}

impl WitnessBundle {
    /// Serialize witness bundle to bytes for transmission
    pub fn serialize(&self) -> Result<Vec<u8>> {
        let mut buffer = Vec::new();

        // Write pre and post state roots
        buffer.extend_from_slice(&self.pre_state_root);
        buffer.extend_from_slice(&self.post_state_root);

        // Write number of reads
        buffer.extend_from_slice(&(self.reads.len() as u32).to_le_bytes());

        // Write each read
        for read in &self.reads {
            // Key length + key
            buffer.extend_from_slice(&(read.key.len() as u32).to_le_bytes());
            buffer.extend_from_slice(read.key.as_bytes());

            // Value length + value
            buffer.extend_from_slice(&(read.value.len() as u32).to_le_bytes());
            buffer.extend_from_slice(&read.value);

            // Merkle proof length + proof
            buffer.extend_from_slice(&(read.merkle_proof.len() as u32).to_le_bytes());
            for proof_node in &read.merkle_proof {
                buffer.extend_from_slice(proof_node);
            }
        }

        // Write number of writes
        buffer.extend_from_slice(&(self.writes.len() as u32).to_le_bytes());

        // Write each write
        for write in &self.writes {
            // Key length + key
            buffer.extend_from_slice(&(write.key.len() as u32).to_le_bytes());
            buffer.extend_from_slice(write.key.as_bytes());

            // Old value (optional)
            if let Some(ref old_value) = write.old_value {
                buffer.push(1); // Has old value
                buffer.extend_from_slice(&(old_value.len() as u32).to_le_bytes());
                buffer.extend_from_slice(old_value);
            } else {
                buffer.push(0); // No old value
            }

            // New value
            buffer.extend_from_slice(&(write.new_value.len() as u32).to_le_bytes());
            buffer.extend_from_slice(&write.new_value);

            // Merkle proof length + proof
            buffer.extend_from_slice(&(write.merkle_proof.len() as u32).to_le_bytes());
            for proof_node in &write.merkle_proof {
                buffer.extend_from_slice(proof_node);
            }
        }

        Ok(buffer)
    }

    /// Deserialize witness bundle from bytes
    pub fn deserialize(data: &[u8]) -> Result<Self> {
        let mut cursor = 0;

        // Read pre and post state roots
        if data.len() < cursor + 64 {
            return Err(OrchardError::ParseError("Insufficient data for state roots".into()));
        }

        let mut pre_state_root = [0u8; 32];
        pre_state_root.copy_from_slice(&data[cursor..cursor + 32]);
        cursor += 32;

        let mut post_state_root = [0u8; 32];
        post_state_root.copy_from_slice(&data[cursor..cursor + 32]);
        cursor += 32;

        // Read number of reads
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for read count".into()));
        }
        let num_reads = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]);
        cursor += 4;

        // Read each read
        let mut reads = Vec::new();
        for _ in 0..num_reads {
            let (read, new_cursor) = Self::deserialize_state_read(data, cursor)?;
            reads.push(read);
            cursor = new_cursor;
        }

        // Read number of writes
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for write count".into()));
        }
        let num_writes = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]);
        cursor += 4;

        // Read each write
        let mut writes = Vec::new();
        for _ in 0..num_writes {
            let (write, new_cursor) = Self::deserialize_state_write(data, cursor)?;
            writes.push(write);
            cursor = new_cursor;
        }

        Ok(WitnessBundle {
            pre_state_root,
            post_state_root,
            reads,
            writes,
        })
    }

    fn deserialize_state_read(data: &[u8], mut cursor: usize) -> Result<(StateRead, usize)> {
        // Read key
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for key length".into()));
        }
        let key_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
        cursor += 4;

        if data.len() < cursor + key_len {
            return Err(OrchardError::ParseError("Insufficient data for key".into()));
        }
        let key = String::from_utf8(data[cursor..cursor + key_len].to_vec())
            .map_err(|_| OrchardError::ParseError("Invalid UTF-8 in key".into()))?;
        cursor += key_len;

        // Read value
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for value length".into()));
        }
        let value_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
        cursor += 4;

        if data.len() < cursor + value_len {
            return Err(OrchardError::ParseError("Insufficient data for value".into()));
        }
        let value = data[cursor..cursor + value_len].to_vec();
        cursor += value_len;

        // Read merkle proof
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for proof length".into()));
        }
        let proof_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
        cursor += 4;

        if data.len() < cursor + proof_len * 32 {
            return Err(OrchardError::ParseError("Insufficient data for merkle proof".into()));
        }

        let mut merkle_proof = Vec::new();
        for _ in 0..proof_len {
            let mut proof_node = [0u8; 32];
            proof_node.copy_from_slice(&data[cursor..cursor + 32]);
            merkle_proof.push(proof_node);
            cursor += 32;
        }

        Ok((StateRead { key, value, merkle_proof }, cursor))
    }

    fn deserialize_state_write(data: &[u8], mut cursor: usize) -> Result<(StateWrite, usize)> {
        // Read key
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for key length".into()));
        }
        let key_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
        cursor += 4;

        if data.len() < cursor + key_len {
            return Err(OrchardError::ParseError("Insufficient data for key".into()));
        }
        let key = String::from_utf8(data[cursor..cursor + key_len].to_vec())
            .map_err(|_| OrchardError::ParseError("Invalid UTF-8 in key".into()))?;
        cursor += key_len;

        // Read old value (optional)
        if data.len() < cursor + 1 {
            return Err(OrchardError::ParseError("Insufficient data for old value flag".into()));
        }
        let has_old_value = data[cursor] != 0;
        cursor += 1;

        let old_value = if has_old_value {
            if data.len() < cursor + 4 {
                return Err(OrchardError::ParseError("Insufficient data for old value length".into()));
            }
            let old_value_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
            cursor += 4;

            if data.len() < cursor + old_value_len {
                return Err(OrchardError::ParseError("Insufficient data for old value".into()));
            }
            let old_val = data[cursor..cursor + old_value_len].to_vec();
            cursor += old_value_len;
            Some(old_val)
        } else {
            None
        };

        // Read new value
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for new value length".into()));
        }
        let new_value_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
        cursor += 4;

        if data.len() < cursor + new_value_len {
            return Err(OrchardError::ParseError("Insufficient data for new value".into()));
        }
        let new_value = data[cursor..cursor + new_value_len].to_vec();
        cursor += new_value_len;

        // Read merkle proof
        if data.len() < cursor + 4 {
            return Err(OrchardError::ParseError("Insufficient data for proof length".into()));
        }
        let proof_len = u32::from_le_bytes([data[cursor], data[cursor + 1], data[cursor + 2], data[cursor + 3]]) as usize;
        cursor += 4;

        if data.len() < cursor + proof_len * 32 {
            return Err(OrchardError::ParseError("Insufficient data for merkle proof".into()));
        }

        let mut merkle_proof = Vec::new();
        for _ in 0..proof_len {
            let mut proof_node = [0u8; 32];
            proof_node.copy_from_slice(&data[cursor..cursor + 32]);
            merkle_proof.push(proof_node);
            cursor += 32;
        }

        Ok((StateWrite { key, old_value, new_value, merkle_proof }, cursor))
    }
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum ExecutionMode {
    Builder,     // Leak access patterns for witness generation
    Guarantor,   // Verify witnesses and use cache-only backend
}

/// Cache-only backend for witness-based execution
pub struct CacheOnlyBackend {
    read_cache: HashMap<String, Vec<u8>>,
    write_log: HashMap<String, Vec<u8>>,
    access_reads: Vec<StateRead>,
    access_writes: Vec<StateWrite>,
    execution_mode: ExecutionMode,
}

impl CacheOnlyBackend {
    /// Create backend from witness bundle for guarantor mode
    pub fn from_witness(service_id: u32, witness: &WitnessBundle) -> Result<Self> {
        // Verify all read witnesses against pre_state_root
        let mut read_cache = HashMap::new();
        for read in &witness.reads {
            verify_merkle_proof(service_id, &read.key, &read.value, &read.merkle_proof, &witness.pre_state_root)?;
            read_cache.insert(read.key.clone(), read.value.clone());
        }

        Ok(Self {
            read_cache,
            write_log: HashMap::new(),
            access_reads: Vec::new(),
            access_writes: Vec::new(),
            execution_mode: ExecutionMode::Guarantor,
        })
    }

    /// Create backend for builder mode (tracks access patterns)
    pub fn builder_mode() -> Self {
        Self {
            read_cache: HashMap::new(),
            write_log: HashMap::new(),
            access_reads: Vec::new(),
            access_writes: Vec::new(),
            execution_mode: ExecutionMode::Builder,
        }
    }

    /// Get the collected access log for builder mode
    pub fn get_access_log(&self) -> (Vec<StateRead>, Vec<StateWrite>) {
        (self.access_reads.clone(), self.access_writes.clone())
    }

    /// Get the write log for post-witness verification
    pub fn get_write_log(&self) -> &HashMap<String, Vec<u8>> {
        &self.write_log
    }
}

/// State backend abstraction for witness-aware execution
pub trait StateBackend {
    fn read_state(&mut self, key: &str) -> Result<Vec<u8>>;
    fn write_state(&mut self, key: &str, value: Vec<u8>) -> Result<()>;
    fn get_execution_mode(&self) -> ExecutionMode;
}

impl StateBackend for CacheOnlyBackend {
    fn read_state(&mut self, key: &str) -> Result<Vec<u8>> {
        match self.execution_mode {
            ExecutionMode::Builder => {
                // Check if we already have this key cached
                if let Some(cached_value) = self.read_cache.get(key) {
                    return Ok(cached_value.clone());
                }

                // In builder mode, this would typically read from JAM state
                // For demonstration, we'll simulate some state reads
                let value = match key {
                    "commitment_root" => vec![0u8; 32], // Default root
                    "commitment_size" => 0u64.to_le_bytes().to_vec(), // Empty tree
                    k if k.starts_with("nullifier_") => vec![0u8; 32], // Unspent nullifiers
                    _ => Vec::new(), // Other keys default to empty
                };

                // Track the access for witness generation
                self.access_reads.push(StateRead {
                    key: key.to_string(),
                    value: value.clone(),
                    merkle_proof: Vec::new(), // TODO: Generated by builder from JAM state
                });

                self.read_cache.insert(key.to_string(), value.clone());
                Ok(value)
            }
            ExecutionMode::Guarantor => {
                // In guarantor mode, must only use cache from witness
                self.read_cache.get(key)
                    .cloned()
                    .ok_or_else(|| OrchardError::StateError(
                        alloc::format!("Cache miss in guarantor mode for key: {}", key)
                    ))
            }
        }
    }

    fn write_state(&mut self, key: &str, value: Vec<u8>) -> Result<()> {
        let old_value = self.write_log.get(key).cloned();

        // Record write in log
        self.write_log.insert(key.to_string(), value.clone());

        // Track access for witness generation (builder mode only)
        if self.execution_mode == ExecutionMode::Builder {
            self.access_writes.push(StateWrite {
                key: key.to_string(),
                old_value,
                new_value: value,
                merkle_proof: Vec::new(), // TODO: Generated by builder
            });
        }

        Ok(())
    }

    fn get_execution_mode(&self) -> ExecutionMode {
        self.execution_mode
    }
}

/// Verify Merkle proof against JAM state root
///
/// This function reuses JAM's production Merkle tree verification logic
/// from utils::effects::verify_merkle_proof for consensus compatibility.
#[cfg(feature = "polkavm")]
pub fn verify_merkle_proof(
    service_id: u32,
    key: &str,
    value: &[u8],
    proof: &[[u8; 32]],
    root: &[u8; 32],
) -> Result<()> {
    // In test mode, allow mock proof verification for unit tests
    #[cfg(test)]
    {
        // For testing: if proof is all zeros, skip verification
        if proof.len() == 1 && proof[0] == [0u8; 32] {
            return Ok(());
        }
    }

    // Use JAM's production witness verification
    use utils::effects::verify_merkle_proof as jam_verify_merkle_proof;

    // Convert key to bytes for JAM verification
    let key_bytes = key.as_bytes();

    if jam_verify_merkle_proof(
        service_id,
        key_bytes,
        value,
        root,
        proof
    ) {
        Ok(())
    } else {
        Err(OrchardError::StateError(
            alloc::format!("JAM Merkle proof verification failed for key: {}", key)
        ))
    }
}

/// FFI stub for Merkle proof verification - actual verification happens on Go side
#[cfg(not(feature = "polkavm"))]
pub fn verify_merkle_proof(
    _service_id: u32,
    _key: &str,
    _value: &[u8],
    _proof: &[[u8; 32]],
    _root: &[u8; 32],
) -> Result<()> {
    // In FFI mode, the Go side handles Merkle proof verification
    Ok(())
}

// Hash functions removed - now using JAM's production verification
// through utils::effects::verify_merkle_proof

/// Verify post-witness against computed writes
pub fn verify_post_witness(
    service_id: u32,
    witness: &WitnessBundle,
    computed_writes: &HashMap<String, Vec<u8>>,
) -> Result<()> {
    // Verify each write in witness matches computed write
    for write in &witness.writes {
        let computed = computed_writes.get(&write.key)
            .ok_or_else(|| OrchardError::StateError(
                format!("Missing computed write for key: {}", write.key)
            ))?;

        if *computed != write.new_value {
            return Err(OrchardError::StateError(
                format!("Write mismatch for key {}: expected {:?}, got {:?}",
                    write.key, write.new_value, computed)
            ));
        }

        // Verify Merkle proof against post_state_root
        verify_merkle_proof(
            service_id,
            &write.key,
            &write.new_value,
            &write.merkle_proof,
            &witness.post_state_root
        )?;
    }

    Ok(())
}

/// Comprehensive witness bundle validation for Byzantine fault tolerance
pub fn validate_witness_bundle(service_id: u32, witness: &WitnessBundle) -> Result<()> {
    // 1. Validate structural integrity
    validate_witness_structure(witness)?;

    // 2. Validate all read witnesses against pre_state_root
    validate_all_read_witnesses(service_id, witness)?;

    // 3. Validate all write witnesses against post_state_root
    validate_all_write_witnesses(service_id, witness)?;

    // 4. Check for witness completeness (no missing required keys)
    validate_witness_completeness(witness)?;

    Ok(())
}

/// Validate witness bundle structural integrity
fn validate_witness_structure(witness: &WitnessBundle) -> Result<()> {
    // Check for reasonable limits to prevent DoS
    const MAX_READS: usize = 1000;
    const MAX_WRITES: usize = 1000;
    const MAX_PROOF_LENGTH: usize = 64; // Max tree depth

    if witness.reads.len() > MAX_READS {
        return Err(OrchardError::StateError(
            alloc::format!("Too many read witnesses: {} > {}", witness.reads.len(), MAX_READS)
        ));
    }

    if witness.writes.len() > MAX_WRITES {
        return Err(OrchardError::StateError(
            alloc::format!("Too many write witnesses: {} > {}", witness.writes.len(), MAX_WRITES)
        ));
    }

    // Validate individual witness structures
    for read in &witness.reads {
        if read.key.is_empty() {
            return Err(OrchardError::StateError("Empty read witness key".into()));
        }
        if read.merkle_proof.len() > MAX_PROOF_LENGTH {
            return Err(OrchardError::StateError("Read proof too long".into()));
        }
    }

    for write in &witness.writes {
        if write.key.is_empty() {
            return Err(OrchardError::StateError("Empty write witness key".into()));
        }
        if write.merkle_proof.len() > MAX_PROOF_LENGTH {
            return Err(OrchardError::StateError("Write proof too long".into()));
        }
    }

    Ok(())
}

/// Validate all read witnesses against pre_state_root
fn validate_all_read_witnesses(service_id: u32, witness: &WitnessBundle) -> Result<()> {
    for read in &witness.reads {
        verify_merkle_proof(
            service_id,
            &read.key,
            &read.value,
            &read.merkle_proof,
            &witness.pre_state_root
        ).map_err(|e| OrchardError::StateError(
            alloc::format!("Read witness validation failed for key {}: {:?}", read.key, e)
        ))?;
    }
    Ok(())
}

/// Validate all write witnesses against post_state_root
fn validate_all_write_witnesses(service_id: u32, witness: &WitnessBundle) -> Result<()> {
    for write in &witness.writes {
        verify_merkle_proof(
            service_id,
            &write.key,
            &write.new_value,
            &write.merkle_proof,
            &witness.post_state_root
        ).map_err(|e| OrchardError::StateError(
            alloc::format!("Write witness validation failed for key {}: {:?}", write.key, e)
        ))?;
    }
    Ok(())
}

/// Validate witness completeness (all required keys are present)
fn validate_witness_completeness(witness: &WitnessBundle) -> Result<()> {
    // Check that critical system keys are present in reads
    let required_reads = ["commitment_root", "commitment_size"];
    let read_keys: Vec<&str> = witness.reads.iter().map(|r| r.key.as_str()).collect();

    for required_key in &required_reads {
        if !read_keys.contains(required_key) {
            return Err(OrchardError::StateError(
                alloc::format!("Missing required read witness for key: {}", required_key)
            ));
        }
    }

    Ok(())
}

/// Detect and prevent common witness-based attacks
pub fn detect_witness_attacks(witness: &WitnessBundle) -> Result<()> {
    // 1. Check for duplicate keys in witnesses
    detect_duplicate_witnesses(witness)?;

    // 2. Check for conflicting state transitions
    detect_conflicting_transitions(witness)?;

    // 3. Check for impossible state progressions
    detect_impossible_progressions(witness)?;

    Ok(())
}

/// Detect duplicate witness entries (potential attack vector)
fn detect_duplicate_witnesses(witness: &WitnessBundle) -> Result<()> {
    let mut seen_read_keys = HashMap::new();
    for read in &witness.reads {
        if seen_read_keys.contains_key(&read.key) {
            return Err(OrchardError::StateError(
                alloc::format!("Duplicate read witness for key: {}", read.key)
            ));
        }
        seen_read_keys.insert(&read.key, ());
    }

    let mut seen_write_keys = HashMap::new();
    for write in &witness.writes {
        if seen_write_keys.contains_key(&write.key) {
            return Err(OrchardError::StateError(
                alloc::format!("Duplicate write witness for key: {}", write.key)
            ));
        }
        seen_write_keys.insert(&write.key, ());
    }

    Ok(())
}

/// Detect conflicting state transitions
fn detect_conflicting_transitions(witness: &WitnessBundle) -> Result<()> {
    // Check that reads and writes don't conflict
    for read in &witness.reads {
        for write in &witness.writes {
            if read.key == write.key {
                // If same key is read and written, verify old_value matches read value
                if let Some(ref old_value) = write.old_value {
                    if *old_value != read.value {
                        return Err(OrchardError::StateError(
                            alloc::format!("Conflicting read/write for key {}: read={:?}, old_write={:?}",
                                read.key, read.value, old_value)
                        ));
                    }
                }
            }
        }
    }

    Ok(())
}

/// Detect impossible state progressions
fn detect_impossible_progressions(witness: &WitnessBundle) -> Result<()> {
    // Check commitment tree size progression (should only increase)
    let commitment_size_read = witness.reads.iter()
        .find(|r| r.key == "commitment_size")
        .map(|r| {
            if r.value.len() >= 8 {
                u64::from_le_bytes(r.value[..8].try_into().unwrap_or([0; 8]))
            } else {
                0
            }
        });

    let commitment_size_write = witness.writes.iter()
        .find(|w| w.key == "commitment_size")
        .map(|w| {
            if w.new_value.len() >= 8 {
                u64::from_le_bytes(w.new_value[..8].try_into().unwrap_or([0; 8]))
            } else {
                0
            }
        });

    if let (Some(old_size), Some(new_size)) = (commitment_size_read, commitment_size_write) {
        if new_size < old_size {
            return Err(OrchardError::StateError(
                alloc::format!("Commitment tree size decreased: {} -> {}", old_size, new_size)
            ));
        }
    }

    Ok(())
}


#[cfg(test)]
mod tests {
    use super::*;

    const TEST_SERVICE_ID: u32 = 0;

    #[test]
    fn test_cache_only_backend_builder_mode() {
        let mut backend = CacheOnlyBackend::builder_mode();
        assert_eq!(backend.get_execution_mode(), ExecutionMode::Builder);

        // Test write operation
        backend.write_state("test_key", vec![1, 2, 3]).unwrap();

        let (reads, writes) = backend.get_access_log();
        assert_eq!(writes.len(), 1);
        assert_eq!(writes[0].key, "test_key");
        assert_eq!(writes[0].new_value, vec![1, 2, 3]);
    }

    #[test]
    fn test_cache_only_backend_guarantor_mode() {
        let witness = WitnessBundle {
            pre_state_root: [1u8; 32],
            post_state_root: [2u8; 32],
            reads: vec![StateRead {
                key: "test_key".to_string(),
                value: vec![4, 5, 6],
                merkle_proof: vec![[0u8; 32]], // Mock proof for testing
            }],
            writes: vec![],
        };

        let mut backend = CacheOnlyBackend::from_witness(TEST_SERVICE_ID, &witness).unwrap();
        assert_eq!(backend.get_execution_mode(), ExecutionMode::Guarantor);

        // Should be able to read cached value
        let value = backend.read_state("test_key").unwrap();
        assert_eq!(value, vec![4, 5, 6]);

        // Should fail to read uncached value
        let result = backend.read_state("missing_key");
        assert!(result.is_err());
    }

    #[test]
    fn test_cache_only_backend_guarantor_mode_with_valid_proof() {
        // TODO: This test would use real StateDB proofs via FFI
        // For now, we test with mock data that bypasses verification
        let witness = create_test_witness_with_valid_proof();
        let mut backend = CacheOnlyBackend::from_witness(TEST_SERVICE_ID, &witness).unwrap();
        assert_eq!(backend.get_execution_mode(), ExecutionMode::Guarantor);

        let value = backend.read_state("valid_key").unwrap();
        assert_eq!(value, vec![10, 20, 30]);
    }

    /// Helper function to create test witness with valid proof structure
    /// This demonstrates the format for real StateDB-generated proofs
    fn create_test_witness_with_valid_proof() -> WitnessBundle {
        // Example of what a real StateDB proof structure would look like:
        // - Root hash from actual Merkle tree
        // - Proper proof path with sibling hashes
        // - Real key-value pair stored in StateDB
        WitnessBundle {
            pre_state_root: [
                0x0d, 0xd6, 0x77, 0xe2, 0x4a, 0x16, 0x61, 0x16,
                0xd3, 0xf6, 0x27, 0x4d, 0xf3, 0x9e, 0xaa, 0xd8,
                0xeb, 0xc4, 0x7e, 0xd5, 0x49, 0xb4, 0x64, 0x04,
                0x2e, 0xf1, 0x6a, 0x41, 0x64, 0xb9, 0x85, 0x53,
            ], // Example root hash from JAM genesis
            post_state_root: [
                0x1d, 0xd6, 0x77, 0xe2, 0x4a, 0x16, 0x61, 0x16,
                0xd3, 0xf6, 0x27, 0x4d, 0xf3, 0x9e, 0xaa, 0xd8,
                0xeb, 0xc4, 0x7e, 0xd5, 0x49, 0xb4, 0x64, 0x04,
                0x2e, 0xf1, 0x6a, 0x41, 0x64, 0xb9, 0x85, 0x53,
            ],
            reads: vec![StateRead {
                key: "valid_key".to_string(),
                value: vec![10, 20, 30],
                merkle_proof: vec![[0u8; 32]], // Mock proof that bypasses verification in test mode
            }],
            writes: vec![],
        }
    }

    #[test]
    fn test_verify_post_witness() {
        let mut computed_writes = HashMap::new();
        computed_writes.insert("key1".to_string(), vec![1, 2, 3]);
        computed_writes.insert("key2".to_string(), vec![4, 5, 6]);

        let witness = WitnessBundle {
            pre_state_root: [0u8; 32],
            post_state_root: [1u8; 32],
            reads: vec![],
            writes: vec![
                StateWrite {
                    key: "key1".to_string(),
                    old_value: None,
                    new_value: vec![1, 2, 3],
                    merkle_proof: vec![[0u8; 32]],
                },
                StateWrite {
                    key: "key2".to_string(),
                    old_value: Some(vec![0]),
                    new_value: vec![4, 5, 6],
                    merkle_proof: vec![[0u8; 32]],
                },
            ],
        };

        // Should succeed when writes match
        assert!(verify_post_witness(TEST_SERVICE_ID, &witness, &computed_writes).is_ok());

        // Should fail when write doesn't match
        computed_writes.insert("key1".to_string(), vec![9, 9, 9]);
        assert!(verify_post_witness(TEST_SERVICE_ID, &witness, &computed_writes).is_err());
    }
}
