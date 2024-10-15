package main

import (
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func getNextTimestampMultipleOf12() int {
	current := time.Now().Unix()
	future := current + 12
	remainder := future % 12
	if remainder != 0 {
		future += 12 - remainder
	}
	return int(future) + types.SecondsPerSlot
}

func main() {
	validators, secrets, err := generateValidatorNetwork()
	if err != nil {
		fmt.Printf("Error: %s", err)
		flag.PrintDefaults()
		os.Exit(0)
	}
	defaultTS := getNextTimestampMultipleOf12()
	// Parse the command-line flags into config
	config := &types.CommandConfig{}
	var help bool
	var validatorIndex int
	flag.BoolVar(&help, "h", false, "Displays help information about the commands and flags.")
	flag.StringVar(&config.DataDir, "datadir", filepath.Join(os.Getenv("HOME"), ".jam"), "Specifies the directory for the blockchain, keystore, and other data.")
	flag.IntVar(&config.Port, "port", 9900, "Specifies the network listening port.")
	flag.IntVar(&config.Epoch0Timestamp, "ts", defaultTS, "Epoch0 Unix timestamp (will override genesis config)")

	flag.IntVar(&validatorIndex, "validatorindex", 0, "Validator Index (only for development)")
	flag.StringVar(&config.Genesis, "genesis", "", "Specifies the genesis state json file.")
	flag.StringVar(&config.Ed25519, "ed25519", "", "Ed25519 Seed (only for development)")
	flag.StringVar(&config.Bandersnatch, "bandersnatch", "", "Bandersnatch Seed (only for development)")
	flag.StringVar(&config.Bls, "bls", "", "BLS private key (only for development)")
	flag.StringVar(&config.NodeName, "metadata", "Alice", "Node metadata")
	flag.Parse()

	// If help is requested, print usage and exit
	if help {
		fmt.Println("Usage: jam [options]")
		flag.PrintDefaults()
		os.Exit(0)
	}
	peers, peerList, err := generatePeerNetwork(validators, config.Port)

	// Load and parse genesis file
	var genesisConfig statedb.GenesisConfig
	if len(config.Genesis) == 0 {
		// If no genesis file is provided, use default values with optional timestamp
		genesisConfig.Authorities = validators
	} else {
		// Load genesis file and parse it into genesisConfig
		data, err := ioutil.ReadFile(config.Genesis)
		if err != nil {
			os.Exit(0)
		}

		// Parse genesis file into genesisConfig
		err = json.Unmarshal(data, &genesisConfig)
		if err != nil {
			os.Exit(0)
		}
	}

	currTS := uint64(time.Now().Unix())
	if config.Epoch0Timestamp > 0 {
		if currTS >= uint64(config.Epoch0Timestamp) {
			fmt.Printf("Invalid Config. Now(%v) > Epoch0Timestamp (%v)", currTS, config.Epoch0Timestamp)
			os.Exit(1)
		}
		genesisConfig.Epoch0Timestamp = uint64(config.Epoch0Timestamp)
	} else if genesisConfig.Epoch0Timestamp == 0 && config.Epoch0Timestamp > 0 {
		genesisConfig.Epoch0Timestamp = currTS + 6
	}

	if validatorIndex >= 0 && validatorIndex < types.TotalValidators && len(config.Bandersnatch) > 0 || len(config.Ed25519) > 0 {
		// set up validator secrets
		if _, _, err := setupValidatorSecret(config.Bandersnatch, config.Ed25519, config.Bls, config.NodeName); err != nil {
			fmt.Println("Error setting up validator secrets:", err)
			os.Exit(1)
		}
		// TODO: use the return values to check against the genesisConfig
	}

	// Set up peers and node
	_, err = node.NewNode(uint16(validatorIndex), secrets[validatorIndex], &genesisConfig, peers, peerList, config.DataDir, config.Port)
	if err != nil {
		panic(1)
	}
	for {

	}
}

func generatePeerNetwork(validators []types.Validator, port int) (peers []string, peerList map[uint16]*node.Peer, err error) {
	peerList = make(map[uint16]*node.Peer)
	for i := uint16(0); i < types.TotalValidators; i++ {
		v := validators[i]
		peerAddr := fmt.Sprintf("node%d:%d", i, port)
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

func generateValidatorNetwork() (validators []types.Validator, secrets []types.ValidatorSecret, err error) {
	for i := uint32(0); i < types.TotalValidators; i++ {
		// assign metadata names for the first 6
		// nodeName := fmt.Sprintf("node%d", i)
		metadata := fmt.Sprintf("node%d", i)
		// Create hex strings for keys
		iHex := fmt.Sprintf("%x", i)
		if len(iHex)%2 != 0 {
			iHex = "0" + iHex
		}
		iHexByteLen := len(iHex) / 2
		ed25519Hex := fmt.Sprintf("0x%s%s", strings.Repeat("00", types.Ed25519SeedInBytes-iHexByteLen), iHex)
		bandersnatchHex := fmt.Sprintf("0x%s%s", strings.Repeat("00", bandersnatch.SecretLen-iHexByteLen), iHex)
		blsHex := fmt.Sprintf("0x%s%s", strings.Repeat("00", types.BlsPrivInBytes-iHexByteLen), iHex)

		// Set up the secret/validator using hex values
		v, s, err := setupValidatorSecret(bandersnatchHex, ed25519Hex, blsHex, metadata)
		if err != nil {
			return validators, secrets, err
		}
		validators = append(validators, v)
		secrets = append(secrets, s)
	}
	return validators, secrets, nil
}

const debug = false

// setupValidatorSecret sets up the validator secret struct and validates input lengths
func setupValidatorSecret(bandersnatchHex, ed25519Hex, blsHex, metadata string) (validator types.Validator, secret types.ValidatorSecret, err error) {

	// Decode hex inputs
	bandersnatch_seed := common.FromHex(bandersnatchHex)
	ed25519_seed := common.FromHex(ed25519Hex)
	bls_secret := common.FromHex(blsHex)
	validator_meta := []byte(metadata)

	// Validate hex input lengths
	if debug {
		fmt.Printf("bandersnatchHex: %s\n", bandersnatchHex)
		fmt.Printf("ed25519Hex: %s\n", ed25519Hex)
		fmt.Printf("blsHex: %s\n", blsHex)
		fmt.Printf("metadata: %s\n", metadata)
	}
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
