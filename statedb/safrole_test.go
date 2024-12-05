package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/colorfulnotion/jam/jamerrors"
	"github.com/colorfulnotion/jam/types"
)

type TestCase struct {
	Input    SInput            `json:"input"`
	PreState SafroleStateCodec `json:"pre_state"`
	// Output    SOutput           `json:"output"`
	PostState SafroleStateCodec `json:"post_state"`
}

func TestSafrole(t *testing.T) {
	testCases := []struct {
		jsonFile     string
		binFile      string
		expectedType interface{}
	}{
		{"enact-epoch-change-with-no-tickets-1.json", "enact-epoch-change-with-no-tickets-1.bin", &TestCase{}},
		{"enact-epoch-change-with-no-tickets-2.json", "enact-epoch-change-with-no-tickets-2.bin", &TestCase{}},
		{"enact-epoch-change-with-no-tickets-3.json", "enact-epoch-change-with-no-tickets-3.bin", &TestCase{}},
		{"enact-epoch-change-with-no-tickets-4.json", "enact-epoch-change-with-no-tickets-4.bin", &TestCase{}},
		{"enact-epoch-change-with-padding-1.json", "enact-epoch-change-with-padding-1.bin", &TestCase{}},
		{"publish-tickets-no-mark-1.json", "publish-tickets-no-mark-1.bin", &TestCase{}},
		{"publish-tickets-no-mark-2.json", "publish-tickets-no-mark-2.bin", &TestCase{}},
		{"publish-tickets-no-mark-3.json", "publish-tickets-no-mark-3.bin", &TestCase{}},
		{"publish-tickets-no-mark-4.json", "publish-tickets-no-mark-4.bin", &TestCase{}},
		{"publish-tickets-no-mark-5.json", "publish-tickets-no-mark-5.bin", &TestCase{}},
		{"publish-tickets-no-mark-6.json", "publish-tickets-no-mark-6.bin", &TestCase{}},
		{"publish-tickets-no-mark-7.json", "publish-tickets-no-mark-7.bin", &TestCase{}},
		{"publish-tickets-no-mark-8.json", "publish-tickets-no-mark-8.bin", &TestCase{}},
		{"publish-tickets-no-mark-9.json", "publish-tickets-no-mark-9.bin", &TestCase{}},
		{"publish-tickets-with-mark-1.json", "publish-tickets-with-mark-1.bin", &TestCase{}},
		{"publish-tickets-with-mark-2.json", "publish-tickets-with-mark-2.bin", &TestCase{}},
		{"publish-tickets-with-mark-3.json", "publish-tickets-with-mark-3.bin", &TestCase{}},
		{"publish-tickets-with-mark-4.json", "publish-tickets-with-mark-4.bin", &TestCase{}},
		{"publish-tickets-with-mark-5.json", "publish-tickets-with-mark-5.bin", &TestCase{}},
		{"skip-epoch-tail-1.json", "skip-epoch-tail-1.bin", &TestCase{}},
		{"skip-epochs-1.json", "skip-epochs-1.json", &TestCase{}},
	}
	for _, tc := range testCases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			jsonPath := filepath.Join("../jamtestvectors/safrole/tiny", tc.jsonFile)
			// binPath := filepath.Join("../jamtestvectors/safrole/tiny", tc.binFile)

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
			encodedBytes, err := types.Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode data: %v", err)
			}
			fmt.Printf("Encoded: %x\n\n", encodedBytes)

			decodedStruct, _, err := types.Decode(encodedBytes, targetedStructType)
			if err != nil {
				t.Fatalf("failed to decode data: %v", err)
			}
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

func safrole_test(jsonFile string, exceptErr error) error {
	jsonPath := filepath.Join("../jamtestvectors/safrole/tiny", jsonFile)
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to read JSON file: %v", err)
	}

	var tc TestCase
	err = json.Unmarshal(jsonData, &tc)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON data: %v", err)
	}
	var db StateDB
	state := NewJamState()
	var block types.Block
	db.Block = &block
	db.JamState = state
	db.JamState.get_state_from_testcase(tc)
	db.Block.Header.Slot = tc.Input.Slot
	// db.Block.Header.EntropySource = tc.Input.Entropy
	db.Block.Extrinsic.Tickets = tc.Input.Extrinsics
	var sig [96]byte
	copy(sig[0:32], tc.Input.Entropy.Bytes())
	db.Block.Header.EntropySource = types.BandersnatchVrfSignature(sig)
	_, err = db.GetSafrole().ApplyStateTransitionTickets(db.Block.Tickets(), db.Block.Header.Slot, db.Block.Header)
	if err != exceptErr {
		return fmt.Errorf("expected error %v, got %v", exceptErr, err)
	}
	return nil
}

func (j *JamState) get_state_from_testcase(tc TestCase) {
	// Tau           uint32             `json:"tau"`
	// Eta           Entropy            `json:"eta"`
	// Lambda        types.Validators   `json:"lambda"`
	// Kappa         types.Validators   `json:"kappa"`
	// GammaK        types.Validators   `json:"gamma_k"`
	// Iota          types.Validators   `json:"iota"`
	// GammaA        []types.TicketBody `json:"gamma_a"`
	// GammaS        TicketsOrKeys      `json:"gamma_s"`
	// GammaZ        [144]byte          `json:"gamma_z"`
	// PostOffenders []types.Ed25519Key `json:"post_offenders"`
	sf := j.SafroleState
	sf.Timeslot = tc.PreState.Tau
	sf.Entropy = tc.PreState.Eta
	sf.PrevValidators = tc.PreState.Lambda
	sf.CurrValidators = tc.PreState.Kappa
	sf.NextValidators = tc.PreState.GammaK
	sf.DesignedValidators = tc.PreState.Iota
	sf.NextEpochTicketsAccumulator = tc.PreState.GammaA
	sf.TicketsOrKeys = tc.PreState.GammaS
	sf.TicketsVerifierKey = tc.PreState.GammaZ[:]
	j.DisputesState.Psi_o = tc.PreState.PostOffenders
}

func TestSafroleVerify(t *testing.T) {
	/*
		publish_tickets_no_mark-1 🔴	safrole	ErrTBadTicketAttemptNumber
		publish_tickets_no_mark-3 🔴	safrole	ErrTTicketAlreadyInState
		publish_tickets_no_mark-4 🔴	safrole	ErrTTicketsBadOrder
		publish_tickets_no_mark-5 🔴	safrole	ErrTBadRingProof
		publish_tickets_no_mark-7 🔴	safrole	ErrTEpochLotteryOver
		enact_epoch_change_with_no_tickets-2 🔴	safrole	ErrTTimeslotNotMonotonic
	*/
	testcases := []struct {
		jsonFile  string
		exceptErr error
	}{
		{"publish-tickets-no-mark-1.json", jamerrors.ErrTBadTicketAttemptNumber},
		{"publish-tickets-no-mark-3.json", jamerrors.ErrTTicketAlreadyInState},
		{"publish-tickets-no-mark-4.json", jamerrors.ErrTTicketsBadOrder},
		{"publish-tickets-no-mark-5.json", jamerrors.ErrTBadRingProof},
		{"publish-tickets-no-mark-7.json", jamerrors.ErrTEpochLotteryOver},
		{"enact-epoch-change-with-no-tickets-2.json", jamerrors.ErrTTimeslotNotMonotonic},
	}
	for _, tc := range testcases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := safrole_test(tc.jsonFile, tc.exceptErr)
			if err != nil {
				t.Fatalf("failed: %v", err)
			}
			fmt.Printf("PASS: %s\n", tc.jsonFile)
		})
	}
}

// // safrole_sstf is the function to be tested
// func safrole_stf(sinput SInput, spreState SState) (SOutput, SState, error) {
// 	//fmt.Printf("input=%v\n", input)
// 	//fmt.Printf("preState=%v\n", preState)
// 	// Implement the function logic here
// 	// input, err := sinput.deserialize()
// 	// if err != nil {
// 	// 	fmt.Printf("DESERIALIZE err %s\n", err.Error())
// 	// 	panic(1)
// 	// }
// 	//
// 	// preState, err2 := spreState.deserialize()
// 	// if err2 != nil {
// 	// 	fmt.Printf("DESERIALIZE err %s\n", err2.Error())
// 	// 	panic(2)
// 	// }
// 	output, postState, err := preState.STF(input)
// 	//TODO:return output.serialize(), postState.serialize(), err
// 	return SOutput{}, SState{}, nil
// }

// func TestBlake2b(t *testing.T) {
// 	// Blake2Hash("data goes here") -> "0xce73267ed8316b4350672f32ba49af86a7ae7af1267beb868a27f3fda03c044a"
// 	expectedHash := common.HexToHash("0xce73267ed8316b4350672f32ba49af86a7ae7af1267beb868a27f3fda03c044a")
// 	data := "data goes here"
// 	actualHash := common.Blake2Hash([]byte(data))
// 	if actualHash != expectedHash {
// 		t.Errorf("Hash mismatch: expected %s, got %s", expectedHash.Hex(), actualHash.Hex())
// 	} else {
// 		t.Logf("Hash match: expected %s, got %s", expectedHash.Hex(), actualHash.Hex())
// 	}
// }

// func TestEntropyHash(t *testing.T) {
// 	acc, _ := hex.DecodeString("2fa3f686df876995167e7c2e5d74c4c7b6e48f8068fe0e44208344d480f7904c")
// 	vrf, _ := hex.DecodeString("b2053e5a0852b9a5673f340d1cffe49f63f451e3b8b1e3b8d6c6ae731c888af1")
// 	expected := "b137ecb42ed3fe7df0281a459acd05e486bea724205cfdddc0b30efa0086c52f"
// 	hasher256, err := blake2b.New256(nil)
// 	if err != nil {
// 		t.Fatalf("failed to create hasher: %v", err)
// 	}
// 	hasher256.Write(append(acc, vrf...))
// 	res := hex.EncodeToString(hasher256.Sum(nil))
// 	if res != expected {
// 		t.Errorf("expected %s, got %s", expected, res)
// 	} else {
// 		fmt.Printf("expected %s, got %s", expected, res)
// 	}
// }

// func TestVerify(t *testing.T) {
// 	// iota "designed" https://github.com/w3f/jamtestvectors/blob/826e64f74a352a059f8b172447af7927f02e0fae/safrole/tiny/publish-tickets-no-mark-2.json#L148-L181
// 	pubkeysHex := []string{
// 		"5e465beb01dbafe160ce8216047f2155dd0569f058afd52dcea601025a8d161d",
// 		"3d5e5a51aab2b048f8686ecd79712a80e3265a114cc73f14bdb2a59233fb66d0",
// 		"aa2b95f7572875b0d0f186552ae745ba8222fc0b5bd456554bfe51c68938f8bc",
// 		"7f6190116d118d643a98878e294ccf62b509e214299931aad8ff9764181a4e33",
// 		"48e5fcdce10e0b64ec4eebd0d9211c7bac2f27ce54bca6f7776ff6fee86ab3e3",
// 		"f16e5352840afb47e206b5c89f560f2611835855cf2e6ebad1acc9520a72591d",
// 	}

// 	var pubkeys []bandersnatch.BanderSnatchKey
// 	for _, hexStr := range pubkeysHex {
// 		bytes, err := hex.DecodeString(hexStr)
// 		if err != nil {
// 			t.Fatalf("Error decoding hex string: %v", err)
// 		}
// 		pubkeys = append(pubkeys, bandersnatch.BanderSnatchKey(bytes))
// 	}

// 	ringsetBytes := bandersnatch.InitRingSet(pubkeys)
// 	decodedHex, _ := hex.DecodeString("bb30a42c1e62f0afda5f0a4e8a562f7a13a24cea00ee81917b86b89e801314aa")
// 	vrfInputData := append(append([]byte("jam_ticket_seal"), decodedHex...), byte(1))

// 	signatureHex := "b342bf8f6fa69c745daad2e99c92929b1da2b840f67e5e8015ac22dd1076343ea95c5bb4b69c197bfdc1b7d2f484fe455fb19bba7e8d17fcaf309ba5814bf54f3a74d75b408da8d3b99bf07f7cde373e4fd757061b1c99e0aac4847f1e393e892b566c14a7f8643a5d976ced0a18d12e32c660d59c66c271332138269cb0fe9c2462d5b3c1a6e9f5ed330ff0d70f64218010ff337b0b69b531f916c67ec564097cd842306df1b4b44534c95ff4efb73b17a14476057fdf8678683b251dc78b0b94712179345c794b6bd99aa54b564933651aee88c93b648e91a613c87bc3f445fff571452241e03e7d03151600a6ee259051a23086b408adec7c112dd94bd8123cf0bed88fddac46b7f891f34c29f13bf883771725aa234d398b13c39fd2a871894f1b1e2dbc7fffbc9c65c49d1e9fd5ee0da133bef363d4ebebe63de2b50328b5d7e020303499d55c07cae617091e33a1ee72ba1b65f940852e93e2905fdf577adcf62be9c74ebda9af59d3f11bece8996773f392a2b35693a45a5a042d88a3dc816b689fe596762d4ea7c6024da713304f56dc928be6e8048c651766952b6c40d0f48afc067ca7cbd77763a2d4f11e88e16033b3343f39bf519fe734db8a139d148ccead4331817d46cf469befa64ae153b5923869144dfa669da36171c20e1f757ed5231fa5a08827d83f7b478ddfb44c9bceb5c6c920b8761ff1e3edb03de48fb55884351f0ac5a7a1805b9b6c49c0529deb97e994deaf2dfd008825e8704cdc04b621f316b505fde26ab71b31af7becbc1154f9979e43e135d35720b93b367bedbe6c6182bb6ed99051f28a3ad6d348ba5b178e3ea0ec0bb4a03fe36604a9eeb609857f8334d3b4b34867361ed2ff9163acd9a27fa20303abe9fc29f2d6c921a8ee779f7f77d940b48bc4fce70a58eed83a206fb7db4c1c7ebe7658603495bb40f6a581dd9e235ba0583165b1569052f8fb4a3e604f2dd74ad84531c6b96723c867b06b6fdd1c4ba150cf9080aa6bbf44cc29041090973d56913b9dc755960371568ef1cf03f127fe8eca209db5d18829f5bfb5826f98833e3f42472b47fad995a9a8bb0e41a1df45ead20285a8"
// 	signatureBytes, err := hex.DecodeString(signatureHex)
// 	if err != nil {
// 		t.Fatalf("Error decoding signature: %v", err)
// 	}

// 	//RingVrfVerify(ringsetBytes, signature, vrfInputData, auxData []byte)
// 	auxData := []byte{}
// 	vrfOutput, err := bandersnatch.RingVrfVerify(ringsetBytes, signatureBytes, vrfInputData, auxData)
// 	fmt.Printf("VRF output hash: %x\n", vrfOutput)

// 	if err != nil {
// 		t.Fatalf("Test failed, got err %v", err)
// 	}

// }

// // TestSafroleStf reads JSON files, parses them, and calls the safrole_stf function
// func TestSafroleStf(t *testing.T) {
// 	// Directory containing jamtestvectors JSON files
// 	dir := "../jamtestvectors/safrole/tiny"

// 	// Read all files in the directory
// 	files, err := ioutil.ReadDir(dir)
// 	if err != nil {
// 		t.Fatalf("Failed to read directory: %v", err)
// 	}

// 	testcases := make(map[string]string)
// 	testcases["publish-tickets-no-mark-2.json"] = errNone
// 	testcases["publish-tickets-no-mark-6.json"] = errNone
// 	testcases["publish-tickets-no-mark-10.json"] = errNone
// 	testcases["publish-tickets-with-mark-1.json"] = errNone
// 	testcases["publish-tickets-with-mark-2.json"] = errNone
// 	testcases["publish-tickets-with-mark-3.json"] = errNone
// 	testcases["publish-tickets-with-mark-5.json"] = errNone
// 	testcases["enact-epoch-change-with-no-tickets-1.json"] = errNone
// 	testcases["enact-epoch-change-with-no-tickets-2.json"] = errNone
// 	testcases["enact-epoch-change-with-no-tickets-3.json"] = errNone
// 	testcases["enact-epoch-change-with-no-tickets-4.json"] = errNone
// 	testcases["publish-tickets-with-mark-2.json"] = errNone
// 	testcases["publish-tickets-with-mark-4.json"] = errNone
// 	testcases["publish-tickets-no-mark-7.json"] = errTicketSubmissionInTail // "Fail: Submit a ticket while in epoch's tail."

// 	testcases["enact-epoch-change-with-no-tickets-4.json"] = errNone
// 	testcases["enact-epoch-change-with-no-tickets-3.json"] = errNone
// 	testcases["enact-epoch-change-with-no-tickets-2.json"] = errTimeslotNotMonotonic     //"Fail: Timeslot must be strictly monotonic."
// 	testcases["publish-tickets-no-mark-1.json"] = errExtrinsicWithMoreTicketsThanAllowed // "Fail: Submit an extrinsic with more tickets than allowed."
// 	testcases["publish-tickets-no-mark-3.json"] = errTicketResubmission                  // "Fail: Re-submit tickets from authority 0."
// 	testcases["publish-tickets-no-mark-4.json"] = errTicketBadOrder                      // "Fail: Submit tickets in bad order."
// 	testcases["publish-tickets-no-mark-5.json"] = errTicketBadRingProof                  // "Fail: Submit tickets with bad ring proof."
// 	testcases["publish-tickets-no-mark-8.json"] = errNone
// 	testcases["publish-tickets-with-mark-3.json"] = errNone
// 	testcases["publish-tickets-no-mark-9.json"] = errNone
// 	testcases["skip-epochs-1.json"] = errNone
// 	testcases["skip-epoch-tail-1.json"] = errNone

// 	testcases["publish-tickets-with-mark-1.json"] = errNone
// 	testcases["publish-tickets-with-mark-2.json"] = errNone
// 	for _, file := range files {
// 		if file.IsDir() {
// 			continue
// 		}

// 		if !strings.Contains(file.Name(), ".json") {
// 			continue
// 		}
// 		expectedErr, ok := testcases[file.Name()]
// 		// Print the file name
// 		if ok {
// 			if len(expectedErr) == 0 {
// 				continue
// 			}
// 			fmt.Printf("\n***Test: %s Expected error: %s\n", file.Name(), expectedErr)
// 		} else {
// 			fmt.Printf("\n***Test SKIPPED: %s\n", file.Name())
// 			continue
// 			//expectedErr = ""
// 			//fmt.Printf("Expected error: NOTFOUND\n")
// 		}

// 		filePath := filepath.Join(dir, file.Name())
// 		data, err := ioutil.ReadFile(filePath)
// 		if err != nil {
// 			t.Fatalf("Failed to read file %s: %v", filePath, err)
// 		}

// 		var testCase TestCase
// 		err = json.Unmarshal(data, &testCase)
// 		if err != nil {
// 			t.Errorf("Failed to unmarshal JSON from file %s: %v", filePath, err)
// 			continue
// 		}

// 		// Print the pretty formatted JSON content
// 		var prettyJSON []byte
// 		prettyJSON, err = json.MarshalIndent(testCase, "", "  ")
// 		if err != nil {
// 			t.Errorf("Failed to marshal JSON content for file %s: %v", filePath, err)
// 			continue
// 		}
// 		if len(string(prettyJSON)) > 0 {
// 			//fmt.Printf("Content:\n%s\n", string(prettyJSON))
// 		}

// 		// Call the safrole_stf function with the input and pre_state
// 		output, postState, err := safrole_stf(testCase.Input, testCase.PreState)
// 		if err.Error() != expectedErr {
// 			fmt.Printf("FAIL: expected '%s', got '%s'\n", expectedErr, err.Error())
// 		}

// 		// Perform assertions to validate the output and post_state
// 		if !equalOutput(output, testCase.Output) {
// 			expectedJSON, _ := json.MarshalIndent(testCase.Output, "", "  ")
// 			gotJSON, _ := json.MarshalIndent(output, "", "  ")
// 			fmt.Printf("FAIL Output mismatch: expected %s, got %s [%s]\n", string(expectedJSON), string(gotJSON), err.Error())

// 		}
// 		err = equalState(postState, testCase.PostState)
// 		if err != nil {
// 			fmt.Printf("FAIL PostState mismatch on %s: %s\n", file.Name(), err.Error())
// 			continue
// 		}
// 		fmt.Printf("... PASSED %s\n\n", file.Name())
// 	}
// }
