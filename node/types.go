package node

import (
	//"crypto/ed25519"

	"github.com/ethereum/go-ethereum/common"
)

type Ed25519Signature []byte
type PublicKey []byte
type BMTProof []common.Hash

const (
	validatorCount = 1023
	coreCount      = 341
	epochLength    = 600
	rotationPeriod = 10

	ExtrinsicSignatureInBytes = 784
)

/*
Section 6.7 - Equation 73.  Ticket Extrinsic is a *sequence* of proofs of valid tickets, each of which is a tuple of an entry index (a natural number less than N) and a proof of ticket validity.
*/
type Ticket struct {
	Attempt   int                             `json:"attempt"`
	Signature [ExtrinsicSignatureInBytes]byte `json:"signature"`
}

/*
11.1.1. Work Report. See Equation 117. A work-report, of the set W, is defined as a tuple of:
* the work-package specification $s$,
* the refinement context $x$,
* the core-index $c$ (i.e. on which the work is done)
* the authorizer hash $a$ and
* output ${\bf o}$ and
* $r$, the results of the evaluation of each of the items in the package r, which is always at least one item and may be no more than I items.
*/
// WorkReport represents a work report.
type WorkReport struct {
	AvailabilitySpec     AvailabilitySpecification `json:"availability"`
	AuthorizerHash       common.Hash               `json:"authorizer_hash"`
	Core                 int                       `json:"total_size"`
	Output               []byte                    `json:"output"`
	RefinementContext    RefinementContext         `json:"refinement_context"`
	PackageSpecification string                    `json:"package_specification"`
	Results              []WorkResult              `json:"results"`
}

// AvailabilitySpecification represents the set of availability specifications.
type AvailabilitySpecification struct {
	WorkPackageHash common.Hash `json:"work_package_hash"`
	BundleLength    int         `json:"bundle_length"`
	// root of a binary Merkle tree which functions as a commitment to all data required for the auditing of the report and for use by later workpackages should they need to retrieve any data yielded
	ErasureRoot common.Hash `json:"erasure_root"`
	// root of a constant-depth, left-biased and zero-hash-padded binary Merkle tree committing to the hashes of each of the exported segments of each work-item
	SegmentRoot common.Hash `json:"segment_root"`
}

/*
11.1.2. Refinement Context. See Eq 119. A refinement context, denoted by the set ${\cal X}$, describes the context of the chain at the point that the report’s corresponding work-package was evaluated. It identifies:
* two historical blocks, the anchor  header hash $a$
* its associated posterior state-root $s$
* posterior Beefy root $b$
* the lookupanchor $l$
* header hash $l$
* timeslot $t$
* the hash of an optional prerequisite work-package p.
*/
// RefinementContext represents the context of the chain at the point of evaluation.
type RefinementContext struct {
	Anchor             common.Hash `json:"anchor"`
	PosteriorStateRoot common.Hash `json:"posterior_state_root"`
	PosteriorBeefyRoot common.Hash `json:"posterior_beefy_root"`
	LookupAnchor       common.Hash `json:"lookup_anchor"`
	HeaderHash         common.Hash `json:"header_hash"`
	Timeslot           int         `json:"timeslot"`
	Prerequisite       common.Hash `json:"prerequisite,omitempty"`
}

// 11.1.4. Work Result. Equation 121. We finally come to define a work result, L, which is the data conduit by which services’ states may be altered through the computation done within a work-package.
type WorkResult struct {
	Service                int         `json:"service_index"`
	CodeHash               common.Hash `json:"code_hash"`
	PayloadHash            common.Hash `json:"payload_hash"`
	GasPrioritizationRatio uint32      `json:"gas_prioritization_ratio"`
	Output                 []byte      `json:"refinement_context"`
	Error                  string      `json:"error"`
}

/*
Section 11.4 - Work Report Guarantees. See Equations 136 - 143. The guarantees extrinsic, ${\bf E}_G$, a *series* of guarantees, at most one for each core, each of which is a tuple of:
* core index
* work-report
* $a$, credential
* $t$, its corresponding timeslot.

The core index of each guarantee must be unique and guarantees must be in ascending order of this.
*/
// Guaranteed Work Report`
type Guarantee struct {
	WorkReport  WorkReport            `json:"work_report"`
	TimeSlot    uint32                `json:"time_slot"`
	Credentials []GuaranteeCredential `json:"credentials"`
}

// Credential represents a series of tuples of a signature and a validator index.
type GuaranteeCredential struct {
	ValidatorIndex uint32           `json:"validator_index"`
	Signature      Ed25519Signature `json:"signature"`
}

// GuaranteesExtrinsic represents the guarantees extrinsic.
type GuaranteesExtrinsic struct {
	Guarantees []Guarantee `json:"guarantees"`
}

/*
11.2.1. The Assurances Extrinsic. ${\bf E}_A$  The assurances extrinsic is a *sequence* of assurance values, at most one per validator. Each assurance:
* is a sequence of binary values (i.e. a bitstring), one per core, together with
* a signature and
* the index of the validator who is assuring

A value of 1 (or ⊺, if interpreted as a Boolean) at any given index implies that the validator assures they are contributing to its availability.  See equations 123-128.

`Assurance` ${\bf E}_A$:
*/
// AssuranceExtrinsic represents the assurances extrinsic.
type AssuranceExtrinsic struct {
	Assurances []Assurance `json:"assurances"`
	ParentHash common.Hash `json:"parent_hash"`
}

// Assurance represents an assurance value.
type Assurance struct {
	// H_p - see Eq 124
	ParentHash common.Hash `json:"parent_hash"`
	// f - 1 means "available"
	Bitstring      []byte           `json:"bitstring"`
	ValidatorIndex int              `json:"validator_index"`
	Signature      Ed25519Signature `json:"signature"`
}

/*
Section 10.2.  The disputes extrinsic, ${\bf E}_D$, may contain one or more verdicts ${\bf v}$.

Dispute` ${\bf E}_D$:
*/
// Disputes represents a one or or more verdicts.
type Disputes struct {
	Verdicts []Verdict `json:"verdicts"`
	Culprits []Culprit `json:"culprits"`
	Faults   []Fault   `json:"faults"`
}

type Verdict struct {
	WorkReportHash common.Hash `json:"report_hash"`
	Epoch          uint32      `json:"epoch"`
}

type Culprit struct {
	R         PublicKey        `json:"r"`
	K         PublicKey        `json:"k"`
	Signature Ed25519Signature `json:"signature"`
}

type Fault struct {
	R         PublicKey        `json:"r"`
	K         PublicKey        `json:"k"`
	V         PublicKey        `json:"v"`
	Signature Ed25519Signature `json:"signature"`
}

type Block struct {
	Header    Header    `json:"h"`
	Extrinsic Extrinsic `json:"e"`
}

type JudgementMarker struct {
}

type SafroleAccumulator struct {
	Id      common.Hash `json:"id"`
	Attempt int         `json:"attempt"`
}

// EpochMark (see 6.4 Epoch change Signal) represents the descriptor for parameters to be used in the next epoch
type EpochMark struct {
	// Randomness accumulator snapshot
	Entropy common.Hash `json:"entropy"`
	// List of authorities scheduled for next epoch -- could be []PublicKey
	Validators []common.Hash `json:"validators"`
}

type Header struct {
	// H_p
	ParentHash common.Hash `json:"parent_hash"`
	// H_r
	PriorStateRoot common.Hash `json:"prior_state_root"`
	// H_x
	ExtrinsicHash common.Hash `json:"prior_state_root"`
	// H_t
	TimeSlot uint32 `json:"timeslot"`
	// H_e
	EpochMark *EpochMark `json:"timeslot"`
	// H_w
	WinningTickets []*SafroleAccumulator `json:"winning_tickets"`
	// H_j
	JudgementsMarkers *JudgementMarker `json:"judgements_markets"`
	// H_k
	BlockAuthorKey uint32 `json:"block_author_key"`
	// H_v
	VRFSignature []byte `json:"vrf_signature"`
	// H_s
	BlockSeal []byte `json:"block_seal"`
}

type Extrinsic struct {
}

/*
Section 12.1. Preimage Integration. Prior to accumulation, we must first integrate all preimages provided in the lookup extrinsic. The lookup extrinsic is a sequence of pairs of service indices and data. These pairs must be ordered and without duplicates (equation 154 requires this). The data must have been solicited by a service but not yet be provided.  See equations 153-155.

`PreimageExtrinsic` ${\bf E}_P$:
*/
type PreimageExtrinsic struct {
	PreimageLookups []PreimageLookup `json:"preimage_lookups"`
}

// LookupEntry represents a single entry in the lookup extrinsic.
type PreimageLookup struct {
	ServiceIndex uint32 `json:"service_index"`
	Data         []byte `json:"data"`
}

const (
	InfinityError = "∞"
	ZeroError     = "∅"
	BadError      = "BAD"
	BigError      = "BIG"
)

/*
14.3. Packages and Items.  A work-package includes: (See Equation 174):
* ${\bf j}$ - a simple blob acting as an authorization token
* $h$ - the index of the service which hosts the authorization code h
* $c$ - an authorization code hash
* ${\bf p}$ - a parameterization blob
* $x$ - context
* ${\bf i}$ - a sequence of work items
*/

// WorkPackage represents a work package.
type WorkPackage struct {
	// $j$ - a simple blob acting as an authorization token
	AuthorizationToken []byte `json:"authorization_token"`
	// $h$ - the index of the service which hosts the authorization code
	ServiceIndex int `json:"service_index"`
	// $c$ - an authorization code hash
	AuthorizationCode common.Hash `json:"authorization_code"`
	// $p$ - a parameterization blob
	ParamBlob []byte `json:"param_blob"`
	// $x$ - context
	Context []byte `json:"context"`
	// $i$ - a sequence of work items

	WorkItems []WorkItem `json:"work_items"`
}

/*
A work item includes: (See Equation 175)
* $s$, the identifier of the service to which it relates
* $c$, the code hash of the service at the time of reporting  (whose preimage must be available from the perspective of the lookup anchor block)
* ${\bf y}$, a payload blob
* $g$, a gas limit and
* the three elements of its manifest:
  - ${\bf i}$, a sequence of imported data segments identified by the root of the segments tree and an index into it;
  - ${\bf x}$, a sequence of hashes of data segments to be introduced in this block (and which we assume the validator knows);
  - $e$, the number of data segments exported by this work item
*/
// WorkItem represents a work item.
type WorkItem struct {
	// s: the identifier of the service to which it relates
	ServiceIdentifier int `json:"service_identifier"`
	// c: the code hash of the service at the time of reporting
	CodeHash common.Hash `json:"code_hash"`
	// y: a payload blob
	PayloadBlob []byte `json:"payload_blob"`
	// g: a gas limit
	GasLimit            int             `json:"gas_limit"`
	ImportedSegments    []ImportSegment `json:"imported_segments"`
	NewDataSegments     []common.Hash   `json:"new_data_segments"`
	NumSegmentsExported uint32          `json:"num_segments_exported"`
}
type ImportSegment struct {
	SegmentRoot  common.Hash `json:"segment_root"`
	SegmentIndex uint32      `json:"segment_index"`
}

/*
Section 11.4.  The guarantees extrinsic, ${\bf E}_G$, a series of guarantees, at most one for each core, each of which is a tuple of:
* a core index
* work-report
* a credential $a$ and
* its corresponding timeslot $t$

The core index of each guarantee must be unique and guarantees must be in ascending order of this.  See Equations 139 - 151.
*/

// Announcement  Section 17.3 Equations (196)-(199) TBD
type Announcement struct {
	Signature Ed25519Signature `json:"signature"`
}

//From Sec 14: Once done, then imported segments must be reconstructed. This process may in fact be lazy as the Refine function makes no usage of the data until the ${\tt import}$ hostcall is made. Fetching generally implies that, for each imported segment, erasure-coded chunks are retrieved from enough unique validators (342, including the guarantor).  Chunks must be fetched for both the data itself and for justification metadata which allows us to ensure that the data is correct.

//Validators, in their role as availability assurers, should index such chunks according to the index of the segmentstree whose reconstruction they facilitate. Since the data for segment chunks is so small at 12 bytes, fixed communications costs should be kept to a bare minimum. A good network protocol (out of scope at present) will allow guarantors to specify only the segments-tree root and index together with a Boolean to indicate whether the proof chunk need be supplied.

// `ImportDAQuery` + `ImportDAResponse` WIP:
type ImportDAQuery struct {
	SegmentRoot    common.Hash `json:"segment_root"`
	SegmentIndex   uint32      `json:"segment_index"`
	ProofRequested bool        `json:"proof_requested"`
}

type ImportDAResponse struct {
	Data  [][]byte `json:"data"`
	Proof BMTProof `json:"proof"`
}

type AuditDAQuery struct {
	SegmentRoot    common.Hash `json:"segment_root"`
	Index          int         `json:"segment_index"`
	ProofRequested bool        `json:"proof_requested"`
}

type AuditDAResponse struct {
	Data  []byte   `json:"data"`
	Proof BMTProof `json:"proof"`
}

type ImportDAReconstructQuery struct {
	SegmentRoot common.Hash `json:"segment_root"`
}

type ImportDAReconstructResponse struct {
	Data []byte `json:"data"`
}

// DeferredTransfer represents a deferred transfer.
type DeferredTransfer struct {
	SenderIndex   int      `json:"sender_index"`
	ReceiverIndex int      `json:"receiver_index"`
	Amount        int      `json:"amount"`
	Memo          [64]byte `json:"memo"`
	GasLimit      int      `json:"gas_limit"`
}

// ServiceAccount represents a service account.
type ServiceAccount struct {
	StorageDict         map[common.Hash]string `json:"storage_dict"`
	PreimageLookupDictP map[common.Hash]string `json:"preimage_lookup_dict_p"`
	PreimageLookupDictL map[common.Hash]int    `json:"preimage_lookup_dict_l"`
	CodeHash            common.Hash            `json:"code_hash"`
	Balance             int                    `json:"balance"`
	GasLimitG           int                    `json:"gas_limit_g"`
	GasLimitM           int                    `json:"gas_limit_m"`
}

// AccumulationState represents the state required for accumulation.
type AccumulationState struct {
	ServiceIndices    []int              `json:"service_indices"`
	WorkReports       []WorkReport       `json:"work_reports"`
	DeferredTransfers []DeferredTransfer `json:"deferred_transfers"`
}

// ValidatorStatistics represents the statistics tracked for each validator.
type ValidatorStatistics struct {
	BlocksProduced         int `json:"blocks_produced"`
	TicketsIntroduced      int `json:"tickets_introduced"`
	PreimagesIntroduced    int `json:"preimages_introduced"`
	OctetsIntroduced       int `json:"octets_introduced"`
	ReportsGuaranteed      int `json:"reports_guaranteed"`
	AvailabilityAssurances int `json:"availability_assurances"`
}

// StatisticalReporter represents the statistics for the current and previous epochs.
type StatisticalReporter struct {
	CurrentEpochStats  []ValidatorStatistics `json:"current_epoch_stats"`
	PreviousEpochStats []ValidatorStatistics `json:"previous_epoch_stats"`
}

// ServiceAccumulation represents a service accumulation result.
type ServiceAccumulation struct {
	ServiceIndex int         `json:"service_index"`
	Result       common.Hash `json:"result"`
}

// GasAttributable represents the gas attributable for each service.
type GasAttributable struct {
	ServiceIndex int     `json:"service_index"`
	Gas          float64 `json:"gas"`
}
