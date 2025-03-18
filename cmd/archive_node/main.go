package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func main() {
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelDebug, true)))
	log.EnableModule("block_sync")
	log.EnableModule("block")
	log.EnableModule("rpc")
	// read the peer list from the file
	peerList := make(map[uint16]*node.Peer)
	var peerListFile string
	var services_dir string
	var graphServerPort int
	var port int
	var webserverport int
	flag.StringVar(&peerListFile, "peer", "peerlist/jam.json", "the peer list file")
	flag.StringVar(&services_dir, "services_dir", "../../services", "the services directory")
	flag.IntVar(&graphServerPort, "graph_server_port", 15263, "the port for the graph server")
	flag.IntVar(&port, "port", 13000, "the port for the node")
	flag.IntVar(&webserverport, "webserverport", 8080, "the port for the webserver")
	flag.Parse()
	peerListJson, err := os.Open(peerListFile)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	err = json.NewDecoder(peerListJson).Decode(&peerList)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	peerListJson.Close()

	epoch0Timestamp := statedb.NewEpoch0Timestamp()
	// Test the archive node
	GenesisStateFile, GenesisBlockFile := node.GetGenesisFile("tiny")
	seed := make([]byte, 32)
	copy(seed[:], "colorful notion")
	archiveNode, err := node.NewArchiveNode(9999, seed,
		GenesisStateFile, GenesisBlockFile, epoch0Timestamp, peerList, "/tmp/archive_node_test", port, webserverport)
	if err != nil {
		fmt.Printf("Error: %s", err)
		os.Exit(1)
	} else {
		fmt.Println("ArchiveNode created successfully")
	}
	fmt.Printf("services_dir: %s\n", services_dir)
	archiveNode.SetServiceDir(services_dir)
	graphServer := types.NewGraphServer(15263)
	go graphServer.StartServer()
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			graphServer.Update(archiveNode.GetBlockTree())
		}
	}
}

func generatePeerNetwork(validators []types.Validator, port int, local bool) (peers []string, peerList map[uint16]*node.Peer, err error) {
	peerList = make(map[uint16]*node.Peer)
	if local {
		for i := uint16(0); i < types.TotalValidators; i++ {
			v := validators[i]
			baseport := 9900
			peerAddr := fmt.Sprintf("127.0.0.1:%d", baseport+int(i))
			peer := fmt.Sprintf("%s", v.Ed25519)
			peers = append(peers, peer)
			peerList[i] = &node.Peer{
				PeerID:    i,
				PeerAddr:  peerAddr,
				Validator: v,
			}
		}
	} else {
		for i := uint16(0); i < types.TotalValidators; i++ {
			v := validators[i]
			peerAddr := fmt.Sprintf("jam-%d:%d", i, port)
			peer := fmt.Sprintf("%s", v.Ed25519)
			peers = append(peers, peer)
			peerList[i] = &node.Peer{
				PeerID:    i,
				PeerAddr:  peerAddr,
				Validator: v,
			}
		}
	}
	return peers, peerList, nil
}
