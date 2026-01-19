package statedb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path"
	"reflect"
	"time"

	bls "github.com/colorfulnotion/jam/bls"
	evmtypes "github.com/colorfulnotion/jam/builder/evm/types"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm/interpreter"
	"github.com/colorfulnotion/jam/pvm/program"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/trie"
	types "github.com/colorfulnotion/jam/types"
)

const (
	debugSpec = false
)

// ExtractContractWitnessBlob extracts the contract witness payload from refine output
// Returns the raw contract witness blob (balance/nonce/code/storage writes)
// Exported for Phase 1 execution in builder network.
func (s *StateDB) ExtractContractWitnessBlob(output []byte, segments [][]byte) ([]byte, error) {
	if len(output) < 2 {
		return []byte{}, nil
	}

	// Parse metashard count
	metashardCount := binary.LittleEndian.Uint16(output[0:2])

	if metashardCount > 0 {
		// Skip metashard entries to find contract witness metadata
		offset := 2
		for i := uint16(0); i < metashardCount; i++ {
			if offset >= len(output) {
				return nil, fmt.Errorf("truncated metashard data")
			}
			ld := output[offset]
			offset += 1
			prefixBytes := ((ld + 7) / 8)
			entrySize := int(prefixBytes) + 5
			if offset+entrySize > len(output) {
				return nil, fmt.Errorf("truncated metashard entry")
			}
			offset += entrySize
		}

		// Contract witness metadata immediately follows metashard entries:
		// [2B index_start][4B payload_length]
		if offset+6 <= len(output) {
			indexStart := binary.LittleEndian.Uint16(output[offset : offset+2])
			payloadLength := binary.LittleEndian.Uint32(output[offset+2 : offset+6])

			if payloadLength > 0 {
				return s.readPayloadFromSegments(segments, uint32(indexStart), payloadLength)
			}
		}
		return []byte{}, nil
	}

	// New format with metashardCount=0: [00, 00][2B contract_index_start][4B contract_payload_length]
	if len(output) >= 8 {
		indexStart := binary.LittleEndian.Uint16(output[2:4])
		payloadLength := binary.LittleEndian.Uint32(output[4:8])

		if payloadLength > 0 {
			return s.readPayloadFromSegments(segments, uint32(indexStart), payloadLength)
		}
	}

	return []byte{}, nil
}

// SEGMENT_SIZE matches the Rust constant for DA segment size
const SEGMENT_SIZE = 4096

// ExtractReceiptsFromRefineOutput extracts transaction receipts from the refine output.
//
// Refine output format:
// 1. Section 1: Meta-shards [2B count][entries...]
// 2. Section 2: Contract witness metadata [2B index_start][4B payload_length]
// 3. Section 3: Block receipts metadata [1B has_receipts][if 1: 2B index_start][4B payload_length]
// 4. AccumulateInstructions (appended in main.rs)
//
// Algorithm:
// 1. Skip Section 1 (meta-shards)
// 2. Skip Section 2 (contract witness)
// 3. Parse Section 3 to get block receipts location
// 4. Read receipts blob from segments and parse
//
// Exported for builder network to capture receipts without relying on DA.
func (s *StateDB) ExtractReceiptsFromRefineOutput(output []byte, segments [][]byte) ([]*evmtypes.TransactionReceipt, error) {
	if len(output) < 2 {
		return nil, nil
	}

	offset := 0

	// Section 1: Skip meta-shard entries
	// Format: [2B count][N entries × (1B ld + prefix_bytes + 5B packed)]
	metashardCount := binary.LittleEndian.Uint16(output[offset : offset+2])
	offset += 2

	log.Debug(log.EVM, "ExtractReceiptsFromRefineOutput: parsing",
		"metashardCount", metashardCount,
		"outputLen", len(output),
		"segmentsCount", len(segments))

	// Skip each meta-shard entry
	for i := uint16(0); i < metashardCount; i++ {
		if offset >= len(output) {
			return nil, fmt.Errorf("truncated metashard data at entry %d", i)
		}

		// Parse ld to know prefix size
		ld := output[offset]
		offset += 1

		prefixBytes := int((ld + 7) / 8)
		if offset+prefixBytes+5 > len(output) {
			return nil, fmt.Errorf("truncated metashard entry %d", i)
		}

		// Skip prefix + packed ObjectRef
		offset += prefixBytes + 5
	}

	// Section 2: Skip contract witness metadata
	// Format: [2B index_start][4B payload_length]
	if offset+6 > len(output) {
		return nil, fmt.Errorf("truncated contract witness metadata")
	}
	offset += 6 // Skip 2 + 4 bytes

	// Section 3: Block receipts metadata
	// Format: [1B has_receipts][if 1: 2B index_start][4B payload_length]
	if offset >= len(output) {
		return nil, fmt.Errorf("missing receipts section")
	}

	hasReceipts := output[offset]
	offset += 1

	if hasReceipts == 0 {
		log.Debug(log.EVM, "ExtractReceiptsFromRefineOutput: no receipts in this block")
		return nil, nil
	}

	if offset+6 > len(output) {
		return nil, fmt.Errorf("truncated receipts metadata")
	}

	receiptsIndexStart := binary.LittleEndian.Uint16(output[offset : offset+2])
	offset += 2

	receiptsPayloadLength := binary.LittleEndian.Uint32(output[offset : offset+4])
	offset += 4

	log.Info(log.EVM, "ExtractReceiptsFromRefineOutput: found receipts section",
		"indexStart", receiptsIndexStart,
		"payloadLength", receiptsPayloadLength)

	if receiptsPayloadLength == 0 {
		return nil, nil
	}

	// Read receipts blob from segments
	receiptsBlob, err := s.readPayloadFromSegments(segments, uint32(receiptsIndexStart), receiptsPayloadLength)
	if err != nil {
		return nil, fmt.Errorf("failed to read receipts blob from segments: %v", err)
	}

	// Parse block receipts blob
	// Format: [4B count][receipt_len:4][receipt_data]...
	receipts, err := evmtypes.ParseBlockReceiptsBlob(receiptsBlob)
	if err != nil {
		return nil, fmt.Errorf("failed to parse block receipts blob: %v", err)
	}

	// Sort receipts by TransactionIndex to ensure correct ordering
	sortReceiptsByTxIndex(receipts)

	log.Info(log.EVM, "ExtractReceiptsFromRefineOutput: complete",
		"totalReceipts", len(receipts))

	return receipts, nil
}

// sortReceiptsByTxIndex sorts receipts slice by TransactionIndex in ascending order.
// Also sorts logs within each receipt by LogIndex.
func sortReceiptsByTxIndex(receipts []*evmtypes.TransactionReceipt) {
	// Sort receipts by TransactionIndex
	for i := 0; i < len(receipts)-1; i++ {
		for j := i + 1; j < len(receipts); j++ {
			if receipts[j].TransactionIndex < receipts[i].TransactionIndex {
				receipts[i], receipts[j] = receipts[j], receipts[i]
			}
		}
	}

	// Sort logs within each receipt by LogIndex
	for _, receipt := range receipts {
		if len(receipt.Logs) > 1 {
			sortLogsByIndex(receipt.Logs)
		}
	}
}

// sortLogsByIndex sorts logs slice by LogIndex in ascending order
func sortLogsByIndex(logs []evmtypes.Log) {
	for i := 0; i < len(logs)-1; i++ {
		for j := i + 1; j < len(logs); j++ {
			if logs[j].LogIndex < logs[i].LogIndex {
				logs[i], logs[j] = logs[j], logs[i]
			}
		}
	}
}

// calculatePayloadLength computes payload length from segment count and last segment size
func calculatePayloadLength(numSegments, lastSegmentSize uint16) uint32 {
	if numSegments == 0 {
		return 0
	}
	if numSegments == 1 {
		return uint32(lastSegmentSize)
	}
	return uint32(numSegments-1)*SEGMENT_SIZE + uint32(lastSegmentSize)
}

// extractReceiptsFromMetaShard parses a meta-shard payload and extracts receipt payloads.
//
// Meta-shard format (with header): [ld:1][prefix56:7][merkle_root:32][count:2][ObjectRefEntry...]
// ObjectRefEntry format: [object_id:32][ObjectRef:37] = 69 bytes each
// ObjectRef format: [work_package_hash:32][packed:5]
func (s *StateDB) extractReceiptsFromMetaShard(payload []byte, segments [][]byte) ([]*evmtypes.TransactionReceipt, error) {
	// Meta-shard with header: [ld:1][prefix56:7][merkle_root:32][count:2][entries...]
	// Minimum size: 1 + 7 + 32 + 2 = 42 bytes
	if len(payload) < 42 {
		return nil, fmt.Errorf("meta-shard payload too short: %d bytes", len(payload))
	}

	// Skip header: ld (1) + prefix56 (7) = 8 bytes
	dataOffset := 8

	// Skip merkle_root (32 bytes)
	dataOffset += 32

	// Read entry count
	entryCount := binary.LittleEndian.Uint16(payload[dataOffset : dataOffset+2])
	dataOffset += 2

	log.Debug(log.EVM, "extractReceiptsFromMetaShard: parsing",
		"payloadLen", len(payload),
		"entryCount", entryCount)

	var receipts []*evmtypes.TransactionReceipt
	const objectRefEntrySize = 69 // 32 bytes object_id + 37 bytes ObjectRef

	for j := uint16(0); j < entryCount; j++ {
		if dataOffset+objectRefEntrySize > len(payload) {
			return receipts, fmt.Errorf("truncated ObjectRefEntry at index %d", j)
		}

		// Parse ObjectRefEntry: [object_id:32][ObjectRef:37]
		// objectID := payload[dataOffset : dataOffset+32]
		dataOffset += 32

		// Parse ObjectRef: [work_package_hash:32][packed:5]
		// workPackageHash := payload[dataOffset : dataOffset+32]
		dataOffset += 32

		// Parse packed (5 bytes, big-endian):
		// index_start (12) | index_end (12) | last_segment_size (12) | object_kind (4)
		packedBytes := payload[dataOffset : dataOffset+5]
		dataOffset += 5

		packed := uint64(packedBytes[0])<<32 | uint64(packedBytes[1])<<24 |
			uint64(packedBytes[2])<<16 | uint64(packedBytes[3])<<8 | uint64(packedBytes[4])

		entryIndexStart := uint16((packed >> 28) & 0xFFF)
		entryIndexEnd := uint16((packed >> 16) & 0xFFF)
		entryLastSegSize := uint16((packed >> 4) & 0xFFF)
		entryKind := evmtypes.ObjectKind(packed & 0x0F)

		// Only process Receipt entries (kind=3)
		if entryKind != evmtypes.ObjectKindReceipt {
			continue
		}

		// Calculate receipt payload length
		entryNumSegments := uint16(0)
		if entryIndexEnd > entryIndexStart {
			entryNumSegments = entryIndexEnd - entryIndexStart
		}
		if entryNumSegments > 0 && entryLastSegSize == 0 {
			entryLastSegSize = SEGMENT_SIZE
		}
		receiptPayloadLen := calculatePayloadLength(entryNumSegments, entryLastSegSize)

		if receiptPayloadLen == 0 {
			continue
		}

		// Read the receipt payload from segments
		receiptPayload, err := s.readPayloadFromSegments(segments, uint32(entryIndexStart), receiptPayloadLen)
		if err != nil {
			log.Debug(log.EVM, "Failed to read receipt payload from segments",
				"entryIndex", j, "indexStart", entryIndexStart, "length", receiptPayloadLen, "err", err)
			continue
		}

		// Parse the receipt
		receipt, err := parseReceiptPayload(receiptPayload, uint32(j))
		if err != nil {
			log.Debug(log.EVM, "Failed to parse receipt payload",
				"entryIndex", j, "err", err)
			continue
		}

		receipts = append(receipts, receipt)
		log.Debug(log.EVM, "Extracted receipt from meta-shard",
			"txHash", receipt.TransactionHash.Hex(),
			"success", receipt.Success,
			"gasUsed", receipt.UsedGas)
	}

	return receipts, nil
}

// parseReceiptPayload parses a raw receipt payload into TransactionReceipt.
// Receipt format (from Rust services/evm/src/receipt.rs):
// [logs_payload_len:4][logs_payload:variable][tx_hash:32][tx_type:1][success:1][used_gas:8]
// [cumulative_gas:4][log_index_start:4][tx_index:4][tx_payload_len:4][tx_payload:variable]
func parseReceiptPayload(data []byte, defaultTxIndex uint32) (*evmtypes.TransactionReceipt, error) {
	minSize := 4 + 32 + 1 + 1 + 8 + 4 + 4 + 4 + 4 // logs_len + hash + type + success + gas + cumulative + log_idx + tx_idx + payload_len
	if len(data) < minSize {
		return nil, fmt.Errorf("receipt payload too short: %d bytes, need at least %d", len(data), minSize)
	}

	offset := 0

	// Logs payload
	logsLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4
	if len(data) < offset+int(logsLen) {
		return nil, fmt.Errorf("insufficient data for logs")
	}
	logsData := make([]byte, logsLen)
	copy(logsData, data[offset:offset+int(logsLen)])
	offset += int(logsLen)

	// Transaction hash
	var txHash common.Hash
	copy(txHash[:], data[offset:offset+32])
	offset += 32

	// Transaction type
	txType := data[offset]
	offset += 1

	// Success flag
	success := data[offset] == 1
	offset += 1

	// Used gas
	usedGas := binary.LittleEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Cumulative gas
	cumulativeGas := uint64(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// Log index start
	logIndexStart := uint64(binary.LittleEndian.Uint32(data[offset : offset+4]))
	offset += 4

	// Transaction index
	txIndex := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	// Transaction payload
	if len(data) < offset+4 {
		return nil, fmt.Errorf("insufficient data for payload length")
	}
	payloadLen := binary.LittleEndian.Uint32(data[offset : offset+4])
	offset += 4

	var payload []byte
	if len(data) >= offset+int(payloadLen) {
		payload = make([]byte, payloadLen)
		copy(payload, data[offset:offset+int(payloadLen)])
	}

	return &evmtypes.TransactionReceipt{
		TransactionHash:  txHash,
		Type:             txType,
		Version:          0,
		Success:          success,
		UsedGas:          usedGas,
		Payload:          payload,
		LogsData:         logsData,
		CumulativeGas:    cumulativeGas,
		LogIndexStart:    logIndexStart,
		TransactionIndex: txIndex,
	}, nil
}

type builderPostStateProof struct {
	postRoot   [32]byte
	writeKeys  [][32]byte
	postValues [][32]byte
	postProof  []byte
}

func parseBuilderPostStateSection(postState []byte) (*builderPostStateProof, error) {
	if len(postState) < 36 {
		return nil, fmt.Errorf("post-state section too short: %d bytes", len(postState))
	}

	offset := 0
	var proof builderPostStateProof

	copy(proof.postRoot[:], postState[offset:offset+32])
	offset += 32

	writeCount := binary.BigEndian.Uint32(postState[offset : offset+4])
	offset += 4

	entryBytes := int(writeCount) * 161
	if len(postState) < offset+entryBytes+4 {
		return nil, fmt.Errorf("post-state section truncated: writeCount=%d", writeCount)
	}

	proof.writeKeys = make([][32]byte, writeCount)
	proof.postValues = make([][32]byte, writeCount)

	for i := 0; i < int(writeCount); i++ {
		// 32B tree key
		copy(proof.writeKeys[i][:], postState[offset:offset+32])
		offset += 32

		// Skip metadata and pre-value: 1B key_type + 20B address + 8B extra + 32B storage_key + 32B pre_value
		offset += 1 + 20 + 8 + 32 + 32

		// 32B PostValue
		copy(proof.postValues[i][:], postState[offset:offset+32])
		offset += 32

		// Skip tx_index (4 bytes)
		offset += 4
	}

	proofLen := binary.BigEndian.Uint32(postState[offset : offset+4])
	offset += 4

	if len(postState) < offset+int(proofLen) {
		return nil, fmt.Errorf("post-state proof truncated: need %d bytes, have %d", proofLen, len(postState)-offset)
	}

	proof.postProof = postState[offset : offset+int(proofLen)]
	return &proof, nil
}

func (s *StateDB) verifyPostStateAgainstExecution(workItem types.WorkItem, extrinsicData [][]byte, contractWitnessBlob []byte) error {
	if len(extrinsicData) < 2 || len(workItem.Extrinsics) < 2 {
		return fmt.Errorf("missing extrinsic data for post-state verification")
	}

	postWitness := extrinsicData[1]
	if uint32(len(postWitness)) != workItem.Extrinsics[1].Len {
		return fmt.Errorf("post witness length mismatch: expected %d, got %d", workItem.Extrinsics[1].Len, len(postWitness))
	}

	stateStorage, ok := s.sdb.(*storage.StateDBStorage)
	if !ok {
		return fmt.Errorf("unexpected storage type %T (need StateDBStorage)", s.sdb)
	}

	// Use GetActiveTree() to get the correct tree for parallel bundle building
	// This returns the active snapshot if set, otherwise CurrentUBT
	tree := stateStorage.GetActiveTree()
	if tree == nil {
		return fmt.Errorf("no UBT tree available for post-state verification")
	}

	builderProof, err := parseBuilderPostStateSection(postWitness)
	if err != nil {
		return fmt.Errorf("failed to parse builder post-state section: %w", err)
	}

	guarantorWrites, err := storage.ExtractUBTWriteMapFromContractWitness(tree, contractWitnessBlob)
	if err != nil {
		return fmt.Errorf("failed to derive guarantor write set: %w", err)
	}

	if err := storage.CompareWriteKeySets(builderProof.writeKeys, guarantorWrites); err != nil {
		return err
	}

	// Non-fatal: surface value mismatches for observability.
	for i, key := range builderProof.writeKeys {
		builderVal := builderProof.postValues[i]
		if guarantorVal, ok := guarantorWrites[key]; ok {
			if !bytes.Equal(builderVal[:], guarantorVal[:]) {
				log.Warn(log.EVM, "verifyPostStateAgainstExecution: value mismatch (non-fatal)",
					"key", fmt.Sprintf("%x", key[:]),
					"builder", fmt.Sprintf("%x", builderVal[:]),
					"guarantor", fmt.Sprintf("%x", guarantorVal[:]))
			}
		}
	}

	log.Trace(log.EVM, "Post-state key-set cross-check passed",
		"writeKeys", len(builderProof.writeKeys),
		"postRoot", fmt.Sprintf("%x", builderProof.postRoot[:8]))

	return nil
}

// populateWitnessCacheFromRefineOutput parses the refine output and loads storage shards into witness cache
// New format: [2B metashard_count][metashard entries...][2B contract_index_start][4B contract_payload_length]
// Old format: [2B count] + count × [32B object_id + 2B index_start + 4B payload_length + 1B object_kind]
func (s *StateDB) populateWitnessCacheFromRefineOutput(serviceID uint32, output []byte, segments [][]byte) error {
	if len(output) < 2 {
		// Empty output or too small - skip
		return nil
	}

	// Parse metashard count
	metashardCount := binary.LittleEndian.Uint16(output[0:2])
	// DIAGNOSTIC: Log metashardCount for debugging receipt lookup
	log.Warn(log.EVM, "📊 REFINE_OUTPUT_PARSED",
		"serviceID", serviceID,
		"metashardCount", metashardCount,
		"segments", len(segments),
		"outputLen", len(output),
	)

	if metashardCount > 0 {
		// Handle metashard entries in the new format
		log.Trace(log.EVM, "Processing metashard entries", "count", metashardCount)

		// Skip metashard entries for now - they're handled by the accumulate process
		// The metashard entries are variable length, so we need to parse them to find where contract witness data starts
		offset := 2 // Start after metashard count

		for i := uint16(0); i < metashardCount; i++ {
			if offset >= len(output) {
				return fmt.Errorf("truncated metashard data at entry %d", i)
			}

			// Each metashard entry starts with 'ld' byte
			ld := output[offset]
			offset += 1

			// Calculate entry size: prefix + 5-byte packed ObjectRef
			prefixBytes := ((ld + 7) / 8)
			entrySize := int(prefixBytes) + 5

			if offset+entrySize > len(output) {
				return fmt.Errorf("truncated metashard entry %d", i)
			}

			offset += entrySize
			log.Debug(log.EVM, "Skipped metashard entry", "i", i, "ld", ld, "entrySize", entrySize)
		}

		// After metashard entries, check if there's contract witness data
		if offset+6 <= len(output) {
			// Contract witness metadata immediately follows the metashard entries:
			// [2B index_start][4B payload_length]
			indexStart := binary.LittleEndian.Uint16(output[offset : offset+2])
			payloadLength := binary.LittleEndian.Uint32(output[offset+2 : offset+6])

			if payloadLength > 0 {
				// Read and load contract witness data
				payload, err := s.readPayloadFromSegments(segments, uint32(indexStart), payloadLength)
				if err != nil {
					return fmt.Errorf("failed to read contract witness payload: %v", err)
				}
				return s.parseAndLoadContractStorage(serviceID, payload)
			}
		}

		return nil
	}

	// New format with metashardCount=0: [00, 00][2B contract_index_start][4B contract_payload_length]
	if len(output) >= 8 {
		return s.loadContractWitnessFromNewFormat(serviceID, output, segments)
	}

	return nil
}

// loadContractWitnessFromNewFormat handles the new contract witness format
func (s *StateDB) loadContractWitnessFromNewFormat(serviceID uint32, output []byte, segments [][]byte) error {
	// Parse contract witness metadata from bytes 2-8
	indexStart := binary.LittleEndian.Uint16(output[2:4])
	payloadLength := binary.LittleEndian.Uint32(output[4:8])

	log.Info(log.EVM, "loadContractWitnessFromNewFormat", "indexStart", indexStart, "payloadLength", payloadLength)

	if payloadLength == 0 {
		log.Info(log.EVM, "No contract witness data to load")
		return nil
	}

	// Read the contract witness payload from segments
	payload, err := s.readPayloadFromSegments(segments, uint32(indexStart), payloadLength)
	if err != nil {
		return fmt.Errorf("failed to read contract witness payload: %v", err)
	}

	// The payload contains serialized contract storage data
	// Parse and load into witness cache
	return s.parseAndLoadContractStorage(serviceID, payload)
}

// parseAndLoadContractStorage parses contract witness blob and loads into witness cache
// The payload contains multiple entries: [20B address][1B kind][4B payload_length][4B tx_index][...payload...]
func (s *StateDB) parseAndLoadContractStorage(serviceID uint32, payload []byte) error {
	log.Trace(log.EVM, "parseAndLoadContractStorage", "serviceID", serviceID, "payloadLen", len(payload))

	evmStorage := s.sdb.(types.EVMJAMStorage)
	evmStorage.InitWitnessCache()

	offset := 0
	for offset < len(payload) {
		// Header: 20B address + 1B kind + 4B payload_len + 4B tx_index
		if len(payload)-offset < 29 {
			return fmt.Errorf("contract witness payload too short at offset %d: %d bytes remaining", offset, len(payload)-offset)
		}

		address := common.BytesToAddress(payload[offset : offset+20])
		kind := payload[offset+20]
		payloadLength := binary.LittleEndian.Uint32(payload[offset+21 : offset+25])
		// Skip tx_index (offset 25-29)

		remaining := len(payload) - offset - 29

		if remaining < int(payloadLength) {
			return fmt.Errorf("insufficient payload data at offset %d: need %d, have %d", offset, payloadLength, remaining)
		}

		entryData := payload[offset+29 : offset+29+int(payloadLength)]
		offset += 29 + int(payloadLength)

		log.Trace(log.EVM, "parseAndLoadContractStorage parsed entry", "address", address.Hex(), "kind", kind, "payloadLength", payloadLength)

		// Only load storage shards (kind=1) into witness cache
		// Other kinds (balance=2, nonce=6, code=0) are handled during witness generation
		if kind == 1 {
			// Parse the contract shard data
			contractStorage, err := evmtypes.DeserializeContractShard(entryData)
			if err != nil {
				return fmt.Errorf("failed to deserialize contract shard: %v", err)
			}

			// Compute UBT root for the contract storage
			ubtRoot, err := computeUBTRoot(address, contractStorage)
			if err != nil {
				log.Warn(log.EVM, "Failed to compute UBT root", "address", address.Hex(), "error", err)
				// Continue without root - this is not a fatal error
			} else {
				log.Trace(log.EVM, "Computed UBT root", "address", address.Hex(), "ubtRoot", ubtRoot.Hex())
			}

			// Load into witness cache
			evmStorage.SetContractStorage(address, evmtypes.ContractStorage{Shard: *contractStorage})

			log.Trace(log.EVM, "parseAndLoadContractStorage complete", "address", address.Hex(), "entries", len(contractStorage.Entries), "ubtRoot", ubtRoot.Hex())
		}
	}

	return nil
}

// readPayloadFromSegments reads a payload from the segments array given index_start and payload_length
func (s *StateDB) readPayloadFromSegments(segments [][]byte, indexStart uint32, payloadLength uint32) ([]byte, error) {
	if payloadLength == 0 {
		return []byte{}, nil
	}

	segmentSize := uint32(types.SegmentSize)
	numSegments := (payloadLength + segmentSize - 1) / segmentSize

	payload := make([]byte, 0, payloadLength)

	for i := uint32(0); i < numSegments; i++ {
		segmentIndex := indexStart + i
		if int(segmentIndex) >= len(segments) {
			return nil, fmt.Errorf("segment index %d out of range (have %d segments)", segmentIndex, len(segments))
		}

		segment := segments[segmentIndex]
		bytesToRead := segmentSize
		if i == numSegments-1 {
			// Last segment - only read remaining bytes
			bytesToRead = payloadLength - (i * segmentSize)
		}

		if uint32(len(segment)) < bytesToRead {
			return nil, fmt.Errorf("segment %d too small: have %d bytes, need %d", segmentIndex, len(segment), bytesToRead)
		}

		payload = append(payload, segment[0:bytesToRead]...)
	}

	return payload, nil
}

// buildSSRKey returns SSR storage key
func buildSSRKey() []byte {
	return []byte("SSR")
}

// pvmlog decodes a 369-byte binary execution log into a human-readable string
// Log format: 1 byte opcode + 4 bytes prevpc + 8 bytes gas + 104 bytes registers + 256 bytes mem_op
func pvmlog(logBytes []byte) string {
	if len(logBytes) != 369 {
		return fmt.Sprintf("Invalid log length: %d (expected 369)", len(logBytes))
	}

	offset := 0

	// 1 byte: opcode
	opcode := logBytes[offset]
	offset++

	// 4 bytes: previous PC (little-endian uint32)
	prevpc := uint32(logBytes[offset]) |
		uint32(logBytes[offset+1])<<8 |
		uint32(logBytes[offset+2])<<16 |
		uint32(logBytes[offset+3])<<24
	offset += 4

	// 8 bytes: gas (little-endian int64)
	gas := uint64(logBytes[offset]) |
		uint64(logBytes[offset+1])<<8 |
		uint64(logBytes[offset+2])<<16 |
		uint64(logBytes[offset+3])<<24 |
		uint64(logBytes[offset+4])<<32 |
		uint64(logBytes[offset+5])<<40 |
		uint64(logBytes[offset+6])<<48 |
		uint64(logBytes[offset+7])<<56
	offset += 8

	// 13*8 bytes: registers (little-endian uint64)
	registers := make([]uint64, 13)
	for i := 0; i < 13; i++ {
		registers[i] = uint64(logBytes[offset]) |
			uint64(logBytes[offset+1])<<8 |
			uint64(logBytes[offset+2])<<16 |
			uint64(logBytes[offset+3])<<24 |
			uint64(logBytes[offset+4])<<32 |
			uint64(logBytes[offset+5])<<40 |
			uint64(logBytes[offset+6])<<48 |
			uint64(logBytes[offset+7])<<56
		offset += 8
	}

	// Format output similar to C printf format
	regStr := fmt.Sprintf("[%d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d]",
		registers[0], registers[1], registers[2], registers[3],
		registers[4], registers[5], registers[6], registers[7],
		registers[8], registers[9], registers[10], registers[11], registers[12])

	return fmt.Sprintf("%s PC:%d Gas:%d Registers:%s", program.OpcodeToString(opcode), prevpc, int64(gas), regStr)
}

// authorizeWP executes the authorization step for a work package
func (statedb *StateDB) authorizeWP(workPackage types.WorkPackage, workPackageCoreIndex uint16, pvmBackend string, logDir string) (r types.Result, p_a common.Hash, authGasUsed uint64, err error) {
	log.Trace(log.Node, "authorizeWP", "NODE", statedb.Id, "workPackage", workPackage.Hash(), "workPackageCoreIndex", workPackageCoreIndex)
	authcode, _, authindex, err := statedb.GetAuthorizeCode(workPackage)
	if err != nil {
		return
	}

	vm_auth := NewVMFromCode(authindex, authcode, 0, 0, statedb, pvmBackend, types.IsAuthorizedGasAllocation)
	if vm_auth == nil {
		err = fmt.Errorf("authorizeWP: failed to create VM for authorization (corrupted bytecode?)")
		return
	}

	r = vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex, logDir)
	p_u := workPackage.AuthorizationCodeHash
	p_p := workPackage.ConfigurationBlob
	p_a = common.Blake2Hash(append(p_u.Bytes(), p_p...))
	authGasUsed = uint64(int64(types.IsAuthorizedGasAllocation) - vm_auth.SafeGetGas())

	return
}

// NOTE: the refinecontext is NOT used here
func (s *StateDB) ExecuteWorkPackageBundle(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32, execContext string, eventID uint64, pvmBackend string, logDir string) (work_report types.WorkReport, err error) {
	importsegments := make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
	results := []types.WorkDigest{}

	SaveToLogDirOnce(logDir, fmt.Sprintf("%v_bundle.bin", package_bundle.WorkPackage.Hash()), package_bundle.Bytes())
	SaveToLogDirOnce(logDir, fmt.Sprintf("%v_bundle.json", package_bundle.WorkPackage.Hash()), []byte(types.ToJSON(package_bundle)))
	stateSnapShotRaw := s.GetStateSnapshotRaw()
	SaveToLogDirOnce(logDir, fmt.Sprintf("%v_statesnapstot.json", package_bundle.WorkPackage.Hash()), []byte(stateSnapShotRaw.String()))
	workPackage := package_bundle.WorkPackage

	// Import Segments
	copy(importsegments, package_bundle.ImportSegmentData)

	// Authorization
	authStart := time.Now()
	// Only build logDir path if logging is enabled (logDir non-empty)
	authLogDir := ""
	if logDir != "" {
		authLogDir = path.Join(logDir, "auth")
	}
	r, p_a, authGasUsed, err := s.authorizeWP(workPackage, workPackageCoreIndex, pvmBackend, authLogDir)
	authElapsed := time.Since(authStart)
	// Check for underflow (gas wrapped around to very large number)
	if authGasUsed > (1 << 63) {
		authGasUsed = 0
	}
	telemetryClient := s.GetStorage().GetTelemetryClient()
	// Telemetry: Authorized (event 93)
	if telemetryClient != nil {
		authCost := telemetry.IsAuthorizedCost{
			TotalGasUsed: authGasUsed,
			TotalTimeNs:  uint64(authElapsed.Nanoseconds()),
		}
		telemetryClient.Authorized(eventID, authCost)
	}

	if err != nil {
		return work_report, err
	}
	var segments [][]byte
	refineCosts := make([]telemetry.RefineCost, 0, len(workPackage.WorkItems))
	vmLogging := "unknown"

	for index, workItem := range workPackage.WorkItems {
		// map workItem.ImportedSegments into segment
		service_index := workItem.Service
		compileStart := time.Now()
		code, ok, err0 := s.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			return work_report, fmt.Errorf("executeWorkPackageBundle(ReadServicePreimageBlob):s_id %v, codehash %v, err %v, ok=%v", service_index, workItem.CodeHash, err0, ok)
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			log.Crit(log.Node, "executeWorkPackageBundle: Code and CodeHash Mismatch")
		}
		vm := NewVMFromCode(service_index, code, 0, 0, s, pvmBackend, workItem.RefineGasLimit)
		if vm == nil {
			return work_report, fmt.Errorf("executeWorkPackageBundle: failed to create VM for service %d (corrupted bytecode?)", service_index)
		}
		// if index == 0 {
		// 	vm.attachFrameServer("0.0.0.0:8080", "./index.html")
		// }
		vm.Timeslot = s.JamState.SafroleState.Timeslot

		vm.SetPVMContext(execContext)
		vmLogging = vm.GetVMLogging()

		// 0.7.1 : core index is part of refine args
		execStart := time.Now()
		vm.hostenv = s
		// Only build logDir path if logging is enabled (logDir non-empty)
		refineLogDir := ""
		if logDir != "" {
			refineLogDir = path.Join(logDir, fmt.Sprintf("%d_%d", index, workItem.Service))
		}
		output, _, exported_segments := vm.ExecuteRefine(workPackageCoreIndex, uint32(index), workPackage, r, importsegments, workItem.ExportCount, package_bundle.ExtrinsicData[index], workPackage.AuthorizationCodeHash, common.BytesToHash(trie.H0), refineLogDir)

		segments = append(segments, exported_segments...)

		execElapsed := time.Since(execStart)
		compileElapsed := time.Since(compileStart)
		if pvmBackend == BackendCompiler {
			fmt.Printf("*** %s compile and execute time %v\n", pvmBackend, compileElapsed)
		}

		// Segments already appended earlier (before parsing refine output)
		z := 0
		for _, extrinsic := range workItem.Extrinsics {
			z += int(extrinsic.Len)
		}
		// Get gas safely before building result struct
		var gasRemaining int64
		if vm != nil {
			gasRemaining = vm.SafeGetGas()
		}
		result := types.WorkDigest{
			ServiceID:           workItem.Service,
			CodeHash:            workItem.CodeHash,
			PayloadHash:         common.Blake2Hash(workItem.Payload),
			Gas:                 workItem.AccumulateGasLimit,
			GasUsed:             uint(workItem.RefineGasLimit - uint64(gasRemaining)),
			NumImportedSegments: uint(len(workItem.ImportedSegments)),
			NumExportedSegments: uint(len(exported_segments)),
			NumExtrinsics: uint(func() int {
				total := 0
				for _, extrinsicBlobs := range package_bundle.ExtrinsicData {
					total += len(extrinsicBlobs)
				}
				return total
			}()),
			NumBytesExtrinsics: uint(z),
		}
		if len(output.Ok)+z > types.MaxEncodedWorkReportSize {
			result.Result.Err = types.WORKDIGEST_OVERSIZE
			result.Result.Ok = nil

			// TODO: renable with BuildBundle witness support
			// } else if segmentCountMismatch {
			// 	// Only first guarantor/auditor flags BAD_EXPORT for mismatched segment counts
			// 	result.Result.Err = types.WORKDIGEST_BAD_EXPORT
			// 	result.Result.Ok = nil
		} else {
			result.Result = output
		}
		results = append(results, result)

		if eventID != 0 {
			gasUsed := workItem.RefineGasLimit - uint64(vm.SafeGetGas())
			refineCosts = append(refineCosts, telemetry.RefineCost{
				TotalGasUsed:      gasUsed,
				TotalTimeNs:       uint64(execElapsed.Nanoseconds()),
				LoadCompileTimeNs: uint64(compileElapsed.Nanoseconds()),
			})
		}

	}

	// Telemetry: Refined (event 94)
	if telemetryClient != nil {
		telemetryClient.Refined(eventID, refineCosts)
	}

	spec, d := NewAvailabilitySpecifier(package_bundle, segments)
	workReport := types.WorkReport{
		AvailabilitySpec:  *spec,
		RefineContext:     workPackage.RefineContext,
		CoreIndex:         uint(workPackageCoreIndex),
		AuthorizerHash:    p_a,
		Trace:             r.Ok,
		SegmentRootLookup: segmentRootLookup,
		Results:           results,
		AuthGasUsed:       uint(authGasUsed),
	}
	log.Trace(log.Node, "executeWorkPackageBundle", "backend", pvmBackend, "role", vmLogging, "reportHash", workReport.Hash(), "workreport", workReport.AvailabilitySpec.String())

	s.GetStorage().GetJAMDA().StoreBundleSpecSegments(spec, d, package_bundle, segments)

	// Telemetry: Work-report built (event 102)
	if telemetryClient != nil {
		bundleBytes := package_bundle.Bytes()
		workReportOutline := telemetry.WorkReportOutline{
			WorkReportHash: workReport.Hash(),
			BundleSize:     uint32(len(bundleBytes)),
			ErasureRoot:    workReport.AvailabilitySpec.ErasureRoot,
			SegmentsRoot:   workReport.AvailabilitySpec.ExportedSegmentRoot,
		}
		telemetryClient.WorkReportBuilt(eventID, workReportOutline)
	}
	bundleSnapshot := &types.WorkPackageBundleSnapshot{
		PackageHash:       workReport.GetWorkPackageHash(),
		CoreIndex:         uint16(workReport.CoreIndex),
		Bundle:            package_bundle,
		SegmentRootLookup: workReport.SegmentRootLookup,
		Slot:              slot,
		Report:            workReport,
	}
	SaveToLogDirOnce(logDir, fmt.Sprintf("%v_bundlesnap.json", workPackage.Hash()), []byte(types.ToJSON(bundleSnapshot)))
	SaveToLogDir(logDir, "bclubs", SerializeHashes(d.BClubs))
	SaveToLogDir(logDir, "sclubs", SerializeHashes(d.SClubs))
	SaveToLogDir(logDir, "bundle_chunks", SerializeECChunks(d.BundleChunks))
	SaveToLogDir(logDir, "segment_chunks", SerializeECChunks(d.SegmentChunks))
	SaveToLogDir(logDir, "workreportderivation.json", []byte(types.ToJSON(d)))
	SaveToLogDir(logDir, "workreport.bin", workReport.Bytes())
	SaveToLogDir(logDir, "workreport.json", []byte(types.ToJSON(workReport)))
	return workReport, nil
}

func (s *StateDB) ReadGlobalDepth(serviceID uint32) (depth uint8, err error) {
	ldBytes, found, readErr := s.ReadServiceStorage(uint32(serviceID), buildSSRKey())
	if readErr != nil {
		return 0, fmt.Errorf("failed to read global_depth hint: %v", readErr)
	}
	if !found {
		return 0, nil
	}
	if len(ldBytes) < 1 {
		return 0, fmt.Errorf("global_depth hint invalid length: %d", len(ldBytes))
	}

	return ldBytes[0], nil
}

func (s *StateDB) ExecuteWorkPackageBundleSteps(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32, execContext string, eventID uint64, pvmBackends []string) (err error) {
	// Enable memory operation logging for step-by-step execution
	oldPvmLogging := interpreter.PvmLogging
	interpreter.PvmLogging = true
	defer func() {
		interpreter.PvmLogging = oldPvmLogging
	}()

	importsegments := make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
	workPackage := package_bundle.WorkPackage
	copy(importsegments, package_bundle.ImportSegmentData)

	// Authorization
	r, _, authGasUsed, err := s.authorizeWP(workPackage, workPackageCoreIndex, pvmBackends[0], "SKIP")
	if err != nil {
		return err
	}
	if authGasUsed < 0 {
		authGasUsed = 0
	}

	for index, workItem := range workPackage.WorkItems {
		vms := make([]*VM, len(pvmBackends))
		vmLog := make([][]byte, len(pvmBackends))
		for backend, pvmBackend := range pvmBackends {
			service_index := workItem.Service
			code, ok, err0 := s.ReadServicePreimageBlob(service_index, workItem.CodeHash)
			if err0 != nil || !ok || len(code) == 0 {
				return fmt.Errorf("executeWorkPackageBundle(ReadServicePreimageBlob):s_id %v, codehash %v, err %v, ok=%v", service_index, workItem.CodeHash, err0, ok)
			}
			vms[backend] = NewVMFromCode(service_index, code, 0, 0, s, pvmBackend, workItem.RefineGasLimit)
			if vms[backend] == nil {
				return fmt.Errorf("executeWorkPackageBundle: failed to create VM for service %d (corrupted bytecode?)", service_index)
			}
			vms[backend].Timeslot = s.JamState.SafroleState.Timeslot
			vms[backend].SetPVMContext(execContext)
			vms[backend].hostenv = s
			vms[backend].SetupRefine(workPackageCoreIndex, uint32(index), workPackage, r, importsegments, workItem.ExportCount, package_bundle.ExtrinsicData[index], workPackage.AuthorizationCodeHash, common.BytesToHash(trie.H0))
			fmt.Printf("Backend %s setup!\n", pvmBackend)
		}
		fmt.Printf("EXECUTING STEP BY STEP\n")
		step := 0
		terminated := false
		for !terminated {
			for backend, vm := range vms {
				vmLog[backend] = vm.ExecuteStep(vm)
				if backend > 0 {
					if !bytes.Equal(vmLog[backend-1], vmLog[backend]) {
						fmt.Printf("Backend[%s] %s\n", pvmBackends[backend-1], pvmlog(vmLog[backend-1]))
						fmt.Printf("Backend[%s] %s\n", pvmBackends[backend], pvmlog(vmLog[backend]))
						panic("VM step logs do not match between backends")
					} else {
						step++
						if step <= 10 || step%100000 == 0 {
							fmt.Printf("Step %d: %s\n", step, pvmlog(vmLog[backend]))
						}
					}
				}
				if vm.terminated == true {
					terminated = true
				}
			}
		}
	}

	return nil
}

// BuildBundle maps a work package into a WorkPackageBundle using JAMDA interface
// It updates the workpackage work items: (1)  ExportCount ImportedSegments with a special HostFetchWitness call
//
// skipApplyWrites: If true, skip storing/applying contract writes to state.
// Use this when Phase 1 has already applied state changes and Phase 2 is only generating witnesses.
func (s *StateDB) BuildBundle(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash, pvmBackend string, skipApplyWrites bool) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
	wp := workPackage.Clone()

	// CRITICAL: Capture RefineContext BEFORE execution (used for witness verification)
	// The state root here matches the root used when witnesses were fetched
	originalRefineContext := s.GetRefineContext()
	wp.RefineContext = originalRefineContext
	log.Trace(log.EVM, "BuildBundle:", "STATEROOT", wp.RefineContext.StateRoot)
	authorization, p_a, _, err := s.authorizeWP(wp, coreIndex, pvmBackend, "SKIP")
	if err != nil {
		return nil, nil, err
	}

	results := []types.WorkDigest{}
	contractWitnessBlobs := make([][]byte, len(wp.WorkItems)) // Store contract witness blobs for later application
	var segments [][]byte                                     // Collect all exported segments for storage

	for index, workItem := range wp.WorkItems {
		code, ok, err0 := s.ReadServicePreimageBlob(workItem.Service, workItem.CodeHash)
		if err != nil || !ok || len(code) == 0 {
			return nil, nil, fmt.Errorf("BuildBundle:ReadServicePreimageBlob:s_id %v, codehash %v, err %v, ok=%v", workItem.Service, workItem.CodeHash, err0, ok)
		}

		// Capture original transaction count BEFORE any witnesses are appended
		originalTxCount := uint32(len(wp.WorkItems[index].Extrinsics))

		vm := NewVMFromCode(workItem.Service, code, 0, 0, s, pvmBackend, workItem.RefineGasLimit)
		if vm == nil {
			return nil, nil, fmt.Errorf("BuildBundle:NewVMFromCode:s_id %v, codehash %v, err %v, ok=%v", workItem.Service, workItem.CodeHash, err0, ok)
		}
		vm.Timeslot = s.JamState.SafroleState.Timeslot
		vm.SetPVMContext(log.Builder)
		importsegments := make([][][]byte, len(wp.WorkItems))

		// Safety guard: verify payload witness_count + tx_count won't exceed extrinsics array bounds
		// PVM uses witness_count as tx_start_index, so witness_count + tx_count must <= len(extrinsics)
		if len(workItem.Payload) >= 8 {
			payloadTxCount := binary.LittleEndian.Uint32(workItem.Payload[1:5])
			payloadWitnessCount := binary.LittleEndian.Uint16(workItem.Payload[6:8])
			extrinsicsLen := uint32(len(extrinsicsBlobs[index]))
			if uint32(payloadWitnessCount)+payloadTxCount > extrinsicsLen {
				return nil, nil, fmt.Errorf("invalid payload: witness_count(%d) + tx_count(%d) = %d exceeds extrinsics_len(%d)",
					payloadWitnessCount, payloadTxCount, uint32(payloadWitnessCount)+payloadTxCount, extrinsicsLen)
			}
		}

		result, _, exported_segments := vm.ExecuteRefine(coreIndex, uint32(index), wp, authorization, importsegments, 0, extrinsicsBlobs[index], p_a, common.BytesToHash(trie.H0), "")
		// Check if this is an EVM service - only EVM needs metashard/state witness processing
		isEVMService := workItem.Service == EVMServiceCode

		// For non-EVM services (e.g., fib), skip EVM-specific witness processing
		var importedSegments []types.ImportSegment
		var witnesses []types.StateWitness
		var contractWitnessBlob []byte

		if isEVMService {
			// Post-SSR: Parse refine output to populate witness cache from exported segments
			// Output format: [2B count] + count × [32B object_id + 2B index_start + 4B payload_length + 1B object_kind]
			if err := s.populateWitnessCacheFromRefineOutput(workItem.Service, result.Ok, exported_segments); err != nil {
				log.Warn(log.EVM, "Failed to populate witness cache from refine output", "error", err)
			}

			importedSegments, witnesses, err = vm.GetBuilderWitnesses()
			if err != nil {
				log.Warn(log.DA, "BuildBundle: GetBuilderWitnesses failed", "err", err)
				return nil, nil, err
			}

			// Generate UBT witness for builder mode (Step 2: Builder Refine)
			// Extract contract witness blob from refine result output
			contractWitnessBlob, err = s.ExtractContractWitnessBlob(result.Ok, exported_segments)
			if err != nil {
				log.Warn(log.EVM, "Failed to extract contract witness blob", "err", err)
				return nil, nil, err
			}
			contractWitnessBlobs[index] = contractWitnessBlob // Store for later application to main tree

			// Build UBT witnesses - all UBT logic handled in storage
			evmStorage := s.sdb.(types.EVMJAMStorage)
			ubtPreWitness, ubtPostWitness, err := evmStorage.BuildUBTWitness(contractWitnessBlob)
			if err != nil {
				log.Warn(log.EVM, "BuildUBTWitness failed", "err", err)
				return nil, nil, err
			}

			// Verify both witnesses immediately after generation
			preStats, err := s.verifyUBTWitness(ubtPreWitness, "pre", storage.WitnessValuePre)
			if err != nil {
				log.Warn(log.EVM, "Pre-witness verification failed", "err", err)
				return nil, nil, fmt.Errorf("pre-witness verification failed: %w", err)
			}
			postStats, err := s.verifyUBTWitness(ubtPostWitness, "post", storage.WitnessValuePost)
			if err != nil {
				log.Warn(log.EVM, "Post-witness verification failed", "err", err)
				return nil, nil, fmt.Errorf("post-witness verification failed: %w", err)
			}
			preHead, preTail := witnessEdgeHex(ubtPreWitness)
			postHead, postTail := witnessEdgeHex(ubtPostWitness)
			log.Info(log.EVM, "BuildBundle: UBT witnesses verified (go)",
				"pre_root", common.BytesToHash(preStats.Root[:]).Hex(),
				"post_root", common.BytesToHash(postStats.Root[:]).Hex(),
				"pre_len", len(ubtPreWitness),
				"post_len", len(ubtPostWitness),
				"pre_head64", preHead,
				"pre_tail64", preTail,
				"post_head64", postHead,
				"post_tail64", postTail)

			// Build Block Access List from witnesses (Phase 4)
			// This computes the BAL hash that will be embedded in the block payload
			blockAccessListHash, accountCount, totalChanges, err := evmStorage.ComputeBlockAccessListHash(ubtPreWitness, ubtPostWitness)
			if err != nil {
				log.Warn(log.EVM, "ComputeBlockAccessListHash failed", "err", err)
				// Non-fatal: continue without BAL hash (will be zeros)
				blockAccessListHash = common.Hash{}
			} else {
				log.Trace(log.EVM, "Block Access List computed", "hash", blockAccessListHash.Hex(), "accounts", accountCount, "changes", totalChanges)
			}

			// Extract post-state UBT root from witness to update block payload
			postStateRoot, err := extractPostStateRootFromWitness(ubtPostWitness)
			if err != nil {
				log.Warn(log.EVM, "Failed to extract post-state root from witness", "err", err)
				postStateRoot = common.Hash{} // leave unchanged if extraction fails
			}

			var stateDelta *evmtypes.UBTStateDelta
			stateDelta = nil

			// Update block payload with BAL hash, post-state root, and delta (Phase 4)
			// The block was already exported during ExecuteRefine, so we need to update it in exported_segments
			err = s.updateBlockPayloadWithDelta(exported_segments, blockAccessListHash, postStateRoot, stateDelta)
			if err != nil {
				log.Warn(log.EVM, "Failed to update block payload", "err", err)
				// Non-fatal: continue with original block payload (BAL hash/root/delta may be zeros/nil)
			}

			// Prepend UBT witnesses as the first two extrinsics (pre, post)
			extrinsicsBlobs[index] = append([][]byte{ubtPreWitness, ubtPostWitness}, extrinsicsBlobs[index]...)

			// Update work item extrinsics to include UBT witnesses
			ubtPreExtrinsic := types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(ubtPreWitness),
				Len:  uint32(len(ubtPreWitness)),
			}
			ubtPostExtrinsic := types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(ubtPostWitness),
				Len:  uint32(len(ubtPostWitness)),
			}
			// Prepend to extrinsics list in order
			wp.WorkItems[index].Extrinsics = append([]types.WorkItemExtrinsic{ubtPreExtrinsic, ubtPostExtrinsic}, wp.WorkItems[index].Extrinsics...)
			log.Info(log.EVM, "BuildBundle: prepended UBT witnesses",
				"pre_hash", ubtPreExtrinsic.Hash.Hex(),
				"pre_len", ubtPreExtrinsic.Len,
				"post_hash", ubtPostExtrinsic.Hash.Hex(),
				"post_len", ubtPostExtrinsic.Len)
			// Verify post-state against execution
			if err := s.verifyPostStateAgainstExecution(wp.WorkItems[index], extrinsicsBlobs[index], contractWitnessBlob); err != nil {
				log.Warn(log.EVM, "Post-state verification failed", "err", err)
				return nil, nil, fmt.Errorf("post-state verification failed: %w", err)
			}

			// Clear UBT read log for next execution via clean interface
			evmStorage.ClearUBTReadLog()

			// Append builder witnesses to extrinsicsBlobs -- this will be the metashards + the object proofs
			builderWitnessCount := appendExtrinsicWitnessesToWorkItem(&wp.WorkItems[index], &extrinsicsBlobs, index, witnesses)
			log.Info(log.EVM, "BuildBundle: Appended builder witnesses", "workItemIndex", index, "builderWitnessCount", builderWitnessCount, "totalExtrinsics", len(extrinsicsBlobs[index]))
			// Update payload metadata with builder witness count and BAL hash
			// ALWAYS update if payload exists (even if builderWitnessCount=0) to ensure BAL hash is included
			if len(wp.WorkItems[index].Payload) >= 7 {
				// witness_count in payload should ONLY include prepended UBT witnesses (2), NOT appended builder witnesses
				// This is because PVM uses witness_count to determine tx_start_index for 0-based extrinsic access
				ubtWitnessCount := uint16(2) // UBT pre/post witnesses are prepended at indices 0-1
				wp.WorkItems[index].Payload = evmtypes.BuildPayload(evmtypes.PayloadTypeTransactions, int(originalTxCount), 0, int(ubtWitnessCount), blockAccessListHash)
			}
		} // end if isEVMService

		wp.WorkItems[index].ExportCount = uint16(len(exported_segments))
		wp.WorkItems[index].ImportedSegments = importedSegments

		// Append exported segments (append slice directly)
		segments = append(segments, exported_segments...)

		// Calculate total extrinsic bytes
		totalExtrinsicBytes := 0
		for _, e := range extrinsicsBlobs[index] {
			totalExtrinsicBytes += len(e)
		}

		// Store result for work report
		results = append(results, types.WorkDigest{
			ServiceID:           workItem.Service,
			CodeHash:            workItem.CodeHash,
			PayloadHash:         common.Blake2Hash(workItem.Payload),
			Gas:                 workItem.AccumulateGasLimit,
			GasUsed:             uint(workItem.RefineGasLimit - uint64(vm.SafeGetGas())),
			NumImportedSegments: uint(len(importedSegments)),
			NumExportedSegments: uint(len(exported_segments)),
			NumExtrinsics:       uint(len(extrinsicsBlobs[index])),
			NumBytesExtrinsics:  uint(totalExtrinsicBytes),
			Result:              result,
		})
	}

	// Use buildBundle to fetch imported segments and justifications
	// CRITICAL: Verify metadata count matches blob count before creating bundle
	for i, wi := range wp.WorkItems {
		metadataCount := len(wi.Extrinsics)
		blobCount := len(extrinsicsBlobs[i])
		if metadataCount != blobCount {
			log.Error(log.EVM, "BuildBundle: MISMATCH between metadata and blob count!",
				"workItemIndex", i,
				"metadataCount", metadataCount,
				"blobCount", blobCount)
		} else {
			log.Info(log.EVM, "BuildBundle: extrinsic counts match",
				"workItemIndex", i,
				"count", metadataCount,
				"blobCount", blobCount)
		}
	}
	wpq := &types.WPQueueItem{
		WorkPackage: wp,
		CoreIndex:   coreIndex,
		Extrinsics:  extrinsicsBlobs[0], // buildBundle expects single ExtrinsicsBlobs
	}

	bundle, _, err := s.GetStorage().GetJAMDA().BuildBundleFromWPQueueItem(wpq)
	if err != nil {
		log.Warn(log.DA, "BuildBundle: BuildBundleFromWPQueueItem failed", "err", err)
		return nil, nil, err
	}

	// Update ExtrinsicData with all work items (buildBundle only handles first work item)
	bundle.ExtrinsicData = extrinsicsBlobs
	bundle.WorkPackage.AuthCodeHost = wp.AuthCodeHost
	bundle.WorkPackage.AuthorizationCodeHash = wp.AuthorizationCodeHash
	bundle.WorkPackage.AuthorizationToken = wp.AuthorizationToken
	bundle.WorkPackage.ConfigurationBlob = wp.ConfigurationBlob
	bundle.WorkPackage.RefineContext = originalRefineContext

	// Create work report from results -- note that this does not have availability spec
	workReport := &types.WorkReport{
		CoreIndex: uint(coreIndex),
		Results:   results,
	}
	log.Trace(log.Node, "BuildBundle: Built", "payload", fmt.Sprintf("%x", bundle.WorkPackage.WorkItems[0].Payload))

	// Compute work package hash once for segment storage
	workPackageHash := wp.Hash()

	// Apply contract writes to the active UBT snapshot (if multi-snapshot mode is active)
	// or store as pending writes for legacy deferred application.
	// Skip if skipApplyWrites is true (Phase 2 mode - Phase 1 already applied writes).
	evmStorage := s.sdb.(types.EVMJAMStorage)
	if !skipApplyWrites {
		var totalBlobSize int
		for _, blob := range contractWitnessBlobs {
			totalBlobSize += len(blob)
		}
		if totalBlobSize > 0 {
			// Concatenate all blobs for this work package
			combinedBlob := make([]byte, 0, totalBlobSize)
			for _, blob := range contractWitnessBlobs {
				combinedBlob = append(combinedBlob, blob...)
			}

			// Try to apply writes to active snapshot (multi-snapshot mode)
			// If no snapshot is active, fall back to deferred writes
			if err := evmStorage.ApplyWritesToActiveSnapshot(combinedBlob); err != nil {
				// No active snapshot - use legacy deferred writes path
				evmStorage.StorePendingWrites(workPackageHash, combinedBlob)
				log.Info(log.EVM, "BuildBundle: stored pending writes for deferred application (legacy mode)",
					"wpHash", workPackageHash.Hex(),
					"combinedBlobSize", len(combinedBlob),
					"workItemCount", len(contractWitnessBlobs))
			} else {
				log.Info(log.EVM, "BuildBundle: applied writes to active snapshot",
					"wpHash", workPackageHash.Hex(),
					"combinedBlobSize", len(combinedBlob),
					"workItemCount", len(contractWitnessBlobs))
			}
		}
	} else {
		log.Debug(log.EVM, "BuildBundle: skipping state writes (Phase 2 mode - already applied in Phase 1)",
			"wpHash", workPackageHash.Hex())
	}

	// Store exported segments for builder - both in memory cache and persisted to disk
	if len(segments) > 0 {
		type segmentStorer interface {
			StoreSegment(common.Hash, uint16, []byte)
		}
		if storer, ok := s.GetStorage().GetJAMDA().(segmentStorer); ok {
			for segmentIdx, segmentData := range segments {
				storer.StoreSegment(workPackageHash, uint16(segmentIdx), segmentData)
			}
			log.Trace(log.DA, "BuildBundle: stored segments in memory cache", "workPackageHash", workPackageHash.Hex(), "numSegments", len(segments))
		}

		// Persist segments to disk keyed by ExportedSegmentRoot for later retrieval
		spec, d := NewAvailabilitySpecifier(bundle, segments)
		s.GetStorage().GetJAMDA().StoreBundleSpecSegments(spec, d, bundle, segments)
		log.Info(log.DA, "BuildBundle: persisted segments to disk",
			"workPackageHash", workPackageHash.Hex(),
			"exportedSegmentRoot", spec.ExportedSegmentRoot.Hex(),
			"numSegments", len(segments))
	}

	return &bundle, workReport, nil
}

// getBeefyRootForAnchor returns the BEEFY root recorded for the given anchor header hash.
func (s *StateDB) getBeefyRootForAnchor(anchor common.Hash) common.Hash {
	recent := s.JamState.RecentBlocks.B_H
	if len(recent) == 0 {
		return common.Hash{}
	}

	for i := len(recent) - 1; i >= 0; i-- {
		if recent[i].HeaderHash == anchor {
			return recent[i].B
		}
	}

	return recent[len(recent)-1].B
}

func appendExtrinsicWitnessesToWorkItem(workItem *types.WorkItem, extrinsicsBlobs *[]types.ExtrinsicsBlobs, index int, witnesses []types.StateWitness) int {
	i := 0
	for _, witness := range witnesses {
		witnessBytes := witness.SerializeWitness()
		(*extrinsicsBlobs)[index] = append((*extrinsicsBlobs)[index], witnessBytes)
		i++
		witnessExtrinsic := types.WorkItemExtrinsic{
			Hash: common.Blake2Hash(witnessBytes),
			Len:  uint32(len(witnessBytes)),
		}
		workItem.Extrinsics = append(workItem.Extrinsics, witnessExtrinsic)
		for objectID, bmtProof := range witness.ObjectProofs {
			ref := witness.ObjectRefs[objectID]

			refBytes := ref.Serialize()
			witnessBytes = append(objectID.Bytes(), refBytes...) // proofBytes...

			// temporary
			extra := make([]byte, 8)
			witnessBytes = append(witnessBytes, extra...)

			proofBytes := []byte{}
			for _, p := range bmtProof {
				proofBytes = append(proofBytes, p.Bytes()...)
			}
			witnessBytes = append(witnessBytes, proofBytes...)
			(*extrinsicsBlobs)[index] = append((*extrinsicsBlobs)[index], witnessBytes)
			i++
			witnessExtrinsic = types.WorkItemExtrinsic{
				Hash: common.Blake2Hash(witnessBytes),
				Len:  uint32(len(witnessBytes)),
			}
			workItem.Extrinsics = append(workItem.Extrinsics, witnessExtrinsic)
		}
	}
	return i
}

func NewAvailabilitySpecifier(package_bundle types.WorkPackageBundle, export_segments [][]byte) (availabilityspecifier *types.AvailabilitySpecifier, d types.AvailabilitySpecifierDerivation) {
	// compile wp into b
	b := package_bundle.Bytes() // check
	// Build b♣ and s♣
	bClubs, bEcChunks := BuildBClub(b)
	sClubs, sEcChunksArr := BuildSClub(export_segments)

	// for segIdx, seg := range export_segments {
	// 	segmentHash := trie.ComputeLeaf(seg) //Blake2Hash(“leaf” || segment)
	// 	fmt.Printf("Exported Segment %d (H=%x): %x\n", segIdx, segmentHash, seg)
	// }

	// ExportedSegmentRoot = CDT(segments)
	exportedSegmentTree := trie.NewCDMerkleTree(export_segments)
	//exportedSegmentTree.PrintTree()

	d = types.AvailabilitySpecifierDerivation{
		BClubs:        bClubs,
		SClubs:        sClubs,
		BundleChunks:  bEcChunks,
		SegmentChunks: sEcChunksArr,
	}

	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:       package_bundle.WorkPackage.Hash(),
		BundleLength:          uint32(len(b)),
		ErasureRoot:           GenerateErasureRoot(bClubs, sClubs), // u = (bClub, sClub)
		ExportedSegmentRoot:   exportedSegmentTree.RootHash(),
		ExportedSegmentLength: uint16(len(export_segments)),
	}

	return &availabilitySpecifier, d
}

// this is the default justification from (b,s) to erasureRoot
func ErasureRootDefaultJustification(b []common.Hash, s []common.Hash) (shardJustifications []types.Justification, err error) {
	shardJustifications = make([]types.Justification, types.TotalValidators)
	erasureTree, _ := GenerateErasureTree(b, s)
	erasureRoot := erasureTree.RootHash()
	for shardIdx := 0; shardIdx < types.TotalValidators; shardIdx++ {
		treeLen, leaf, path, _, _ := erasureTree.Trace(shardIdx)
		verified, _ := VerifyWBTJustification(treeLen, erasureRoot, uint16(shardIdx), leaf, path, "ErasureRootDefaultJustification")
		if !verified {
			return shardJustifications, fmt.Errorf("verifyWBTJustification Failure")
		}
		shardJustifications[shardIdx] = types.Justification{
			Root:     erasureRoot,
			ShardIdx: shardIdx,
			TreeLen:  types.TotalValidators,
			LeafHash: leaf,
			Path:     path,
		}
	}
	return shardJustifications, nil
}

// Verify T(s,i,H)
func VerifyWBTJustification(treeLen int, root common.Hash, shardIndex uint16, leafHash []byte, path [][]byte, caller string) (bool, common.Hash) {
	recoveredRoot, verified, _ := trie.VerifyWBT(treeLen, int(shardIndex), root, leafHash, path)
	encodedPath, _ := common.EncodeJustification(path, types.NumECPiecesPerSegment)
	reversedEncodedPath, _ := common.EncodeJustification((common.ReversedByteArray(path)), types.NumECPiecesPerSegment)
	if root != recoveredRoot {
		log.Warn(log.Node, "VerifyWBTJustification Failure Part.A", "caller", caller, "shardIdx", shardIndex, "Expected", root, "recovered", recoveredRoot, "verified", verified, "treeLen", treeLen, "leafHash", fmt.Sprintf("%x", leafHash), "path", fmt.Sprintf("%x", path))
		log.Warn(log.Node, "VerifyWBTJustification Failure Part.B", "caller", caller, "shardIdx", shardIndex, "Expected", root, "encodedPath", common.Bytes2String(encodedPath), "reversedEncodedPath", common.Bytes2String(reversedEncodedPath))
		return false, recoveredRoot
	}
	log.Trace(log.Node, "VerifyWBTJustification Success", "caller", caller, "shardIdx", shardIndex, "Expected", root, "recovered", recoveredRoot, "verified", verified, "treeLen", treeLen, "leafHash", fmt.Sprintf("%x", leafHash), "path", fmt.Sprintf("%x", path))
	return true, recoveredRoot
}

// Generating co-path for T(s,i,H)
// s: [(b♣T,s♣T)...] -  sequence of (work-package bundle shard hash, segment shard root) pairs satisfying u = MB(s)
// i: shardIdx or ChunkIdx
// H: Blake2b
func GenerateWBTJustification(root common.Hash, shardIndex uint16, leaves [][]byte) (treeLen int, leafHash []byte, path [][]byte, isFound bool) {
	wbt := trie.NewWellBalancedTree(leaves, types.Blake2b)
	// fmt.Printf("GenerateWBTJustification:root %v, shardIndex %v, leaves %x\n", root, shardIndex, leaves)
	// wbt.PrintTree()
	treeLen, leafHash, path, isFound, _ = wbt.Trace(int(shardIndex))
	return treeLen, leafHash, path, isFound
}

// Compute b♣ using the EncodeWorkPackage function
func BuildBClub(b []byte) ([]common.Hash, []types.DistributeECChunk) {
	// Padding b to the length of W_G
	paddedB := common.PadToMultipleOfN(b, types.ECPieceSize) // this makes sense

	if debugSpec {
		fmt.Printf("Padded %d bytes to %d bytes (multiple of %d bytes) => %x\n", len(b), len(paddedB), types.ECPieceSize, paddedB)
	}

	// instead of a tower of abstraction, collapse it to the minimal number of lines
	chunks, err := bls.Encode(paddedB, types.TotalValidators)
	if err != nil {
		log.Error(log.Node, "BuildBClub", "err", err)
	}

	// Hash each element of the encoded data
	bClubs := make([]common.Hash, types.TotalValidators)
	bundleShards := chunks // this should be of size 1
	ecChunks := make([]types.DistributeECChunk, types.TotalValidators)
	for shardIdx, shard := range bundleShards {
		bClubs[shardIdx] = common.Blake2Hash(shard)
		if debugSpec {
			fmt.Printf("BuildBClub hash %d: %s Shard: %x (%d bytes)\n", shardIdx, bClubs[shardIdx], shard, len(shard))
		}
		ecChunks[shardIdx] = types.DistributeECChunk{
			//SegmentRoot: bClubs[shardIdx].Bytes(), // SegmentRoot used to store the hash of the shard
			Data: shard,
		}
	}
	return bClubs, ecChunks
}

func BuildSClub(segments [][]byte) (sClub []common.Hash, ecChunksArr []types.DistributeECChunk) {
	ecChunksArr = make([]types.DistributeECChunk, types.TotalValidators)

	// EC encode segments in ecChunksArr
	for segmentIdx, segmentData := range segments {
		if segmentIdx == 0 {
			for i := range types.TotalValidators {
				ecChunksArr[i] = types.DistributeECChunk{
					Data: []byte{},
				}
			}
		}

		// Encode segmentData into leaves
		erasureCodingSegments, err := bls.Encode(segmentData, types.TotalValidators)
		if err != nil {
			log.Error(log.DA, "BuildSClub", "segmentIdx", segmentIdx, "err", err)
		}
		for shardIndex, shard := range erasureCodingSegments {
			ecChunksArr[shardIndex].Data = append(ecChunksArr[shardIndex].Data, shard...)
		}
	}

	// now take up to 64 segments at a time and build a page proof
	// IMPORTANT: these pageProofs are provided in OTHER bundles for imported segments
	//   The guarantor who builds the bundle must pull out a specific pageproof and verify it against the correct exported segment root
	pageProofs, pageProofGenerationErr := trie.GeneratePageProof(segments)
	if pageProofGenerationErr != nil {
		log.Error(log.DA, "GeneratePageProof", "Error", pageProofGenerationErr)
	}
	for pageIdx, pagedProofByte := range pageProofs {
		if false {
			tree := trie.NewCDMerkleTree(segments)
			global_segmentsRoot := tree.Root()
			decodedData, _, decodingErr := types.Decode(pagedProofByte, reflect.TypeOf(types.PageProof{}))
			if decodingErr != nil {
				log.Error(log.DA, "BuildSClub Proof decoding err", "Error", decodingErr)
			}
			recoveredPageProof := decodedData.(types.PageProof)
			for subTreeIdx := 0; subTreeIdx < len(recoveredPageProof.LeafHashes); subTreeIdx++ {
				leafHash := recoveredPageProof.LeafHashes[subTreeIdx]
				pageSize := 1 << trie.PageFixedDepth
				index := pageIdx*pageSize + subTreeIdx
				fullJustification, err := trie.PageProofToFullJustification(pagedProofByte, pageIdx, subTreeIdx)
				if err != nil {
					log.Error(log.DA, "BuildSClub PageProofToFullJustification ERR", "Error", err)
				}
				derived_global_segmentsRoot := trie.VerifyCDTJustificationX(leafHash.Bytes(), index, fullJustification, 0)
				if !common.CompareBytes(derived_global_segmentsRoot, global_segmentsRoot) {
					log.Error(log.DA, "BuildSClub fullJustification Root hash mismatch", "expected", fmt.Sprintf("%x", global_segmentsRoot), "got", fmt.Sprintf("%x", derived_global_segmentsRoot))
				}
			}
		}
		paddedProof := common.PadToMultipleOfN(pagedProofByte, types.SegmentSize)
		erasureCodingPageSegments, err := bls.Encode(paddedProof, types.TotalValidators)
		if err != nil {
			return
		}
		for shardIndex, shard := range erasureCodingPageSegments {
			ecChunksArr[shardIndex].Data = append(ecChunksArr[shardIndex].Data, shard...)
		}
	}
	sClub = make([]common.Hash, types.TotalValidators)

	// Build chunks by tracking actual shard sizes from encoding
	// We need to re-encode each segment/pageProof to know the shard boundaries
	for shardIndex := 0; shardIndex < types.TotalValidators; shardIndex++ {
		chunks := make([][]byte, 0, len(segments)+len(pageProofs))
		offset := 0

		// Add segment shards
		for _, segmentData := range segments {
			erasureCodingSegments, _ := bls.Encode(segmentData, types.TotalValidators)
			shardSize := len(erasureCodingSegments[shardIndex])
			chunks = append(chunks, ecChunksArr[shardIndex].Data[offset:offset+shardSize])
			offset += shardSize
		}

		// Add pageProof shards
		for _, pagedProofByte := range pageProofs {
			paddedProof := common.PadToMultipleOfN(pagedProofByte, types.SegmentSize)
			erasureCodingPageSegments, _ := bls.Encode(paddedProof, types.TotalValidators)
			shardSize := len(erasureCodingPageSegments[shardIndex])
			chunks = append(chunks, ecChunksArr[shardIndex].Data[offset:offset+shardSize])
			offset += shardSize
		}

		t := trie.NewWellBalancedTree(chunks, types.Blake2b)
		sClub[shardIndex] = common.BytesToHash(t.Root())
	}

	return sClub, ecChunksArr
}

func GenerateErasureTree(bClubs []common.Hash, sClubs []common.Hash) (*trie.WellBalancedTree, [][]byte) {
	// Combine b♣, s♣ into 64bytes pairs
	bundle_segment_pairs := common.BuildBundleSegmentPairs(bClubs, sClubs)

	// Generate and return erasureroot
	t := trie.NewWellBalancedTree(bundle_segment_pairs, types.Blake2b)
	if debugSpec {
		fmt.Printf("\nWBT of bclub-sclub pairs:\n")
		t.PrintTree()
	}
	return t, bundle_segment_pairs
}

// MB([x∣x∈T[b♣,s♣]]) - Encode b♣ and s♣ into a matrix
func GenerateErasureRoot(b []common.Hash, s []common.Hash) common.Hash {
	erasureTree, _ := GenerateErasureTree(b, s)
	return erasureTree.RootHash()
}

// M(s) - CDT of exportedSegment
func GenerateExportedSegmentsRoot(segments [][]byte) common.Hash {
	cdt := trie.NewCDMerkleTree(segments)
	return common.Hash(cdt.Root())
}

// Verify the justifications (picked out of PageProofs) for the imported segments, which can come from different work packages
func (n *StateDB) VerifyBundle(b *types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, eventID uint64) (verified bool, err error) {
	// verify the segments with CDT_6 justification included by first guarantor
	telemetryClient := n.GetStorage().GetTelemetryClient()
	for itemIndex, workItem := range b.WorkPackage.WorkItems {
		importedSegments := b.ImportSegmentData[itemIndex]
		if len(importedSegments) != len(workItem.ImportedSegments) {
			return false, fmt.Errorf(" VerifyBundle %d != %d", len(importedSegments), len(workItem.ImportedSegments))
		}
		for segmentIdx, i := range workItem.ImportedSegments {
			exportedSegmentRoot := i.RequestedHash
			for _, x := range segmentRootLookup {
				if x.WorkPackageHash == i.RequestedHash {
					exportedSegmentRoot = x.SegmentRoot

					// Telemetry: Work package hash mapped to segments-root for segment recovery
					// Emit WorkPackageHashMapped (event 160)
					telemetryClient.WorkPackageHashMapped(eventID, x.WorkPackageHash, x.SegmentRoot)
				}
				if i.RequestedHash == x.SegmentRoot {
					// Also emit SegmentsRootMapped (event 161) - mapping segments-root to erasure-root
					// Note: In this context, we don't have direct access to the erasure root,
					// but this would typically be the availability spec's erasure root, so we use WorkReportSearch (is this a problem?)
					// wph := x.WorkPackageHash
					// si := s.WorkReportSearch(wph)
					// if si != nil {
					// 	erasureRoot := si.WorkReport.AvailabilitySpec.ErasureRoot
					// 	telemetryClient.SegmentsRootMapped(eventID, x.SegmentRoot, erasureRoot)
					// }
				}
			}
			// requestedHash MUST map to exportedSegmentRoot
			segmentData := importedSegments[segmentIdx]
			global_segmentsRoot := trie.VerifyCDTJustificationX(trie.ComputeLeaf(segmentData), int(i.Index), b.Justification[itemIndex][segmentIdx], 0)
			if !common.CompareBytes(exportedSegmentRoot[:], global_segmentsRoot) {
				log.Warn(log.Node, "trie.VerifyCDTJustificationX NOT VERIFIED", "index", i.Index)
				return false, fmt.Errorf("justification failure computedRoot %s != exportedSegmentRoot %s", exportedSegmentRoot, exportedSegmentRoot)
			} else {
				log.Trace(log.DA, "VerifyBundle: Justification Verified", "index", i.Index, "exportedSegmentRoot", exportedSegmentRoot)
			}
		}
	}

	return true, nil
}

// computeUBTRoot creates a UBT tree from contract storage entries and computes its root.
func computeUBTRoot(address common.Address, contractStorage *evmtypes.ContractShard) (common.Hash, error) {
	log.Debug(log.EVM, "computeUBTRoot", "address", address.Hex(), "entries", len(contractStorage.Entries))

	tree := storage.NewUnifiedBinaryTree(storage.Config{Profile: storage.JAMProfile})
	for _, entry := range contractStorage.Entries {
		key := storage.GetStorageSlotKey(storage.JAMProfile, address, entry.KeyH)
		tree.Insert(key, entry.Value)
	}

	root := tree.RootHash()
	ubtRoot := common.BytesToHash(root[:])
	log.Info(log.EVM, "computeUBTRoot complete", "address", address.Hex(), "ubtRoot", ubtRoot.Hex(), "entries", len(contractStorage.Entries))

	return ubtRoot, nil
}

// updateBlockPayload updates the block payload in exported segments with the computed BAL hash
// and optionally the post-state root (if provided).
// The block is the first exported segment (ObjectKind::Block is exported first in refiner.rs)
func (s *StateDB) updateBlockPayload(exportedSegments [][]byte, balHash common.Hash, postStateRoot common.Hash) error {
	return s.updateBlockPayloadWithDelta(exportedSegments, balHash, postStateRoot, nil)
}

// updateBlockPayloadWithDelta updates the block payload in exported segments with the computed BAL hash,
// post-state root, and optionally the state delta.
// The block is the first exported segment (ObjectKind::Block is exported first in refiner.rs)
func (s *StateDB) updateBlockPayloadWithDelta(exportedSegments [][]byte, balHash common.Hash, postStateRoot common.Hash, delta *evmtypes.UBTStateDelta) error {
	if len(exportedSegments) == 0 {
		return fmt.Errorf("no exported segments")
	}

	// The block payload is the first exported segment
	blockSegment := exportedSegments[0]

	// Deserialize the block payload
	blockPayload, err := evmtypes.DeserializeEvmBlockPayload(blockSegment, false) // headerOnly=false, need full payload
	if err != nil {
		return fmt.Errorf("failed to deserialize block payload: %w", err)
	}

	// Update the BAL hash
	blockPayload.BlockAccessListHash = balHash

	// Update UBT root if provided (non-zero)
	if postStateRoot != (common.Hash{}) {
		blockPayload.UBTRoot = postStateRoot
	}

	// Update UBT state delta if provided (UBT-CODEX.md)
	if delta != nil {
		blockPayload.UBTStateDelta = delta
		log.Debug(log.EVM, "Added state delta to block payload", "entries", delta.NumEntries, "bytes", 4+len(delta.Entries))
	}

	// Re-serialize the block payload
	updatedSegment := evmtypes.SerializeEvmBlockPayload(blockPayload)

	// Replace the first segment with the updated one
	exportedSegments[0] = updatedSegment

	log.Debug(log.EVM, "Updated block payload", "balHash", balHash.Hex(), "ubtRoot", postStateRoot.Hex(), "segment_size", len(updatedSegment))

	return nil
}

// extractPostStateRootFromWitness extracts the post-state UBT root from a witness section.
// Witness section format: 32B root + 4B count + (161B * entries) + 4B proof_len + proof.
func extractPostStateRootFromWitness(witness []byte) (common.Hash, error) {
	if len(witness) < 32 {
		return common.Hash{}, fmt.Errorf("witness too short: %d bytes", len(witness))
	}
	var postRoot common.Hash
	copy(postRoot[:], witness[:32])
	return postRoot, nil
}

// verifyUBTWitness verifies a UBT witness section and returns summary stats.
func (s *StateDB) verifyUBTWitness(witness []byte, label string, kind storage.WitnessValueKind) (*storage.UBTWitnessStats, error) {
	hasher := storage.NewBlake3Hasher(storage.JAMProfile)
	var expectedRoot *[32]byte
	if kind == storage.WitnessValuePre {
		if stateStorage, ok := s.sdb.(*storage.StateDBStorage); ok {
			// Use GetActiveTree() to get the correct tree for parallel bundle building
			// This returns the active snapshot if set, otherwise CurrentUBT
			tree := stateStorage.GetActiveTree()
			if tree != nil {
				root := tree.RootHash()
				expectedRoot = &root
			}
		}
	}
	stats, err := storage.VerifyUBTWitnessSection(witness, expectedRoot, kind, hasher)
	if err != nil {
		return nil, fmt.Errorf("%s witness verification failed: %w", label, err)
	}
	log.Trace(log.EVM, "UBT witness verified", "label", label, "root", fmt.Sprintf("%x", stats.Root[:8]), "entries", stats.EntryCount)
	return stats, nil
}

func witnessEdgeHex(data []byte) (string, string) {
	if len(data) == 0 {
		return "", ""
	}
	headLen := 64
	if len(data) < headLen {
		headLen = len(data)
	}
	tailLen := 64
	if len(data) < tailLen {
		tailLen = len(data)
	}
	head := fmt.Sprintf("%x", data[:headLen])
	tail := fmt.Sprintf("%x", data[len(data)-tailLen:])
	return head, tail
}
