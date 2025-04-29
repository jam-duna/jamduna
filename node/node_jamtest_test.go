//go:build network_test
// +build network_test

package node

import (
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"testing"
)

const (
	SafroleTestEpochLen   = 4  // Safrole
	FallbackEpochLen      = 4  // Fallback
	FibTestEpochLen       = 1  // Assurance
	MegaTronEpochLen      = 50 // Orderaccumalation
	TransferEpochLen      = 3  // Transfer
	BalancesEpochLen      = 6  // Balance
	ScaleBalancesEpochLen = 6
	EmptyEpochLen         = 10
	Blake2bEpochLen       = 1
)

const (
	TargetedN_Mega_L          = 911 // Long  megaTron for rebustness test
	TargetedN_Mega_S          = 20  // Short megaTron for data publishing
	TargetedN_Fib             = 10
	TargetedN_Transfer        = 10
	TargetedN_Balances        = 20 // not used !!
	TargetedN_Scaled_Transfer = 600
	Targetedn_Scaled_Balances = 100
	TargetedN_Empty           = 8
	TargetedN_Blake2b         = 1
)

var targetNum = flag.Int("targetN", -1, "targetN")

// IMPORTANT:
// THIS FILE IS THE DEPLOYER FOR JAM TEST
// DONT PUT ANY INTERNAL TEST LOGIC HERE. KEEP IT SIMPLE!

func TestFallback(t *testing.T) {
	jamtest(t, "fallback", 0)
}

func TestSafrole(t *testing.T) {
	jamtest(t, "safrole", 0)
}

func TestFib(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "fib", targetN)
}

func TestFib2(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "fib2", targetN)
}

func TestAuthCopy(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "auth_copy", targetN)
}
func TestMegatron(t *testing.T) {
	// Open file to save CPU Profile
	//fmt.Printf("prereq_test: %v\n", *prereq_test)
	//fmt.Printf("authoring_log: %v\n", *authoring_log)

	cpuProfile, err := os.Create("cpu.pprof")
	if err != nil {
		t.Fatalf("Unable to create CPU Profile file: %v", err)
	}
	defer cpuProfile.Close()
	// Start CPU Profile
	if err := pprof.StartCPUProfile(cpuProfile); err != nil {
		t.Fatalf("Unable to start CPU Profile: %v", err)
	}
	defer pprof.StopCPUProfile() // Stop profiling after test completion

	// Generate memory Profile
	memProfile, err := os.Create("mem.pprof")
	if err != nil {
		t.Fatalf("Unable to create memory Profile file: %v", err)
	}
	defer memProfile.Close()

	if err := pprof.WriteHeapProfile(memProfile); err != nil {
		t.Fatalf("Unable to write memory Profile: %v", err)
	}

	targetN := TargetedN_Mega_S
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("megatron targetNum: %v\n", targetN)

	jamtest(t, "megatron", targetN)
}

func TestTransfer(t *testing.T) {
	targetN := TargetedN_Transfer
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("transfer targetNum: %v\n", targetN)

	jamtest(t, "transfer", targetN)
}

func TestScaledTransfer(t *testing.T) {
	targetN := TargetedN_Scaled_Transfer
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("scaled_transfer targetNum: %v\n", targetN)
	jamtest(t, "scaled_transfer", targetN)
}

func TestBalances(t *testing.T) {
	targetN := TargetedN_Balances
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("balances targetNum: %v\n", targetN)
	jamtest(t, "balances", targetN)
}

func TestScaledBalances(t *testing.T) {
	targetN := Targetedn_Scaled_Balances
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("scaled_balances targetNum: %v\n", targetN)
	jamtest(t, "scaled_balances", targetN)
}

func TestEmpty(t *testing.T) {
	targetN := TargetedN_Empty
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("empty targetNum: %v\n", targetN)
	jamtest(t, "empty", targetN)
}

func TestBlake2b(t *testing.T) {
	targetN := TargetedN_Blake2b
	if *targetNum > 0 {
		targetN = *targetNum
	}
	fmt.Printf("blake2b targetNum: %v\n", targetN)
	jamtest(t, "blake2b", targetN)
}

func TestGameOfLife(t *testing.T) {
	targetN := 10
	fmt.Printf("game_of_life targetNum: %v\n", targetN)
	jamtest(t, "game_of_life", targetN)
}

func TestRevm(t *testing.T) {
	const targetN = 10
	fmt.Printf("revm targetNum: %v\n", targetN)
	jamtest(t, "revm", targetN)
}

// This is just fib with malicious nodes
func TestFibDisputes(t *testing.T) {
	targetN := TargetedN_Fib
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "fib_dispute", targetN)
}
