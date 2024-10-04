package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

type DAvailabilityAssignments [types.TotalCores]*DRho_state
type DRho_state struct {
	DummyWorkReport [353]byte `json:"dummy_work_report"`
	Timeout         uint32    `json:"timeout"`
}

type DJamState struct {
	Psi    Psi_state                `json:"psi"`
	Rho    DAvailabilityAssignments `json:"rho"`
	Tau    uint32                   `json:"tau"`
	Kappa  Validators               `json:"kappa"`
	Lambda Validators               `json:"lambda"`
}

type DInput struct {
	Disputes types.Dispute `json:"disputes"`
}

type DisputeData struct {
	Input    DInput    `json:"input"`
	PreState DJamState `json:"pre_state"`
	// Output    DOutput   `json:"output"`
	PostState DJamState `json:"post_state"`
}

func (a *DRho_state) UnmarshalJSON(data []byte) error {
	var s struct {
		DummyWorkReport string `json:"dummy_work_report"`
		Timeout         uint32 `json:"timeout"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	dummy_work_report := common.FromHex(s.DummyWorkReport)
	copy(a.DummyWorkReport[:], dummy_work_report)
	a.Timeout = s.Timeout

	return nil
}

func (a *DRho_state) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		DummyWorkReport string `json:"dummy_work_report"`
		Timeout         uint32 `json:"timeout"`
	}{
		DummyWorkReport: common.HexString(a.DummyWorkReport[:]),
		Timeout:         a.Timeout,
	})
}

func TestDisputeState(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"progress_with_bad_signatures-1.json", "progress_with_bad_signatures-1.bin", &DisputeData{}},
		{"progress_with_bad_signatures-2.json", "progress_with_bad_signatures-2.bin", &DisputeData{}},
		{"progress_with_culprits-1.json", "progress_with_culprits-1.bin", &DisputeData{}},
		{"progress_with_culprits-2.json", "progress_with_culprits-2.bin", &DisputeData{}},
		{"progress_with_culprits-3.json", "progress_with_culprits-3.bin", &DisputeData{}},
		{"progress_with_culprits-4.json", "progress_with_culprits-4.bin", &DisputeData{}},
		{"progress_with_culprits-5.json", "progress_with_culprits-5.bin", &DisputeData{}},
		{"progress_with_culprits-6.json", "progress_with_culprits-6.bin", &DisputeData{}},
		{"progress_with_culprits-7.json", "progress_with_culprits-7.bin", &DisputeData{}},
		{"progress_with_faults-1.json", "progress_with_faults-1.bin", &DisputeData{}},
		{"progress_with_faults-2.json", "progress_with_faults-2.bin", &DisputeData{}},
		{"progress_with_faults-3.json", "progress_with_faults-3.bin", &DisputeData{}},
		{"progress_with_faults-4.json", "progress_with_faults-4.bin", &DisputeData{}},
		{"progress_with_faults-5.json", "progress_with_faults-5.bin", &DisputeData{}},
		{"progress_with_faults-6.json", "progress_with_faults-6.bin", &DisputeData{}},
		{"progress_with_faults-7.json", "progress_with_faults-7.bin", &DisputeData{}},
		{"progress_with_no_verdicts-1.json", "progress_with_no_verdicts-1.bin", &DisputeData{}},
		{"progress_with_verdict_signatures_from_previous_set-1.json", "progress_with_verdict_signatures_from_previous_set-1.bin", &DisputeData{}},
		{"progress_with_verdict_signatures_from_previous_set-2.json", "progress_with_verdict_signatures_from_previous_set-2.bin", &DisputeData{}},
		{"progress_with_verdicts-1.json", "progress_with_verdicts-1.bin", &DisputeData{}},
		{"progress_with_verdicts-2.json", "progress_with_verdicts-2.bin", &DisputeData{}},
		{"progress_with_verdicts-3.json", "progress_with_verdicts-3.bin", &DisputeData{}},
		{"progress_with_verdicts-4.json", "progress_with_verdicts-4.bin", &DisputeData{}},
		{"progress_with_verdicts-5.json", "progress_with_verdicts-5.bin", &DisputeData{}},
		{"progress_with_verdicts-6.json", "progress_with_verdicts-6.bin", &DisputeData{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/disputes/tiny", tc.jsonFile)
			// binPath := filepath.Join("../jamtestvectors/history/data", tc.binFile)

			targetedStructType := reflect.TypeOf(tc.expectedType)

			fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.expectedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			fmt.Printf("Unmarshaled %s\n", jsonPath)
			fmt.Printf("Expected: %v\n", tc.expectedType)
			// Encode the struct to bytes
			encodedBytes := types.Encode(tc.expectedType)

			fmt.Printf("Encoded: %x\n\n", encodedBytes)

			decodedStruct, _ := types.Decode(encodedBytes, targetedStructType)
			fmt.Printf("Decoded:  %v\n\n", decodedStruct)

			// Marshal the struct to JSON
			encodedJSON, err := json.MarshalIndent(decodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			fmt.Printf("Encoded JSON:\n%s\n", encodedJSON)

			// output bin file
			// err = os.WriteFile("./output.bin", encodedBytes, 0644)
			// if err != nil {
			// 	t.Fatalf("failed to write binary file: %v", err)
			// }

			// Read the expected bytes from the binary file
			// expectedBytes, err := os.ReadFile(binPath)
			// if err != nil {
			// 	t.Fatalf("failed to read binary file: %v", err)
			// }
			// assert.Equal(t, expectedBytes, encodedBytes, "encoded bytes do not match expected bytes")

			// if false {
			// 	decoded, _ := types.Decode(expectedBytes, reflect.TypeOf(tc.expectedType))
			// 	encodedBytes2 := types.Encode(decoded)
			// 	// Compare the encoded bytes with the expected bytes
			// 	assert.Equal(t, expectedBytes, encodedBytes2, "encoded bytes do not match expected bytes")
			// }

			// // Compare the encoded JSON with the original JSON
			// assert.JSONEq(t, string(jsonData), string(encodedJSON), "encoded JSON does not match original JSON")
		})
	}
}

// type HexBytes []byte

// =========================Test Function=========================
// func TestReadAndConvertJson(t *testing.T) {
// 	filePath := "../jamtestvectors/disputes/full/progress_with_culprits-7.json"
// 	fmt.Println("Start File Name: ", filePath)
// 	disputeData, err := ReadAndConvertJson(filePath)
// 	if err != nil {
// 		t.Fatalf("Failed to read and convert JSON: %v, file name %s", err, filePath)
// 	}
// 	_ = disputeData
// 	fmt.Println("Finish File Name: ", filePath)
// }
// func TestReadAndConvertJson_All(t *testing.T) {
// 	dir_path := "../jamtestvectors/disputes/full/"
// 	files, err := ioutil.ReadDir(dir_path)
// 	if err != nil {
// 		log.Fatalf("Failed to read directory: %v", err)
// 	}
// 	for _, file := range files {

// 		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
// 			continue
// 		}
// 		filePath := dir_path + file.Name()
// 		fmt.Println("Start File Name: ", file.Name())
// 		disputeData, err := ReadAndConvertJson(filePath)
// 		if err != nil {
// 			t.Fatalf("Failed to read and convert JSON: %v, file name %s", err, filePath)
// 		}
// 		_ = disputeData
// 		fmt.Println("Finish File Name: ", file.Name())
// 	}

// }

// func TestDispute_State(t *testing.T) {
// 	filePath := "../jamtestvectors/disputes/full/progress_invalidates_avail_assignments-1.json"
// 	disputeData, _ := ReadAndConvertJson(filePath)
// 	fmt.Println("=====================================================")
// 	oMark, err := disputeData.PreState.Disputes(&disputeData.Input)
// 	//test if the postState is equal to the expected postState
// 	fmt.Println("PostState Actual:")
// 	PrettyPrint(disputeData.PreState)
// 	fmt.Println("PostState Expected:")
// 	PrettyPrint(disputeData.PostState)
// 	if err != nil {
// 		fmt.Println("Error: ", err)
// 	}
// 	assert.Equal(t, disputeData.PostState, disputeData.PreState)
// 	assert.Equal(t, disputeData.Output.DOk.OffenderMark, oMark.OffenderKey)

// 	// test if the output is equal to the expected output
// }
// func TestDispute_State_Full(t *testing.T) {
// 	dir_path := "../jamtestvectors/disputes/full/"
// 	files, err := ioutil.ReadDir(dir_path)
// 	if err != nil {
// 		log.Fatalf("Failed to read directory: %v", err)
// 	}
// 	for _, file := range files {

// 		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
// 			continue
// 		}
// 		filePath := dir_path + file.Name()
// 		fmt.Println("=====================================================")
// 		fmt.Printf("Start File Name: %s\n", file.Name())
// 		disputeData, _ := ReadAndConvertJson(filePath)
// 		Omark, err := disputeData.PreState.Disputes(&disputeData.Input)
// 		//test if the postState is equal to the expected postState
// 		assert.Equal(t, disputeData.PostState, disputeData.PreState)
// 		if err != nil {
// 			fmt.Println("Actual Error: ", err)
// 			fmt.Println("Expected Error: ", disputeData.Output.Err)
// 		} else {
// 			assert.Equal(t, disputeData.Output.DOk.OffenderMark, Omark.OffenderKey)
// 		}
// 		// test if the output is equal to the expected output
// 		fmt.Printf("Finish File Name: %s\n", file.Name())
// 		fmt.Println("=====================================================")
// 	}
// }
// func TestDispute_State_Tiny(t *testing.T) {
// 	dir_path := "../jamtestvectors/disputes/tiny/"
// 	files, err := ioutil.ReadDir(dir_path)
// 	if err != nil {
// 		log.Fatalf("Failed to read directory: %v", err)
// 	}
// 	for _, file := range files {

// 		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
// 			continue
// 		}
// 		filePath := dir_path + file.Name()
// 		fmt.Println("=====================================================")
// 		fmt.Printf("Start File Name: %s\n", file.Name())
// 		disputeData, _ := ReadAndConvertJson(filePath)
// 		disputeData.PreState.Disputes(&disputeData.Input)
// 		//test if the postState is equal to the expected postState
// 		assert.Equal(t, disputeData.PostState, disputeData.PreState)
// 		// test if the output is equal to the expected output
// 		fmt.Printf("Finish File Name: %s\n", file.Name())
// 		fmt.Println("=====================================================")
// 	}
// }

// // ========================= Helper Functions ====================
// func (hb HexBytes) MarshalJSON() ([]byte, error) {
// 	hexStr := hex.EncodeToString(hb)
// 	return json.Marshal(hexStr)
// }
// func PrettyPrint(data interface{}) {
// 	prettyJSON, _ := json.MarshalIndent(data, "", "  ")
// 	fmt.Println(string(prettyJSON))
// }

// func ReadAndConvertJson(filePath string) (DisputeData, error) {
// 	data, err := ioutil.ReadFile(filePath)
// 	if err != nil {
// 		log.Fatalf("Failed to read JSON file: %v", err)
// 	}
// 	if len(data) == 0 {
// 		log.Fatalf("Empty JSON file: %v", err)
// 	}

// 	var rawDisputeData struct {
// 		Input struct {
// 			Disputes struct {
// 				Verdicts []struct {
// 					Target string `json:"target"`
// 					Age    uint32 `json:"age"`
// 					Votes  []struct {
// 						Vote      bool   `json:"vote"`
// 						Index     uint16 `json:"index"`
// 						Signature string `json:"signature"`
// 					} `json:"votes"`
// 				} `json:"verdicts"`
// 				Culprits []struct {
// 					Target    string `json:"target"`
// 					Key       string `json:"key"`
// 					Signature string `json:"signature"`
// 				} `json:"culprits"`
// 				Faults []struct {
// 					Target    string `json:"target"`
// 					Vote      bool   `json:"vote"`
// 					Key       string `json:"key"`
// 					Signature string `json:"signature"`
// 				} `json:"faults"`
// 			} `json:"disputes"`
// 		} `json:"input"`
// 		PreState struct {
// 			Psi struct {
// 				PsiG []string `json:"psi_g"`
// 				PsiB []string `json:"psi_b"`
// 				PsiW []string `json:"psi_w"`
// 				PsiO []string `json:"psi_o"`
// 			} `json:"psi"`
// 			Rho []struct {
// 				DummyWorkReport string `json:"dummy_work_report"`
// 				Timeout         uint32 `json:"timeout"`
// 			} `json:"rho"`
// 			Tau   uint32 `json:"tau"`
// 			Kappa []struct {
// 				Ed25519      string `json:"ed25519"`
// 				Bandersnatch string `json:"bandersnatch"`
// 				Bls          string `json:"bls"`
// 				Metadata     string `json:"metadata"`
// 			} `json:"kappa"`
// 			Lambda []struct {
// 				Ed25519      string `json:"ed25519"`
// 				Bandersnatch string `json:"bandersnatch"`
// 				Bls          string `json:"bls"`
// 				Metadata     string `json:"metadata"`
// 			}
// 		} `json:"pre_state"`
// 		Output struct {
// 			DOk *struct {
// 				OffenderMark []string `json:"offenders_mark"`
// 			} `json:"ok"`
// 			Err string `json:"err"`
// 		} `json:"output"`
// 		PostState struct {
// 			Psi struct {
// 				PsiG []string `json:"psi_g"`
// 				PsiB []string `json:"psi_b"`
// 				PsiW []string `json:"psi_w"`
// 				PsiO []string `json:"psi_o"`
// 			} `json:"psi"`
// 			Rho []struct {
// 				DummyWorkReport string `json:"dummy_work_report"`
// 				Timeout         uint32 `json:"timeout"`
// 			} `json:"rho"`
// 			Tau   uint32 `json:"tau"`
// 			Kappa []struct {
// 				Ed25519      string `json:"ed25519"`
// 				Bandersnatch string `json:"bandersnatch"`
// 				Bls          string `json:"bls"`
// 				Metadata     string `json:"metadata"`
// 			} `json:"kappa"`
// 			Lambda []struct {
// 				Ed25519      string `json:"ed25519"`
// 				Bandersnatch string `json:"bandersnatch"`
// 				Bls          string `json:"bls"`
// 				Metadata     string `json:"metadata"`
// 			}
// 		} `json:"post_state"`
// 	}

// 	err = json.Unmarshal(data, &rawDisputeData)
// 	if err != nil {
// 		log.Fatalf("Failed to unmarshal JSON: %v", err)
// 	}

// 	var disputeData DisputeData

// 	// Convert Input.Verdicts
// 	for _, verdict := range rawDisputeData.Input.Disputes.Verdicts {
// 		verdictStruct := types.Verdict{
// 			Target: common.Hex2Hash(verdict.Target),
// 			Epoch:  verdict.Age,
// 		}
// 		for i, vote := range verdict.Votes {
// 			v := types.Vote{
// 				Voting:    vote.Vote,
// 				Index:     vote.Index,
// 				Signature: types.HexToEd25519Sig(vote.Signature),
// 			}
// 			verdictStruct.Votes[i] = v
// 		}
// 		disputeData.Input.Verdict = append(disputeData.Input.Verdict, verdictStruct)
// 	}

// 	// Convert Input.Culprits
// 	for _, culprit := range rawDisputeData.Input.Disputes.Culprits {
// 		c := types.Culprit{
// 			Target:    common.Hex2Hash(culprit.Target),
// 			Key:       types.HexToEd25519Key(culprit.Key),
// 			Signature: types.HexToEd25519Sig(culprit.Signature),
// 		}
// 		disputeData.Input.Culprit = append(disputeData.Input.Culprit, c)
// 	}

// 	// Convert Input.Faults
// 	for _, fault := range rawDisputeData.Input.Disputes.Faults {
// 		f := types.Fault{
// 			Target:    common.Hex2Hash(fault.Target),
// 			Voting:    fault.Vote,
// 			Key:       types.HexToEd25519Key(fault.Key),
// 			Signature: types.HexToEd25519Sig(fault.Signature),
// 		}
// 		disputeData.Input.Fault = append(disputeData.Input.Fault, f)
// 	}

// 	// Convert PreState
// 	disputeData.PreState.DisputesState.Psi_g = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiG)
// 	disputeData.PreState.DisputesState.Psi_b = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiB)
// 	disputeData.PreState.DisputesState.Psi_w = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiW)
// 	disputeData.PreState.DisputesState.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PreState.Psi.PsiO)

// 	for i, rho := range rawDisputeData.PreState.Rho {
// 		if rho.DummyWorkReport == "" {
// 			disputeData.PreState.AvailabilityAssignments[i] = nil
// 			continue
// 		}

// 		// disputeData.PreState.AvailabilityAssignments[i] = &Rho_state{
// 		// 	WorkReport: hexDecodeOrPanic(rho.DummyWorkReport),
// 		// 	Timeslot:   rho.Timeout,
// 		// }
// 		if disputeData.PreState.AvailabilityAssignments[i] == nil {
// 			disputeData.PreState.AvailabilityAssignments[i] = &Rho_state{}
// 		}
// 	}
// 	if rawDisputeData.PreState.Rho == nil {
// 		disputeData.PreState.AvailabilityAssignments = [types.TotalCores]*Rho_state{}
// 	}
// 	if disputeData.PreState.SafroleState == nil {
// 		disputeData.PreState.SafroleState = &SafroleState{}
// 	}
// 	if disputeData.PostState.SafroleState == nil {
// 		disputeData.PostState.SafroleState = &SafroleState{}
// 	}
// 	disputeData.PreState.SafroleState.Timeslot = rawDisputeData.PreState.Tau
// 	for _, kappa := range rawDisputeData.PreState.Kappa {
// 		disputeData.PreState.SafroleState.CurrValidators = append(disputeData.PreState.SafroleState.CurrValidators, types.Validator{
// 			Ed25519:      types.HexToEd25519Key(kappa.Ed25519),
// 			Bandersnatch: types.HexToBandersnatchKey(kappa.Bandersnatch),
// 			Bls:          types.HexToBLS(kappa.Bls),
// 			Metadata:     types.HexToMetadata(kappa.Metadata),
// 		})
// 	}

// 	// Convert PreState.Lambda
// 	for _, lambda := range rawDisputeData.PreState.Lambda {
// 		disputeData.PreState.SafroleState.PrevValidators = append(disputeData.PreState.SafroleState.PrevValidators, types.Validator{
// 			Ed25519:      types.HexToEd25519Key(lambda.Ed25519),
// 			Bandersnatch: types.HexToBandersnatchKey(lambda.Bandersnatch),
// 			Bls:          types.HexToBLS(lambda.Bls),
// 			Metadata:     types.HexToMetadata(lambda.Metadata),
// 		})
// 	}

// 	// Convert PostState

// 	disputeData.PostState.DisputesState.Psi_g = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiG)
// 	disputeData.PostState.DisputesState.Psi_b = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiB)
// 	disputeData.PostState.DisputesState.Psi_w = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiW)
// 	disputeData.PostState.DisputesState.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PostState.Psi.PsiO)

// 	for i, rho := range rawDisputeData.PostState.Rho {
// 		if rho.DummyWorkReport == "" {
// 			disputeData.PostState.AvailabilityAssignments[i] = nil
// 			continue
// 		}
// 		// disputeData.PostState.AvailabilityAssignments[i] = &Rho_state{
// 		// 	WorkReport: hexDecodeOrPanic(rho.DummyWorkReport),
// 		// 	Timeslot:   rho.Timeout,
// 		// }
// 		if disputeData.PostState.AvailabilityAssignments[i] == nil {
// 			disputeData.PostState.AvailabilityAssignments[i] = &Rho_state{}
// 		}
// 	}

// 	disputeData.PostState.SafroleState.Timeslot = rawDisputeData.PostState.Tau
// 	for _, kappa := range rawDisputeData.PostState.Kappa {
// 		disputeData.PostState.SafroleState.CurrValidators = append(disputeData.PostState.SafroleState.CurrValidators, types.Validator{
// 			Ed25519:      types.HexToEd25519Key(kappa.Ed25519),
// 			Bandersnatch: types.HexToBandersnatchKey(kappa.Bandersnatch),
// 			Bls:          types.HexToBLS(kappa.Bls),
// 			Metadata:     types.HexToMetadata(kappa.Metadata),
// 		})
// 	}

// 	// Convert PostState.Lambda
// 	for _, lambda := range rawDisputeData.PostState.Lambda {
// 		disputeData.PostState.SafroleState.PrevValidators = append(disputeData.PostState.SafroleState.PrevValidators, types.Validator{
// 			Ed25519:      types.HexToEd25519Key(lambda.Ed25519),
// 			Bandersnatch: types.HexToBandersnatchKey(lambda.Bandersnatch),
// 			Bls:          types.HexToBLS(lambda.Bls),
// 			Metadata:     types.HexToMetadata(lambda.Metadata),
// 		})
// 	}

// 	// Convert Output
// 	if rawDisputeData.Output.DOk != nil {
// 		disputeData.Output.DOk = &struct {
// 			OffenderMark []types.Ed25519Key `json:"offender_mark"`
// 		}{}
// 		disputeData.Output.DOk.OffenderMark = convertHexStringsToPublicKeys(rawDisputeData.Output.DOk.OffenderMark)
// 	}
// 	disputeData.Output.Err = rawDisputeData.Output.Err

// 	return disputeData, nil
// }

// func convertHexStringsToBytes(hexStrings []string) [][]byte {
// 	bytesArray := make([][]byte, len(hexStrings))
// 	for i, hexStr := range hexStrings {
// 		bytesArray[i] = common.Hex2Bytes(hexStr)
// 	}
// 	return bytesArray
// }

// func convertHexStringsToPublicKeys(hexStrings []string) []types.Ed25519Key {
// 	publicKeys := make([]types.Ed25519Key, len(hexStrings))
// 	for i, hexStr := range hexStrings {
// 		publicKeys[i] = types.HexToEd25519Key(hexStr)
// 	}
// 	return publicKeys
// }
