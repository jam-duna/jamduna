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

func main() {
	validators, secrets, peers, peerList, err := generateValidatorNetwork(types.TotalValidators)
	if err != nil {
		fmt.Println("Error: %s", err)
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Parse the command-line flags into config
	config := &types.CommandConfig{}
	var help bool
	var validatorIndex int
	flag.BoolVar(&help, "h", false, "Displays help information about the commands and flags.")
	flag.StringVar(&config.DataDir, "datadir", filepath.Join(os.Getenv("HOME"), ".jam"), "Specifies the directory for the blockchain, keystore, and other data.")
	flag.IntVar(&config.Port, "port", 9900, "Specifies the network listening port.")
	flag.IntVar(&config.Epoch0Timestamp, "ts", 0, "Epoch0 Unix timestamp (will override genesis config)")

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
	if config.Epoch0Timestamp > 0 {
		genesisConfig.Epoch0Timestamp = uint64(config.Epoch0Timestamp)
	} else if genesisConfig.Epoch0Timestamp == 0 && config.Epoch0Timestamp > 0 {
		genesisConfig.Epoch0Timestamp = uint64(time.Now().Unix()) + 6
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
	_, err = node.NewNode(config.NodeName, secrets[validatorIndex], &genesisConfig, peers, peerList)
	if err != nil {
		panic(1)
	}
	fmt.Println(genesisConfig.String())
	for {

	}
}

func generateValidatorNetwork(N uint32) (validators []types.Validator, secrets []types.ValidatorSecret, peers []string, peerList map[string]node.NodeInfo, err error) {
	peerList = make(map[string]node.NodeInfo)
	for i := uint32(0); i < N; i++ {
		var nodeName string
		// assign metadata names for the first 6
		switch i {
		case 0:
			nodeName = "Alice"
		case 1:
			nodeName = "Bob"
		case 2:
			nodeName = "Charlie"
		case 3:
			nodeName = "Dave"
		case 4:
			nodeName = "Eve"
		case 5:
			nodeName = "Fergie"
		default:
			nodeName = fmt.Sprintf("Node%d", i)
		}
		peerAddr := fmt.Sprintf("localhost:%d", 9900+i)
		remoteAddr := fmt.Sprintf("localhost:%d", 9900+i)
		metadata := fmt.Sprintf("%s:%s", remoteAddr, nodeName)
		// Create hex strings for keys
		iHex := fmt.Sprintf("%02x", i)
		ed25519Hex := fmt.Sprintf("0x%s%s", strings.Repeat("00", types.Ed25519SeedInBytes-1), iHex)
		bandersnatchHex := fmt.Sprintf("0x%s%s", strings.Repeat("00", bandersnatch.SecretLen-1), iHex)
		blsHex := fmt.Sprintf("0x%s%s", strings.Repeat("00", types.BlsPrivInBytes-1), iHex)
		// Set up the secret/validator using hex values
		v, s, err := setupValidatorSecret(bandersnatchHex, ed25519Hex, blsHex, metadata)
		if err != nil {
			return validators, secrets, peers, peerList, err
		}
		peer := fmt.Sprintf("%x", v.Ed25519)
		validators = append(validators, v)
		secrets = append(secrets, s)
		peers = append(peers, peer)
		peerList[peer] = node.NodeInfo{
			PeerID:     i,
			PeerAddr:   peerAddr,
			RemoteAddr: remoteAddr,
			Validator:  v,
		}
	}
	return validators, secrets, peers, peerList, nil
}

const debug = false

// setupValidatorSecret sets up the validator secret struct and validates input lengths
func setupValidatorSecret(bandersnatchHex, ed25519Hex, blsHex, metadata string) (validator types.Validator, secret types.ValidatorSecret, err error) {
	// Validate hex input lengths
	if debug {
		fmt.Printf("bandersnatchHex: %s\n", bandersnatchHex)
		fmt.Printf("ed25519Hex: %s\n", ed25519Hex)
		fmt.Printf("blsHex: %s\n", blsHex)
		fmt.Printf("metadata: %s\n", metadata)
	}
	if len(bandersnatchHex) != (bandersnatch.SecretLen+1)*2 {
		return validator, secret, fmt.Errorf("invalid input length (%d) for bandersnatch seed %s - expected len of %d", bandersnatchHex, len(bandersnatchHex), (bandersnatch.SecretLen+1)*2)
	}
	if len(ed25519Hex) != (ed25519.SeedSize+1)*2 {
		return validator, secret, fmt.Errorf("invalid input length for ed25519 seed %s", ed25519Hex)
	}
	if len(blsHex) != (types.BlsPrivInBytes+1)*2 {
		return validator, secret, fmt.Errorf("invalid input length for bls private key %s", blsHex)
	}
	if len(metadata) > types.MetadataSizeInBytes {
		return validator, secret, fmt.Errorf("invalid input length for metadata %s", metadata)
	}

	// Decode hex inputs
	bandersnatch_seed := common.FromHex(bandersnatchHex)
	ed25519_seed := common.FromHex(ed25519Hex)
	bls_secret := common.FromHex(blsHex)
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
