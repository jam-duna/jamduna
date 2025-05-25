package pvm

import (
	"fmt"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/types"
)

func TestCodeCompile(t *testing.T) {
	// read code from file
	//pvm/test_code/json.pvm

	code, err := os.ReadFile("test_code/json.pvm")
	if err != nil {
		t.Fatalf("failed to read code file: %v", err)
	}
	vm := NewVMFromCode(0, code, 0, nil)
	vm.Standard_Program_Initialization(nil) // eq 264/265
	vm.Execute(types.EntryPointRefine, false)
	r, _ := vm.getArgumentOutputs()
	fmt.Printf("result: %v\n", r)
	fmt.Printf("name: %s\n", string(r.Ok))
}
