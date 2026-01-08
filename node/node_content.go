package node

import (
	"fmt"
	"net"
	"strings"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	telemetry "github.com/colorfulnotion/jam/telemetry"
	types "github.com/colorfulnotion/jam/types"
)

func (n *NodeContent) GetBlockTree() *types.BlockTree {
	return n.block_tree
}

// PubkeyBytes converts an Ed25519 pubkey string (SAN or hex format) to [32]byte.
func PubkeyBytes(peerKey string) [32]byte {
	// Check if it's SAN format (starts with 'e' and is 53 chars)
	if len(peerKey) == 53 && peerKey[0] == 'e' {
		bytes, err := common.FromSAN(peerKey)
		if err == nil && len(bytes) == 32 {
			var result [32]byte
			copy(result[:], bytes)
			return result
		}
	}
	// Fall back to hex format
	return [32]byte(common.HexToHash(peerKey))
}

func (n *NodeContent) SetServiceDir(dir string) {
	n.loaded_services_dir = dir
}

// InitTelemetry initializes the telemetry client with the given host:port.
// An empty hostPort disables telemetry by keeping a no-op client configured.
func (n *Node) InitTelemetry(hostPort string) error {
	trimmed := strings.TrimSpace(hostPort)
	if trimmed == "" {
		client := telemetry.NewNoOpTelemetryClient()
		n.telemetryClient = client
		if n.store != nil {
			n.store.SetTelemetryClient(client)
		}
		return nil
	}

	host, port, err := net.SplitHostPort(trimmed)
	if err != nil {
		return fmt.Errorf("invalid telemetry address format, expected host:port, got: %s: %w", hostPort, err)
	}

	client := telemetry.NewTelemetryClient(host, port)

	nodeInfo, err := n.buildTelemetryNodeInfo()
	if err != nil {
		return fmt.Errorf("failed to build telemetry node info: %w", err)
	}

	if err := client.Connect(nodeInfo); err != nil {
		return fmt.Errorf("failed to connect telemetry client: %w", err)
	}

	n.telemetryClient = client
	if n.store != nil {
		n.store.SetTelemetryClient(client)
	}

	return nil
}

// buildTelemetryNodeInfo constructs the NodeInfo payload required by the
// telemetry server connection handshake.
func (n *Node) buildTelemetryNodeInfo() (telemetry.NodeInfo, error) {
	var info telemetry.NodeInfo

	if n.block_tree == nil {
		return info, fmt.Errorf("block tree not initialized")
	}

	root := n.block_tree.GetRoot()
	if root == nil || root.Block == nil {
		return info, fmt.Errorf("genesis block not available")
	}
	info.GenesisHeaderHash = root.Block.Header.HeaderHash()

	edKey := common.Hash(n.credential.Ed25519Pub)
	copy(info.PeerID[:], edKey.Bytes())

	listenerAddr := n.server.Addr()
	if listenerAddr == nil {
		return info, fmt.Errorf("network listener not initialized")
	}
	host, port, err := net.SplitHostPort(listenerAddr.String())
	if err != nil {
		return info, fmt.Errorf("invalid listen address %q: %w", listenerAddr.String(), err)
	}
	// Use 127.0.0.1 for telemetry if bound to any address (:: or 0.0.0.0)
	// This ensures consistent IPv4-mapped format in NodeInfo
	if host == "::" || host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	addrBytes, addrPort, err := telemetry.ParseTelemetryAddress(host, port)
	if err != nil {
		return info, fmt.Errorf("failed to encode peer address: %w", err)
	}
	info.PeerAddress = addrBytes
	info.PeerPort = addrPort

	if n.pvmBackend == pvm.BackendCompiler {
		info.NodeFlags |= 1
	}

	info.NodeName = n.node_name
	info.NodeVersion = n.GetBuild()
	info.Note = fmt.Sprintf("validator %d", n.id)

	return info, nil
}

func (n *NodeContent) LoadService(service_name string) ([]byte, error) {
	// read the .pvm from the service directory
	service_path := fmt.Sprintf("%s/%s.pvm", n.loaded_services_dir, service_name)
	return types.ReadCodeWithMetadata(service_path, service_name)
}

func IsWorkPackageInHistory(latestdb *statedb.StateDB, workPackageHash common.Hash) bool {
	for _, block := range latestdb.JamState.RecentBlocks.B_H {
		if len(block.Reported) != 0 {
			for _, segmentRootLookup := range block.Reported {
				if segmentRootLookup.WorkPackageHash == workPackageHash {
					return true
				}
			}
		}
	}

	prereqSetFromAccumulationHistory := make(map[common.Hash]struct{})
	for i := 0; i < types.EpochLength; i++ {
		for _, hash := range latestdb.JamState.AccumulationHistory[i].WorkPackageHash {
			prereqSetFromAccumulationHistory[hash] = struct{}{}
		}
	}
	_, exists := prereqSetFromAccumulationHistory[workPackageHash]
	return exists
}
