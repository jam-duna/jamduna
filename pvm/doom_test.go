package pvm

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

const DoomServiceID = uint32(60) // Doom service ID

func TestSnapShot(t *testing.T) {

	PvmLogging = true
	PvmTrace = true
	debugRecompiler = true
	VMsCompare = true
	name := "inst_add_32"
	filePath := "../jamtestvectors/pvm/programs/" + name + ".json"
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", filePath, err)
	}

	var testCase TestCase
	err = json.Unmarshal(data, &testCase)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON from file %s: %v", filePath, err)
	}

	hostENV := NewMockHostEnv()
	serviceAcct := uint32(DoomServiceID) // stub
	tc := testCase
	// metadata, c := types.SplitMetadataAndCode(tc.Code)
	pvm := NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendRecompilerSandbox)
	// Set the initial memory
	// if len(tc.InitialMemory) == 0 {
	// 	pvm.Ram.SetPageAccess(32, 1, AccessMode{Readable: false, Writable: false, Inaccessible: true})
	// }
	rvm, err := NewRecompilerSandboxVM(pvm)
	rvm.Gas = 1000000 // set a high gas limit for the sandbox
	if err != nil {
		t.Fatalf("Failed to create RecompilerSandboxVM: %v", err)
	}
	for _, pm := range tc.InitialPageMap {
		// Set the page access based on the initial page map
		if pm.IsWritable {
			err := rvm.sandBox.SetMemAccessSandBox(pm.Address, pm.Length, PageMutable)
			if err != nil {
				t.Fatalf("Failed to set memory access for page %d: %v", pm.Address, err)
			}
		}
	}

	for i, regV := range tc.InitialRegs {
		// Write the initial register values
		rvm.Ram.WriteRegister(i, regV)
	}

	for _, mem := range tc.InitialMemory {
		// Write the initial memory contents
		rvm.sandBox.WriteMemorySandBox(mem.Address, mem.Data)
	}
	rvm.pc = 0
	err = rvm.ExecuteSandBox(0)
	if err != nil {
		// we don't have to return this , just print it
		fmt.Printf("ExecuteX86 crash detected: %v\n", err)
	}
	fmt.Printf("registers: %v\n", rvm.Ram.ReadRegisters())
	post_register := rvm.Ram.ReadRegisters()
	post_register[7] = 0x25
	post_register[8] = 0x33
	snapshot := rvm.TakeSnapShot(name, uint32(rvm.pc), post_register, uint64(rvm.Gas), 0x1900000E0, guestBase, 0)
	rvm.Close()
	pvm = NewVM(serviceAcct, tc.Code, tc.InitialRegs, uint64(tc.InitialPC), hostENV, false, []byte{}, BackendRecompilerSandbox)
	rvm, err = NewRecompilerSandboxVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create RecompilerSandboxVM: %v", err)
	}
	err = rvm.ExecuteFromSnapshot(snapshot)
	if err != nil {
		fmt.Printf("ExecuteFromSnapshot crash detected: %v\n", err)
	}
	fmt.Printf("registers after snapshot: %v\n", rvm.Ram.ReadRegisters())

}

func TestDoomNoSandbox(t *testing.T) {
	useRawRam = true // use raw RAM for this test
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	PvmLogging = false
	fp := "../services/doom_self_playing.pvm"

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
	pvm := NewVM(DoomServiceID, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendInterpreter)
	pvm.initLogs()

	if err := pvm.attachFrameServer("0.0.0.0:8080", "./index.html"); err != nil {
		t.Fatalf("frame server error: %v", err)
	}
	defer pvm.CloseFrameServer()

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)
	pvm.Standard_Program_Initialization(a)

	fmt.Printf("PVM start execution...\n")
	pvm.Execute(types.EntryPointRefine, false)

	pvm.saveLogs()
}
func TestDoomX86(t *testing.T) {
	useRawRam = false    // use raw RAM for this test
	useEcalli500 = false // use ecalli500 for this test
	log.InitLogger("debug")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule("pvm_validator")
	UseTally = true // enable tally for this test
	PvmLogging = false
	fp := "../services/doom_self_playing.pvm"

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
	pvm := NewVM(DoomServiceID, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendRecompiler)
	pvm.initLogs()
	rvm, err := NewRecompilerVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create RecompilerVM: %v", err)
	}
	if err := pvm.attachFrameServer("0.0.0.0:8080", "./index.html"); err != nil {
		t.Fatalf("frame server error: %v", err)
	}
	defer pvm.CloseFrameServer()

	a := make([]byte, 0)
	pvm.Gas = int64(9999999999999999)
	rvm.Standard_Program_Initialization(a)

	fmt.Printf("PVM start execution...\n")
	rvm.Execute(types.EntryPointRefine)
	pvm.saveLogs()
}

func TestDoomSandBox(t *testing.T) {
	PvmLogging = false
	PvmTrace = false

	useRawRam = true // use raw RAM for this test
	debugRecompiler = true
	VMsCompare = true

	// set up the code for the Doom self-playing test case
	fp := "../services/doom_self_playing.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	// set up the VM with the test case
	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	metadata := "doom"
	pvm := NewVM(DoomServiceID, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendRecompilerSandbox)
	rvm, err := NewRecompilerSandboxVM(pvm)
	if err != nil {
		t.Fatalf("Failed to create RecompilerSandboxVM: %v", err)
	}
	a := make([]byte, 0)
	rvm.Standard_Program_Initialization_SandBox(a)
	rvm.initLogs()
	rvm.Execute(types.EntryPointRefine)
}

func TestDoomSnapShot(t *testing.T) {
	PvmLogging = false
	PvmTrace = false
	useRawRam = true // use raw RAM for this test
	debugRecompiler = true
	VMsCompare = true

	// set up the code for the Doom self-playing test case
	fp := "../services/doom_self_playing.pvm"
	raw_code, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("Failed to read file %s: %v", fp, err)
		return
	}
	fmt.Printf("Read %d bytes from %s\n", len(raw_code), fp)

	// set up the VM with the test case
	hostENV := NewMockHostEnv()
	initial_regs := make([]uint64, 13)
	initial_pc := uint64(0)
	metadata := "doom"
	pvm := NewVM(DoomServiceID, raw_code, initial_regs, initial_pc, hostENV, true, []byte(metadata), BackendRecompilerSandbox)
	rvm, err := NewRecompilerSandboxVM(pvm)
	a := make([]byte, 0)
	rvm.Standard_Program_Initialization_SandBox(a)
	rvm.initLogs()

	rvm.Gas = int64(9999999999999999)
	if err != nil {
		t.Fatalf("Failed to create RecompilerSandboxVM: %v", err)
	}

	snapshotName := "BB966150000" // from the interpreter
	snapshot, err := rvm.LoadSnapshot(snapshotName)
	if err != nil {
		fmt.Printf("Failed to load snapshot: %v\n", err)
		return
	}
	debugRecompiler = false // disable debug mode for the snapshot execution
	err = rvm.ExecuteFromSnapshot(snapshot)
	if err != nil {
		fmt.Printf("ExecuteFromSnapshot crash detected: %v\n", err)
	}
}
