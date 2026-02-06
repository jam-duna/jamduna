//go:build !linux || !amd64
// +build !linux !amd64

package recompiler

import "github.com/jam-duna/jamduna/log"

func GetEcalliAddress() uintptr {
	log.Error("x86", "GetEcalliAddress is not supported on this platform")
	return 0
}

func GetSbrkAddress() uintptr {
	log.Error("x86", "GetSbrkAddress is not supported on this platform")
	return 0
}
