// run test: go test ./pvm -v
package pvm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{})
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
	pvm.Execute(int(tc.InitialPC), false, nil)
	resultCode = pvm.ResultCode

	// Check the registers
	if equalIntSlices(pvm.Ram.ReadRegisters(), tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}

	resultCodeStr := types.ResultCodeToString[resultCode]
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
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.Ram.ReadRegisters())
}

// awaiting 64 bit
func TestPVM(t *testing.T) {
	// Directory containing the JSON files
	dir := "../jamtestvectors/pvm/programs"

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
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata))

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	pvm.Standard_Program_Initialization(a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false, nil)

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
	fp := "../services/hello_world.pvm"
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
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata))

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)

	start := time.Now()

	pvm.Standard_Program_Initialization(a)

	// pvm.Ram.DebugStatus()

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false, nil)

	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
}

// adjust “LogEntry” to whatever the element type of VMLogs actually is
func TestCompareLogs(t *testing.T) {
	f1, err := os.Open("interpreter/vm_log.json")
	if err != nil {
		t.Fatalf("failed to open interpreter/vm_log.json: %v", err)
	}
	defer f1.Close()

	f2, err := os.Open("recompiler_sandbox/vm_log.json")
	if err != nil {
		t.Fatalf("failed to open recompiler_sandbox/vm_log.json: %v", err)
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
			t.Fatalf("error scanning vm_log_recompiler.json at line %d: %v", i, err)
		}

		// both files ended → success
		if !has1 && !has2 {
			break
		}
		// one ended early → length mismatch
		if has1 != has2 {
			t.Fatalf("log length mismatch at index %d: has vm_log=%v, has vm_log_recompiler=%v", i, has1, has2)
		}

		// unmarshal each line into your entry type
		var orig, recp VMLog
		if err := json.Unmarshal(s1.Bytes(), &orig); err != nil {
			t.Fatalf("failed to unmarshal line %d of vm_log.json: %v", i, err)
		}
		if err := json.Unmarshal(s2.Bytes(), &recp); err != nil {
			t.Fatalf("failed to unmarshal line %d of vm_log_recompiler.json: %v", i, err)
		}

		// compare
		if !reflect.DeepEqual(orig, recp) {
			fmt.Printf("Difference at index %d:\nOriginal: %+v\nRecompiler: %+v\n", i, orig, recp)
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

func TestSnapShots(t *testing.T) {
	snapshots_dir := "/root/recompiler_sandbox"
	compares_dir := "./recompiler"
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

/*
		    {
	            "Gas": 9999992996392400,
	            "OpStr": "SHLO_L_IMM_64",
	            "Opcode": 151,
	            "Operands": "iAM=",
	            "PvmPc": 97204,
	            "Registers": [
	                1402,
	                {"changed":[4278055928, 1044447]},
	                48,
	                108331,
	                48,
	                1,
	                4278057552,
	                24655,
	                6,
	                4278057752,
	                44,
	                44,
	                0
	            ]
	        }
*/
func TestLogEntry(t *testing.T) {
	PvmLogging = true
	PvmTrace = true
	VM_MODE = "recompiler_sandbox"

	// a real pvm test case for it to run
	name := "inst_store_indirect_u16_with_offset_ok"
	filePath := "../jamtestvectors/pvm/programs/" + name + ".json"
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var tc TestCase
	err = json.Unmarshal(data, &tc)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}
	hostENV := NewMockHostEnv()
	serviceAcct := uint32(0) // stub
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{})

	//jsonStr := `{"Opcode":198,"OpStr":"SHLO_R_32","Operands":"qwo=","PvmPc":97201,"Registers":[1402,4278055928,48,108331,48,1,4278057552,24655,6,4278057752,1,44,0],"Gas":9999992996015693}`
	jsonStr := `{"Opcode":198,"OpStr":"SHLO_R_32","Operands":"qwo=","PvmPc":97201,"Registers":[1402,4278055928,48,108331,48,1,4278057552,24655,6,4278057752,0,44,0],"Gas":9999992996392400}`
	var entry VMLog
	if err := json.Unmarshal([]byte(jsonStr), &entry); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	var inst Instruction
	inst.Opcode = entry.Opcode
	inst.Args = entry.Operands
	inst.Pc = entry.PvmPc

	rvm, err := NewRecompilerSandboxVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create recompiler sandbox VM: %v", err)
		return
	}
	// the register we get it from the log entry
	for i, reg := range entry.Registers {
		rvm.Ram.WriteRegister(i, reg)
	}
	// Set the initial memory
	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.SetMemAccessSandBox(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}
	// Set the initial memory
	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.WriteMemorySandBox(mem.Address, mem.Data)
	}

	for i := 0; i < regSize; i++ {
		immVal, _ := rvm.Ram.ReadRegister(i)
		code := encodeMovImm(i, immVal)
		fmt.Printf("Initialize Register %d (%s) = %d\n", i, regInfoList[i].Name, immVal)
		rvm.startCode = append(rvm.startCode, code...)
	}
	rvm.x86Code = rvm.startCode
	rvm.x86Code = append(rvm.x86Code, pvmByteCodeToX86Code[inst.Opcode](inst)...)
	str := rvm.Disassemble(rvm.x86Code)
	if showDisassembly {
		fmt.Printf("Disassembled x86 code:\n%s\n", str)
	}
	rvm.ExecuteX86Code_SandBox(rvm.x86Code)
	for i, reg := range rvm.Ram.ReadRegisters() {
		fmt.Printf("Register %d (%s) value: %d\n", i, regInfoList[i].Name, reg)
	}

}
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

	vm.pushFrame = func(data []byte) {
		connMu.Lock()
		defer connMu.Unlock()
		if wsConn != nil {
			if err := wsConn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				fmt.Println("WS write error:", err)
				wsConn.Close()
				wsConn = nil
			}
		}
	}

	vm.stopFrameServer = func() {
		connMu.Lock()
		if wsConn != nil {
			wsConn.Close()
		}
		connMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		fmt.Println("Viewer server shut down")
	}

	return nil
}

func (vm *VM) SetFrame(b []byte) {
	if vm.pushFrame != nil {
		vm.pushFrame(b)
	}
}

func (vm *VM) CloseFrameServer() {
	if vm.stopFrameServer != nil {
		vm.stopFrameServer()
	}
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
