package statedb

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

type DisputeData struct {
	Input     types.Dispute `json:"input"`
	PreState  JamState      `json:"pre_state"`
	Output    DOutput       `json:"output"`
	PostState JamState      `json:"post_state"`
}
type HexBytes []byte

// =========================Test Function=========================
func TestReadAndConvertJson(t *testing.T) {
	filePath := "../jamtestvectors/disputes/full/progress_with_culprits-7.json"
	fmt.Println("Start File Name: ", filePath)
	disputeData, err := ReadAndConvertJson(filePath)
	if err != nil {
		t.Fatalf("Failed to read and convert JSON: %v, file name %s", err, filePath)
	}
	_ = disputeData
	fmt.Println("Finish File Name: ", filePath)
}
func TestReadAndConvertJson_All(t *testing.T) {
	dir_path := "../jamtestvectors/disputes/full/"
	files, err := ioutil.ReadDir(dir_path)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}
	for _, file := range files {

		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		filePath := dir_path + file.Name()
		fmt.Println("Start File Name: ", file.Name())
		disputeData, err := ReadAndConvertJson(filePath)
		if err != nil {
			t.Fatalf("Failed to read and convert JSON: %v, file name %s", err, filePath)
		}
		_ = disputeData
		fmt.Println("Finish File Name: ", file.Name())
	}

}

func TestDispute_State(t *testing.T) {
	filePath := "../jamtestvectors/disputes/full/progress_invalidates_avail_assignments-1.json"
	disputeData, _ := ReadAndConvertJson(filePath)
	fmt.Println("=====================================================")
	oMark, err := disputeData.PreState.Disputes(&disputeData.Input)
	//test if the postState is equal to the expected postState
	fmt.Println("PostState Actual:")
	PrettyPrint(disputeData.PreState)
	fmt.Println("PostState Expected:")
	PrettyPrint(disputeData.PostState)
	if err != nil {
		fmt.Println("Error: ", err)
	}
	assert.Equal(t, disputeData.PostState, disputeData.PreState)
	assert.Equal(t, disputeData.Output.DOk.OffenderMark, oMark.OffenderKey)

	// test if the output is equal to the expected output
}
func TestDispute_State_Full(t *testing.T) {
	dir_path := "../jamtestvectors/disputes/full/"
	files, err := ioutil.ReadDir(dir_path)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}
	for _, file := range files {

		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		filePath := dir_path + file.Name()
		fmt.Println("=====================================================")
		fmt.Printf("Start File Name: %s\n", file.Name())
		disputeData, _ := ReadAndConvertJson(filePath)
		Omark, err := disputeData.PreState.Disputes(&disputeData.Input)
		//test if the postState is equal to the expected postState
		assert.Equal(t, disputeData.PostState, disputeData.PreState)
		if err != nil {
			fmt.Println("Actual Error: ", err)
			fmt.Println("Expected Error: ", disputeData.Output.Err)
		} else {
			assert.Equal(t, disputeData.Output.DOk.OffenderMark, Omark.OffenderKey)
		}
		// test if the output is equal to the expected output
		fmt.Printf("Finish File Name: %s\n", file.Name())
		fmt.Println("=====================================================")
	}
}
func TestDispute_State_Tiny(t *testing.T) {
	dir_path := "../jamtestvectors/disputes/tiny/"
	files, err := ioutil.ReadDir(dir_path)
	if err != nil {
		log.Fatalf("Failed to read directory: %v", err)
	}
	for _, file := range files {

		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}
		filePath := dir_path + file.Name()
		fmt.Println("=====================================================")
		fmt.Printf("Start File Name: %s\n", file.Name())
		disputeData, _ := ReadAndConvertJson(filePath)
		disputeData.PreState.Disputes(&disputeData.Input)
		//test if the postState is equal to the expected postState
		assert.Equal(t, disputeData.PostState, disputeData.PreState)
		// test if the output is equal to the expected output
		fmt.Printf("Finish File Name: %s\n", file.Name())
		fmt.Println("=====================================================")
	}
}

// ========================= Helper Functions ====================
func (hb HexBytes) MarshalJSON() ([]byte, error) {
	hexStr := hex.EncodeToString(hb)
	return json.Marshal(hexStr)
}
func PrettyPrint(data interface{}) {
	prettyJSON, _ := json.MarshalIndent(data, "", "  ")
	fmt.Println(string(prettyJSON))
}

func ReadAndConvertJson(filePath string) (DisputeData, error) {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read JSON file: %v", err)
	}
	if len(data) == 0 {
		log.Fatalf("Empty JSON file: %v", err)
	}

	var rawDisputeData struct {
		Input struct {
			Disputes struct {
				Verdicts []struct {
					Target string `json:"target"`
					Age    uint32 `json:"age"`
					Votes  []struct {
						Vote      bool   `json:"vote"`
						Index     uint16 `json:"index"`
						Signature string `json:"signature"`
					} `json:"votes"`
				} `json:"verdicts"`
				Culprits []struct {
					Target    string `json:"target"`
					Key       string `json:"key"`
					Signature string `json:"signature"`
				} `json:"culprits"`
				Faults []struct {
					Target    string `json:"target"`
					Vote      bool   `json:"vote"`
					Key       string `json:"key"`
					Signature string `json:"signature"`
				} `json:"faults"`
			} `json:"disputes"`
		} `json:"input"`
		PreState struct {
			Psi struct {
				PsiG []string `json:"psi_g"`
				PsiB []string `json:"psi_b"`
				PsiW []string `json:"psi_w"`
				PsiO []string `json:"psi_o"`
			} `json:"psi"`
			Rho []struct {
				DummyWorkReport string `json:"dummy_work_report"`
				Timeout         uint32 `json:"timeout"`
			} `json:"rho"`
			Tau   uint32 `json:"tau"`
			Kappa []struct {
				Ed25519      string `json:"ed25519"`
				Bandersnatch string `json:"bandersnatch"`
				Bls          string `json:"bls"`
				Metadata     string `json:"metadata"`
			} `json:"kappa"`
			Lambda []struct {
				Ed25519      string `json:"ed25519"`
				Bandersnatch string `json:"bandersnatch"`
				Bls          string `json:"bls"`
				Metadata     string `json:"metadata"`
			}
		} `json:"pre_state"`
		Output struct {
			DOk *struct {
				OffenderMark []string `json:"offenders_mark"`
			} `json:"ok"`
			Err string `json:"err"`
		} `json:"output"`
		PostState struct {
			Psi struct {
				PsiG []string `json:"psi_g"`
				PsiB []string `json:"psi_b"`
				PsiW []string `json:"psi_w"`
				PsiO []string `json:"psi_o"`
			} `json:"psi"`
			Rho []struct {
				DummyWorkReport string `json:"dummy_work_report"`
				Timeout         uint32 `json:"timeout"`
			} `json:"rho"`
			Tau   uint32 `json:"tau"`
			Kappa []struct {
				Ed25519      string `json:"ed25519"`
				Bandersnatch string `json:"bandersnatch"`
				Bls          string `json:"bls"`
				Metadata     string `json:"metadata"`
			} `json:"kappa"`
			Lambda []struct {
				Ed25519      string `json:"ed25519"`
				Bandersnatch string `json:"bandersnatch"`
				Bls          string `json:"bls"`
				Metadata     string `json:"metadata"`
			}
		} `json:"post_state"`
	}

	err = json.Unmarshal(data, &rawDisputeData)
	if err != nil {
		log.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	var disputeData DisputeData

	// Convert Input.Verdicts
	for _, verdict := range rawDisputeData.Input.Disputes.Verdicts {
		verdictStruct := types.Verdict{
			Target: common.Hex2Hash(verdict.Target),
			Epoch:  verdict.Age,
		}
		for i, vote := range verdict.Votes {
			v := types.Vote{
				Voting:    vote.Vote,
				Index:     vote.Index,
				Signature: types.HexToEd25519Sig(vote.Signature),
			}
			verdictStruct.Votes[i] = v
		}
		disputeData.Input.Verdict = append(disputeData.Input.Verdict, verdictStruct)
	}

	// Convert Input.Culprits
	for _, culprit := range rawDisputeData.Input.Disputes.Culprits {
		c := types.Culprit{
			Target:    common.Hex2Hash(culprit.Target),
			Key:       types.HexToEd25519Pub(culprit.Key),
			Signature: types.HexToEd25519Sig(culprit.Signature),
		}
		disputeData.Input.Culprit = append(disputeData.Input.Culprit, c)
	}

	// Convert Input.Faults
	for _, fault := range rawDisputeData.Input.Disputes.Faults {
		f := types.Fault{
			Target:    common.Hex2Hash(fault.Target),
			Voting:    fault.Vote,
			Key:       types.HexToEd25519Pub(fault.Key),
			Signature: types.HexToEd25519Sig(fault.Signature),
		}
		disputeData.Input.Fault = append(disputeData.Input.Fault, f)
	}

	// Convert PreState
	disputeData.PreState.DisputesState.Psi_g = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiG)
	disputeData.PreState.DisputesState.Psi_b = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiB)
	disputeData.PreState.DisputesState.Psi_w = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiW)
	disputeData.PreState.DisputesState.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PreState.Psi.PsiO)

	for i, rho := range rawDisputeData.PreState.Rho {
		if rho.DummyWorkReport == "" {
			disputeData.PreState.AvailabilityAssignments[i] = nil
			continue
		}

		// disputeData.PreState.AvailabilityAssignments[i] = &Rho_state{
		// 	WorkReport: hexDecodeOrPanic(rho.DummyWorkReport),
		// 	Timeslot:   rho.Timeout,
		// }
		if disputeData.PreState.AvailabilityAssignments[i] == nil {
			disputeData.PreState.AvailabilityAssignments[i] = &Rho_state{}
		}
	}
	if rawDisputeData.PreState.Rho == nil {
		disputeData.PreState.AvailabilityAssignments = [types.TotalCores]*Rho_state{}
	}
	if disputeData.PreState.SafroleState == nil {
		disputeData.PreState.SafroleState = &SafroleState{}
	}
	if disputeData.PostState.SafroleState == nil {
		disputeData.PostState.SafroleState = &SafroleState{}
	}
	disputeData.PreState.SafroleState.Timeslot = rawDisputeData.PreState.Tau
	for _, kappa := range rawDisputeData.PreState.Kappa {
		disputeData.PreState.SafroleState.CurrValidators = append(disputeData.PreState.SafroleState.CurrValidators, types.Validator{
			Ed25519:      types.HexToEd25519Pub(kappa.Ed25519),
			Bandersnatch: common.Hex2Hash(kappa.Bandersnatch),
			Bls:          types.HexToBLS(kappa.Bls),
			Metadata:     types.HexToMetadata(kappa.Metadata),
		})
	}

	// Convert PreState.Lambda
	for _, lambda := range rawDisputeData.PreState.Lambda {
		disputeData.PreState.SafroleState.PrevValidators = append(disputeData.PreState.SafroleState.PrevValidators, types.Validator{
			Ed25519:      types.HexToEd25519Pub(lambda.Ed25519),
			Bandersnatch: common.Hex2Hash(lambda.Bandersnatch),
			Bls:          types.HexToBLS(lambda.Bls),
			Metadata:     types.HexToMetadata(lambda.Metadata),
		})
	}

	// Convert PostState

	disputeData.PostState.DisputesState.Psi_g = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiG)
	disputeData.PostState.DisputesState.Psi_b = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiB)
	disputeData.PostState.DisputesState.Psi_w = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiW)
	disputeData.PostState.DisputesState.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PostState.Psi.PsiO)

	for i, rho := range rawDisputeData.PostState.Rho {
		if rho.DummyWorkReport == "" {
			disputeData.PostState.AvailabilityAssignments[i] = nil
			continue
		}
		// disputeData.PostState.AvailabilityAssignments[i] = &Rho_state{
		// 	WorkReport: hexDecodeOrPanic(rho.DummyWorkReport),
		// 	Timeslot:   rho.Timeout,
		// }
		if disputeData.PostState.AvailabilityAssignments[i] == nil {
			disputeData.PostState.AvailabilityAssignments[i] = &Rho_state{}
		}
	}

	disputeData.PostState.SafroleState.Timeslot = rawDisputeData.PostState.Tau
	for _, kappa := range rawDisputeData.PostState.Kappa {
		disputeData.PostState.SafroleState.CurrValidators = append(disputeData.PostState.SafroleState.CurrValidators, types.Validator{
			Ed25519:      types.HexToEd25519Pub(kappa.Ed25519),
			Bandersnatch: common.Hex2Hash(kappa.Bandersnatch),
			Bls:          types.HexToBLS(kappa.Bls),
			Metadata:     types.HexToMetadata(kappa.Metadata),
		})
	}

	// Convert PostState.Lambda
	for _, lambda := range rawDisputeData.PostState.Lambda {
		disputeData.PostState.SafroleState.PrevValidators = append(disputeData.PostState.SafroleState.PrevValidators, types.Validator{
			Ed25519:      types.HexToEd25519Pub(lambda.Ed25519),
			Bandersnatch: common.Hex2Hash(lambda.Bandersnatch),
			Bls:          types.HexToBLS(lambda.Bls),
			Metadata:     types.HexToMetadata(lambda.Metadata),
		})
	}

	// Convert Output
	if rawDisputeData.Output.DOk != nil {
		disputeData.Output.DOk = &struct {
			OffenderMark []types.Ed25519Key `json:"offender_mark"`
		}{}
		disputeData.Output.DOk.OffenderMark = convertHexStringsToPublicKeys(rawDisputeData.Output.DOk.OffenderMark)
	}
	disputeData.Output.Err = rawDisputeData.Output.Err

	return disputeData, nil
}

func convertHexStringsToBytes(hexStrings []string) [][]byte {
	bytesArray := make([][]byte, len(hexStrings))
	for i, hexStr := range hexStrings {
		bytesArray[i] = common.Hex2Bytes(hexStr)
	}
	return bytesArray
}

func convertHexStringsToPublicKeys(hexStrings []string) []types.Ed25519Key {
	publicKeys := make([]types.Ed25519Key, len(hexStrings))
	for i, hexStr := range hexStrings {
		publicKeys[i] = types.HexToEd25519Pub(hexStr)
	}
	return publicKeys
}
