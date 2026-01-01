# Orchard Wallet Integration for JAM

**Status**: Phase 1 Complete - Ready for Production Integration
**Date**: 2025-12-30

## Overview

Complete wallet abstraction implementation for Orchard privacy-preserving transactions, providing secure separation between key management, note discovery, and proof generation while maintaining full Zcash Orchard compatibility.

## Current Implementation

### **Wallet Interface**: `builder/orchard/wallet/orchard_wallet.go`

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
    return float64(total) / 100000000.0, nil // Convert to ZEC
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

## Phase 2: MetaMask Integration with WASM Proof Generation

**Status**: NEXT PRIORITY - Production wallet integration
**Timeline**: 2-3 weeks development + 1 week testing
**Complexity**: HIGH (cryptographic integration across browser/Rust/Go stack)

### Overview

Implement browser-based MetaMask wallet integration using signed-message secret derivation and WASM-compiled Orchard proof generation for seamless user experience.

### Architecture Design

```
┌─────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   Browser   │    │ Orchard Builder  │    │ Orchard WASM    │
│  (MetaMask) │◄──►│   Node (Go RPC)  │◄──►│ (Rust Proofs)   │
└─────────────┘    └──────────────────┘    └─────────────────┘
       │                   │                     │
       ▼                   ▼                     ▼
   eth_sign         OrchardWallet          orchard_prove()
   personal_sign    interface              orchard_verify()
                          │
                          ▼
                    JAM Network
                 (work packages)
```

### Implementation Plan

#### **Phase 2.1: MetaMask Wallet Provider** (Week 1)

**Location**: `builder/orchard/wallet/metamask/`

```go
// MetaMaskWallet implements OrchardWallet using MetaMask signatures
type MetaMaskWallet struct {
    rpcURL      string
    domain      string
    addresses   map[string]*MetaMaskOrchardAddress
    wasmProver  WASMProver
}

type MetaMaskOrchardAddress struct {
    EthAddress   string          // Ethereum address (0x...)
    OrchardAddr  string          // Derived Orchard address (u1...)
    SpendingKey  [32]byte        // Derived from MetaMask signature
    FullViewingKey [32]byte      // Derived from spending key
    IncomingViewingKey [32]byte  // Derived from FVK
}

// Core interface implementation
func (w *MetaMaskWallet) NewAddress() (string, error) {
    // 1. Call eth_accounts to get MetaMask address
    // 2. Generate domain-separated message for address derivation
    // 3. Call personal_sign for signature
    // 4. Derive Orchard keys using standard key derivation
    // 5. Return u1-prefixed unified address
}

func (w *MetaMaskWallet) NullifierFor(spendingKey, rho, psi, commitment [32]byte) ([32]byte, error) {
    // 1. Find MetaMask address for spendingKey
    // 2. Generate nullifier derivation message
    // 3. Call personal_sign for spend key confirmation
    // 4. Compute nullifier via WASM Orchard nullifier derivation
    // 5. Clear spending key from memory
}

func (w *MetaMaskWallet) BundleProofFor(bundleReq *OrchardBundleRequest) (*OrchardBundle, error) {
    // 1. Parse bundle request and validate spends/outputs
    // 2. Generate proving key derivation message
    // 3. Call personal_sign for witness generation
    // 4. Call WASM Orchard bundle proof generation
    // 5. Return complete Orchard bundle with proof
}
```

**Key Features**:
- **Signed-Message Secret Derivation**: `orchard_sk = Hash(Sign(sk_ethereum, domain_message))`
- **Domain Separation**: Prevents cross-application key reuse
- **Standard Orchard Keys**: Full compatibility with Zcash Orchard key hierarchy
- **Memory Safety**: Automatic clearing of sensitive material

#### **Phase 2.2: WASM Proof Generation** (Week 1-2)

**Location**: `services/orchard/wasm/`

```rust
// WASM bindings for browser-based Orchard proving
#[wasm_bindgen]
pub struct OrchardWASMProver {
    proving_key: ProvingKey,
    verifying_key: VerifyingKey,
}

#[wasm_bindgen]
impl OrchardWASMProver {
    #[wasm_bindgen(constructor)]
    pub fn new() -> OrchardWASMProver { /* ... */ }

    // Core Orchard cryptographic operations
    #[wasm_bindgen]
    pub fn generate_spending_key(&self, seed: &[u8]) -> Result<Vec<u8>, JsValue>;

    #[wasm_bindgen]
    pub fn derive_full_viewing_key(&self, spending_key: &[u8]) -> Result<Vec<u8>, JsValue>;

    #[wasm_bindgen]
    pub fn derive_incoming_viewing_key(&self, full_viewing_key: &[u8]) -> Result<Vec<u8>, JsValue>;

    #[wasm_bindgen]
    pub fn generate_address(&self, ivk: &[u8], diversifier_index: u64) -> Result<OrchardAddressJS, JsValue>;

    #[wasm_bindgen]
    pub fn generate_nullifier(&self, spending_key: &[u8], rho: &[u8], psi: &[u8], commitment: &[u8]) -> Result<Vec<u8>, JsValue>;

    #[wasm_bindgen]
    pub fn generate_commitment(&self, value: u64, diversifier: &[u8], pk_d: &[u8], rho: &[u8], psi: &[u8]) -> Result<Vec<u8>, JsValue>;

    #[wasm_bindgen]
    pub fn prove_bundle(&self, bundle_data: &[u8]) -> Result<OrchardBundleProofJS, JsValue>;

    #[wasm_bindgen]
    pub fn verify_bundle(&self, bundle: &[u8], proof: &[u8]) -> Result<bool, JsValue>;
}

#[wasm_bindgen]
pub struct OrchardBundleProofJS {
    proof_bytes: Vec<u8>,
    public_inputs: Vec<u8>,
}
```

**Build Configuration**:
```toml
# services/orchard/Cargo.toml
[lib]
crate-type = ["rlib", "cdylib"]

[features]
default = []
wasm = ["wasm-bindgen", "console_error_panic_hook", "wee_alloc", "orchard/wasm"]
ffi = []

[dependencies]
orchard = "0.6"
wasm-bindgen = { version = "0.2", optional = true }
js-sys = { version = "0.3", optional = true }
console_error_panic_hook = { version = "0.1", optional = true }
wee_alloc = { version = "0.4", optional = true }
```

**JavaScript Interface**:
```typescript
// Generated bindings from wasm-pack
import { OrchardWASMProver, OrchardBundleProofJS } from './pkg/orchard_wasm';

export class OrchardBrowserProver {
    private prover: OrchardWASMProver;

    constructor() {
        this.prover = new OrchardWASMProver();
    }

    async generateBundle(bundleRequest: OrchardBundleRequest): Promise<OrchardBundle> {
        const bundleData = this.serializeBundleRequest(bundleRequest);
        const result = await this.prover.prove_bundle(bundleData);

        return {
            actions: bundleRequest.actions,
            flags: bundleRequest.flags,
            valueBalance: bundleRequest.valueBalance,
            anchor: bundleRequest.anchor,
            proof: result.proof_bytes,
            bindingSig: await this.generateBindingSignature(bundleRequest)
        };
    }
}
```

#### **Phase 2.3: Browser Integration** (Week 2)

**Location**: `web/orchard/` (new frontend module)

```javascript
// MetaMask integration with OrchardWASMProver
class MetaMaskOrchardWallet {
    constructor(provider, domain = "orchard.jam.chain") {
        this.provider = provider; // window.ethereum
        this.domain = domain;
        this.prover = new OrchardBrowserProver();
        this.addresses = new Map();
    }

    async newAddress() {
        // Get user's Ethereum address
        const accounts = await this.provider.request({method: 'eth_accounts'});
        const ethAddr = accounts[0];

        // Generate domain-separated message
        const message = `Generate Orchard address for ${this.domain}\nEthereum: ${ethAddr}\nNonce: ${Date.now()}`;

        // Request signature from MetaMask
        const signature = await this.provider.request({
            method: 'personal_sign',
            params: [message, ethAddr]
        });

        // Derive Orchard keys using standard derivation
        const orchardKeys = await this.deriveOrchardKeys(signature);
        const orchardAddr = await this.encodeUnifiedAddress(orchardKeys);

        this.addresses.set(orchardAddr, {
            ethAddr,
            orchardKeys,
            derivationMessage: message
        });

        return orchardAddr;
    }

    async sendTransaction(fromAddress, recipients, fee) {
        // 1. Select notes for spending
        const spends = await this.selectSpends(fromAddress, recipients, fee);

        // 2. Build outputs
        const outputs = recipients.map(r => ({
            address: this.parseOrchardAddress(r.address),
            value: r.amount,
            memo: r.memo || ""
        }));

        // 3. Generate bundle request
        const bundleRequest = {
            spends,
            outputs,
            valueBalance: fee,
            anchor: await this.getCurrentAnchor()
        };

        // 4. Generate proof via WASM
        const bundle = await this.prover.generateBundle(bundleRequest);

        // 5. Submit to JAM via builder
        return await this.submitToBuilder(bundle);
    }

    async deriveOrchardKeys(signature) {
        const seed = await crypto.subtle.digest('SHA-256',
            new TextEncoder().encode(signature)
        );

        const spendingKey = await this.prover.generate_spending_key(new Uint8Array(seed));
        const fullViewingKey = await this.prover.derive_full_viewing_key(spendingKey);
        const incomingViewingKey = await this.prover.derive_incoming_viewing_key(fullViewingKey);

        return {
            spendingKey,
            fullViewingKey,
            incomingViewingKey
        };
    }
}
```

#### **Phase 2.4: Builder Node/Browser Bridge** (Week 2-3)

**Location**: `builder/orchard/wallet/bridge/`

```go
// HTTPMetaMaskWallet bridges browser MetaMask to OrchardWallet interface
type HTTPMetaMaskWallet struct {
    browserURL string
    client     *http.Client
    sessionID  string
}

func NewHTTPMetaMaskWallet(browserURL string) *HTTPMetaMaskWallet {
    return &HTTPMetaMaskWallet{
        browserURL: browserURL,
        client: &http.Client{Timeout: 30 * time.Second},
        sessionID: generateSessionID(),
    }
}

// Implement OrchardWallet interface via HTTP calls to browser
func (w *HTTPMetaMaskWallet) NewAddress() (string, error) {
    req := BrowserRequest{
        Method: "newAddress",
        SessionID: w.sessionID,
    }

    resp, err := w.callBrowser(req)
    if err != nil {
        return "", err
    }

    return resp.Address, nil
}

func (w *HTTPMetaMaskWallet) BundleProofFor(bundleReq *OrchardBundleRequest) (*OrchardBundle, error) {
    req := BrowserRequest{
        Method: "generateBundle",
        Params: map[string]interface{}{
            "spends":       bundleReq.Spends,
            "outputs":      bundleReq.Outputs,
            "valueBalance": bundleReq.ValueBalance,
            "anchor":       hex.EncodeToString(bundleReq.Anchor[:]),
        },
        SessionID: w.sessionID,
    }

    resp, err := w.callBrowser(req)
    if err != nil {
        return nil, err
    }

    return w.parseOrchardBundle(resp.Bundle)
}
```

### Security Considerations

#### **Threat Model**:
- ✅ **MetaMask compromise**: Orchard spending keys derived fresh per operation
- ✅ **Network interception**: No private keys transmitted over HTTP
- ✅ **Browser memory**: Automatic clearing of sensitive material
- ✅ **Domain separation**: App-specific secrets prevent cross-contamination
- ⚠️ **Browser-side attacks**: WASM module isolation, CSP headers required

#### **Implementation Requirements**:
1. **Message formats**: Standardized, versioned domain separation
2. **Session management**: Secure browser/Go communication
3. **Performance**: Sub-5s proof generation for basic transactions
4. **Compatibility**: Support MetaMask + WalletConnect + other EIP-1193 providers

### Delivery Milestones

#### **Week 1**: WASM Foundation
- [ ] WASM build configuration and Orchard cryptographic functions
- [ ] Browser TypeScript bindings and test harness
- [ ] Performance baseline (target: <5s proof generation)

#### **Week 2**: MetaMask Integration
- [ ] Browser wallet with signature-based key derivation
- [ ] HTTP bridge implementation
- [ ] Basic transaction proof generation

#### **Week 3**: Production Readiness
- [ ] End-to-end testing with real MetaMask
- [ ] Security audit and penetration testing
- [ ] Performance optimization and monitoring
- [ ] Documentation and deployment guides

**Success Criteria**: Users can generate Orchard bundles directly in browser using MetaMask signatures, with no private key exposure and full Zcash Orchard compatibility.

---

## Alternative Wallet Integration Patterns

### **Hardware Wallet Integration** (Phase 3)

**Location**: `builder/orchard/wallet/hardware/`

```go
// Hardware wallet integration via USB/Bluetooth
type LedgerOrchardWallet struct {
    device     *ledger.Device
    derivePath string
}

// Key operations delegated to hardware device
func (w *LedgerOrchardWallet) BundleProofFor(bundleReq *OrchardBundleRequest) (*OrchardBundle, error) {
    // 1. Send bundle request to Ledger
    // 2. User confirms on device
    // 3. Ledger generates proof using internal HSM
    // 4. Return signed bundle
}
```

### **Custodial Wallet Integration** (Phase 4)

**Location**: `builder/orchard/wallet/custodial/`

```go
// Enterprise custodial wallet integration
type CustodialOrchardWallet struct {
    apiEndpoint string
    credentials *CustodialCredentials
}

// Proof generation via secure custody service
func (w *CustodialOrchardWallet) BundleProofFor(bundleReq *OrchardBundleRequest) (*OrchardBundle, error) {
    // 1. Authenticate with custody service
    // 2. Submit bundle request via API
    // 3. Service generates proof in secure enclave
    // 4. Return signed bundle with audit trail
}
```

---

## Current Implementation Status Summary

✅ **Phase 1 Complete**: Orchard wallet abstraction with standard cryptographic operations
🚧 **Phase 2 Next**: MetaMask integration with WASM proof generation
📋 **Phase 3 Future**: Hardware wallet support, mobile integration
📋 **Phase 4 Future**: Enterprise custodial integration, advanced features

This Orchard wallet integration provides a secure foundation for JAM's privacy-preserving transactions while maintaining full compatibility with standard Zcash Orchard protocols.