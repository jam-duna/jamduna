package node

import (
	"fmt"

	_ "net/http/pprof"
	"os"
)

const DefaultQuicPort = 9800
const DefaultTCPPort = 11100
const WSPort = 10800

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
func GetAddresses(local bool) (addresses []string, wsUrl string) {
	addresses = make([]string, 6)
	if local {
		for i := 0; i < 6; i++ {
			addresses[i] = fmt.Sprintf("localhost:%d", DefaultTCPPort+i)
		}
		wsUrl = fmt.Sprintf("ws://localhost:%d/ws", WSPort)
	} else {
		for i := 0; i < 6; i++ {
			addresses[i] = fmt.Sprintf("%s-%d.jamduna.org:%d", GetJAMNetwork(), i, DefaultTCPPort+i)
		}
		wsUrl = fmt.Sprintf("ws://%s-%d.jamduna.org:%d/ws", GetJAMNetwork(), 0, WSPort)
	}
	return addresses, wsUrl
}
