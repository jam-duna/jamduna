package node

import (
	"fmt"

	_ "net/http/pprof"
	"os"
)

var DefaultQuicPort = 40000
var DefaultTCPPort = 11100
var WSPort = 19800

// Duna node indices
const (
	DunaLastValidatorNode = 5 // N5 is the last validator node (N0-N5 are validators)
	DunaBuilderNode       = 6 // N6 is the builder/full node
)

func GetJAMNetwork() string {
	if val := os.Getenv("JAM_NETWORK"); val != "" {
		return val
	}
	return "jam"
}

func GetJAMNetworkPort() int {
	if val := os.Getenv("JAM_NETWORK"); val != "" {
		return DefaultQuicPort
	}
	return DefaultQuicPort
}

func GetJAMNetworkWSPort() int {
	if val := os.Getenv("JAM_NETWORK"); val != "" {
		return WSPort
	}
	return WSPort
}

func GetAddresses(local bool) (address string, wsUrl string) {
	//i := DunaLastValidatorNode
	i := DunaBuilderNode
	if local {
		address = fmt.Sprintf("localhost:%d", DefaultTCPPort+i)
		wsUrl = fmt.Sprintf("ws://127.0.0.1:%d/ws", WSPort+i)
	} else {
		address = fmt.Sprintf("%s-%d.jamduna.org:%d", GetJAMNetwork(), i, DefaultTCPPort+i)
		wsUrl = fmt.Sprintf("ws://%s-%d.jamduna.org:%d/ws", GetJAMNetwork(), i, WSPort+i)
	}
	return
}
