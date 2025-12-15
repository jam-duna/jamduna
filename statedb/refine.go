package statedb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path"
	"reflect"
	"time"

	bls "github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb/evmtypes"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/telemetry"
	"github.com/colorfulnotion/jam/trie"
	types "github.com/colorfulnotion/jam/types"
	"github.com/ethereum/go-verkle"
)

const (
	debugSpec = false
)

// extractContractWitnessBlob extracts the contract witness payload from refine output
// Returns the raw contract witness blob (balance/nonce/code/storage writes)
func (s *StateDB) extractContractWitnessBlob(output []byte, segments [][]byte) ([]byte, error) {
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

		// Contract witness metadata at end: [2B index_start][4B payload_length]
		if offset+6 <= len(output) {
			contractOffset := len(output) - 6
			indexStart := binary.LittleEndian.Uint16(output[contractOffset : contractOffset+2])
			payloadLength := binary.LittleEndian.Uint32(output[contractOffset+2 : contractOffset+6])

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
		// 32B VerkleKey
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
	if len(extrinsicData) == 0 || len(workItem.Extrinsics) == 0 {
		return fmt.Errorf("missing extrinsic data for post-state verification")
	}

	verkleWitness := extrinsicData[0]
	if uint32(len(verkleWitness)) != workItem.Extrinsics[0].Len {
		return fmt.Errorf("verkle witness length mismatch: expected %d, got %d", workItem.Extrinsics[0].Len, len(verkleWitness))
	}

	stateStorage, ok := s.sdb.(*storage.StateDBStorage)
	if !ok {
		return fmt.Errorf("unexpected storage type %T (need StateDBStorage)", s.sdb)
	}

	if stateStorage.CurrentVerkleTree == nil {
		return fmt.Errorf("no verkle tree available for post-state verification")
	}

	_, postState, err := storage.SplitWitnessSections(verkleWitness)
	if err != nil {
		return fmt.Errorf("failed to split dual-proof witness: %w", err)
	}

	builderProof, err := parseBuilderPostStateSection(postState)
	if err != nil {
		return fmt.Errorf("failed to parse builder post-state section: %w", err)
	}

	guarantorWrites, err := storage.ExtractWriteMapFromContractWitness(stateStorage.CurrentVerkleTree, contractWitnessBlob)
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
	log.Trace(log.EVM, "populateWitnessCacheFromRefineOutput", "metashardCount", metashardCount, "segments", len(segments), "outputLen", len(output))

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
			// Contract witness metadata should be at the end: [2B index_start][4B payload_length]
			contractOffset := len(output) - 6
			indexStart := binary.LittleEndian.Uint16(output[contractOffset : contractOffset+2])
			payloadLength := binary.LittleEndian.Uint32(output[contractOffset+2 : contractOffset+6])

			log.Trace(log.EVM, "Found contract witness after metashards", "indexStart", indexStart, "payloadLength", payloadLength)

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

	s.sdb.InitWitnessCache()

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

		if len(payload)-offset-29 < int(payloadLength) {
			return fmt.Errorf("insufficient payload data at offset %d", offset)
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

			// Compute verkle root for the contract storage
			verkleRoot, err := computeVerkleRoot(address, contractStorage)
			if err != nil {
				log.Warn(log.EVM, "Failed to compute verkle root", "address", address.Hex(), "error", err)
				// Continue without verkle root - this is not a fatal error
			} else {
				log.Trace(log.EVM, "Computed verkle root", "address", address.Hex(), "verkleRoot", verkleRoot.Hex())
			}

			// Load into witness cache
			s.sdb.SetContractStorage(address, evmtypes.ContractStorage{Shard: *contractStorage})

			log.Trace(log.EVM, "parseAndLoadContractStorage complete", "address", address.Hex(), "entries", len(contractStorage.Entries), "verkleRoot", verkleRoot.Hex())
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

	return fmt.Sprintf("%s PC:%d Gas:%d Registers:%s", opcode_str(opcode), prevpc, int64(gas), regStr)
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
	authGasUsed = types.IsAuthorizedGasAllocation - vm_auth.GetGas()

	return
}

// NOTE: the refinecontext is NOT used here
func (s *StateDB) ExecuteWorkPackageBundle(workPackageCoreIndex uint16, package_bundle types.WorkPackageBundle, segmentRootLookup types.SegmentRootLookup, slot uint32, execContext string, eventID uint64, pvmBackend string, logDir string) (work_report types.WorkReport, err error) {
	importsegments := make([][][]byte, len(package_bundle.WorkPackage.WorkItems))
	results := []types.WorkDigest{}
	saveToLogDir(logDir, "bundle.bin", package_bundle.Bytes())
	saveToLogDir(logDir, "bundle.json", []byte(types.ToJSON(package_bundle)))
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
		result := types.WorkDigest{
			ServiceID:           workItem.Service,
			CodeHash:            workItem.CodeHash,
			PayloadHash:         common.Blake2Hash(workItem.Payload),
			Gas:                 workItem.AccumulateGasLimit,
			GasUsed:             uint(workItem.RefineGasLimit - uint64(vm.GetGas())),
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
			gasUsed := workItem.RefineGasLimit - uint64(vm.GetGas())
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
	log.Trace(log.Node, "executeWorkPackageBundle", "backend", pvmBackend, "role", vmLogging, "workreport", workReport.String())

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
	saveToLogDir(logDir, "bclubs", serializeHashes(d.BClubs))
	saveToLogDir(logDir, "sclubs", serializeHashes(d.SClubs))
	saveToLogDir(logDir, "bundle_chunks", serializeECChunks(d.BundleChunks))
	saveToLogDir(logDir, "segment_chunks", serializeECChunks(d.SegmentChunks))
	saveToLogDir(logDir, "workreportderivation.json", []byte(types.ToJSON(d)))
	saveToLogDir(logDir, "workreport.bin", workReport.Bytes())
	saveToLogDir(logDir, "workreport.json", []byte(types.ToJSON(workReport)))
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
	oldPvmLogging := PvmLogging
	PvmLogging = true
	defer func() {
		PvmLogging = oldPvmLogging
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
func (s *StateDB) BuildBundle(workPackage types.WorkPackage, extrinsicsBlobs []types.ExtrinsicsBlobs, coreIndex uint16, rawObjectIDs []common.Hash, pvmBackend string) (b *types.WorkPackageBundle, wr *types.WorkReport, err error) {
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
		result, _, exported_segments := vm.ExecuteRefine(coreIndex, uint32(index), wp, authorization, importsegments, 0, extrinsicsBlobs[index], p_a, common.BytesToHash(trie.H0), "")

		// Post-SSR: Parse refine output to populate witness cache from exported segments
		// Output format: [2B count] + count × [32B object_id + 2B index_start + 4B payload_length + 1B object_kind]
		if err := s.populateWitnessCacheFromRefineOutput(workItem.Service, result.Ok, exported_segments); err != nil {
			log.Warn(log.EVM, "Failed to populate witness cache from refine output", "error", err)
		}

		importedSegments, witnesses, err := vm.GetBuilderWitnesses()
		if err != nil {
			log.Warn(log.DA, "BuildBundle: GetBuilderWitnesses failed", "err", err)
			return nil, nil, err
		}

		// Generate Verkle witness for builder mode (Step 2: Builder Refine)
		// Extract contract witness blob from refine result output
		contractWitnessBlob, err := s.extractContractWitnessBlob(result.Ok, exported_segments)
		if err != nil {
			log.Warn(log.EVM, "Failed to extract contract witness blob", "err", err)
			return nil, nil, err
		}
		contractWitnessBlobs[index] = contractWitnessBlob // Store for later application to main tree

		// Build verkle witness - all verkle logic handled in storage
		verkleWitnessBytes, err := s.sdb.BuildVerkleWitness(contractWitnessBlob)
		if err != nil {
			log.Warn(log.EVM, "BuildVerkleWitness failed", "err", err)
			return nil, nil, err
		}

		// Build Block Access List from witness (Phase 4)
		// This computes the BAL hash that will be embedded in the block payload
		blockAccessListHash, accountCount, totalChanges, err := s.sdb.ComputeBlockAccessListHash(verkleWitnessBytes)
		if err != nil {
			log.Warn(log.EVM, "ComputeBlockAccessListHash failed", "err", err)
			// Non-fatal: continue without BAL hash (will be zeros)
			blockAccessListHash = common.Hash{}
		} else {
			log.Trace(log.EVM, "Block Access List computed", "hash", blockAccessListHash.Hex(), "accounts", accountCount, "changes", totalChanges)
		}

		// Extract post-state Verkle root from witness to update block payload
		postStateRoot, err := extractPostStateRootFromWitness(verkleWitnessBytes)
		if err != nil {
			log.Warn(log.EVM, "Failed to extract post-state root from witness", "err", err)
			postStateRoot = common.Hash{} // leave unchanged if extraction fails
		}

		// Update block payload with BAL hash and post-state root (Phase 4)
		// The block was already exported during ExecuteRefine, so we need to update it in exported_segments
		err = s.updateBlockPayload(exported_segments, blockAccessListHash, postStateRoot)
		if err != nil {
			log.Warn(log.EVM, "Failed to update block payload", "err", err)
			// Non-fatal: continue with original block payload (BAL hash/root may be zeros)
		}

		// Prepend Verkle witness as FIRST extrinsic
		extrinsicsBlobs[index] = append([][]byte{verkleWitnessBytes}, extrinsicsBlobs[index]...)

		// Update work item extrinsics to include Verkle witness
		verkleWitnessExtrinsic := types.WorkItemExtrinsic{
			Hash: common.Blake2Hash(verkleWitnessBytes),
			Len:  uint32(len(verkleWitnessBytes)),
		}
		// Prepend to extrinsics list
		wp.WorkItems[index].Extrinsics = append([]types.WorkItemExtrinsic{verkleWitnessExtrinsic}, wp.WorkItems[index].Extrinsics...)

		// Verify post-state against execution
		if err := s.verifyPostStateAgainstExecution(wp.WorkItems[index], extrinsicsBlobs[index], contractWitnessBlob); err != nil {
			log.Warn(log.EVM, "Post-state verification failed", "err", err)
			return nil, nil, fmt.Errorf("post-state verification failed: %w", err)
		}

		// Clear verkleReadLog for next execution via clean interface
		s.sdb.ClearVerkleReadLog()

		wp.WorkItems[index].ExportCount = uint16(len(exported_segments))
		wp.WorkItems[index].ImportedSegments = importedSegments
		// Append builder witnesses to extrinsicsBlobs -- this will be the metashards + the object proofs
		builderWitnessCount := appendExtrinsicWitnessesToWorkItem(&wp.WorkItems[index], &extrinsicsBlobs, index, witnesses)
		log.Trace(log.DA, "BuildBundle: Appended builder witnesses", "workItemIndex", index, "builderWitnessCount", builderWitnessCount, "totalExtrinsics", len(extrinsicsBlobs[index]))
		// Update payload metadata with builder witness count and BAL hash
		// ALWAYS update if payload exists (even if builderWitnessCount=0) to ensure BAL hash is included
		if len(wp.WorkItems[index].Payload) >= 7 {
			totalWitnessCount := uint16(builderWitnessCount)
			wp.WorkItems[index].Payload = BuildPayload(PayloadTypeTransactions, int(originalTxCount), 0, int(totalWitnessCount), blockAccessListHash)
		}

		// Append exported segments (append slice directly)
		//segments = append(segments, exported_segments...)

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
			GasUsed:             uint(workItem.RefineGasLimit - uint64(vm.GetGas())),
			NumImportedSegments: uint(len(importedSegments)),
			NumExportedSegments: uint(len(exported_segments)),
			NumExtrinsics:       uint(len(extrinsicsBlobs[index])),
			NumBytesExtrinsics:  uint(totalExtrinsicBytes),
			Result:              result,
		})
	}

	// Use buildBundle to fetch imported segments and justifications
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
		Results: results,
	}
	log.Trace(log.Node, "BuildBundle: Built", "payload", fmt.Sprintf("%x", bundle.WorkPackage.WorkItems[0].Payload))

	for i, blob := range contractWitnessBlobs {
		if len(blob) > 0 {
			if err := s.sdb.ApplyContractWrites(blob); err != nil {
				return &bundle, workReport, fmt.Errorf("failed to apply contract writes for work item %d: %v", i, err)
			}
			log.Trace(log.EVM, "Applied contract writes to state", "workItemIndex", i, "blobSize", len(blob))
		}
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

	chunkSize := (types.SegmentSize / (types.TotalValidators / 3))
	for shardIndex, ec := range ecChunksArr {
		chunks := make([][]byte, len(segments)+len(pageProofs))
		for n := 0; n < len(chunks); n++ {
			chunks[n] = ec.Data[n*chunkSize : (n+1)*chunkSize]
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

// computeVerkleRoot creates a verkle tree from contract storage entries and computes its root
func computeVerkleRoot(address common.Address, contractStorage *evmtypes.ContractShard) (common.Hash, error) {
	log.Debug(log.EVM, "computeVerkleRoot", "address", address.Hex(), "entries", len(contractStorage.Entries))

	// Create a new verkle tree
	tree := verkle.New()

	// Insert each storage entry into the verkle tree
	for _, entry := range contractStorage.Entries {
		// Use the storage key hash as the verkle key
		key := entry.KeyH[:]
		value := entry.Value[:]

		err := tree.Insert(key, value, nil)
		if err != nil {
			return common.Hash{}, fmt.Errorf("failed to insert key %x into verkle tree: %v", key, err)
		}

		log.Info(log.EVM, "computeVerkleRoot: inserted entry", "key", fmt.Sprintf("%x", key), "value", fmt.Sprintf("%x", value))
	}

	// Compute the commitment (verkle root)
	commitment := tree.Commit()
	if commitment == nil {
		return common.Hash{}, fmt.Errorf("failed to compute verkle commitment")
	}

	// Get the hash representation of the commitment
	hashFr := tree.Hash()
	if hashFr == nil {
		return common.Hash{}, fmt.Errorf("failed to get verkle hash")
	}

	// Convert the field element to bytes and then to Hash
	// Note: This conversion may need adjustment based on the actual field element representation
	hashBytes := hashFr.Bytes()
	verkleRoot := common.BytesToHash(hashBytes[:])

	log.Info(log.EVM, "computeVerkleRoot complete", "address", address.Hex(), "verkleRoot", verkleRoot.Hex(), "entries", len(contractStorage.Entries))

	return verkleRoot, nil
}

// updateBlockPayload updates the block payload in exported segments with the computed BAL hash
// and optionally the post-state Verkle root (if provided).
// The block is the first exported segment (ObjectKind::Block is exported first in refiner.rs)
func (s *StateDB) updateBlockPayload(exportedSegments [][]byte, balHash common.Hash, postStateRoot common.Hash) error {
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

	// Update Verkle root if provided (non-zero)
	if postStateRoot != (common.Hash{}) {
		blockPayload.VerkleRoot = postStateRoot
	}

	// Re-serialize the block payload
	updatedSegment := evmtypes.SerializeEvmBlockPayload(blockPayload)

	// Replace the first segment with the updated one
	exportedSegments[0] = updatedSegment

	log.Debug(log.EVM, "Updated block payload with BAL hash", "hash", balHash.Hex(), "segment_size", len(updatedSegment))

	return nil
}

// extractPostStateRootFromWitness extracts the post-state Verkle root from a serialized witness
// Witness format: 32B pre_root + 4B read_count + (161B * reads) + 4B pre_proof_len + pre_proof +
//
//	32B post_root + ...
func extractPostStateRootFromWitness(witness []byte) (common.Hash, error) {
	if len(witness) < 68 {
		return common.Hash{}, fmt.Errorf("witness too short")
	}

	offset := 32 // skip pre_root

	if len(witness)-offset < 4 {
		return common.Hash{}, fmt.Errorf("witness missing read_count")
	}
	readCount := binary.BigEndian.Uint32(witness[offset : offset+4])
	offset += 4

	entryLen := 161
	need := int(readCount) * entryLen
	if len(witness)-offset < need {
		return common.Hash{}, fmt.Errorf("witness truncated in read entries: need %d", need)
	}
	offset += need

	if len(witness)-offset < 4 {
		return common.Hash{}, fmt.Errorf("witness missing pre_proof_len")
	}
	preProofLen := binary.BigEndian.Uint32(witness[offset : offset+4])
	offset += 4

	if len(witness)-offset < int(preProofLen) {
		return common.Hash{}, fmt.Errorf("witness truncated in pre_proof: need %d", preProofLen)
	}
	offset += int(preProofLen)

	if len(witness)-offset < 32 {
		return common.Hash{}, fmt.Errorf("witness missing post_root")
	}
	var postRoot common.Hash
	copy(postRoot[:], witness[offset:offset+32])
	return postRoot, nil
}
