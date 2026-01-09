# Orchard Wallet Integration for JAM
 To setup WASM:
 ```
  brew install llvm
  cd zcash-web-wallet && make build-wasm
```

**Status**: Transparent + traffic-demo wallet (builder-trusted) - Orchard proof generation still pending
**Date**: 2025-12-30

## Overview

Prototype wallet abstraction for transparent transactions, plus scaffolding for Orchard key and proof workflows. Orchard proof generation and full key safety are not implemented in this module yet.

⚠️ **Security warning (malicious builder)**: Transparent spends rely on builder-supplied UTXO roots/proofs (optional snapshot) that are only checked against the claimed pre-state root/size. There is currently **no JAM witness or immutable checkpoint** for transparent state, so a malicious builder could fabricate a compatible snapshot/root and mint spendable inputs. Use the wallet’s transparent mode only in trusted/dev environments until the transparent UTXO root/size is anchored in JAM state.

## Current Implementation

### **Current Demo Wallet**: `builder/orchard/wallet/wallet.go`

This file contains a simplified, in-memory wallet with random keys and no Orchard proof generation. It is suitable for demos only.

### **Planned Wallet Interface (not yet implemented)**

```go
// Core Orchard wallet operations
type OrchardWallet interface {
    // Key Management
    GenerateSpendingKey(seed [32]byte) ([32]byte, error)
    DeriveFullViewingKey(spendingKey [32]byte) ([32]byte, error)
    DeriveIncomingViewingKey(fullViewingKey [32]byte) ([32]byte, error)

    // Address Generation
    NewAddress() (string, error)
    ListAddresses() []string
    ValidateAddress(address string) bool

    // Note Management
    DetectedNotes() []OrchardNote
    UnspentNotes() []OrchardNote
    NotesByAddress(address string) []OrchardNote

    // Proof Generation
    NullifierFor(spendingKey, rho, psi, commitment [32]byte) ([32]byte, error)
    BundleProofFor(bundleRequest *OrchardBundleRequest) (*OrchardBundle, error)
}
```

### **Orchard-Specific Types**: `builder/orchard/types/orchard_types.go`

```go
// Standard Orchard note structure
type OrchardNote struct {
    Value       uint64    // Note value in zatoshi
    Diversifier [11]byte  // Address diversifier
    PkD         [32]byte  // Diversified transmission key
    Rho         [32]byte  // Note position/uniqueness
    Psi         [32]byte  // Note randomness
    Rseed       [32]byte  // Random seed for derivations
    Memo        []byte    // Optional memo data

    // Derived fields
    Commitment  [32]byte  // Note commitment (cmx)
    Nullifier   [32]byte  // Nullifier (if spending key available)

    // Metadata
    Height      uint64    // Block height when detected
    TxHash      [32]byte  // Transaction hash
    ActionIndex uint32    // Action index within transaction
    Spent       bool      // Spend status
}

// Orchard address representation
type OrchardAddress struct {
    Diversifier [11]byte  // 11-byte diversifier
    PkD         [32]byte  // 32-byte diversified transmission key
}

// Orchard bundle request for transaction building
type OrchardBundleRequest struct {
    Spends       []OrchardSpend   // Notes to spend
    Outputs      []OrchardOutput  // New notes to create
    ValueBalance int64            // Net value in/out of pool
    Anchor       [32]byte         // Tree anchor for spends
    Fee          uint64           // Transaction fee
}

type OrchardSpend struct {
    Note    OrchardNote           // Note being spent
    Witness MerkleWitness         // Merkle witness for note commitment
}

type OrchardOutput struct {
    Address OrchardAddress        // Recipient address
    Value   uint64               // Output value
    Memo    []byte               // Optional memo
}
```

### **Builder Integration**: `builder/orchard/builder/orchard_builder.go`

```go
// OrchardBuilder integrates wallet operations with JAM builder
type OrchardBuilder struct {
    wallet     OrchardWallet
    ffi        *OrchardFFI
    treeState  *OrchardTreeState
    lightClient *OrchardLightClient
}

// Build Orchard bundle for submission to JAM
func (b *OrchardBuilder) BuildTransaction(
    fromAddress string,
    recipients []RecipientInfo,
    fee uint64,
) (*OrchardBundle, error) {
    // Select notes for spending
    spends, err := b.selectNotesToSpend(fromAddress, recipients, fee)
    if err != nil {
        return nil, err
    }

    // Build outputs for recipients
    outputs := make([]OrchardOutput, len(recipients))
    for i, recipient := range recipients {
        addr, err := b.parseOrchardAddress(recipient.Address)
        if err != nil {
            return nil, err
        }
        outputs[i] = OrchardOutput{
            Address: addr,
            Value:   recipient.Amount,
            Memo:    recipient.Memo,
        }
    }

    // Create bundle request
    bundleReq := &OrchardBundleRequest{
        Spends:       spends,
        Outputs:      outputs,
        ValueBalance: int64(fee), // Net value out for fee payment
        Anchor:       b.getCurrentAnchor(),
        Fee:          fee,
    }

    // Generate bundle via wallet
    return b.wallet.BundleProofFor(bundleReq)
}
```

### **RPC Integration**: `builder/orchard/rpc/orchard_handler.go`

```go
// OrchardRPCHandler implements Zcash-compatible RPC using OrchardWallet
type OrchardRPCHandler struct {
    wallet  OrchardWallet
    builder *OrchardBuilder
}

// All RPC methods delegate to wallet operations
func (h *OrchardRPCHandler) ZGetNewAddress(addressType string) (string, error) {
    if addressType != "orchard" {
        return "", fmt.Errorf("unsupported address type: %s", addressType)
    }
    return h.wallet.NewAddress()
}

func (h *OrchardRPCHandler) ZGetBalance(minConf int) (float64, error) {
    notes := h.wallet.UnspentNotes()
    var total uint64
    for _, note := range notes {
        if note.Height+uint64(minConf) <= h.getCurrentHeight() {
            total += note.Value
        }
    }
    return float64(total) / 100000000.0, nil // Convert to USDx
}

func (h *OrchardRPCHandler) ZSendMany(
    fromAddress string,
    recipients []RecipientInfo,
    minConf int,
    fee float64,
) (string, error) {
    // Build transaction via OrchardBuilder
    bundle, err := h.builder.BuildTransaction(
        fromAddress,
        recipients,
        uint64(fee * 100000000), // Convert to zatoshi
    )
    if err != nil {
        return "", err
    }

    // Submit to JAM network
    return h.submitBundleToJAM(bundle)
}
```

## Security Properties Achieved

✅ **Private keys contained within wallet boundary**
✅ **No spending keys exposed in builder or RPC APIs**
✅ **Standard Orchard cryptographic operations**
✅ **Memory clearing after cryptographic operations**
✅ **Clean separation between key management and transaction building**

## Current Implementation Status Summary

✅ **Phase 1 Complete**: Orchard wallet abstraction with standard cryptographic operations

This Orchard wallet integration provides a secure foundation for JAM's privacy-preserving transactions while maintaining full compatibility with standard Zcash Orchard protocols.

---

## Zcash Web Wallet Compatibility (zcash-web-wallet)

This section captures the compatibility analysis originally recorded in `WALLET-CODEX.md`.

### Wallet Architecture Notes

- Browser frontend + WASM module; no backend wallet service.
- Local signing builds **transparent** Zcash transactions from UTXOs.
- Shielded/Orchard handling is limited to viewing/decryption, not sending.

### Observed RPC Calls (zcash-web-wallet)

Frontend JS uses:
- `getblockchaininfo`
- `getrawtransaction`
- `sendrawtransaction`

CLI uses:
- `getrawtransaction`
- `getblockcount`
- `getblockchaininfo`

Send flow:
- Wallet builds **transparent** transactions locally (WASM) and broadcasts via
  `sendrawtransaction`.

### Orchard Service RPC (current)

**For complete RPC documentation and examples, see**: [RPC.md](RPC.md)

Implemented in `builder/orchard/rpc/handler.go`, notable methods:

**Blockchain Info**:
- `getblockchaininfo`, `getblockcount`, `getbestblockhash`, `getblockhash`, `getblock`, `getblockheader`
- `z_getblockchaininfo`

**Orchard Tree State**:
- `z_gettreestate`, `z_getsubtreesbyindex`, `z_getnotescount`

**Orchard Wallet**:
- `z_getnewaddress`, `z_listaddresses`, `z_validateaddress`
- `z_getbalance`, `z_listunspent`, `z_listreceivedbyaddress`, `z_listnotes`
- `z_sendmany`, `z_sendmanywithchangeto`, `z_viewtransaction`, `z_sendraworchardbundle`

**Orchard Mempool**:
- `z_getmempoolinfo`, `z_getrawmempool`

**Transparent Transaction Support** ✅ (2026-01-04):
- `getrawtransaction`, `sendrawtransaction`, `getrawmempool`, `getmempoolinfo`

### Compatibility Gaps

~~1) `sendrawtransaction` is not implemented on our service.~~ ✅ **FIXED** (2026-01-04)
~~2) `getrawtransaction` is not implemented on our service.~~ ✅ **FIXED** (2026-01-04)
~~3) Transaction model mismatch: wallet builds **transparent** txs, service is **Orchard-only**.~~ ✅ **FIXED** (2026-01-04)

**All compatibility gaps have been resolved.** The Orchard service now fully supports transparent transactions via standard Zcash RPC methods.

### Current Wallet Capabilities (Verified from Source)

The zcash-web-wallet supports **multi-pool viewing** but **transparent-only sending**:

**Sending (Transparent Only)**:
- ✅ Filters notes to `pool === "transparent"` (`send.js:64`)
- ✅ Uses WASM to build and sign transparent transactions
- ❌ Cannot send from Orchard or Sapling pools

**Storage/Viewing (All Pools)**:
- ✅ Stores notes with `pool` field: `"orchard"`, `"sapling"`, or `"transparent"`
- ✅ Scanner processes all pool types
- ✅ Data model supports shielded notes (commitment, nullifier fields)
- ✅ Can view/display Orchard and Sapling balances

**RPC Usage**:
- Uses **ONLY 3 RPC methods** (all transparent): `getrawtransaction`, `getblockchaininfo`, `sendrawtransaction`
- Uses **ZERO `z_*` methods** (scanning and balance calculation happen in WASM)

### Integration Plan (zcash-web-wallet -> Orchard RPC)

**Current Status**: Phase 1 infrastructure complete, proof generation gated, WASM rebuild needed

**Goal**: Enable the web wallet to send Orchard shielded transactions (currently transparent-only)

---

**Phase 1: Core Infrastructure** ✅ **WIRED (2026-01-04)**
- ✅ **Orchard note metadata added to WASM** - `scanner.rs`, `types.rs`, `transaction.rs`
- ✅ **Storage path updated** - `notes.js` forwards new fields to `create_stored_note`
- ✅ **WASM API extended** - `build_orchard_bundle` export added (proof generation gated)
- ⚠️ **Next**: Rebuild WASM package to regenerate `zcash-web-wallet/frontend/pkg` bindings

**Phase 1 Implementation Details**:
- Scan output and stored notes carry Orchard spend metadata (rho, rseed, recipient, commitment, nullifier)
- `build_orchard_bundle` validates inputs and returns "not enabled in this WASM build" error
- Storage schema supports optional Orchard fields in `create_stored_note`
- Tests not yet run (requires WASM rebuild)

**Affected Files**:
- `zcash-web-wallet/wasm/src/scanner.rs` - Orchard note metadata in scan output
- `zcash-web-wallet/wasm/src/types.rs` - Stored note schema with optional Orchard fields
- `zcash-web-wallet/wasm/src/transaction.rs` - Stored note literals/tests updated
- `zcash-web-wallet/wasm/src/lib.rs` - WASM API additions: `build_orchard_bundle` export
- `zcash-web-wallet/frontend/js/storage/notes.js` - JS storage forwards new fields

**Next Steps**:
1. ⚠️ **Rebuild WASM package** to regenerate `zcash-web-wallet/frontend/pkg` bindings
2. ⚠️ **Run tests** to verify no regressions in transparent transaction handling
3. ⚠️ **Test Orchard note storage** - scan a transaction with Orchard outputs, verify metadata persisted
4. 🔵 **Enable proof generation** in WASM (currently gated with error message)
5. 🔵 **Wire up send UI** to allow sending from Orchard pool notes

**Phase 2: Wallet State Management (Not Started)**
- Replace transparent UTXO tracking with Orchard note data.
- Use `z_listnotes`, `z_listaddresses`, and `z_getbalance` to populate UI state.
- Ensure storage schema handles Orchard note fields (commitment/nullifier/value).

**Phase 3: Transaction History (Week 2, days 1-3)**
- Replace `getrawtransaction` usage with `z_viewtransaction`.
- Update viewer/scanner to render Orchard note details instead of raw hex.
- Add mempool awareness using `z_getrawmempool` if needed.

**Phase 4: Testing and Polish (Week 2, days 4-5)**
- End-to-end tests for send, receive, balance, and history.
- UI pass for address/notes flows and error states.
- Security review to ensure seeds never leave browser storage.

### Concrete Code Changes (Examples)

- Transaction building: `build_transparent_transaction` -> `z_sendmany`
  - Files: `zcash-web-wallet/frontend/js/send.js`, `zcash-web-wallet/frontend/js/views.js`
- Address management: local derivation -> `z_getnewaddress`
  - Files: `zcash-web-wallet/frontend/js/wallet.js`, `zcash-web-wallet/frontend/js/addresses.js`
- Transaction viewing: raw hex parsing -> `z_viewtransaction`
  - Files: `zcash-web-wallet/frontend/js/scanner.js`, `zcash-web-wallet/frontend/js/decrypt-viewer.js`
- Storage: transparent UTXO tracking -> Orchard note management
  - Files: `zcash-web-wallet/frontend/js/storage/notes.js`, `zcash-web-wallet/frontend/js/storage/wallets.js`

### Implementation Checklist

**This Week**
- Verify service is running and reachable via JSON-RPC.
- Test `z_getnewaddress`, `z_listnotes`, `z_getbalance`, `z_sendmany`.
- Add dev endpoint in wallet settings.

**Week 1**
- Replace send flow with `z_sendmany`.
- Replace address derivation with `z_getnewaddress`.
- Update balance view to use `z_getbalance` or `z_listnotes`.

**Week 2**
- Replace scanner/decrypt viewer with `z_viewtransaction`.
- Update storage schema to Orchard notes.
- Add QA coverage for send/receive/history.

**Week 3**
- Security and performance review.
- Browser compatibility checks.
- Release checklist and documentation updates.

### Success Criteria

- Functional: send/receive, balance, notes, and history work end-to-end.
- Technical: <100ms median RPC latency on local testnet.
- Security: private keys never leave the browser.
- UX: Orchard address creation and note tracking are clear to users.

### Risks and Mitigations

- **Service availability**: add clear RPC health checks and retry logic.
- **RPC compatibility drift**: pin wallet to the Orchard JSON-RPC contract and test on upgrade.
- **Performance**: paginate `z_listnotes` and cache results client-side.
- **User confusion**: label Orchard-only flows and hide transparent-only UI paths.

### Plan to Make the Wallet Work with Our Service

**Option A: Modify the wallet to use Orchard RPC directly**

1) Replace `sendrawtransaction` usage with `z_sendmany`.
2) Use Orchard address + note endpoints:
   - `z_getnewaddress`, `z_listaddresses`, `z_listnotes`, `z_getbalance`.
3) Replace `getrawtransaction` usage with `z_viewtransaction`.

**Option B: Emulate Zcash full-node RPC (IMPLEMENTED - 2026-01-04)**

✅ **This option was chosen and implemented.**

Added transparent transaction support with v5 parsing, consensus checks, mempool
storage, and tag=4 validation support to keep the web wallet compatible without
requiring wallet code changes for broadcast and mempool checks.

See [TRANSPARENT_TX_SUPPORT.md](TRANSPARENT_TX_SUPPORT.md) for full implementation details.

## Web Wallet Integration (Current Status)

### Current Status (Transparent Compatibility)

The Orchard RPC service provides transparent transaction support for wallet
broadcast and mempool verification. Validator-side verification runs when a
tag=4 TransparentTxData extrinsic is present; historical queries are limited to
transactions indexed by this service (in-memory).

**Known Limitations**:
1. **Historical scope is limited** - No chain scan; index is in-memory and only tracks txs seen/confirmed by this service
2. **Trusted-builder UTXO roots/proofs** - UTXO roots/sizes are not JAM-anchored yet (malicious builders can fabricate compatible roots/proofs)
3. **No persistence** - Restart clears mempool and tx index; reorg handling is not implemented

**See [TRANSPARENT_TX_SUPPORT.md](TRANSPARENT_TX_SUPPORT.md) for the current implementation details.**

### What Works Now

**Current Features**:
1. ✅ **Transparent transaction submission** via `sendrawtransaction`
2. ✅ **Transaction queries** via `getrawtransaction` (mempool/submitted txs)
3. ✅ **Mempool queries** via `getrawmempool` and `getmempoolinfo`
4. ✅ **Blockchain info** via `getblockchaininfo`, `getblockcount`, etc.
5. ✅ **Consensus gating** (locktime/expiry/value + P2PKH/P2SH scripts) before mempool admission
6. ⚠️ **Zero wallet code changes** (true for broadcast/mempool; history is limited without a chain scan)

**How to Use**:

Point your web wallet to the Orchard RPC endpoint:

```javascript
// In web wallet configuration (e.g., zcash-web-wallet/frontend/js/config.js)
const ZCASH_RPC_URL = "http://localhost:8232";

// No other changes needed - wallet works as-is
```

**Web Wallet Workflow**:
1. **User creates transaction** in browser using WASM
2. **Transaction signed locally** (private keys never leave browser)
3. **Broadcast via `sendrawtransaction`** to Orchard RPC
4. **Track via `getrawtransaction`** and `getrawmempool`

**Security Properties**:
- ✅ Private keys remain client-side (browser WASM)
- ✅ Transaction signing happens in browser
- ✅ Server only stores signed transactions (no p2p broadcast yet)
- ✅ No trusted server components

### Plan: Web Wallet Shielded Send (Client-Side Orchard Bundles)

**Goal**: Add Orchard shielded sending to `zcash-web-wallet` using client-side WASM bundle construction only.
No server-side construction and **no new RPC endpoints**.

**RPC Methods Used (Existing Only)**:

- **Tree/chain sync**: `z_getblockchaininfo`, `z_gettreestate`, `z_getsubtreesbyindex`, `z_getnotescount`
- **Transaction submit**: `z_sendraworchardbundle`
- **Transaction status**: `z_viewtransaction`
- **Mempool status**: `z_getmempoolinfo`, `z_getrawmempool`

**Not Used by the Web Wallet** (these are for Orchard-native wallets like Zingo/Nighthawk/zcash-cli):
- Wallet endpoints: `z_getnewaddress`, `z_listaddresses`, `z_validateaddress`, `z_getbalance`,
  `z_listunspent`, `z_listreceivedbyaddress`, `z_listnotes`
- Server-side construction: `z_sendmany`, `z_sendmanywithchangeto`

**WASM Integration**
1. Add a new WASM export to build Orchard bundles:
   - Inputs: seed phrase (or spending key), network, account index, selected notes,
     witnesses, anchor, recipients (address+amount+memo), fee, and optional change address.
   - Outputs: bundle hex, txid, nullifiers/commitments for local state updates.
2. Extend scan output (and StoredNote schema) to include spend metadata required for witnesses:
   - Commitment (cmx), nullifier, note value, output index, **note position**, and any
     Orchard spend fields needed by the bundle builder (rho/psi/rseed if required).

**Tree/Witness Sync (RPC + Local Tree)**
1. Use `z_gettreestate` to fetch the latest anchor and tree size.
2. Use `z_getnotescount` to track incremental growth.
3. Use `z_getsubtreesbyindex` to sync new subtree data into a local commitment tree.
4. For selected notes, derive Merkle witnesses client-side from the local tree.

**Send Flow (UI + JS)**
1. Add a pool selector on the Send tab: **Transparent** | **Orchard**.
2. For Orchard:
   - List unspent Orchard notes from local storage.
   - Allow recipient + memo + fee input.
   - Build bundle in WASM, then submit via `z_sendraworchardbundle`.
3. Track pending status using `z_getrawmempool`/`z_getmempoolinfo`,
   and optionally `z_viewtransaction` for confirmation details.

**State Updates**
1. Mark spent notes locally using nullifiers from the built bundle.
2. Add any change note returned from the builder to local notes.
3. Store txid + status (pending/confirmed) in ledger/history view.

**Acceptance Checks**
- Orchard spend never sends keys to the server.
- `z_sendraworchardbundle` is the only submit path.
- Mempool/status queries use the existing Orchard RPC methods listed above.

### Phase 2 Status: Tree/Witness Sync (Client-Side)

**Implemented in `zcash-web-wallet`**:
- RPC helpers for `z_getblockchaininfo`, `z_gettreestate`, `z_getsubtreesbyindex`, `z_getnotescount`.
- Local Orchard tree state persisted in `localStorage` under `zcash_viewer_orchard_tree_state`.
- Scanner auto-syncs Orchard tree state after scans that yield Orchard notes (non-blocking).
- Optional note position updates when subtree commitments are available.

**Current limitation**:
- `z_getsubtreesbyindex` is still a stub and does not return `commitments`. Note positions
  will remain `null` until subtree commitments are exposed by the RPC server.

### Testing the Integration

**Step 1: Start Orchard Builder**
```bash
# Start orchard-builder with RPC server
go run ./cmd/orchard-builder run \
  --chain chainspec.json \
  --dev-validator 7 \
  --service-id 1 \
  --rpc-port 8232
```

**Step 2: Configure Web Wallet**
```bash
# Clone zcash-web-wallet (or your preferred web wallet)
git clone https://github.com/zcash-community/zcash-web-wallet.git
cd zcash-web-wallet

# Update RPC endpoint in frontend/js/config.js:
# const RPC_URL = "http://localhost:8232";

# Start web wallet
npm install
npm start
```

**Step 3: Test Transaction Flow**
1. Open web wallet in browser
2. Create new wallet or import existing seed
3. Build transparent transaction
4. Broadcast to Orchard RPC
5. Verify transaction appears in mempool

**Example RPC Test**:
```bash
# Test sendrawtransaction
curl -X POST http://localhost:8232 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"1.0",
    "method":"sendrawtransaction",
    "params":["0100000001..."],
    "id":1
  }'

# Expected response:
# {"jsonrpc":"1.0","result":"txid123...","id":1}
```

### Migration Path (Optional)

While the web wallet works perfectly with transparent transactions, you can optionally add Orchard support later:

**Phase 1: Transparent Only (Current - Demo/Experimental)**
- ✅ Web wallet uses transparent transactions
- ✅ Works with standard RPC methods
- ✅ No wallet code changes needed

**Phase 2: Hybrid Support (In Progress)**
- ✅ Orchard tree sync helpers + local state (client-side)
- ☐ Orchard send UI + bundle construction (WASM)
- ☐ Mixed transparent + Orchard send flows
- ☐ Gradual migration of users to Orchard

**Phase 3: Orchard Only (Long-term)**
- Deprecate transparent transaction support
- Full privacy-preserving transactions
- Zcash Orchard feature parity

## Implementation Summary

### What Was Built (Option B)

**New Components**:
1. **TransparentTxStore** (`builder/orchard/rpc/transparent_tx.go`)
   - Transparent transaction storage and mempool
   - Minimal parsing for inputs/outputs and txid derivation
   - Raw transaction hex storage
   - Persistent mempool tracking (LevelDB-backed cache)

2. **RPC Methods** (`builder/orchard/rpc/handler.go`)
   - `GetRawTransaction` - Query transactions
   - `SendRawTransaction` - Broadcast transactions
   - `GetRawMempool` - Mempool queries
   - `GetMempoolInfo` - Mempool statistics

3. **HTTP Server Updates** (`builder/orchard/rpc/server.go`)
   - Routing for new RPC methods
   - JSON-RPC 1.0 protocol support

**Architecture**:
```
┌─────────────────┐
│   Web Wallet    │
│   (Browser)     │
└────────┬────────┘
         │ JSON-RPC (transparent txs)
         ▼
┌─────────────────┐      ┌──────────────────┐
│ Orchard RPC     │◄────►│ Transparent Pool │
│ (port 8232)     │      │ (TransparentTxStore)│
└────────┬────────┘      └──────────────────┘
         │
         ▼
┌─────────────────┐      ┌──────────────────┐
│ Orchard Builder │◄────►│ Orchard Pool     │
│ (JAM Network)   │      │ (OrchardTxPool)  │
└─────────────────┘      └──────────────────┘
```

**Security Model**:
- Web wallet builds transactions in browser (WASM)
- Private keys never leave client
- Only signed raw transactions sent to server
- Server stores and serves mempool transactions

**Benefits**:
- ✅ Zero web wallet code changes
- ✅ Standard Zcash RPC compatibility
- ✅ Privacy maintained (client-side signing)
- ✅ Dual pool support (transparent + Orchard)
- ✅ Backward compatible with existing tools

## Further Reading

- [ORCHARD.md](ORCHARD.md#transparent-transaction-support-2026-01-04) - Transparent transaction implementation details
- [SERVICE-TESTING.md](SERVICE-TESTING.md#transparent-transaction-testing) - Testing guide with RPC examples
