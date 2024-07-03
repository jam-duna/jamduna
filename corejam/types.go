package corejam

import (
	//"crypto/ed25519"

	"github.com/ethereum/go-ethereum/common"
)

type Ed25519Signature []byte

// Assurance represents an assurance value.
type Assurance struct {
	ValidatorIndex int              `json:"validator_index"`
	CoreIndex      int              `json:"core_index"`
	Bitstring      []byte           `json:"bitstring"`
	Signature      Ed25519Signature `json:"signature"`
}

const (
	validatorCount = 1023
	coreCount      = 341
	epochLength    = 600
	rotationPeriod = 10
)

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

// AvailabilitySpecification represents the set of availability specifications.
type AvailabilitySpecification struct {
	WorkPackageHash common.Hash `json:"work_package_hash"`
	BundleLength    int         `json:"bundle_length"`
	ErasureRoot     common.Hash `json:"erasure_root"`
	SegmentRoot     common.Hash `json:"segment_root"`
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

// WorkResult represents a work result.
type WorkResult struct {
	ServiceIndex      int         `json:"service_index"`
	CodeHash          common.Hash `json:"code_hash"`
	PayloadHash       common.Hash `json:"payload_hash"`
	GasPrioritization int         `json:"gas_prioritization"`
	Output            WorkOutput  `json:"output"`
}

// WorkOutput represents the output or error of the execution.
type WorkOutput struct {
	Output []byte `json:"o,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

// Error represents the possible errors.
type Error struct {
	ErrorType ErrorType `json:"error_type"`
}

// ErrorType is an enumeration of possible error types.
type ErrorType string

const (
	InfinityError ErrorType = "∞"
	ZeroError     ErrorType = "∅"
	BadError      ErrorType = "BAD"
	BigError      ErrorType = "BIG"
)

// Guarantee represents a single guarantee.
type Guarantee struct {
	CoreIndex  int         `json:"core_index"`
	WorkReport common.Hash `json:"work_report"`
	Credential Credential  `json:"credential"`
	Timeslot   int         `json:"timeslot"`
}

// Credential represents a series of tuples of a signature and a validator index.
type Credential struct {
	Signatures     []Ed25519Signature `json:"signatures"`
	ValidatorIndex int                `json:"validator_index"`
}

// LookupEntry represents a single entry in the lookup extrinsic.
type LookupEntry struct {
	ServiceIndex int    `json:"service_index"`
	Data         []byte `json:"data"`
}

// Operand represents the accumulation operand element.
type Operand struct {
	WorkOutputOrError string      `json:"work_output_or_error"`
	PayloadHash       common.Hash `json:"payload_hash"`
	PackageHash       common.Hash `json:"package_hash"`
	AuthorizationHash common.Hash `json:"authorization_hash"`
}

// WorkReport represents a work report.
type WorkReport struct {
	Operands             []Operand         `json:"operands"`
	AuthorizerHash       common.Hash       `json:"authorizer_hash"`
	Output               string            `json:"output"`
	RefinementContext    RefinementContext `json:"refinement_context"`
	PackageSpecification string            `json:"package_specification"`
	Results              []string          `json:"results"`
	TotalSize            int               `json:"total_size"`
}

// DeferredTransfer represents a deferred transfer.
type DeferredTransfer struct {
	SenderIndex   int      `json:"sender_index"`
	ReceiverIndex int      `json:"receiver_index"`
	Amount        int      `json:"amount"`
	Memo          [64]byte `json:"memo"`
	GasLimit      int      `json:"gas_limit"`
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

// WorkItem represents a work item.
type WorkItem struct {
	ServiceIdentifier int         `json:"service_identifier"`
	CodeHash          common.Hash `json:"code_hash"`
	PayloadBlob       []byte      `json:"payload_blob"`
	GasLimit          int         `json:"gas_limit"`
	Manifest          Manifest    `json:"manifest"`
}

// Manifest represents the manifest elements of a work item.
type Manifest struct {
	SegmentsTreeRoot common.Hash   `json:"segments_tree_root"`
	IndexIntoTree    int           `json:"index_into_tree"`
	DataSegments     []common.Hash `json:"data_segments"`
	ExportedSegments int           `json:"exported_segments"`
}

// WorkPackage represents a work package.
type WorkPackage struct {
	AuthorizationToken []byte      `json:"authorization_token"`
	ServiceIndex       int         `json:"service_index"`
	AuthorizationCode  common.Hash `json:"authorization_code"`
	ParamBlob          []byte      `json:"param_blob"`
	Context            []byte      `json:"context"`
	WorkItems          []WorkItem  `json:"work_items"`
}

// ServiceAccumulation represents a service accumulation result.
type ServiceAccumulation struct {
	ServiceIndex int         `json:"service_index"`
	Result       common.Hash `json:"result"`
}

// LookupExtrinsic represents the lookup extrinsic.
type LookupExtrinsic struct {
	Entries []LookupEntry `json:"entries"`
}

// AssuranceExtrinsic represents the assurances extrinsic.
type AssuranceExtrinsic struct {
	Assurances []Assurance `json:"assurances"`
	ParentHash common.Hash `json:"parent_hash"`
}

// GuaranteesExtrinsic represents the guarantees extrinsic.
type GuaranteesExtrinsic struct {
	Guarantees []Guarantee `json:"guarantees"`
}

// GasAttributable represents the gas attributable for each service.
type GasAttributable struct {
	ServiceIndex int     `json:"service_index"`
	Gas          float64 `json:"gas"`
}

// Judgment represents a single judgment in the extrinsic.
type Judgment struct {
	ReportHash     common.Hash      `json:"report_hash"`
	ValidatorIndex int              `json:"validator_index"`
	Vote           int              `json:"vote"`
	Signature      Ed25519Signature `json:"signature"`
}

// JudgmentExtrinsic represents the extrinsic for judgments.
type JudgmentExtrinsic struct {
	Judgments  []Judgment     `json:"judgments"`
	ParentHash common.Hash    `json:"parent_hash"`
	Header     JudgmentHeader `json:"header"`
}

// JudgmentHeader represents the header of the judgment extrinsic.
type JudgmentHeader struct {
	ReportHashes []common.Hash `json:"report_hashes"`
	Threshold    int           `json:"threshold"`
}

// PastJudgments represents the past judgments on work reports and validators.
type PastJudgments struct {
	AllowSet  []common.Hash `json:"allow_set"`
	BanSet    []common.Hash `json:"ban_set"`
	PunishSet []common.Hash `json:"punish_set"`
}

// JudgmentSets represents the sets of judgments.
type JudgmentSets struct {
	ValidReports   []common.Hash `json:"valid_reports"`
	InvalidReports []common.Hash `json:"invalid_reports"`
	GuarantorKeys  []common.Hash `json:"guarantor_keys"`
}
