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

func TestDispute(t *testing.T) {
	filePath := "../jamtestvectors/disputes/full/progress_invalidates_avail_assignments-1.json"
	disputeData, _ := ReadAndConvertJson(filePath)
	postState, output, _ := Dispute(disputeData.Input, disputeData.PreState)
	//test if the postState is equal to the expected postState
	assert.Equal(t, disputeData.PostState, postState)
	// test if the output is equal to the expected output
	assert.Equal(t, disputeData.Output.DOk, output.DOk)
	_ = postState
	fmt.Println("Err except: ", disputeData.Output.Err)
	fmt.Println("Err actual: ", output.Err)
	assert.Equal(t, disputeData.PostState, postState)
	assert.Equal(t, disputeData.Output.DOk, output.DOk)
	prettyPrint(postState)

}
func TestDispute_Tiny(t *testing.T) {
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
		disputeData, err := ReadAndConvertJson(filePath)
		if err != nil {
			t.Fatalf("Failed to read and convert JSON: %v, file name %s", err, filePath)
		}
		postState, output, _ := Dispute(disputeData.Input, disputeData.PreState)
		assert.Equal(t, disputeData.PostState, postState)
		assert.Equal(t, disputeData.Output.DOk, output.DOk)
		fmt.Println("Post State and Output match the expected values")
		if disputeData.Output.Err != "" {
			fmt.Println("Err except:", disputeData.Output.Err)
			fmt.Println("Err actual:", output.Err)
		}
		fmt.Printf("Finish File Name: %s\n", file.Name())
		fmt.Println("=====================================================")
	}
}
func TestDispute_Full(t *testing.T) {
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
		disputeData, err := ReadAndConvertJson(filePath)
		if err != nil {
			t.Fatalf("Failed to read and convert JSON: %v, file name %s", err, filePath)
		}

		postState, output, _ := Dispute(disputeData.Input, disputeData.PreState)

		assert.Equal(t, disputeData.PostState, postState)
		assert.Equal(t, disputeData.Output.DOk, output.DOk)
		fmt.Println("Post State and Output match the expected values")
		if disputeData.Output.Err != "" {
			fmt.Println("Err except: ", disputeData.Output.Err)
			fmt.Println("Err actual: ", output.Err)
		}

		fmt.Printf("Finish File Name: %s\n", file.Name())
		fmt.Println("=====================================================")
	}
}

func TestDispute_State(t *testing.T) {
	filePath := "../jamtestvectors/disputes/full/progress_invalidates_avail_assignments-1.json"
	disputeData, _ := ReadAndConvertJson(filePath)
	disputeData.PreState.Disputes(disputeData.Input)
	//test if the postState is equal to the expected postState
	assert.Equal(t, disputeData.PostState, disputeData.PreState)
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
		disputeData.PreState.Disputes(disputeData.Input)
		//test if the postState is equal to the expected postState
		assert.Equal(t, disputeData.PostState, disputeData.PreState)
		// test if the output is equal to the expected output
		fmt.Printf("Finish File Name: %s\n", file.Name())
		fmt.Println("=====================================================")
	}
}

// ========================= Helper Functions ====================
func parseWorkReport(b string) types.WorkReport {
	return types.WorkReport{}
}

// ========================= Helper Functions ====================
func (hb HexBytes) MarshalJSON() ([]byte, error) {
	hexStr := hex.EncodeToString(hb)
	return json.Marshal(hexStr)
}
func prettyPrint(data interface{}) {
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
				WorkReport string `json:"work_report"`
				Timeout    uint32 `json:"timeout"`
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
				VerdictMark  []string `json:"verdicts_mark"`
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
				WorkReport string `json:"work_report"`
				Timeout    uint32 `json:"timeout"`
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
			Target: hexDecodeOrPanic(verdict.Target),
			Epoch:  verdict.Age,
		}
		for _, vote := range verdict.Votes {
			verdictStruct.Votes = append(verdictStruct.Votes, types.Vote{
				Voting:    vote.Vote,
				Index:     vote.Index,
				Signature: common.Hex2Bytes(vote.Signature),
			})
		}
		disputeData.Input.Verdict = append(disputeData.Input.Verdict, verdictStruct)
	}

	// Convert Input.Culprits
	for _, culprit := range rawDisputeData.Input.Disputes.Culprits {
		disputeData.Input.Culprit = append(disputeData.Input.Culprit, types.Culprit{
			Target:    common.BytesToHash(common.Hex2Bytes(culprit.Target)),
			Key:       common.Hex2Bytes(culprit.Key),
			Signature: common.Hex2Bytes(culprit.Signature),
		})
	}

	// Convert Input.Faults
	for _, fault := range rawDisputeData.Input.Disputes.Faults {
		disputeData.Input.Fault = append(disputeData.Input.Fault, types.Fault{
			WorkReportHash: common.BytesToHash(common.Hex2Bytes(fault.Target)),
			Voting:         fault.Vote,
			Key:            common.Hex2Bytes(fault.Key),
			Signature:      common.Hex2Bytes(fault.Signature),
		})
	}

	// Convert PreState
	disputeData.PreState.DisputesState.Psi_g = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiG)
	disputeData.PreState.DisputesState.Psi_b = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiB)
	disputeData.PreState.DisputesState.Psi_w = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiW)
	disputeData.PreState.DisputesState.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PreState.Psi.PsiO)

	for i, rho := range rawDisputeData.PreState.Rho {
		disputeData.PreState.AvailabilityAssignments[i] = &Rho_state{
			WorkReport: parseWorkReport(rho.WorkReport),
			Timeslot:   rho.Timeout,
		}
	}
	disputeData.PreState.SafroleState.Timeslot = rawDisputeData.PreState.Tau
	for _, kappa := range rawDisputeData.PreState.Kappa {
		disputeData.PreState.SafroleState.CurrValidators = append(disputeData.PreState.SafroleState.CurrValidators, types.Validator{
			Ed25519:      common.BytesToHash(common.Hex2Bytes(kappa.Ed25519)),
			Bandersnatch: common.BytesToHash(common.Hex2Bytes(kappa.Bandersnatch)),
			Bls:          common.Hex2BLS(kappa.Bls),
			Metadata:     common.Hex2Metadata(kappa.Metadata),
		})
	}

	// Convert PreState.Lambda
	for _, lambda := range rawDisputeData.PreState.Lambda {
		disputeData.PreState.SafroleState.PrevValidators = append(disputeData.PreState.SafroleState.PrevValidators, types.Validator{
			Ed25519:      common.BytesToHash(common.Hex2Bytes(lambda.Ed25519)),
			Bandersnatch: common.BytesToHash(common.Hex2Bytes(lambda.Bandersnatch)),
			Bls:          common.Hex2BLS(lambda.Bls),
			Metadata:     common.Hex2Metadata(lambda.Metadata),
		})
	}

	// Convert PostState

	disputeData.PostState.DisputesState.Psi_g = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiG)
	disputeData.PostState.DisputesState.Psi_b = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiB)
	disputeData.PostState.DisputesState.Psi_w = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiW)
	disputeData.PostState.DisputesState.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PostState.Psi.PsiO)

	for i, rho := range rawDisputeData.PostState.Rho {
		disputeData.PostState.AvailabilityAssignments[i] = &Rho_state{
			WorkReport: parseWorkReport(rho.WorkReport),
			Timeslot:   rho.Timeout,
		}
	}

	disputeData.PostState.SafroleState.Timeslot = rawDisputeData.PostState.Tau

	for _, kappa := range rawDisputeData.PostState.Kappa {
		disputeData.PostState.SafroleState.CurrValidators = append(disputeData.PostState.SafroleState.CurrValidators, types.Validator{
			Ed25519:      common.BytesToHash(common.Hex2Bytes(kappa.Ed25519)),
			Bandersnatch: common.BytesToHash(common.Hex2Bytes(kappa.Bandersnatch)),
			Bls:          common.Hex2BLS(kappa.Bls),
			Metadata:     common.Hex2Metadata(kappa.Metadata),
		})
	}

	// Convert PostState.Lambda
	for _, lambda := range rawDisputeData.PostState.Lambda {
		disputeData.PostState.SafroleState.PrevValidators = append(disputeData.PostState.SafroleState.PrevValidators, types.Validator{
			Ed25519:      common.BytesToHash(common.Hex2Bytes(lambda.Ed25519)),
			Bandersnatch: common.BytesToHash(common.Hex2Bytes(lambda.Bandersnatch)),
			Bls:          common.Hex2BLS(lambda.Bls),
			Metadata:     common.Hex2Metadata(lambda.Metadata),
		})
	}

	// Convert Output
	if rawDisputeData.Output.DOk != nil {
		disputeData.Output.DOk = &struct {
			VerdictMark  []common.Hash     `json:"verdict_mark"`
			OffenderMark []types.PublicKey `json:"offender_mark"`
		}{}
		verdictMarks := make([]common.Hash, len(rawDisputeData.Output.DOk.VerdictMark))
		for i, hexStr := range rawDisputeData.Output.DOk.VerdictMark {
			verdictMarks[i] = common.BytesToHash(common.Hex2Bytes(hexStr))
		}
		disputeData.Output.DOk.VerdictMark = verdictMarks
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

func convertHexStringsToPublicKeys(hexStrings []string) []types.PublicKey {
	publicKeys := make([]types.PublicKey, len(hexStrings))
	for i, hexStr := range hexStrings {
		publicKeys[i] = common.Hex2Bytes(hexStr)
	}
	return publicKeys
}

func hexDecodeOrPanic(hexStr string) common.Hash {
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		log.Fatalf("Failed to decode hex string: %v", err)
	}
	return common.BytesToHash(data)
}
