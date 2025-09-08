//go:build !linux || !amd64
// +build !linux !amd64

package recompiler

import (
	"fmt"

	"github.com/colorfulnotion/jam/log"
)

func ExecuteX86(code []byte, regBuf []byte) (ret int, usec int, err error) {
	log.Error("x86", "x86 execution is not supported on this platform")
	return -1, 0, fmt.Errorf("x86 execution is not supported on this platform")
}

func GetEcalliAddress() uintptr {
	log.Error("x86", "GetEcalliAddress is not supported on this platform")
	return 0
}

func GetSbrkAddress() uintptr {
	log.Error("x86", "GetSbrkAddress is not supported on this platform")
	return 0
}
