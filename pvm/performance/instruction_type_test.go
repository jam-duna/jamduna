package performance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func TestSimpleTestServiceInstructionTypes(t *testing.T) {
	// Get the path to simple-test.pvm
	projectRoot := "../.." // From pvm/performance to project root
	pvmPath := filepath.Join(projectRoot, "services/test_services/simple-test/simple-test.pvm")

	// Check if the file exists
	if _, err := os.Stat(pvmPath); os.IsNotExist(err) {
		t.Skipf("simple-test.pvm not found at %s. Run 'make test-services' first.", pvmPath)
	}

	// Read the PVM file
	code, err := os.ReadFile(pvmPath)
	if err != nil {
		t.Fatalf("Failed to read PVM file: %v", err)
	}
	code = types.CombineMetadataAndCode("test-metadata", code) // Ensure metadata is included
	codelen := len(code)
	if codelen == 0 {
		t.Fatal("PVM file is empty")
	} else {
		t.Logf("Read PVM file of length %d bytes", codelen)
	}

	// Create a simple host environment
	hostEnv := statedb.NewMockHostEnv()

	// Create VM using NewVMFromCode (similar to how services are loaded)
	// Parameters: serviceIndex, code, initialPC, initialHeap, hostEnv, backend, initialGas
	vm := statedb.NewVMFromCode(
		0,                          // service_index
		code,                       // code
		0,                          // initial PC (entry point)
		1024*1024,                  // 1MB initial heap
		hostEnv,                    // host environment
		statedb.BackendInterpreter, // use interpreter backend
		10_000_000,                 // initial gas (10 million)
	)

	if vm == nil {
		t.Fatal("Failed to create VM")
	}

	// Get the VMGo instance and enable instruction type recording
	vmGo := vm.GetInterpreterVM()
	if vmGo == nil {
		t.Fatal("Failed to get interpreter VM")
	}

	// Enable instruction type recording
	vmGo.RecordInstructionType = true

	// Execute the VM
	t.Logf("Executing simple-test.pvm...")
	vm.ExecuteWithBackend([]byte{1, 2, 3}, 0, "")
	if err != nil {
		t.Logf("VM execution ended with: %v", err)
	}

	// Get instruction type statistics
	stats := vmGo.GetInstructionTypeStats()

	// Print statistics
	t.Logf("\n=== Instruction Type Statistics ===")
	t.Logf("Arithmetic:   %d (%.2f%%)", stats.ArithmeticCount,
		float64(stats.ArithmeticCount)/float64(stats.TotalCount)*100)
	t.Logf("Memory:       %d (%.2f%%)", stats.MemoryCount,
		float64(stats.MemoryCount)/float64(stats.TotalCount)*100)
	t.Logf("Control Flow: %d (%.2f%%)", stats.ControlFlowCount,
		float64(stats.ControlFlowCount)/float64(stats.TotalCount)*100)
	t.Logf("Total:        %d", stats.TotalCount)
	t.Logf("===================================")

	// Verify we got some instructions
	if stats.TotalCount == 0 {
		t.Error("Expected to execute at least some instructions")
	}

	// Verify all three categories were used (simple-test does hashing which uses all types)
	if stats.ArithmeticCount == 0 {
		t.Error("Expected some arithmetic instructions")
	}
	if stats.MemoryCount == 0 {
		t.Error("Expected some memory instructions")
	}
	if stats.ControlFlowCount == 0 {
		t.Error("Expected some control flow instructions")
	}

	// Additional VM state checks
	t.Logf("VM Result Code: %d", vmGo.ResultCode)
	t.Logf("VM Machine State: %d", vmGo.MachineState)
	t.Logf("Gas Remaining: %d", vmGo.Gas)
}
