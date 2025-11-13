package node

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	chainspecs "github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	log "github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	storage "github.com/colorfulnotion/jam/storage"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	types "github.com/colorfulnotion/jam/types"
)

const (
	debugSpec = false
)

type SpecTestCase struct {
	B                string                      `json:"b"`
	BClubs           []common.Hash               `json:"bClubs,omitempty"`
	BShards          []string                    `json:"bShards,omitempty"`
	SClubs           []common.Hash               `json:"sClubs,omitempty"`
	SShards          []string                    `json:"sShards,omitempty"`
	AvailabilitySpec types.AvailabilitySpecifier `json:"package_spec,omitempty"`
}

func testAvailabilitySpec(t *testing.T, exportCount uint16) {
	b := []byte{0x14, 0x21, 0xdd, 0xac, 0x87, 0x3a, 0x19, 0x9a, 0x7c, 0x87}
	bLength := uint32(len(b))

	packageHash := common.ComputeHash(b)
	// Compute a hash chain of 32-byte hashes using common.ComputeHash
	h := common.ComputeHash([]byte{})

	// Fill 65 segments of 4104 bytes each with hashes
	if debugSpec {
		fmt.Printf("H('') = %x  ", h)
	}
	segments := make([][]byte, exportCount)
	for i := range segments {
		segments[i] = make([]byte, 4104)
		for j := 0; j < 4104; j += 32 {
			copy(segments[i][j:], h[:])
			h = common.ComputeHash(h)
			if debugSpec {
				if i == 0 {
					if j < 128 {
						fmt.Printf("<= %x ", h)
					} else if j == 128 {
						fmt.Printf("\n")
					}
				}
			}
		}
		if debugSpec {
			fmt.Printf("segment %d: %x... (%d bytes)\n", i, segments[i][0:36], len(segments[i]))
		}
	}
	// Build b♣ and s♣
	bClubs, bShards := statedb.BuildBClub(b)
	sClubs, sShards := statedb.BuildSClub(segments)
	// u = (bClub, sClub)
	erasure_root_u := statedb.GenerateErasureRoot(bClubs, sClubs)
	exported_segment_root_e := statedb.GenerateExportedSegmentsRoot(segments)

	// Return the Availability Specifier
	availabilitySpecifier := types.AvailabilitySpecifier{
		WorkPackageHash:       common.BytesToHash(packageHash),
		BundleLength:          bLength,
		ErasureRoot:           erasure_root_u,
		ExportedSegmentRoot:   exported_segment_root_e,
		ExportedSegmentLength: uint16(exportCount),
	}

	bchunks := make([]string, len(bShards))
	for i, bS := range bShards {
		bchunks[i] = fmt.Sprintf("0x%x", bS.Data)
	}
	schunks := make([]string, len(sShards))
	for i, sS := range sShards {
		schunks[i] = fmt.Sprintf("0x%x", sS.Data)
	}

	tc := SpecTestCase{
		B:                fmt.Sprintf("0x%x", b),
		BClubs:           bClubs,
		BShards:          bchunks,
		SClubs:           sClubs,
		SShards:          schunks,
		AvailabilitySpec: availabilitySpecifier,
	}
	fn := fmt.Sprintf("spec-exportcount-%d.json", exportCount)
	// Save tc in a JSON file
	jsonData, err := json.MarshalIndent(tc, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal SpecTestCase to JSON: %v", err)
	}
	err = os.WriteFile(fn, jsonData, 0644)
	if err != nil {
		t.Fatalf("Failed to write JSON file: %v", err)
	}
	fmt.Printf("AvailabilitySpecifier (exportCount=%d): %s\n", exportCount, availabilitySpecifier.String())
}

func TestAvailabilitySpec(t *testing.T) {
	exportCounts := []uint16{1, 2, 3, 64, 65, 128, 129}
	for _, exportCount := range exportCounts {
		testAvailabilitySpec(t, exportCount)
	}
}

func TestBootstrapCodeFromSpec(t *testing.T) {

	log.InitLogger("trace")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule(log.FirstGuarantorOrAuditor)
	log.EnableModule(log.GeneralAuthoring)
	chainspec, err := chainspecs.ReadSpec("dev")
	if err != nil {
		t.Fatalf("Failed to read chainspec: %v", err)
	}
	stateTransition := &statedb.StateTransition{}
	stateTransition.PreState.KeyVals = chainspec.GenesisState
	stateTransition.PreState.StateRoot = common.Hash{}
	stateTransition.PostState.KeyVals = chainspec.GenesisState
	stateTransition.PostState.StateRoot = common.Hash{}
	header, _, err := types.Decode(chainspec.GenesisHeader, reflect.TypeOf(types.BlockHeader{}))
	if err != nil {
		t.Fatalf("Failed to decode genesis header: %v", err)
	}
	stateTransition.Block.Header = header.(types.BlockHeader)
	levelDBPath := "/tmp/testdb"
	store, err := storage.NewStateDBStorage(levelDBPath, storage.NewMockJAMDA(), telemetry.NewNoOpTelemetryClient(), 0)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)

	}
	s, err := statedb.NewStateDBFromStateTransition(store, stateTransition)
	if err != nil {
		t.Fatalf("Failed to create StateDB: %v", err)
	}
	workPackageJson := `{
  "authorization": "0x",
  "auth_code_host": 0,
  "authorizer": {
    "code_hash": "0xfa002583136a79daec5ca5e802d27732517c3f7dc4970dc2d68abcaad891e99d",
    "params": "0x"
  },
  "context": {
    "anchor": "0xd7230738d0de15491eb3fae7157098358085a3f9ee200a8b025e121ae3da8a4b",
    "state_root": "0xfbf4d042b434faf587814f3b09154da0a8ec9952afaa6f9eceaacbbdee3aa82e",
    "beefy_root": "0x21ddb9a356815c3fac1026b6dec5df3124afbadb485c9ba5a3e3398a04b7ba85",
    "lookup_anchor": "0xf1b7a6336887739ea34fa1eec605b12fc2c4392b9694df0a7333f5dde173fdc7",
    "lookup_anchor_slot": 2031193,
    "prerequisites": []
  },
  "items": [
    {
      "service": 20,
      "code_hash": "0x5e95b316ba390ee50c595f0e10923a24831647f1d3f800942b5a58f9972baf51",
      "payload": "0x06e90be226abaccc638a0addae983f814190ba97367f8abc5e21642a1e48f23d577241000000000000",
      "refine_gas_limit": 1600000000,
      "accumulate_gas_limit": 10000000,
      "import_segments": [],
      "extrinsic": [],
      "export_count": 0
    }
  ]
}`

	workPackage := types.WorkPackage{}
	err = json.Unmarshal([]byte(workPackageJson), &workPackage)
	if err != nil {
		t.Fatalf("Failed to unmarshal work package: %v", err)
	}
	var service_index uint32
	// Import Segments
	authcode, _, authindex, err := s.GetAuthorizeCode(workPackage)
	if err != nil {
		return
	}

	pvmStart := time.Now()

	vm_auth := statedb.NewVMFromCode(authindex, authcode, 0, 0, s, statedb.BackendInterpreter, types.IsAuthorizedGasAllocation)
	if vm_auth == nil {
		t.Fatalf("Failed to create VM for authorization (corrupted bytecode?)")
	}
	workPackageCoreIndex := uint16(0)
	r := vm_auth.ExecuteAuthorization(workPackage, workPackageCoreIndex)
	p_u := workPackage.AuthorizationCodeHash
	p_p := workPackage.ConfigurationBlob
	p_a := common.Blake2Hash(append(p_u.Bytes(), p_p...))

	var segments [][]byte

	for index, workItem := range workPackage.WorkItems {
		// map workItem.ImportedSegments into segment
		service_index = workItem.Service
		code, ok, err0 := s.ReadServicePreimageBlob(service_index, workItem.CodeHash)
		if err0 != nil || !ok || len(code) == 0 {
			pvmFailedElapsed := common.Elapsed(pvmStart)
			t.Fatalf("Failed to read service preimage blob: %v, elapsed: %d", err0, pvmFailedElapsed)
		}
		if common.Blake2Hash(code) != workItem.CodeHash {
			log.Crit(log.Node, "executeWorkPackageBundle: Code and CodeHash Mismatch")
		}
		// fmt.Printf("index %d, code len=%d\n", service_index, len(code))
		vm := statedb.NewVMFromCode(service_index, code, 0, 0, s, statedb.BackendInterpreter, workItem.RefineGasLimit)
		if vm == nil {
			t.Fatalf("Failed to create VM for service %d (corrupted bytecode?)", service_index)
		}
		vm.Timeslot = s.JamState.SafroleState.Timeslot

		output, _, exported_segments := vm.ExecuteRefine(workPackageCoreIndex, uint32(index), workPackage, r, make([][][]byte, 0), workItem.ExportCount, types.ExtrinsicsBlobs{}, p_a, common.Hash{})

		expectedSegmentCnt := int(workItem.ExportCount)
		if expectedSegmentCnt != len(exported_segments) {
			log.Warn(log.Node, "executeWorkPackageBundle: ExportCount and ExportedSegments Mismatch", "ExportCount", expectedSegmentCnt, "ExportedSegments", len(exported_segments), "ExportedSegments", common.FormatPaddedBytesArray(exported_segments, 20))
			expectedSegmentCnt = len(exported_segments)
		}
		if expectedSegmentCnt != 0 {
			for i := 0; i < expectedSegmentCnt; i++ {
				segment := common.PadToMultipleOfN(exported_segments[i], types.SegmentSize)
				segments = append(segments, segment)
			}
		}
		z := 0
		for _, extrinsic := range workItem.Extrinsics {
			z += int(extrinsic.Len)
		}
		result := types.WorkDigest{
			ServiceID:           workItem.Service,
			CodeHash:            workItem.CodeHash,
			PayloadHash:         common.Blake2Hash(workItem.Payload),
			Gas:                 workItem.AccumulateGasLimit, // put a
			GasUsed:             uint(workItem.RefineGasLimit - uint64(vm.GetGas())),
			NumImportedSegments: uint(len(workItem.ImportedSegments)),
			NumExportedSegments: uint(expectedSegmentCnt),
			NumExtrinsics:       0,
			NumBytesExtrinsics:  uint(z),
		}

		result.Result = output
		fmt.Printf("result: %x\n", result.Result)

		o := types.AccumulateOperandElements{
			WorkPackageHash:     common.Hash{},
			ExportedSegmentRoot: common.Hash{},
			AuthorizerHash:      p_a,
			Trace:               r.Ok,
			PayloadHash:         result.PayloadHash,
			Gas:                 uint(result.Gas),
			Result:              result.Result,
		}
		fmt.Printf("o: %x res: %s\n", o, result.String())
	}
	fmt.Printf("segments count: %d\n", len(segments))
}
