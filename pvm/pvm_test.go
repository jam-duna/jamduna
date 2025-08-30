// run test: go test ./pvm -v
package pvm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/gorilla/websocket"
	"github.com/nsf/jsondiff"
)

// memory_for test
type TestMemory struct {
	Address uint32 `json:"address"`
	Data    []byte `json:"contents"`
}

type TestPageMap struct {
	Address    uint32 `json:"address"`
	Length     uint32 `json:"length"`
	IsWritable bool   `json:"is-writable"` // true if the memory is written to, false if it is read from
}

// TestCase
type TestCase struct {
	Name           string        `json:"name"`
	InitialRegs    []uint64      `json:"initial-regs"`
	InitialPC      uint32        `json:"initial-pc"`
	InitialPageMap []TestPageMap `json:"initial-page-map"`
	InitialMemory  []TestMemory  `json:"initial-memory"`
	Code           []byte        `json:"program"`
	ExpectedStatus string        `json:"expected-status"`
	ExpectedRegs   []uint64      `json:"expected-regs"`
	ExpectedPC     uint32        `json:"expected-pc"`
	ExpectedMemory []TestMemory  `json:"expected-memory"`
}

func pvm_test(tc TestCase) error {
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendInterpreter)
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		//pvm.Ram.SetPageAccess(mem.Address/PageSize, 1, AccessMode{Readable: false, Writable: true, Inaccessible: false})
		pvm.Ram.WriteRAMBytes(mem.Address, mem.Data[:])
	}

	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	resultCode := uint8(0)
	pvm.Gas = int64(100) // Set a high gas limit
	pvm.Execute(int(tc.InitialPC), false)
	resultCode = pvm.ResultCode

	// Check the registers
	if equalIntSlices(pvm.register[:], tc.ExpectedRegs[:]) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}

	resultCodeStr := types.HostResultCodeToString[resultCode]
	if resultCodeStr == "page-fault" {
		resultCodeStr = "panic"
	}
	expectedCodeStr := tc.ExpectedStatus
	if expectedCodeStr == "page-fault" {
		expectedCodeStr = "panic"
	}
	if resultCodeStr == expectedCodeStr {
		fmt.Printf("Result code match for test %s: %s\n", tc.Name, resultCodeStr)
	} else {
		return fmt.Errorf("result code mismatch for test %s: expected %s, got %s", tc.Name, expectedCodeStr, resultCodeStr)
	}
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.register)
}

// awaiting 64 bit
func TestPVM(t *testing.T) {
	// Directory containing the JSON files
	dir := path.Join(common.GetJAMTestVectorPath("stf"), "pvm/programs")

	// Read all files in the directory
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}
	count := 0
	num_mismatch := 0
	total_mismatch := 0
	for _, file := range files {

		if strings.Contains(file.Name(), "riscv") {
			continue // skip riscv tests
		}
		count++
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}

		var testCase TestCase
		err = json.Unmarshal(data, &testCase)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}
		name := testCase.Name
		t.Run(name, func(t *testing.T) {
			err = pvm_test(testCase)
			if err != nil {
				t.Errorf("❌ [%s] Test failed: %v", name, err)
			} else {
				t.Logf("✅ [%s] Test passed", name)
			}
		})
		total_mismatch += num_mismatch
	}
	// show the match rate
	fmt.Printf("Match rate: %v/%v\n", count-total_mismatch, count)
}

func TestRevm(t *testing.T) {
	t.Skip("Temporarily disabled for debugging")
	log.InitLogger("info")
	fp := "../services/revm_test.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	hostENV := NewMockHostEnv()
	metadata := "revm_test"
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendInterpreter)

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	pvm.Standard_Program_Initialization(a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false)

	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
}

func TestHelloWorld(t *testing.T) {
	// f, err := os.Create("cpu.pprof")
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// pprof.StartCPUProfile(f)
	// defer func() {
	// 	pprof.StopCPUProfile()
	// 	f.Close()
	// }()

	log.InitLogger("info")
	fp := "../services/blake2b_parent.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	hostENV := NewMockHostEnv()
	metadata := "hello_world"
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendInterpreter)

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	pvm.Standard_Program_Initialization(a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")

	pvm.Execute(types.EntryPointRefine, false)
	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
	r, _ := pvm.getArgumentOutputs()
	fmt.Printf("result as string: %s\n", r.String())

	var hashExpectedResult common.Hash
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	//hash 100 times 0xdeadbeef
	for i := 0; i < 100; i++ {
		if i == 0 {
			hashExpectedResult = common.Blake2Hash(data)
		} else {
			hashExpectedResult = common.Blake2Hash(hashExpectedResult.Bytes())
		}

	}
	fmt.Printf("Expected hash: %s\n", hashExpectedResult.Hex())
	result := r.Ok
	// compare the result with the expected hash
	// use bytes.Equal to compare the two hashes
	if !bytes.Equal(result, hashExpectedResult.Bytes()) {
		t.Errorf("Expected hash %s, got %s", hashExpectedResult.Hex(), common.BytesToHash(result).Hex())
	} else {
		fmt.Printf("Hash matches: %s\n", common.BytesToHash(result).Hex())
	}
}

// adjust “LogEntry” to whatever the element type of VMLogs actually is
func TestCompareLogs(t *testing.T) {
	f1, err := os.Open(fmt.Sprintf("%s/vm_log.json", BackendInterpreter))
	if err != nil {
		t.Fatalf("failed to open %s/vm_log.json: %v", BackendInterpreter, err)
	}
	defer f1.Close()

	f2, err := os.Open(fmt.Sprintf("%s/vm_log.json", BackendSandbox))
	if err != nil {
		t.Fatalf("failed to open %s/vm_log.json: %v", BackendSandbox, err)
	}
	defer f2.Close()

	s1 := bufio.NewScanner(f1)
	s2 := bufio.NewScanner(f2)

	var i int
	for {
		has1 := s1.Scan()
		has2 := s2.Scan()

		if err := s1.Err(); err != nil {
			t.Fatalf("error scanning vm_log.json at line %d: %v", i, err)
		}
		if err := s2.Err(); err != nil {
			t.Fatalf("error scanning vm_log_compiler.json at line %d: %v", i, err)
		}

		// both files ended → success
		if !has1 && !has2 {
			break
		}
		// one ended early → length mismatch
		if has1 != has2 {
			t.Fatalf("log length mismatch at index %d: has vm_log=%v, has vm_log_compiler=%v", i, has1, has2)
		}

		// unmarshal each line into your entry type
		var orig, recp VMLog
		if err := json.Unmarshal(s1.Bytes(), &orig); err != nil {
			t.Fatalf("failed to unmarshal line %d of vm_log.json: %v", i, err)
		}
		if err := json.Unmarshal(s2.Bytes(), &recp); err != nil {
			t.Fatalf("failed to unmarshal line %d of vm_log_compiler.json: %v", i, err)
		}

		// compare
		if !reflect.DeepEqual(orig, recp) {
			fmt.Printf("Difference at index %d:\nOriginal: %+v\nCompiler: %+v\n", i, orig, recp)
			if diff := CompareJSON(orig, recp); diff != "" {
				fmt.Println("Differences:", diff)
				t.Fatalf("differences at index %d: %s", i, diff)
			}
		} else if i%100000 == 0 {
			fmt.Printf("Index %d: no difference %s\n", i, s1.Bytes())
		}
		i++
	}
}

/*
func TestSnapShots(t *testing.T) {
	snapshots_dir := "/root/sandbox"
	compares_dir := "./compiler"
	files, err := os.ReadDir(snapshots_dir)
	if err != nil {
		t.Fatalf("Failed to read directory %s: %v", snapshots_dir, err)
	}
	for _, file := range files {
		// read as a json
		// and unmarshal it into a EmulatorSnapShot
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		filePath := filepath.Join(snapshots_dir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("Failed to read file %s: %v", filePath, err)
		}
		var snapshot EmulatorSnapShot
		err = json.Unmarshal(data, &snapshot)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
		}

		// get the compare file
		compareFilePath := filepath.Join(compares_dir, file.Name())
		// check if the compare file exists
		if _, err := os.Stat(compareFilePath); os.IsNotExist(err) {
			continue // skip if the compare file does not exist
		}
		compareData, err := os.ReadFile(compareFilePath)
		if err != nil {
			t.Fatalf("Failed to read compare file %s: %v", compareFilePath, err)
		}
		var compareSnapshot EmulatorSnapShot
		err = json.Unmarshal(compareData, &compareSnapshot)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON from compare file %s: %v", compareFilePath, err)
		}
		// compare the two snapshots
		if !reflect.DeepEqual(snapshot.InitialRegs, compareSnapshot.InitialRegs) {
			t.Errorf("InitialRegs mismatch in snapshot %s: expected %v, got %v", file.Name(), compareSnapshot.InitialRegs, snapshot.InitialRegs)
		}
		if snapshot.InitialPC != compareSnapshot.InitialPC {
			t.Errorf("InitialPC mismatch in snapshot %s: expected %d, got %d", file.Name(), compareSnapshot.InitialPC, snapshot.InitialPC)
		}
		for pageIndex, pageData := range snapshot.InitialMemory {
			pageHash := common.BytesToHash(pageData)
			comparePageData := compareSnapshot.InitialMemory[pageIndex]
			comparePageHash := common.BytesToHash(comparePageData)
			if pageHash != comparePageHash {
				t.Errorf("InitialMemory mismatch in snapshot %s at page %d: expected %x, got %x", file.Name(), pageIndex, comparePageHash, pageHash)
			}
		}
		fmt.Printf("Snapshot %s matches with compare file %s\n", file.Name(), compareFilePath)
	}
}
*/
/*
in node:
go test -run=TestCompareLogs
in pvm:


go test -run=TestLogEntry

Differences: Diff detected:
{
    "Gas": 2499072842,
    "OpStr": "ROT_R_32",
    "Opcode": 223,
    "Operands": "eAc=",
    "PvmPc": 177485,
    "Registers": [
        2634,
        {"changed":[4278057904, 4024417039]},
        18,
        4278057670,
        16,
        4278058864,
        215666,
        {"changed":[5221614, 10443228]},
        4305410524,
        4305394171,
        55374,
        32,
        52
    ]
}



*/

func CompareJSON(obj1, obj2 interface{}) string {
	json1, err1 := json.Marshal(obj1)
	json2, err2 := json.Marshal(obj2)
	if err1 != nil || err2 != nil {
		return "Error marshalling JSON"
	}
	opts := jsondiff.DefaultJSONOptions()
	diff, diffStr := jsondiff.Compare(json1, json2, &opts)

	if diff == jsondiff.FullMatch {
		return "JSONs are identical"
	}
	return fmt.Sprintf("Diff detected:\n%s", diffStr)
}

func (vm *VM) attachFrameServer(addr, htmlPath string) error {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	var (
		connMu sync.Mutex
		wsConn *websocket.Conn
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, htmlPath)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Println("upgrade error:", err)
			return
		}
		fmt.Println("WS client connected")

		connMu.Lock()
		if wsConn != nil {
			wsConn.Close()
		}
		wsConn = c
		connMu.Unlock()

		c.SetCloseHandler(func(code int, text string) error {
			fmt.Printf("WS closed: %d %s\n", code, text)
			connMu.Lock()
			if wsConn == c {
				wsConn = nil
			}
			connMu.Unlock()
			return nil
		})
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	go func() {
		fmt.Println("Viewer server listening on", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("ListenAndServe:", err)
		}
	}()

	// vm.pushFrame = func(data []byte) {
	// 	connMu.Lock()
	// 	defer connMu.Unlock()
	// 	if wsConn != nil {
	// 		if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
	// 			fmt.Println("WS write error:", err)
	// 			wsConn.Close()
	// 			wsConn = nil
	// 		}
	// 	}
	// }

	// vm.stopFrameServer = func() {
	// 	connMu.Lock()
	// 	if wsConn != nil {
	// 		wsConn.Close()
	// 	}
	// 	connMu.Unlock()

	// 	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	// 	defer cancel()
	// 	_ = srv.Shutdown(ctx)
	// 	fmt.Println("Viewer server shut down")
	// }

	return nil
}

func (vm *VM) SetFrame(b []byte) {
	if vm.pushFrame != nil {
		vm.pushFrame(b)
	}
}

func (vm *VM) CloseFrameServer() {
	// if vm.stopFrameServer != nil {
	// 	vm.stopFrameServer()
	// }
}

// Helper function to compare two integer slices
func equalIntSlices(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
