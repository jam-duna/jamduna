//go:build network_test
// +build network_test

package node

import "testing"

func TestFallback(t *testing.T) {
	jamtest(t, "fallback", 0)
}

func TestSafrole(t *testing.T) {
	jamtest(t, "safrole", 0)
}

func TestEVM(t *testing.T) {
	initPProf(t)
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "evm", targetN)
}

func TestAlgo(t *testing.T) {
	initPProf(t)
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "algo", targetN)
}

func TestAuthCopy(t *testing.T) {
	initPProf(t)
	targetN := TargetedN_EVM
	if *targetNum > 0 {
		targetN = *targetNum
	}
	jamtest(t, "auth_copy", targetN)
}
