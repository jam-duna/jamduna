//go:build testing
// +build testing

package statedb

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/stretchr/testify/assert"
)

type AccumulateTestCase struct {
	Input     AccumulateInput `json:"input"`
	PreState  AccumulateState `json:"pre_state"`
	PostState AccumulateState `json:"post_state"`
}

type AccumulateInput struct {
	Slot    uint32             `json:"slot"`
	Reports []types.WorkReport `json:"reports"` // available reports
}

type AccumulateState struct {
	Slot                uint32                                       `json:"slot"`
	Entropy             common.Hash                                  `json:"entropy"`
	AccumulationQueue   [types.EpochLength][]types.AccumulationQueue `json:"ready_queue"` // theta - The accumulation queue  θ eq 164
	AccumulationHistory [types.EpochLength][]common.Hash             `json:"accumulated"` // xi - The accumulation history  ξ eq 162
	Privileges          tmpKaiState                                  `json:"privileges"`  // kai - The privileges
	Accounts            []TmpAccount                                 `json:"accounts"`    // a - The accounts
}

//	type Kai_state struct {
//		Kai_m uint32            `json:"chi_m"` // The index of the bless service
//		Kai_a uint32            `json:"chi_a"` // The index of the designate service
//		Kai_v uint32            `json:"chi_v"` // The index of the assign service
//		Kai_g map[uint32]uint32 `json:"chi_g"` // g is a small dictionary containing the indices of services which automatically accumulate in each block together with a basic amount of gas with which each accumulates
//	}
type tmpKaiState struct {
	Kai_m uint32 `json:"bless"`     // The index of the bless service
	Kai_a uint32 `json:"designate"` // The index of the designate service
	Kai_v uint32 `json:"assign"`    // The index of the assign service
	// Kai_g always_acc `json:"always_acc"` // g is a small dictionary containing the indices of services which automatically accumulate in each block together with a basic amount of gas with which each accumulates
}

type always_acc struct {
	key uint32
	val uint32
}

type TmpAccount struct {
	Index uint32 `json:"id"`
	Data  Data   `json:"data"`
}

type Data struct {
	Service ServiceData `json:"service"`
	Images  []CodeImage `json:"preimages"`
}

type ServiceData struct {
	CodeHash        common.Hash `json:"code_hash"`    //a_c - account code hash c
	Balance         uint64      `json:"balance"`      //a_b - account balance b, which must be greater than a_t (The threshold needed in terms of its storage footprint)
	GasLimitG       uint64      `json:"min_item_gas"` //a_g - the minimum gas required in order to execute the Accumulate entry-point of the service's code,
	GasLimitM       uint64      `json:"min_memo_gas"` //a_m - the minimum required for the On Transfer entry-point.
	StorageSize     uint64      `json:"bytes"`        //a_l - total number of octets used in storage (9.3)
	NumStorageItems uint32      `json:"items"`        //a_i - the number of items in storage (9.3)
}

type CodeImage struct {
	PreimageHash common.Hash `json:"hash"`
	Blob         Blob        `json:"blob"`
}
type Blob struct {
	byte []byte
}

// costum marshal and Unmarshal json for CodeImage
// use string read into and hex decode to []byte
// use hex encode and string to write out
// use hex decode and string to read in
func (c *Blob) MarshalJSON() ([]byte, error) {
	return json.Marshal(common.Bytes2Hex(c.byte))
}

func (c *Blob) UnmarshalJSON(data []byte) error {
	var hexStr string
	if err := json.Unmarshal(data, &hexStr); err != nil {
		return err
	}
	c.byte = common.FromHex(hexStr)
	return nil
}

// testing .....

func TestParseAccumulateVector(t *testing.T) {
	// read the json file
	// parse the json file
	json_file := "../jamtestvectors/accumulate/tiny/enqueue_and_unlock_chain-1.json"
	jsonData, err := os.ReadFile(json_file)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	var testCase AccumulateTestCase
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		t.Fatalf("failed to parse JSON file: %v", err)
	}
	// check the parsed values
	// pretty print the parsed values
	expectedJson, err := json.MarshalIndent(testCase, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}

	fmt.Printf("Expected: %s\n", expectedJson)

}

func TestAccumulateSTF(t *testing.T) {
	accumulate_vector_dir := "../jamtestvectors/accumulate"
	network_args := *network
	// test_cases is all the json files in the directory
	json_dir := fmt.Sprintf("%s/%s", accumulate_vector_dir, network_args)
	fmt.Printf("json_dir: %s\n", json_dir)
	// look for all the json files in the directory
	json_files, err := os.ReadDir(json_dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}
	for _, json_file := range json_files {
		if json_file.IsDir() || filepath.Ext(json_file.Name()) != ".json" {
			continue
		}
		json_file_path := fmt.Sprintf("%s/%s", json_dir, json_file.Name())
		// read the json file
		jsonData, err := os.ReadFile(json_file_path)
		if err != nil {
			t.Errorf("failed to read JSON file: %v", err)
			continue
		}
		var testCase AccumulateTestCase
		err = json.Unmarshal(jsonData, &testCase)
		if err != nil {
			t.Errorf("failed to parse JSON file: %v", err)
			continue
		}
		// run the test case
		err = AccumulateSTF(json_file.Name(), testCase)
		if err != nil {
			fmt.Printf("\033[31mAccumulateSTF FAIL: %s, %s\033[0m\n", json_file.Name(), err)
		} else {
			fmt.Printf("\033[32mAccumulateSTF PASS: %s\033[0m\n", json_file.Name())
		}
	}
}

func TestSingleAccumulateSTF(t *testing.T) {
	filepath := "../jamtestvectors/accumulate/tiny/enqueue_and_unlock_chain-1.json"
	jsonData, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("failed to read JSON file: %v", err)
	}
	var testCase AccumulateTestCase
	err = json.Unmarshal(jsonData, &testCase)
	if err != nil {
		t.Fatalf("failed to parse JSON file: %v", err)
	}
	testAccumulateSTF(filepath, testCase, t)
}

func (j *JamState) GetStateFromAccumulateState(state AccumulateState) (services map[uint32]*types.ServiceAccount, codes map[uint32][]byte) {
	j.SafroleState.Timeslot = state.Slot
	for i := range state.AccumulationQueue {
		j.AccumulationQueue[i] = state.AccumulationQueue[i]
	}

	for i, historyfromprestate := range state.AccumulationHistory {
		j.AccumulationHistory[i].WorkPackageHash = make([]common.Hash, 0)
		j.AccumulationHistory[i].WorkPackageHash = append(j.AccumulationHistory[i].WorkPackageHash, historyfromprestate...)
	}

	// skip previlege state for now
	services = make(map[uint32]*types.ServiceAccount)
	codes = make(map[uint32][]byte)
	for _, account := range state.Accounts {
		key := account.Index
		data := account.Data
		services[key] = &types.ServiceAccount{
			ServiceIndex:    key,
			CodeHash:        data.Service.CodeHash,
			Balance:         data.Service.Balance,
			GasLimitG:       data.Service.GasLimitG,
			GasLimitM:       data.Service.GasLimitM,
			StorageSize:     data.Service.StorageSize,
			NumStorageItems: data.Service.NumStorageItems,
			Mutable:         true,
		}
		for _, image := range account.Data.Images {
			if image.PreimageHash == data.Service.CodeHash {
				codes[key] = image.Blob.byte
			}
		}
	}
	return services, codes
}

func testAccumulateSTF(testname string, TestCase AccumulateTestCase, t *testing.T) {
	var db StateDB
	rand.Seed(time.Now().UnixNano()) // Seed the random number generator
	db_path := fmt.Sprintf("/tmp/testReport_%d", rand.Intn(100000000))

	sdb, err := storage.NewStateDBStorage(db_path)
	if err != nil {
		t.Errorf("Reports FAIL: failed to create state db: %v", err)
	}
	db = *newEmptyStateDB(sdb)
	db.Block = &types.Block{
		Header: types.BlockHeader{
			Slot: TestCase.Input.Slot,
		},
	}
	state := NewJamState()
	post_state := NewJamState()
	db.JamState = state
	// write state to the jam state
	services, codes := db.JamState.GetStateFromAccumulateState(TestCase.PreState)
	// post_services, post_codes := post_state.GetStateFromAccumulateState(TestCase.PostState)
	post_state.GetStateFromAccumulateState(TestCase.PostState)
	for key, service := range services {
		// write the service to the db
		err := db.writeAccount(service)
		if err != nil {
			t.Errorf("Reports FAIL: failed to write account: %v", err)
		}
		// write code to the db
		db.WriteServicePreimageBlob(key, codes[key])
	}
	var f map[uint32]uint32
	s := db
	s.JamState.SafroleState.Timeslot = TestCase.Input.Slot
	var g uint64 = 1000000000000000000
	o := s.JamState.newPartialState()
	old_timeslot := TestCase.PreState.Slot
	s.AvailableWorkReport = TestCase.Input.Reports
	accumulate_input_wr := TestCase.Input.Reports
	accumulate_input_wr = s.AccumulatableSequence(accumulate_input_wr)
	n, T, _ := s.OuterAccumulate(g, accumulate_input_wr, o, f)
	// Not sure whether transfer happens here
	tau := s.GetTimeslot() // Not sure whether τ ′ is set up like this
	if len(T) > 0 {
		s.ProcessDeferredTransfers(o, tau, T)
	}
	// make sure all service accounts can be written
	for _, sa := range o.D {
		sa.Mutable = true
		sa.Dirty = true
	}

	s.ApplyXContext(o)
	s.ApplyStateTransitionAccumulation(accumulate_input_wr, n, old_timeslot)
	// check if the state is equal to the post state
	//use json to compare the states
	newJam := s.JamState
	// AccumulationHistory
	history := newJam.AccumulationHistory
	expected_history := post_state.AccumulationHistory
	history_pass := assert.Equal(t, expected_history, history, "AccumulationHistory does not match")
	if !history_pass {
		// output two json files
		// one for the expected history
		expected_history_json, err := json.MarshalIndent(expected_history, "", "  ")
		if err != nil {
			t.Errorf("failed to marshal JSON: %v", err)
		}
		// one for the actual history
		actual_history_json, err := json.MarshalIndent(history, "", "  ")
		if err != nil {
			t.Errorf("failed to marshal JSON: %v", err)
		}
		// write the files to disk
		// /testdata/accumulate_vector_test
		testdata_dir := fmt.Sprintf("./testdata/%s", testname)
		err = os.MkdirAll(testdata_dir, 0755)
		if err != nil {
			t.Errorf("failed to create directory: %v", err)
		}
		err = os.WriteFile(fmt.Sprintf("%s/expected_history.json", testdata_dir), expected_history_json, 0644)
		if err != nil {
			t.Errorf("failed to write file: %v", err)
		}
		err = os.WriteFile(fmt.Sprintf("%s/actual_history.json", testdata_dir), actual_history_json, 0644)
		if err != nil {
			t.Errorf("failed to write file: %v", err)
		}
	}
	// AccumulationQueue
	queue := newJam.AccumulationQueue
	expected_queue := post_state.AccumulationQueue
	queued_history_pass := assert.Equal(t, expected_queue, queue, "AccumulationQueue does not match")
	if !queued_history_pass {
		// output two json files
		// one for the expected
		expected_queue_json, err := json.MarshalIndent(expected_queue, "", "  ")
		if err != nil {
			t.Errorf("failed to marshal JSON: %v", err)
		}
		// one for the actual
		actual_queue_json, err := json.MarshalIndent(queue, "", "  ")
		if err != nil {
			t.Errorf("failed to marshal JSON: %v", err)
		}
		// write the files to disk
		// /testdata/accumulate_vector_test
		testdata_dir := fmt.Sprintf("./testdata/%s", testname)
		err = os.MkdirAll(testdata_dir, 0755)
		if err != nil {
			t.Errorf("failed to create directory: %v", err)
		}
		err = os.WriteFile(fmt.Sprintf("%s/expected_queue.json", testdata_dir), expected_queue_json, 0644)
		if err != nil {
			t.Errorf("failed to write file: %v", err)
		}
		err = os.WriteFile(fmt.Sprintf("%s/actual_queue.json", testdata_dir), actual_queue_json, 0644)
		if err != nil {
			t.Errorf("failed to write file: %v", err)
		}

	}

	if !history_pass || !queued_history_pass {
		t.Errorf("STF FAIL: AccumulationHistory or AccumulationQueue does not match")
	} else {
		fmt.Printf("%s PASS: AccumulationHistory and AccumulationQueue match", testname)
	}
}

func AccumulateSTF(testname string, TestCase AccumulateTestCase) error {
	var db StateDB
	rand.Seed(time.Now().UnixNano()) // Seed the random number generator
	db_path := fmt.Sprintf("/tmp/testReport_%d", rand.Intn(100000000))

	sdb, err := storage.NewStateDBStorage(db_path)
	if err != nil {
		return fmt.Errorf("Reports FAIL: failed to create state db: %v", err)
	}
	db = *newEmptyStateDB(sdb)
	db.Block = &types.Block{
		Header: types.BlockHeader{
			Slot: TestCase.Input.Slot,
		},
	}
	state := NewJamState()
	post_state := NewJamState()
	db.JamState = state
	// write state to the jam state
	services, codes := db.JamState.GetStateFromAccumulateState(TestCase.PreState)
	// post_services, post_codes := post_state.GetStateFromAccumulateState(TestCase.PostState)
	post_state.GetStateFromAccumulateState(TestCase.PostState)
	for key, service := range services {
		// write the service to the db
		err := db.writeAccount(service)
		if err != nil {
			return fmt.Errorf("Reports FAIL: failed to write account: %v", err)
		}
		// write code to the db
		db.WriteServicePreimageBlob(key, codes[key])
	}
	var f map[uint32]uint32
	s := db
	s.JamState.SafroleState.Timeslot = TestCase.Input.Slot
	var g uint64 = 1000000000000000000
	o := s.JamState.newPartialState()
	old_timeslot := TestCase.PreState.Slot
	s.AvailableWorkReport = TestCase.Input.Reports
	accumulate_input_wr := TestCase.Input.Reports
	accumulate_input_wr = s.AccumulatableSequence(accumulate_input_wr)
	n, T, _ := s.OuterAccumulate(g, accumulate_input_wr, o, f)
	if debug {
		fmt.Printf("ApplyStateTransitionFromBlock - Accumulate\n")
	}

	// Not sure whether transfer happens here
	tau := s.GetTimeslot() // Not sure whether τ ′ is set up like this
	if len(T) > 0 {
		s.ProcessDeferredTransfers(o, tau, T)
	}
	// make sure all service accounts can be written
	for _, sa := range o.D {
		sa.Mutable = true
		sa.Dirty = true
	}

	s.ApplyXContext(o)
	s.ApplyStateTransitionAccumulation(accumulate_input_wr, n, old_timeslot)
	// check if the state is equal to the post state
	//use json to compare the states
	newJam := s.JamState
	// AccumulationHistory
	if !reflect.DeepEqual(newJam.AccumulationHistory, post_state.AccumulationHistory) {
		return fmt.Errorf("STF FAIL: AccumulationHistory does not match")
	}
	if !reflect.DeepEqual(newJam.AccumulationQueue, post_state.AccumulationQueue) {
		return fmt.Errorf("STF FAIL: AccumulationQueue does not match")
	}
	return nil
}
