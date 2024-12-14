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
			// Read and unmarshal JSON file
			jsonData, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatalf("failed to read JSON file: %v", err)
			}

			err = json.Unmarshal(jsonData, tc.expectedType)
			if err != nil {
				t.Fatalf("failed to unmarshal JSON data: %v", err)
			}
			if debug {
				fmt.Printf("\n\n\nTesting %v\n", targetedStructType)
				fmt.Printf("Unmarshaled %s\n", jsonPath)
				fmt.Printf("Expected: %v\n", tc.expectedType)
			}
			// Encode the struct to bytes
			encodedBytes, err := types.Encode(tc.expectedType)
			if err != nil {
				t.Fatalf("failed to encode data: %v", err)
			}

			decodedStruct, _, err := types.Decode(encodedBytes, targetedStructType)
			if err != nil {
				t.Fatalf("failed to decode data: %v", err)
			}
			if debug {
				fmt.Printf("Encoded: %x\n\n", encodedBytes)
				fmt.Printf("Decoded:  %v\n\n", decodedStruct)
			}

			// Marshal the struct to JSON
			encodedJSON, err := json.MarshalIndent(decodedStruct, "", "  ")
			if err != nil {
				t.Fatalf("failed to marshal JSON data: %v", err)
			}
			if debug {
				fmt.Printf("Encoded JSON:\n%s\n", encodedJSON)
			}

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
	// _, err = db.GetSafrole().ApplyStateTransitionTickets(db.Block.Tickets(), db.Block.Header.Slot, db.Block.Header)
	err = db.GetSafrole().ValidateSaforle(db.Block.Extrinsic.Tickets, db.Block.Header.Slot, db.Block.Header)
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
		enact_epoch_change_with_no_tickets-1 🟢

		Progress by one slot.
		Randomness accumulator is updated.
		enact_epoch_change_with_no_tickets-2 🔴

		Progress from slot X to slot X.
		Timeslot must be strictly monotonic.
		enact_epoch_change_with_no_tickets-3 🟢

		Progress from a slot at the begining of the epoch to a slot in the epoch's tail.
		Tickets mark is not generated (no enough tickets).
		enact_epoch_change_with_no_tickets-4 🟢

		Progress from epoch's tail to next epoch.
		Authorities and entropies are rotated. Epoch mark is generated.
		skip_epochs-1 🟢

		Progress skipping epochs with a full tickets accumulator.
		Tickets mark is not generated. Accumulated tickets discarded. Fallback method enacted.
		skip_epoch_tail-1 🟢

		Progress to next epoch by skipping epochs tail with a full tickets accumulator.
		Tickets mark has no chance to be generated. Accumulated tickets discarded. Fallback method enacted.
		publish_tickets_no_mark-1 🔴

		Submit an extrinsic with a bad ticket attempt number.
		publish_tickets_no_mark-2 🟢

		Submit good tickets extrinsic from some authorities.
		publish_tickets_no_mark-3 🔴

		Submit one ticket already recorded in the state.
		publish_tickets_no_mark-4 🔴

		Submit tickets in bad order.
		publish_tickets_no_mark-5 🔴

		Submit tickets with bad ring proof.
		publish_tickets_no_mark-6 🟢

		Submit some tickets.
		publish_tickets_no_mark-7 🔴

		Submit tickets when epoch's lottery is over.
		publish_tickets_no_mark-8 🟢

		Progress into epoch tail, closing the epoch's lottery.
		No enough tickets, thus no tickets mark is generated.
		publish_tickets_no_mark-9 🟢

		Progress into next epoch with no enough tickets.
		Accumulated tickets are discarded. Epoch mark generated. Fallback method enacted.
		publish_tickets_with_mark-1 🟢

		Publish some tickets with an almost full tickets accumulator.
		Tickets accumulator is not full yet. No ticket is dropped from accumulator.
		publish_tickets_with_mark-2 🟢

		Publish some tickets filling the accumulator.
		Two old tickets are removed from the accumulator.
		publish_tickets_with_mark-3 🟢

		Publish some tickets with a full accumulator.
		Some old ticket are removed to make space for new ones.
		publish_tickets_with_mark-4 🟢

		With a full accumulator, conclude the lottery.
		Tickets mark is generated.
		publish_tickets_with_mark-5 🟢

		With a published tickets mark, progress into next epoch.
		Epoch mark is generated. Tickets are enacted.
		enact-epoch-change-with-padding-1 🟢

		On epoch change we recompute the ring commitment.
		One of the keys to be used is invalidated (zeroed out) because it belongs to the (posterior) offenders list.
		One of the keys is just invalid (i.e. it can't be decoded into a valid Bandersnatch point).
		Both the invalid keys are replaced with the padding point during ring commitment computation.
	*/
	/* except Err
	publish_tickets_no_mark-1 🔴	0.5.0 6.7	safrole	ErrTBadTicketAttemptNumber
	publish_tickets_no_mark-3 🔴	0.5.0 6.32	safrole	ErrTTicketAlreadyInState
	publish_tickets_no_mark-4 🔴	0.5.0 6.33	safrole	ErrTTicketsBadOrder
	publish_tickets_no_mark-5 🔴	0.5.0 6.7	safrole	ErrTBadRingProof
	publish_tickets_no_mark-7 🔴	0.5.0 6.30	safrole	ErrTEpochLotteryOver
	enact_epoch_change_with_no_tickets-2 🔴	0.5.0 6.1	safrole	ErrTTimeslotNotMonotonic
	*/
	testcases := []struct {
		jsonFile  string
		exceptErr error
	}{
		{"enact-epoch-change-with-no-tickets-1.json", nil},
		{"enact-epoch-change-with-no-tickets-2.json", jamerrors.ErrTTimeslotNotMonotonic},
		{"enact-epoch-change-with-no-tickets-3.json", nil},
		{"enact-epoch-change-with-no-tickets-4.json", nil},
		{"enact-epoch-change-with-padding-1.json", nil},
		{"publish-tickets-no-mark-1.json", jamerrors.ErrTBadTicketAttemptNumber},
		{"publish-tickets-no-mark-2.json", nil},
		{"publish-tickets-no-mark-3.json", jamerrors.ErrTTicketAlreadyInState},
		{"publish-tickets-no-mark-4.json", jamerrors.ErrTTicketsBadOrder},
		{"publish-tickets-no-mark-5.json", jamerrors.ErrTBadRingProof},
		{"publish-tickets-no-mark-6.json", nil},
		{"publish-tickets-no-mark-7.json", jamerrors.ErrTEpochLotteryOver},
		{"publish-tickets-no-mark-8.json", nil},
		{"publish-tickets-no-mark-9.json", nil},
		{"publish-tickets-with-mark-1.json", nil},
		{"publish-tickets-with-mark-2.json", nil},
		{"publish-tickets-with-mark-3.json", nil},
		{"publish-tickets-with-mark-4.json", nil},
		{"publish-tickets-with-mark-5.json", nil},
		{"skip-epoch-tail-1.json", nil},
		{"skip-epochs-1.json", nil},
	}
	for _, tc := range testcases {
		t.Run(tc.jsonFile, func(t *testing.T) {
			err := safrole_test(tc.jsonFile, tc.exceptErr)
			if err != nil {
				t.Fatalf("failed: %v", err)
			}
			fmt.Printf("Safrole PASS: %s\n", tc.jsonFile)
		})
	}
}
