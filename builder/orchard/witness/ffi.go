// Package witness provides FFI bindings to Rust Orchard cryptographic operations
package witness

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../services/target/release -lorchard_service -Wl,-rpath,${SRCDIR}/../../../services/target/release
#include <stdint.h>
#include <stdlib.h>

// FFI Result codes
#define FFI_SUCCESS 0
#define FFI_INVALID_INPUT 1
#define FFI_PROOF_GENERATION_FAILED 2
#define FFI_VERIFICATION_FAILED 3
#define FFI_SERIALIZATION_ERROR 4
#define FFI_INTERNAL_ERROR 5

// Extrinsic type constants
#define EXTRINSIC_DEPOSIT_PUBLIC 0
#define EXTRINSIC_SUBMIT_PRIVATE 1
#define EXTRINSIC_WITHDRAW_PUBLIC 2
#define EXTRINSIC_ISSUANCE_V1 3
#define EXTRINSIC_BATCH_AGG_V1 4

// Orchard FFI functions
uint32_t orchard_init();
void orchard_cleanup();
uint32_t orchard_get_service_id();

// Cryptographic operations
uint32_t orchard_nullifier(
    const uint8_t* sk_spend,
    const uint8_t* rho,
    const uint8_t* commitment,
    uint8_t* output
);

uint32_t orchard_commitment(
    uint32_t asset_id,
    uint64_t amount_lo,
    uint64_t amount_hi,
    const uint8_t* owner_pk,
    const uint8_t* rho,
    const uint8_t* note_rseed,
    uint64_t unlock_height,
    const uint8_t* memo_hash,
    uint8_t* output
);

uint32_t orchard_poseidon_hash(
    const uint8_t* domain,
    uint32_t domain_len,
    const uint8_t* inputs,
    uint32_t input_count,
    uint8_t* output
);

// Mock proof operations have been completely removed for security.
// Real Halo2 proof generation and verification must be implemented.
// Any attempt to use mock proofs will result in compilation errors.

// Service ID management
uint32_t orchard_set_service_id(uint32_t service_id);
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

// Uint128 provides a simple 128-bit unsigned integer representation using two uint64 limbs.
type Uint128 struct {
	Lo uint64
	Hi uint64
}

// ExtrinsicType represents the type of Orchard extrinsic
type ExtrinsicType uint32

const (
	DepositPublic  ExtrinsicType = C.EXTRINSIC_DEPOSIT_PUBLIC
	SubmitPrivate  ExtrinsicType = C.EXTRINSIC_SUBMIT_PRIVATE
	WithdrawPublic ExtrinsicType = C.EXTRINSIC_WITHDRAW_PUBLIC
	IssuanceV1     ExtrinsicType = C.EXTRINSIC_ISSUANCE_V1
	BatchAggV1     ExtrinsicType = C.EXTRINSIC_BATCH_AGG_V1
)

// OrchardProofSize is the Halo2 proof size in bytes (services/orchard/keys/*.proof).
const OrchardProofSize = 2464

// FFI error types
var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrProofGenerationFailed = errors.New("proof generation failed")
	ErrVerificationFailed    = errors.New("verification failed")
	ErrSerializationError    = errors.New("serialization error")
	ErrInternalError         = errors.New("internal error")
)

// OrchardFFI provides access to Rust Orchard cryptographic operations
type OrchardFFI struct {
	initialized bool
}

// NewOrchardFFI creates a new Orchard FFI instance
func NewOrchardFFI() (*OrchardFFI, error) {
	ffi := &OrchardFFI{}
	if err := ffi.Init(); err != nil {
		return nil, err
	}
	return ffi, nil
}

// Init initializes the Orchard FFI system
func (r *OrchardFFI) Init() error {
	result := C.orchard_init()
	if result != C.FFI_SUCCESS {
		return ErrInternalError
	}
	r.initialized = true
	return nil
}

// Cleanup cleans up the Orchard FFI system
func (r *OrchardFFI) Cleanup() {
	if r.initialized {
		C.orchard_cleanup()
		r.initialized = false
	}
}

// GetServiceID returns the Orchard service ID
func (r *OrchardFFI) GetServiceID() uint32 {
	return uint32(C.orchard_get_service_id())
}

// SetServiceID sets the service ID for subsequent operations
func (r *OrchardFFI) SetServiceID(serviceID uint32) error {
	result := C.orchard_set_service_id(C.uint32_t(serviceID))
	if result != C.FFI_SUCCESS {
		return ffiResultToError(result)
	}
	return nil
}

// GenerateNullifier creates a nullifier from spending key, rho, and commitment
func (r *OrchardFFI) GenerateNullifier(skSpend, rho, commitment [32]byte) ([32]byte, error) {
	var output [32]byte

	result := C.orchard_nullifier(
		(*C.uint8_t)(unsafe.Pointer(&skSpend[0])),
		(*C.uint8_t)(unsafe.Pointer(&rho[0])),
		(*C.uint8_t)(unsafe.Pointer(&commitment[0])),
		(*C.uint8_t)(unsafe.Pointer(&output[0])),
	)

	if result != C.FFI_SUCCESS {
		return [32]byte{}, ffiResultToError(result)
	}

	return output, nil
}

// GenerateCommitment creates a note commitment
func (r *OrchardFFI) GenerateCommitment(
	assetID uint32,
	amount Uint128,
	ownerPk, rho, noteRseed [32]byte,
	unlockHeight uint64,
	memoHash [32]byte,
) ([32]byte, error) {
	var output [32]byte

	result := C.orchard_commitment(
		C.uint32_t(assetID),
		C.uint64_t(amount.Lo),
		C.uint64_t(amount.Hi),
		(*C.uint8_t)(unsafe.Pointer(&ownerPk[0])),
		(*C.uint8_t)(unsafe.Pointer(&rho[0])),
		(*C.uint8_t)(unsafe.Pointer(&noteRseed[0])),
		C.uint64_t(unlockHeight),
		(*C.uint8_t)(unsafe.Pointer(&memoHash[0])),
		(*C.uint8_t)(unsafe.Pointer(&output[0])),
	)

	if result != C.FFI_SUCCESS {
		return [32]byte{}, ffiResultToError(result)
	}

	return output, nil
}

// PoseidonHash computes a domain-separated Poseidon hash of the given inputs.
func (r *OrchardFFI) PoseidonHash(domain string, inputs [][32]byte) ([32]byte, error) {
	var output [32]byte

	if len(domain) == 0 || len(domain) > 64 {
		return [32]byte{}, ErrInvalidInput
	}
	if len(inputs) == 0 || len(inputs) > 10 {
		return [32]byte{}, ErrInvalidInput
	}

	domainBytes := []byte(domain)

	// Flatten inputs into a single byte slice
	inputBytes := make([]byte, len(inputs)*32)
	for i, input := range inputs {
		copy(inputBytes[i*32:], input[:])
	}

	result := C.orchard_poseidon_hash(
		(*C.uint8_t)(unsafe.Pointer(&domainBytes[0])),
		C.uint32_t(len(domainBytes)),
		(*C.uint8_t)(unsafe.Pointer(&inputBytes[0])),
		C.uint32_t(len(inputs)),
		(*C.uint8_t)(unsafe.Pointer(&output[0])),
	)

	if result != C.FFI_SUCCESS {
		return [32]byte{}, ffiResultToError(result)
	}

	return output, nil
}

// GenerateProof: REMOVED FOR SECURITY - No mock proof generation allowed
// Mock proof functions were a critical security vulnerability that bypassed all cryptographic validation.
// Real Halo2 proof generation must be implemented using proper cryptographic circuits.
func (r *OrchardFFI) GenerateProof(extrinsicType ExtrinsicType, inputData []byte) ([]byte, error) {
	return nil, fmt.Errorf("mock proof generation removed for security - implement real Halo2 proof generation")
}

// VerifyProof: REMOVED FOR SECURITY - No mock proof verification allowed
// Mock proof functions were a critical security vulnerability that bypassed all cryptographic validation.
// Real Halo2 proof verification must be implemented using proper cryptographic circuits.
func (r *OrchardFFI) VerifyProof(extrinsicType ExtrinsicType, proof, publicInputs []byte) error {
	return fmt.Errorf("mock proof verification removed for security - implement real Halo2 proof verification")
}

// ffiResultToError converts FFI result codes to Go errors
func ffiResultToError(result C.uint32_t) error {
	switch result {
	case C.FFI_SUCCESS:
		return nil
	case C.FFI_INVALID_INPUT:
		return ErrInvalidInput
	case C.FFI_PROOF_GENERATION_FAILED:
		return ErrProofGenerationFailed
	case C.FFI_VERIFICATION_FAILED:
		return ErrVerificationFailed
	case C.FFI_SERIALIZATION_ERROR:
		return ErrSerializationError
	case C.FFI_INTERNAL_ERROR:
		return ErrInternalError
	default:
		return fmt.Errorf("unknown FFI error code: %d", result)
	}
}

// ExtrinsicData represents the data for a Orchard extrinsic
type ExtrinsicData struct {
	Type         ExtrinsicType
	AssetID      uint32
	Amount       Uint128
	OwnerPk      [32]byte
	Rho          [32]byte
	NoteRseed    [32]byte
	UnlockHeight uint64
	MemoHash     [32]byte
}

// ProcessExtrinsic is a convenience function that performs the full builder flow
// for a given extrinsic: generate commitment, nullifier, and proof
func (r *OrchardFFI) ProcessExtrinsic(data ExtrinsicData, wallet OrchardWallet) (*ExtrinsicResult, error) {
	if wallet == nil {
		return nil, ErrInvalidInput
	}

	// Generate commitment
	commitment, err := r.GenerateCommitment(
		data.AssetID,
		data.Amount,
		data.OwnerPk,
		data.Rho,
		data.NoteRseed,
		data.UnlockHeight,
		data.MemoHash,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate commitment: %v", err)
	}

	// Generate nullifier using the provided wallet
	nullifier, err := wallet.NullifierFor(data.OwnerPk, data.Rho, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to generate nullifier: %v", err)
	}

	// Generate proof
	inputData := append(commitment[:], nullifier[:]...)
	proof, publicInputs, err := wallet.ProofFor(data.Type, inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate proof: %v", err)
	}

	if len(publicInputs) == 0 {
		publicInputs = append([]byte(nil), inputData...)
	}

	return &ExtrinsicResult{
		Commitment:   commitment,
		Nullifier:    nullifier,
		Proof:        proof,
		PublicInputs: publicInputs,
	}, nil
}

// ExtrinsicResult contains the outputs of processing an extrinsic
type ExtrinsicResult struct {
	Commitment   [32]byte
	Nullifier    [32]byte
	Proof        []byte
	PublicInputs []byte
}

// VerifyExtrinsic verifies an extrinsic result using the guarantor flow
func (r *OrchardFFI) VerifyExtrinsic(data ExtrinsicData, result *ExtrinsicResult, wallet OrchardWallet) error {
	if wallet == nil {
		return ErrInvalidInput
	}

	// Verify the proof
	if err := r.VerifyProof(data.Type, result.Proof, result.PublicInputs); err != nil {
		return fmt.Errorf("proof verification failed: %v", err)
	}

	// Verify commitment consistency
	expectedCommitment, err := r.GenerateCommitment(
		data.AssetID,
		data.Amount,
		data.OwnerPk,
		data.Rho,
		data.NoteRseed,
		data.UnlockHeight,
		data.MemoHash,
	)
	if err != nil {
		return fmt.Errorf("failed to recompute commitment: %v", err)
	}

	if expectedCommitment != result.Commitment {
		return fmt.Errorf("commitment mismatch: expected %x, got %x", expectedCommitment, result.Commitment)
	}

	// Verify nullifier consistency using the provided wallet
	expectedNullifier, err := wallet.NullifierFor(data.OwnerPk, data.Rho, result.Commitment)
	if err != nil {
		return fmt.Errorf("failed to recompute nullifier: %v", err)
	}

	if expectedNullifier != result.Nullifier {
		return fmt.Errorf("nullifier mismatch: expected %x, got %x", expectedNullifier, result.Nullifier)
	}

	return nil
}
