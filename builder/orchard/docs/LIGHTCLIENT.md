# Orchard Light Client Protocol for JAM

## Overview

This document describes the **Orchard light client protocol** for the JAM blockchain. The implementation provides Zcash-compatible note discovery and synchronization using standard Orchard protocols without any custom extensions.

**Key Features:**
- Standard Zcash Orchard note encryption and trial decryption
- CompactBlock format for bandwidth efficiency
- Incremental witness maintenance for spend proofs
- JAM builder/guarantor integration

---

## Standard Orchard Note Discovery

### Orchard Cryptographic Foundation

The light client uses standard Orchard note encryption as specified in the Zcash protocol:

#### **1. CompactBlock Streaming**
- **CompactBlocks**: Contain Orchard action data (commitments, nullifiers, encrypted notes)
- **JAM Integration**: Served via RPC from JAM nodes with Orchard service
- **Format**: Standard Zcash CompactBlock with Orchard actions only

#### **2. Key Management**
Light clients hold Orchard viewing keys and perform trial decryption:
- **Incoming Viewing Key (IVK)**: For detecting incoming notes
- **Full Viewing Key (FVK)**: For outgoing note detection
- **Diversified Addresses**: Generated from IVK as needed

#### **3. Trial Decryption Process**

**Per-Action Note Detection** (standard Orchard):
```rust
// For each Orchard action in CompactBlock
for action in compact_block.actions {
    // Standard Orchard key agreement
    let shared_secret = key_agreement(
        incoming_viewing_key,
        action.ephemeral_key
    );

    // Orchard note plaintext decryption
    match decrypt_note_plaintext(action.encrypted_note, shared_secret) {
        Ok(note_plaintext) => {
            // Successfully detected incoming payment
            let note = Note::from_plaintext(note_plaintext);
            self.add_detected_note(note, action.cmx);
        }
        Err(_) => {
            // Decryption failed - not intended for this viewing key
            continue;
        }
    }
}
```

**Orchard Benefits:**
- **Standard Protocol**: Uses proven Orchard encryption without modifications
- **Efficient Scanning**: Trial decryption cost is bounded and predictable
- **Privacy Preserving**: No metadata leakage during note discovery

#### **4. Nullifier Processing and Spend Detection**

**Track Own Nullifiers** (standard Orchard):
```rust
// Derive nullifiers using Orchard nullifier computation
for own_note in detected_notes {
    let nullifier = derive_orchard_nullifier(
        spending_key,
        own_note.rho,       // Note position
        own_note.psi,       // Note randomness
        own_note.commitment
    );
    nullifier_cache.insert(nullifier, own_note);
}

// Check CompactBlock for spend detection
for action in compact_block.actions {
    if let Some(spent_note) = nullifier_cache.get(&action.nullifier) {
        // Detected spend of our note
        self.mark_note_spent(spent_note, block_height);
    }
}
```

#### **5. Commitment Tree Maintenance**

**Orchard Tree Structure**:
- **Depth**: 32 levels (MERKLE_DEPTH_ORCHARD)
- **Hash Function**: Sinsemilla with Orchard domain separation
- **Incremental Witnesses**: Maintained for spend proof generation

**Incremental Witness Updates**:
```rust
// Maintain Orchard note commitment tree per block
for action in compact_block.actions {
    // Add new commitments to tree (using Orchard Sinsemilla)
    commitment_tree.append(action.cmx);

    // Update incremental witnesses for detected notes
    for witness in &mut incremental_witnesses {
        witness.append(action.cmx);
    }
}

// Create witness for newly detected notes
if let Some(detected_note) = newly_detected_notes {
    let witness = commitment_tree.create_witness(detected_note.position);
    witnesses.insert(detected_note.commitment, witness);
}
```

---

## JAM Integration Specifics

### **CompactBlock Format for Orchard**

JAM nodes serve standard Orchard CompactBlocks:

```rust
// Standard Zcash CompactBlock with Orchard actions only
pub struct CompactBlock {
    pub height: u64,
    pub hash: [u8; 32],
    pub prev_hash: [u8; 32],
    pub time: u32,
    pub vtx: Vec<CompactTx>,  // Orchard transactions only
}

pub struct CompactTx {
    pub hash: [u8; 32],
    pub actions: Vec<CompactOrchardAction>,
}

pub struct CompactOrchardAction {
    pub nullifier: [u8; 32],         // Old note nullifier
    pub cmx: [u8; 32],              // New note commitment (extracted)
    pub ephemeral_key: [u8; 32],     // Ephemeral public key
    pub ciphertext: Vec<u8>,         // Encrypted note (52 bytes for Orchard)
}
```

### **JAM State Integration**

**RPC Endpoints** (served by JAM nodes with Orchard service):
```rust
// Standard lightwalletd-compatible endpoints
pub trait OrchardLightClient {
    // Blockchain information
    fn get_blockchain_info() -> ChainInfo;
    fn get_block_range(start: u64, end: u64) -> Vec<CompactBlock>;

    // Tree state for witnesses
    fn get_tree_state(height: u64) -> TreeState;
    fn get_subtrees_by_index(start_index: u64, limit: u64) -> Vec<SubtreeRoot>;

    // Address and note management
    fn get_address_utxos(addresses: Vec<String>) -> Vec<Utxo>;
}
```

**Tree State Management**:
```rust
// JAM stores Orchard tree state in service storage
pub struct TreeState {
    pub network: String,
    pub height: u64,
    pub hash: String,
    pub time: u32,
    pub orchard: OrchardTreeState,
}

pub struct OrchardTreeState {
    pub commitment_tree: CommitmentTree,  // Sinsemilla-based tree
    pub note_commitment_tree_size: u64,
}
```

---

## Builder/Guarantor Architecture Integration

### **Builder Role**
- Serves CompactBlocks to light clients
- Maintains full Orchard tree state
- Provides witness generation services

### **Light Client Operations**
```rust
// Light client workflow with JAM builders
pub struct OrchardLightClient {
    builder_endpoints: Vec<String>,
    viewing_keys: Vec<IncomingViewingKey>,
    detected_notes: HashMap<Commitment, Note>,
    witnesses: HashMap<Commitment, IncrementalWitness>,
}

impl OrchardLightClient {
    pub async fn sync_to_tip(&mut self) -> Result<()> {
        let current_height = self.get_current_height();
        let chain_tip = self.query_chain_tip().await?;

        // Download CompactBlocks from builders
        let compact_blocks = self.download_compact_blocks(
            current_height + 1,
            chain_tip.height
        ).await?;

        // Process each block for note detection
        for block in compact_blocks {
            self.scan_block(block).await?;
        }

        Ok(())
    }

    async fn scan_block(&mut self, block: CompactBlock) -> Result<()> {
        for tx in block.vtx {
            for action in tx.actions {
                // Trial decryption for each viewing key
                for ivk in &self.viewing_keys {
                    if let Some(note) = self.try_decrypt_note(ivk, &action).await? {
                        self.detected_notes.insert(action.cmx, note);

                        // Create witness for new note
                        let witness = self.create_witness_for_note(&action, block.height).await?;
                        self.witnesses.insert(action.cmx, witness);
                    }
                }

                // Check for spends of our notes
                if let Some(spent_note) = self.detected_notes.get(&action.nullifier) {
                    self.mark_note_spent(spent_note);
                }

                // Update all existing witnesses
                self.update_witnesses(&action.cmx);
            }
        }
        Ok(())
    }

    pub async fn create_spend_proof(&self, note: &Note, recipient: OrchardAddress, amount: u64) -> Result<OrchardBundle> {
        let witness = self.witnesses.get(&note.commitment)
            .ok_or("No witness available for note")?;

        // Build Orchard bundle via builder
        let bundle_request = OrchardBundleRequest {
            spends: vec![SpendInfo {
                note: note.clone(),
                witness: witness.clone(),
            }],
            outputs: vec![OutputInfo {
                address: recipient,
                value: amount,
            }],
        };

        // Submit to builder for proof generation
        self.submit_to_builder(bundle_request).await
    }
}
```

---

## Privacy and Security Properties

### **Standard Orchard Privacy**
- **No Address Linkability**: Diversified addresses prevent correlation
- **Value Privacy**: Note amounts encrypted in standard Orchard format
- **Sender Privacy**: No transaction graph analysis possible
- **Recipient Privacy**: Trial decryption required for note detection

### **JAM-Specific Considerations**

#### **Builder Trust Model**
- Light clients can connect to multiple builders for redundancy
- Builders cannot correlate users across sessions (no persistent identifiers)
- CompactBlock data is publicly available and verifiable

#### **Network Privacy**
```rust
// Best practices for network privacy
pub struct PrivacyConfig {
    pub use_tor: bool,              // Route requests through Tor
    pub rotate_builders: bool,      // Use different builders per session
    pub uniform_timing: bool,       // Constant-time scanning
    pub batch_requests: bool,       // Batch block requests
}
```

---

## Performance Characteristics

### **Bandwidth Optimization**
- **Full Transaction**: ~2-5 KB per Orchard transaction
- **CompactBlock**: ~200 bytes per Orchard action
- **Bandwidth Reduction**: ~90-95% savings over full blocks

### **Scanning Performance**
- **Trial Decryption**: O(1) per action (bounded by Orchard protocol)
- **Witness Updates**: O(log n) per new commitment
- **Nullifier Checks**: O(1) with indexed cache

### **Storage Requirements**
- **CompactBlocks**: ~10-50 MB per 100k blocks (depending on action density)
- **Incremental Witnesses**: ~1 KB per tracked note
- **Note Database**: ~500 bytes per detected note

---

## Implementation Roadmap

### **Phase 1: Core Protocol** ✅
- [x] Standard Orchard note encryption/decryption
- [x] CompactBlock format definition
- [x] Basic trial decryption implementation

### **Phase 2: JAM Integration** 🚧
- [ ] JAM RPC endpoint implementation
- [ ] Builder CompactBlock generation
- [ ] Tree state synchronization

### **Phase 3: Production Features** 📋
- [ ] Multi-builder redundancy
- [ ] Tor integration for privacy
- [ ] Performance optimization
- [ ] Mobile client support

### **Phase 4: Advanced Features** 📋
- [ ] Hardware wallet integration
- [ ] Background synchronization
- [ ] Advanced witness management

---

## Security Considerations

### **Threat Model**
- ✅ **Malicious Builders**: Light client can verify all cryptographic proofs
- ✅ **Network Surveillance**: Tor integration prevents traffic analysis
- ✅ **Timing Attacks**: Uniform scanning prevents selection bias
- ⚠️ **Eclipse Attacks**: Mitigated by connecting to multiple builders

### **Implementation Requirements**
1. **Standard Compliance**: Use only standard Orchard protocols
2. **Witness Integrity**: Verify all Merkle witnesses against known tree roots
3. **Key Management**: Secure storage of viewing keys
4. **Network Privacy**: Tor support and uniform request patterns

---

## Testing and Validation

### **Compatibility Testing**
```rust
#[test]
fn test_orchard_compatibility() {
    // Verify compatibility with standard Zcash Orchard
    let test_vector = load_orchard_test_vector();
    let decrypted_note = decrypt_note_with_ivk(
        &test_vector.encrypted_note,
        &test_vector.ivk
    );
    assert_eq!(decrypted_note, test_vector.expected_note);
}

#[test]
fn test_compactblock_parsing() {
    // Verify CompactBlock format matches Zcash specification
    let compact_block = parse_compact_block(&test_block_bytes);
    assert_eq!(compact_block.actions.len(), expected_action_count);
}
```

### **Performance Benchmarks**
- **Sync Speed**: Target <10 minutes for 100k blocks
- **Memory Usage**: <500 MB for active scanning
- **Battery Life**: Minimal impact on mobile devices

This Orchard light client protocol provides efficient, private, and secure synchronization for JAM's Orchard service while maintaining full compatibility with standard Zcash protocols.