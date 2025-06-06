// run test: go test ./pvm -v
package pvm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
	"github.com/gorilla/websocket"
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

var RecompilerFlag = false // set to false to run the interpreter

func pvm_test(tc TestCase) (int, error) {
	var num_mismatch int
	fmt.Printf("Test case: %s\n", tc.Name)

	// if tc.Name != "inst_div_signed_64" {
	// 	return 0, nil
	// }

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
	if RecompilerFlag {
		err := pvm.RunRecompiler()
		if err != nil {
			return 0, fmt.Errorf("error in recompiler: %v", err)
		}
	} else {
		pvm.Execute(int(tc.InitialPC), false)
	}
	// Check the registers
	if equalIntSlices(pvm.register, tc.ExpectedRegs) {
		fmt.Printf("Register match for test %s \n", tc.Name)
	} else {
		fmt.Printf("Register mismatch for test %s: expected %v, got %v \n", tc.Name, tc.ExpectedRegs, pvm.register)
		num_mismatch++
	}

	// t.Log("pvm_test")
	/*
		// Check the status
			if status != testCase.ExpectedStatus {
				//t.Errorf("Status mismatch for test %s: expected %s, got %s", testCase.Name, testCase.ExpectedStatus, status)
			}


			// Check the program counter
			if pc != testCase.ExpectedPC {
				//t.Errorf("Program counter mismatch for test %s: expected %d, got %d", testCase.Name, testCase.ExpectedPC, pc)
			}

			// Check the memory
			if !equalInterfaceSlices(memory, testCase.ExpectedMemory) {
				//t.Errorf("Memory mismatch for test %s: expected %v, got %v", testCase.Name, testCase.ExpectedMemory, memory)
			}
	*/
	return num_mismatch, nil // "trap", tc.InitialRegs, tc.InitialPC, tc.InitialMemory
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
		num_mismatch, err = pvm_test(testCase)
		if err != nil {
			t.Fatalf("%v", err)
		}
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
	pvm.Execute(types.EntryPointRefine, false)

	fmt.Printf("pvm.pc: %d, gas: %d, vm.ResultCode: %d, vm.Fault_address: %d\n", pvm.pc, pvm.Gas, pvm.ResultCode, pvm.Fault_address)
	elapsed := time.Since(start)
	fmt.Printf("Execution took %s\n", elapsed)
}

func TestDoom(t *testing.T) {
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
	fp := "../services/doom_self_playing.pvm"
	// fp := "../services/doom_w_input_100_steps_.pvm"

	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	hostENV := NewMockHostEnv()
	metadata := "doom"
	pvm := NewVM(0, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata))

	if err := pvm.attachFrameServer("127.0.0.1:80", "./index.html"); err != nil {
		t.Fatalf("frame server error: %v", err)
	}
	defer pvm.CloseFrameServer()

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

	// time.Sleep(10 * time.Second)
	// frame, _ := os.ReadFile("./frame_00010.bin")

	// pvm.SetFrame(frame)
	// os.WriteFile("./pvm_frame.bin", frame, 0644)
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
