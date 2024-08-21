package disputes

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"path/filepath"
	"testing"
)

type DisputeData struct {
	Input     types.Dispute `json:"input"`
	PreState  StateDispute  `json:"pre_state"`
	Output    Output        `json:"output"`
	PostState StateDispute  `json:"post_state"`
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
	assert.Equal(t, disputeData.Output.Ok, output.Ok)
	_ = postState
	fmt.Println("Err except: ", disputeData.Output.Err)
	fmt.Println("Err actual: ", output.Err)
	assert.Equal(t, disputeData.PostState, postState)
	assert.Equal(t, disputeData.Output.Ok, output.Ok)
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
		assert.Equal(t, disputeData.Output.Ok, output.Ok)
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
		assert.Equal(t, disputeData.Output.Ok, output.Ok)
		fmt.Println("Post State and Output match the expected values")
		if disputeData.Output.Err != "" {
			fmt.Println("Err except: ", disputeData.Output.Err)
			fmt.Println("Err actual: ", output.Err)
		}

		fmt.Printf("Finish File Name: %s\n", file.Name())
		fmt.Println("=====================================================")
	}
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
			Ok *struct {
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
			WorkReportHash: hexDecodeOrPanic(verdict.Target),
			Epoch:          verdict.Age,
		}
		for _, vote := range verdict.Votes {
			verdictStruct.Votes = append(verdictStruct.Votes, types.Vote{
				Voting:    vote.Vote,
				Index:     vote.Index,
				Signature: hexDecodeOrPanic(vote.Signature),
			})
		}
		disputeData.Input.Verdict = append(disputeData.Input.Verdict, verdictStruct)
	}

	// Convert Input.Culprits
	for _, culprit := range rawDisputeData.Input.Disputes.Culprits {
		disputeData.Input.Culprit = append(disputeData.Input.Culprit, types.Culprit{
			WorkReportHash: hexDecodeOrPanic(culprit.Target),
			Key:            hexDecodeOrPanic(culprit.Key),
			Signature:      hexDecodeOrPanic(culprit.Signature),
		})
	}

	// Convert Input.Faults
	for _, fault := range rawDisputeData.Input.Disputes.Faults {
		disputeData.Input.Fault = append(disputeData.Input.Fault, types.Fault{
			WorkReportHash: hexDecodeOrPanic(fault.Target),
			Voting:         fault.Vote,
			Key:            hexDecodeOrPanic(fault.Key),
			Signature:      hexDecodeOrPanic(fault.Signature),
		})
	}

	// Convert PreState
	disputeData.PreState.Psi.Psi_g = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiG)
	disputeData.PreState.Psi.Psi_b = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiB)
	disputeData.PreState.Psi.Psi_w = convertHexStringsToBytes(rawDisputeData.PreState.Psi.PsiW)
	disputeData.PreState.Psi.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PreState.Psi.PsiO)

	for _, rho := range rawDisputeData.PreState.Rho {
		disputeData.PreState.Rho = append(disputeData.PreState.Rho, Rho_state{
			DummyWorkReport: hexDecodeOrPanic(rho.DummyWorkReport),
			Timeout:         rho.Timeout,
		})
	}
	disputeData.PreState.Tau = rawDisputeData.PreState.Tau
	for _, kappa := range rawDisputeData.PreState.Kappa {
		disputeData.PreState.Kappa = append(disputeData.PreState.Kappa, types.Validator{
			Ed25519:      hexDecodeOrPanic(kappa.Ed25519),
			Bandersnatch: hexDecodeOrPanic(kappa.Bandersnatch),
			Bls:          hexDecodeOrPanic(kappa.Bls),
			Metadata:     hexDecodeOrPanic(kappa.Metadata),
		})
	}

	// Convert PreState.Lambda
	for _, lambda := range rawDisputeData.PreState.Lambda {
		disputeData.PreState.Lambda = append(disputeData.PreState.Lambda, types.Validator{
			Ed25519:      hexDecodeOrPanic(lambda.Ed25519),
			Bandersnatch: hexDecodeOrPanic(lambda.Bandersnatch),
			Bls:          hexDecodeOrPanic(lambda.Bls),
			Metadata:     hexDecodeOrPanic(lambda.Metadata),
		})
	}

	// Convert PostState

	disputeData.PostState.Psi.Psi_g = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiG)
	disputeData.PostState.Psi.Psi_b = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiB)
	disputeData.PostState.Psi.Psi_w = convertHexStringsToBytes(rawDisputeData.PostState.Psi.PsiW)
	disputeData.PostState.Psi.Psi_o = convertHexStringsToPublicKeys(rawDisputeData.PostState.Psi.PsiO)

	for _, rho := range rawDisputeData.PostState.Rho {
		disputeData.PostState.Rho = append(disputeData.PostState.Rho, Rho_state{
			DummyWorkReport: hexDecodeOrPanic(rho.DummyWorkReport),
			Timeout:         rho.Timeout,
		})
	}

	disputeData.PostState.Tau = rawDisputeData.PostState.Tau

	for _, kappa := range rawDisputeData.PostState.Kappa {
		disputeData.PostState.Kappa = append(disputeData.PostState.Kappa, types.Validator{
			Ed25519:      hexDecodeOrPanic(kappa.Ed25519),
			Bandersnatch: hexDecodeOrPanic(kappa.Bandersnatch),
			Bls:          hexDecodeOrPanic(kappa.Bls),
			Metadata:     hexDecodeOrPanic(kappa.Metadata),
		})
	}

	// Convert PostState.Lambda
	for _, lambda := range rawDisputeData.PostState.Lambda {
		disputeData.PostState.Lambda = append(disputeData.PostState.Lambda, Validator{
			Ed25519:      hexDecodeOrPanic(lambda.Ed25519),
			Bandersnatch: hexDecodeOrPanic(lambda.Bandersnatch),
			Bls:          hexDecodeOrPanic(lambda.Bls),
			Metadata:     hexDecodeOrPanic(lambda.Metadata),
		})
	}

	// Convert Output
	if rawDisputeData.Output.Ok != nil {
		disputeData.Output.Ok = &struct {
			VerdictMark  [][]byte          `json:"verdict_mark"`
			OffenderMark []types.PublicKey `json:"offender_mark"`
		}{}
		disputeData.Output.Ok.VerdictMark = convertHexStringsToBytes(rawDisputeData.Output.Ok.VerdictMark)
		disputeData.Output.Ok.OffenderMark = convertHexStringsToPublicKeys(rawDisputeData.Output.Ok.OffenderMark)
	}
	disputeData.Output.Err = rawDisputeData.Output.Err

	return disputeData, nil
}

func convertHexStringsToBytes(hexStrings []string) [][]byte {
	bytesArray := make([][]byte, len(hexStrings))
	for i, hexStr := range hexStrings {
		bytesArray[i] = hexDecodeOrPanic(hexStr)
	}
	return bytesArray
}

func convertHexStringsToPublicKeys(hexStrings []string) []types.PublicKey {
	publicKeys := make([]PublicKey, len(hexStrings))
	for i, hexStr := range hexStrings {
		publicKeys[i] = hexDecodeOrPanic(hexStr)
	}
	return publicKeys
}
func hexDecodeOrPanic(hexStr string) []byte {
	if len(hexStr) >= 2 && hexStr[:2] == "0x" {
		hexStr = hexStr[2:]
	}
	data, err := hex.DecodeString(hexStr)
	if err != nil {
		log.Fatalf("Failed to decode hex string: %v", err)
	}
	return data
}
