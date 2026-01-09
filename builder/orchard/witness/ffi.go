// Package witness provides FFI bindings to Rust Orchard cryptographic operations
package witness

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../services/target/debug -lorchard_service -Wl,-rpath,${SRCDIR}/../../../services/target/debug -Wl,-rpath,${SRCDIR}/../../../services/target/debug/deps -L${SRCDIR}/../../../services/target/release -lorchard_service -Wl,-rpath,${SRCDIR}/../../../services/target/release -Wl,-rpath,${SRCDIR}/../../../services/target/release/deps -L${SRCDIR}/../target/debug -lorchard_builder -Wl,-rpath,${SRCDIR}/../target/debug -Wl,-rpath,${SRCDIR}/../target/debug/deps -L${SRCDIR}/../target/release -lorchard_builder -Wl,-rpath,${SRCDIR}/../target/release -Wl,-rpath,${SRCDIR}/../target/release/deps
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

uint32_t orchard_issue_bundle_commitments(
    const uint8_t* issue_bundle_ptr,
    uint32_t issue_bundle_len,
    uint8_t* commitments_ptr,
    uint32_t commitments_cap,
    uint32_t* commitments_len_out
);

uint32_t orchard_refine_witness_aware(
    uint32_t service_id,
    const uint8_t* pre_state_payload_ptr,
    uint32_t pre_state_payload_len,
    const uint8_t* pre_witness_ptr,
    uint32_t pre_witness_len,
    const uint8_t* post_witness_ptr,
    uint32_t post_witness_len,
    const uint8_t* bundle_proof_ptr,
    uint32_t bundle_proof_len
);

uint32_t orchard_builder_generate_proof(
    const uint8_t* seed,
    uint32_t seed_len,
    uint8_t* proof_buf,
    uint32_t proof_cap,
    uint8_t* inputs_buf,
    uint32_t inputs_cap,
    uint32_t* inputs_len_out
);

uint32_t orchard_builder_generate_bundle(
    const uint8_t* seed,
    uint32_t seed_len,
    const uint8_t* anchor,
    uint32_t anchor_len,
    uint8_t* bundle_buf,
    uint32_t bundle_cap,
    uint32_t* bundle_len_out
);

uint32_t orchard_builder_decode_bundle(
    const uint8_t* bundle_ptr,
    uint32_t bundle_len,
    uint8_t* nullifiers_buf,
    uint32_t nullifiers_cap,
    uint8_t* commitments_buf,
    uint32_t commitments_cap,
    uint8_t* proof_buf,
    uint32_t proof_cap,
    uint8_t* inputs_buf,
    uint32_t inputs_cap,
    uint32_t* action_count_out,
    uint32_t* inputs_len_out,
    uint32_t* proof_len_out
);

uint32_t orchard_decode_bundle_v6(
    const uint8_t* orchard_bundle_ptr,
    uint32_t orchard_bundle_len,
    const uint8_t* issue_bundle_ptr,
    uint32_t issue_bundle_len,
    uint8_t* bundle_type_out,
    uint32_t* group_sizes_ptr,
    uint32_t group_sizes_cap,
    uint32_t* group_count_out,
    uint8_t* nullifiers_ptr,
    uint32_t nullifiers_cap,
    uint8_t* commitments_ptr,
    uint32_t commitments_cap,
    uint8_t* issue_bundle_out,
    uint32_t issue_bundle_cap,
    uint32_t* issue_bundle_len_out
);

uint32_t orchard_verify_halo2_proof(
    uint32_t vk_id,
    const uint8_t* proof_ptr,
    uint32_t proof_len,
    const uint8_t* public_inputs_ptr,
    uint32_t public_inputs_len
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

// OrchardProofSize is the Halo2 proof size in bytes for a 2-action Orchard bundle.
const OrchardProofSize = 7264
const orchardProofActions = 2
const orchardInputsPerAction = 9
const orchardMaxActions = 4
const orchardMaxActionGroups = orchardMaxActions
const orchardMaxBundleSize = 64 * 1024

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

// GenerateProofWithPublicInputs creates a real Orchard proof and returns its public inputs.
func (r *OrchardFFI) GenerateProofWithPublicInputs(extrinsicType ExtrinsicType, seed []byte) ([]byte, []byte, error) {
	if extrinsicType != SubmitPrivate {
		return nil, nil, ErrInvalidInput
	}

	proof := make([]byte, OrchardProofSize)
	maxInputs := orchardProofActions * orchardInputsPerAction
	inputsBuf := make([]byte, maxInputs*32)
	var inputsLen C.uint32_t

	var seedPtr *C.uint8_t
	if len(seed) > 0 {
		seedPtr = (*C.uint8_t)(unsafe.Pointer(&seed[0]))
	}

	result := C.orchard_builder_generate_proof(
		seedPtr,
		C.uint32_t(len(seed)),
		(*C.uint8_t)(unsafe.Pointer(&proof[0])),
		C.uint32_t(len(proof)),
		(*C.uint8_t)(unsafe.Pointer(&inputsBuf[0])),
		C.uint32_t(len(inputsBuf)),
		(*C.uint32_t)(unsafe.Pointer(&inputsLen)),
	)
	if result != C.FFI_SUCCESS {
		return nil, nil, ffiResultToError(result)
	}

	inputCount := int(inputsLen)
	if inputCount == 0 || inputCount > maxInputs {
		return nil, nil, ErrInvalidInput
	}

	publicInputs := inputsBuf[:inputCount*32]
	return proof, publicInputs, nil
}

// DecodedBundle contains Orchard bundle data extracted from serialized bytes.
type DecodedBundle struct {
	Nullifiers   [][32]byte
	Commitments  [][32]byte
	Proof        []byte
	PublicInputs [][32]byte
}

// DecodedBundleV6 contains Orchard V6 bundle data extracted from serialized bytes.
type DecodedBundleV6 struct {
	BundleType       uint8
	ActionGroupSizes []int
	Nullifiers       [][32]byte
	Commitments      [][32]byte
	IssueBundle      []byte
}

// GenerateBundle builds a deterministic Orchard bundle and returns serialized bytes.
func (r *OrchardFFI) GenerateBundle(seed []byte, anchor [32]byte) ([]byte, error) {
	if r == nil {
		return nil, ErrInvalidInput
	}

	bundleBuf := make([]byte, orchardMaxBundleSize)
	var bundleLen C.uint32_t

	var seedPtr *C.uint8_t
	if len(seed) > 0 {
		seedPtr = (*C.uint8_t)(unsafe.Pointer(&seed[0]))
	}

	result := C.orchard_builder_generate_bundle(
		seedPtr,
		C.uint32_t(len(seed)),
		(*C.uint8_t)(unsafe.Pointer(&anchor[0])),
		C.uint32_t(len(anchor)),
		(*C.uint8_t)(unsafe.Pointer(&bundleBuf[0])),
		C.uint32_t(len(bundleBuf)),
		(*C.uint32_t)(unsafe.Pointer(&bundleLen)),
	)
	if result != C.FFI_SUCCESS {
		return nil, ffiResultToError(result)
	}

	if bundleLen == 0 || int(bundleLen) > len(bundleBuf) {
		return nil, ErrInvalidInput
	}

	return append([]byte(nil), bundleBuf[:bundleLen]...), nil
}

// DecodeBundle parses a serialized Orchard bundle and returns its fields for work packages.
func (r *OrchardFFI) DecodeBundle(bundle []byte) (*DecodedBundle, error) {
	if r == nil || len(bundle) == 0 {
		return nil, ErrInvalidInput
	}

	nullifiersBuf := make([]byte, orchardMaxActions*32)
	commitmentsBuf := make([]byte, orchardMaxActions*32)
	proofBuf := make([]byte, OrchardProofSize)
	inputsBuf := make([]byte, orchardMaxActions*orchardInputsPerAction*32)
	var actionCount C.uint32_t
	var inputsLen C.uint32_t
	var proofLen C.uint32_t

	result := C.orchard_builder_decode_bundle(
		(*C.uint8_t)(unsafe.Pointer(&bundle[0])),
		C.uint32_t(len(bundle)),
		(*C.uint8_t)(unsafe.Pointer(&nullifiersBuf[0])),
		C.uint32_t(len(nullifiersBuf)),
		(*C.uint8_t)(unsafe.Pointer(&commitmentsBuf[0])),
		C.uint32_t(len(commitmentsBuf)),
		(*C.uint8_t)(unsafe.Pointer(&proofBuf[0])),
		C.uint32_t(len(proofBuf)),
		(*C.uint8_t)(unsafe.Pointer(&inputsBuf[0])),
		C.uint32_t(len(inputsBuf)),
		(*C.uint32_t)(unsafe.Pointer(&actionCount)),
		(*C.uint32_t)(unsafe.Pointer(&inputsLen)),
		(*C.uint32_t)(unsafe.Pointer(&proofLen)),
	)
	if result != C.FFI_SUCCESS {
		return nil, ffiResultToError(result)
	}

	actionCountInt := int(actionCount)
	if actionCountInt == 0 || actionCountInt > orchardMaxActions {
		return nil, ErrInvalidInput
	}

	nullifiers := make([][32]byte, actionCountInt)
	commitments := make([][32]byte, actionCountInt)
	for i := 0; i < actionCountInt; i++ {
		copy(nullifiers[i][:], nullifiersBuf[i*32:(i+1)*32])
		copy(commitments[i][:], commitmentsBuf[i*32:(i+1)*32])
	}

	inputsCount := int(inputsLen)
	if inputsCount == 0 || inputsCount > orchardMaxActions*orchardInputsPerAction {
		return nil, ErrInvalidInput
	}
	publicInputs := make([][32]byte, inputsCount)
	for i := 0; i < inputsCount; i++ {
		copy(publicInputs[i][:], inputsBuf[i*32:(i+1)*32])
	}

	proofSize := int(proofLen)
	if proofSize <= 0 || proofSize > len(proofBuf) {
		return nil, ErrInvalidInput
	}

	return &DecodedBundle{
		Nullifiers:   nullifiers,
		Commitments:  commitments,
		Proof:        append([]byte(nil), proofBuf[:proofSize]...),
		PublicInputs: publicInputs,
	}, nil
}

// DecodeBundleV6 parses a serialized Orchard V6 bundle and returns action group metadata.
func (r *OrchardFFI) DecodeBundleV6(orchardBundle []byte, issueBundle []byte) (*DecodedBundleV6, error) {
	if r == nil {
		return nil, ErrInvalidInput
	}
	if len(orchardBundle) == 0 && len(issueBundle) == 0 {
		return nil, ErrInvalidInput
	}

	groupSizesBuf := make([]C.uint32_t, orchardMaxActionGroups)
	nullifiersBuf := make([]byte, orchardMaxActions*32)
	commitmentsBuf := make([]byte, orchardMaxActions*32)

	var bundleType C.uint8_t
	var groupCount C.uint32_t
	var issueBundleLen C.uint32_t

	var issueBundlePtr *C.uint8_t
	var issueBundleOut []byte
	if len(issueBundle) > 0 {
		issueBundlePtr = (*C.uint8_t)(unsafe.Pointer(&issueBundle[0]))
		issueBundleOut = make([]byte, len(issueBundle))
	}

	var orchardPtr *C.uint8_t
	if len(orchardBundle) > 0 {
		orchardPtr = (*C.uint8_t)(unsafe.Pointer(&orchardBundle[0]))
	}

	result := C.orchard_decode_bundle_v6(
		orchardPtr,
		C.uint32_t(len(orchardBundle)),
		issueBundlePtr,
		C.uint32_t(len(issueBundle)),
		(*C.uint8_t)(unsafe.Pointer(&bundleType)),
		(*C.uint32_t)(unsafe.Pointer(&groupSizesBuf[0])),
		C.uint32_t(len(groupSizesBuf)),
		(*C.uint32_t)(unsafe.Pointer(&groupCount)),
		(*C.uint8_t)(unsafe.Pointer(&nullifiersBuf[0])),
		C.uint32_t(len(nullifiersBuf)),
		(*C.uint8_t)(unsafe.Pointer(&commitmentsBuf[0])),
		C.uint32_t(len(commitmentsBuf)),
		func() *C.uint8_t {
			if len(issueBundleOut) == 0 {
				return nil
			}
			return (*C.uint8_t)(unsafe.Pointer(&issueBundleOut[0]))
		}(),
		C.uint32_t(len(issueBundleOut)),
		(*C.uint32_t)(unsafe.Pointer(&issueBundleLen)),
	)
	if result != C.FFI_SUCCESS {
		return nil, ffiResultToError(result)
	}

	groupCountInt := int(groupCount)
	if groupCountInt > len(groupSizesBuf) {
		return nil, ErrInvalidInput
	}

	actionGroupSizes := make([]int, 0, groupCountInt)
	totalActions := 0
	if groupCountInt > 0 {
		for i := 0; i < groupCountInt; i++ {
			size := int(groupSizesBuf[i])
			if size <= 0 {
				return nil, ErrInvalidInput
			}
			actionGroupSizes = append(actionGroupSizes, size)
			totalActions += size
		}
	}

	if totalActions > orchardMaxActions {
		return nil, ErrInvalidInput
	}

	var nullifiers [][32]byte
	var commitments [][32]byte
	if totalActions > 0 {
		nullifiers = make([][32]byte, totalActions)
		commitments = make([][32]byte, totalActions)
		for i := 0; i < totalActions; i++ {
			copy(nullifiers[i][:], nullifiersBuf[i*32:(i+1)*32])
			copy(commitments[i][:], commitmentsBuf[i*32:(i+1)*32])
		}
	}

	var issueBundleBytes []byte
	if issueBundleLen > 0 {
		if int(issueBundleLen) > len(issueBundleOut) {
			return nil, ErrInvalidInput
		}
		issueBundleBytes = append([]byte(nil), issueBundleOut[:issueBundleLen]...)
	}

	if totalActions == 0 && issueBundleLen == 0 {
		return nil, ErrInvalidInput
	}

	return &DecodedBundleV6{
		BundleType:       uint8(bundleType),
		ActionGroupSizes: actionGroupSizes,
		Nullifiers:       nullifiers,
		Commitments:      commitments,
		IssueBundle:      issueBundleBytes,
	}, nil
}

// IssueBundleCommitments computes note commitments from IssueBundle bytes.
func (r *OrchardFFI) IssueBundleCommitments(issueBundle []byte) ([][32]byte, error) {
	if r == nil || len(issueBundle) == 0 {
		return nil, ErrInvalidInput
	}

	const minIssueNoteSize = 115
	maxNotes := len(issueBundle)/minIssueNoteSize + 1
	if maxNotes == 0 {
		return nil, ErrInvalidInput
	}
	commitmentsBuf := make([]byte, maxNotes*32)
	var commitmentsLen C.uint32_t

	result := C.orchard_issue_bundle_commitments(
		(*C.uint8_t)(unsafe.Pointer(&issueBundle[0])),
		C.uint32_t(len(issueBundle)),
		(*C.uint8_t)(unsafe.Pointer(&commitmentsBuf[0])),
		C.uint32_t(len(commitmentsBuf)),
		(*C.uint32_t)(unsafe.Pointer(&commitmentsLen)),
	)
	if result != C.FFI_SUCCESS {
		return nil, ffiResultToError(result)
	}

	count := int(commitmentsLen)
	if count < 0 || count > maxNotes {
		return nil, ErrInvalidInput
	}

	commitments := make([][32]byte, count)
	for i := 0; i < count; i++ {
		copy(commitments[i][:], commitmentsBuf[i*32:(i+1)*32])
	}
	return commitments, nil
}

// GenerateProof creates a real Orchard proof for the single Orchard circuit.
func (r *OrchardFFI) GenerateProof(extrinsicType ExtrinsicType, seed []byte) ([]byte, error) {
	proof, _, err := r.GenerateProofWithPublicInputs(extrinsicType, seed)
	return proof, err
}

// VerifyProof verifies a real Orchard proof with the provided public inputs.
func (r *OrchardFFI) VerifyProof(extrinsicType ExtrinsicType, proof, publicInputs []byte) error {
	if extrinsicType != SubmitPrivate {
		return ErrInvalidInput
	}
	if len(proof) == 0 || len(publicInputs) == 0 || len(publicInputs)%32 != 0 {
		return ErrInvalidInput
	}

	result := C.orchard_verify_halo2_proof(
		C.uint32_t(1),
		(*C.uint8_t)(unsafe.Pointer(&proof[0])),
		C.uint32_t(len(proof)),
		(*C.uint8_t)(unsafe.Pointer(&publicInputs[0])),
		C.uint32_t(len(publicInputs)/32),
	)
	if result != C.FFI_SUCCESS {
		return ffiResultToError(result)
	}
	return nil
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

// RefineWorkPackage runs witness-aware refinement using the Orchard service FFI.
func (r *OrchardFFI) RefineWorkPackage(
	serviceID uint32,
	preStatePayload []byte,
	preWitness []byte,
	postWitness []byte,
	bundleProof []byte,
) error {
	if r == nil {
		return ErrInvalidInput
	}
	if len(preStatePayload) == 0 || len(preWitness) == 0 || len(bundleProof) == 0 {
		return ErrInvalidInput
	}

	var postPtr *C.uint8_t
	if len(postWitness) > 0 {
		postPtr = (*C.uint8_t)(unsafe.Pointer(&postWitness[0]))
	}

	result := C.orchard_refine_witness_aware(
		C.uint32_t(serviceID),
		(*C.uint8_t)(unsafe.Pointer(&preStatePayload[0])),
		C.uint32_t(len(preStatePayload)),
		(*C.uint8_t)(unsafe.Pointer(&preWitness[0])),
		C.uint32_t(len(preWitness)),
		postPtr,
		C.uint32_t(len(postWitness)),
		(*C.uint8_t)(unsafe.Pointer(&bundleProof[0])),
		C.uint32_t(len(bundleProof)),
	)

	return ffiResultToError(result)
}
