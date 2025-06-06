// run test: go test ./pvm -v
package pvm

import (
	"bytes"
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

var RecompilerFlag = true // set to false to run the interpreter

func pvm_test(tc TestCase) error {
	var num_mismatch int
	fmt.Printf("\n------\nTest case: %s\n", tc.Name)

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
		rvm, err := NewRecompilerVM(pvm)
		if err != nil {
			return fmt.Errorf("failed to create recompiler VM: %w", err)
		}

		for _, pm := range tc.InitialPageMap {
			// Set the page access based on the initial page map
			if pm.IsWritable {
				err := rvm.SetMemAssess(pm.Address, pm.Length, PageMutable)
				if err != nil {
					return fmt.Errorf("failed to set memory access for address %x: %w", pm.Address, err)
				}
			}
		}

		for _, mem := range tc.InitialMemory {
			// Write the initial memory contents
			rvm.WriteMemory(mem.Address, mem.Data)
		}
		rvm.GetBasicBlocks()
		if len(rvm.BasicBlocks) == 0 {
			return fmt.Errorf("no basic blocks found")
		}
		fmt.Printf("RecompilerVM has %d basic blocks\n", len(rvm.BasicBlocks))
		if err := rvm.Translate(); err != nil {
			return fmt.Errorf("error translating bytecode: %w", err)
		}
		// Now we have rvm.x86Code ready, we can execute it
		if err := rvm.ExecuteX86Code(); err != nil {
			return fmt.Errorf("error executing x86 code: %w", err)
		}
		//check the memory
		for _, mem := range tc.ExpectedMemory {
			data, err := rvm.ReadMemory(mem.Address, uint32(len(mem.Data)))
			if err != nil {
				return fmt.Errorf("failed to read memory at address %x: %w", mem.Address, err)
			}
			if !bytes.Equal(data, mem.Data) {
				fmt.Printf("Memory mismatch for test %s at address %x: expected %x, got %x \n", tc.Name, mem.Address, mem.Data, data)
				num_mismatch++
			} else {
				fmt.Printf("Memory match for test %s at address %x \n", tc.Name, mem.Address)
			}
		}

	} else {
		pvm.Execute(int(tc.InitialPC), false)
	}
	// Check the registers
	if equalIntSlices(pvm.register, tc.ExpectedRegs) {
		// fmt.Printf("Register match for test %s \n", tc.Name)
		return nil
	}
	return fmt.Errorf("register mismatch for test %s: expected %v, got %v", tc.Name, tc.ExpectedRegs, pvm.register)

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
		err = pvm_test(testCase)
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
