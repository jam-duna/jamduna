package main

import (
	"crypto/ed25519"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
)

func getGenesisFile(network string) (string, string) {
	return fmt.Sprintf("/chainspecs/traces/genesis-%s.json", network), fmt.Sprintf("/chainspecs/blocks/genesis-%s.json", network)
}

func getNextTimestampMultipleOf12() int {
	current := time.Now().Unix()
	future := current + 12
	remainder := future % 12
	if remainder != 0 {
		future += 12 - remainder
	}
	return int(future) + types.SecondsPerSlot
}

func startPProf(port int) *os.File {
	// stop the pprof file before the program exits	// start the pprof file
	filename := fmt.Sprintf("pprof_%d.prof", port)
	file, err := os.Create(filename)
	if err != nil {
		log.Crit(module, "startPProf", "err", err)
		return nil
	}

	if err := pprof.StartCPUProfile(file); err != nil {
		fmt.Printf("Error starting pprof: %v\n", err)
		file.Close()
		return nil
	}

	fmt.Printf("Started pprof profiling to file: %s\n", filename)
	return file
}

func main() {

	var validatorIndex int
	validators, secrets, err := node.GenerateValidatorNetwork()
	if err != nil {
		fmt.Printf("Error: %s", err)
		flag.PrintDefaults()
		os.Exit(0)
	}

	defaultTS := getNextTimestampMultipleOf12()
	// Parse the command-line flags into config
	config := &types.CommandConfig{}
	var help bool
	flag.BoolVar(&help, "h", false, "Displays help information about the commands and flags.")
	flag.StringVar(&config.DataDir, "datadir", filepath.Join(os.Getenv("HOME"), ".jam"), "Specifies the directory for the blockchain, keystore, and other data.")
	flag.IntVar(&config.Port, "port", 9000, "Specifies the network listening port.")
	flag.IntVar(&config.Epoch0Timestamp, "ts", defaultTS, "Epoch0 Unix timestamp (will override genesis config)")

	flag.IntVar(&validatorIndex, "validatorindex", 0, "Validator Index (only for development)")
	flag.StringVar(&config.GenesisState, "genesis", "", "Specifies the genesis state json file.")
	flag.StringVar(&config.Ed25519, "ed25519", "", "Ed25519 Seed (only for development)")
	flag.StringVar(&config.Bandersnatch, "bandersnatch", "", "Bandersnatch Seed (only for development)")
	flag.StringVar(&config.Bls, "bls", "", "BLS private key (only for development)")
	flag.StringVar(&config.NodeName, "metadata", "Alice", "Node metadata")
	flag.StringVar(&config.HostnamePrefix, "hp", "", "prefix for hostname prefix")
	flag.Parse()

	var pprofFile *os.File
	lastValidatorIndex := types.TotalValidators - 1
	// Start pprof server on specified nodes
	runtime.SetCPUProfileRate(10000000)
	switch config.Port {
	case 9000, 9001, lastValidatorIndex + 9000:
		pprofFile = startPProf(config.Port)
	}

	GenesisStateFile, GenesisBlockFile := getGenesisFile(types.Network)

	// If help is requested, print usage and exit
	if help {
		fmt.Println("Usage: jam [options]")
		flag.PrintDefaults()
		os.Exit(0)
	}
	// Use config.HostnamePrefix flag presence to decide which one to call
	var peers []string
	var peerList map[uint16]*node.Peer
	if len(config.HostnamePrefix) > 0 {
		peers, peerList, err = generatePeerNetworkHP(validators, config.HostnamePrefix, config.Port)
		if err != nil {
			fmt.Printf("generatePeerNetworkHP Error: %s", err)
			panic("generatePeerNetworkHP Error")
		}
	} else {
		peers, peerList, err = generatePeerNetwork(validators, config.Port)
		if err != nil {
			panic("generatePeerNetwork Error")
		}
	}
	epoch0Timestamp := uint32(0)

	if validatorIndex >= 0 && validatorIndex < types.TotalValidators && len(config.Bandersnatch) > 0 || len(config.Ed25519) > 0 {
		// set up validator secrets
		if _, _, err := setupValidatorSecret(config.Bandersnatch, config.Ed25519, config.Bls, config.NodeName); err != nil {
			fmt.Println("Error setting up validator secrets:", err)
			os.Exit(1)
		}
		// TODO: use the return values to check against the genesisConfig
	}
	config.GenesisState = GenesisStateFile
	config.GenesisBlock = GenesisBlockFile
	// Set up peers and node

	// _, err = node.NewNode(uint16(validatorIndex), secrets[validatorIndex], config.Genesis, epoch0Timestamp, peers, peerList, config.DataDir, config.Port)
	paths := SetLevelDBPaths(types.TotalValidators)
	n, err := node.NewNodeDA(uint16(validatorIndex), secrets[validatorIndex], config.GenesisState, config.GenesisBlock, epoch0Timestamp, peers, peerList, paths[validatorIndex], config.Port)
	if err != nil {
		fmt.Printf("Error: %v", err)
		panic(1)
	}
	pprofTime := 10 * time.Second
	n.RunDASimulation(pprofFile, pprofTime)
	// ticker := time.NewTicker(1 * time.Millisecond)
	// defer ticker.Stop()
	// for {
	// 	select {
	// 	case <-ticker.C:

	// 	}
	// }
}

func init() {
	pprof.StopCPUProfile() // Stop the CPU profile before the program exits
}

func generatePeerNetwork(validators []types.Validator, port int) (peers []string, peerList map[uint16]*node.Peer, err error) {
	peerList = make(map[uint16]*node.Peer)
	for i := uint16(0); i < types.TotalValidators; i++ {
		originalPort := uint16(9000)
		listernerPort := originalPort + i
		v := validators[i]
		peerAddr := fmt.Sprintf("127.0.0.1:%d", listernerPort)
		peer := fmt.Sprintf("%s", v.Ed25519)
		peers = append(peers, peer)
		peerList[i] = &node.Peer{
			PeerID:    i,
			PeerAddr:  peerAddr,
			Validator: v,
		}
	}
	return peers, peerList, nil
}

func generatePeerNetworkHP(validators []types.Validator, hp string, port int) (peers []string, peerList map[uint16]*node.Peer, err error) {
	peerList = make(map[uint16]*node.Peer)
	for i := uint16(0); i < types.TotalValidators; i++ {
		v := validators[i]
		peerAddr := fmt.Sprintf("%s%d:%d", hp, i, port)
		fmt.Printf("generatePeerNetworkHP: %d => %s\n", i, peerAddr)
		peer := fmt.Sprintf("%s", v.Ed25519)
		peers = append(peers, peer)
		peerList[i] = &node.Peer{
			PeerID:    i,
			PeerAddr:  peerAddr,
			Validator: v,
		}
	}
	return peers, peerList, nil
}

// setupValidatorSecret sets up the validator secret struct and validates input lengths
func setupValidatorSecret(bandersnatchHex, ed25519Hex, blsHex, metadata string) (validator types.Validator, secret types.ValidatorSecret, err error) {

	// Decode hex inputs
	bandersnatch_seed := common.FromHex(bandersnatchHex)
	ed25519_seed := common.FromHex(ed25519Hex)
	bls_secret := common.FromHex(blsHex)
	validator_meta := []byte(metadata)

	// Validate hex input lengths
	if len(bandersnatch_seed) != (bandersnatch.SecretLen) {
		return validator, secret, fmt.Errorf("invalid input length (%d) for bandersnatch seed %s - expected len of %d", len(bandersnatch_seed), bandersnatchHex, bandersnatch.SecretLen)
	}
	if len(ed25519_seed) != (ed25519.SeedSize) {
		return validator, secret, fmt.Errorf("invalid input length for ed25519 seed %s", ed25519Hex)
	}
	if len(bls_secret) != (types.BlsPrivInBytes) {
		return validator, secret, fmt.Errorf("invalid input length for bls private key %s", blsHex)
	}
	if len(validator_meta) > types.MetadataSizeInBytes {
		return validator, secret, fmt.Errorf("invalid input length for metadata %s", metadata)
	}

	validator, err = statedb.InitValidator(bandersnatch_seed, ed25519_seed, bls_secret, metadata)
	if err != nil {
		return validator, secret, err
	}
	secret, err = statedb.InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_secret, metadata)
	if err != nil {
		return validator, secret, err
	}
	return validator, secret, nil
}

func SetLevelDBPaths(numNodes int) []string {
	node_paths := make([]string, numNodes)
	// timeslot mark
	// currJCE := common.ComputeCurrentJCETime()
	currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
	currTS := currJCE * types.SecondsPerSlot
	for i := 0; i < numNodes; i++ {
		node_idx := fmt.Sprintf("%d", i)
		node_path, err := computeLevelDBPath(node_idx, int(currTS))
		if err == nil {
			node_paths[i] = node_path
		}
	}
	return node_paths
}

func computeLevelDBPath(id string, unixtimestamp int) (string, error) {
	/* standardize on
	/tmp/<user>/jam/<unixtimestamp>/testdb#

	/tmp/ntust/jam/1727903082/node1/leveldb/
	/tmp/ntust/jam/1727903082/node1/data/

	/tmp/root/jam/1727903082/node1/

	*/
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("could not get current user: %v", err)
	}
	username := currentUser.Username
	path := fmt.Sprintf("/tmp/%s/jam/%v/node%v", username, unixtimestamp, id)
	return path, nil
}
