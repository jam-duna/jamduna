package main

import (
	"crypto/ed25519"
	"strconv"

	"flag"
	"fmt"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"

	"os"
	"path/filepath"
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
func setUserPort(config *types.CommandConfig) (validator_indx int, is_local bool) {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println("Error getting current user:", err)
		os.Exit(1)
	}
	userName := hostname
	fmt.Printf("User: %s\n", userName)
	if userName == "rise" || userName == "jam-6" {
		config.Port = 9900
		return 4, false
	}
	if len(userName) >= 4 && userName[:3] == "jam" {
		number := userName[4:]
		intNum, err := strconv.Atoi(number)
		if err != nil {
			fmt.Println("Error getting the number after jam:", err)
			os.Exit(1)
		}
		fmt.Printf("User: %s, Number: %d\n", userName, intNum)
		config.Port = 9900
		return intNum, false
	} else {
		return config.Port - 9900, true
	}
}
func main() {
	log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelDebug, true)))
	log.EnableModule("blk_mod")
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
	var validatorIndex int
	var network string
	var start_time string
	flag.BoolVar(&help, "h", false, "Displays help information about the commands and flags.")
	flag.StringVar(&config.DataDir, "datadir", filepath.Join(os.Getenv("HOME"), ".jam"), "Specifies the directory for the blockchain, keystore, and other data.")
	flag.IntVar(&config.Port, "port", 9900, "Specifies the network listening port.")
	flag.IntVar(&config.Epoch0Timestamp, "ts", defaultTS, "Epoch0 Unix timestamp (will override genesis config)")
	flag.StringVar(&start_time, "start_time", "", "Start time in format: YYYY-MM-DD HH:MM:SS")
	flag.IntVar(&validatorIndex, "validatorindex", 0, "Validator Index (only for development)")
	flag.StringVar(&network, "net_spec", "", "Specifies the genesis state json file.")
	flag.StringVar(&config.Ed25519, "ed25519", "", "Ed25519 Seed (only for development)")
	flag.StringVar(&config.Bandersnatch, "bandersnatch", "", "Bandersnatch Seed (only for development)")
	flag.StringVar(&config.Bls, "bls", "", "BLS private key (only for development)")
	flag.StringVar(&config.NodeName, "metadata", "Alice", "Node metadata")
	flag.Parse()
	now := time.Now()
	loc := now.Location()

	fmt.Printf("System time: %s (%s)\n", now.Format("2006-01-02 15:04:05"), loc)

	config.GenesisState, config.GenesisBlock = node.GetGenesisFile(network)
	// If help is requested, print usage and exit
	if help {
		fmt.Println("Usage: jam [options]")
		flag.PrintDefaults()
		os.Exit(0)
	}
	validatorIndex, is_local := setUserPort(config)
	fmt.Printf("Starting node with port %d\n", config.Port)
	peers, peerList, err := generatePeerNetwork(validators, config.Port, is_local)
	for _, peer := range peerList {
		fmt.Printf("Peer %d: %s\n", peer.PeerID, peer.PeerAddr)
	}

	if validatorIndex >= 0 && validatorIndex < types.TotalValidators && len(config.Bandersnatch) > 0 || len(config.Ed25519) > 0 {
		// set up validator secrets
		if _, _, err := setupValidatorSecret(config.Bandersnatch, config.Ed25519, config.Bls, config.NodeName); err != nil {
			fmt.Println("Error setting up validator secrets:", err)
			os.Exit(1)
		}
		// TODO: use the return values to check against the genesisConfig
	}
	// to make sure our genesis timestamp is not too far from javajam setup
	if start_time != "" {
		for len(start_time) > 0 && (start_time[0] < '0' || start_time[0] > '9') {
			start_time = start_time[1:]
		}
		if len(start_time) > 0 && start_time[len(start_time)-1] == ' ' {
			start_time = start_time[:len(start_time)-1]
		}

		startTime, err := time.ParseInLocation("2006-01-02 15:04:05", start_time, loc)
		if err != nil {
			fmt.Printf("start_time: %s\n", start_time)
			fmt.Println("Invalid time format. Use YYYY-MM-DD HH:MM:SS")
			return
		}

		duration := time.Until(startTime)
		if duration <= 0 {
			fmt.Println("Start time already passed. Running now...")
		} else {
			fmt.Printf("Waiting until start time: %s (%v seconds remaining)\n",
				startTime.Format("2006-01-02 15:04:05"), duration.Seconds())
			const logInterval = 20 * time.Second

			for time.Until(startTime) > logInterval {
				fmt.Printf("Time remaining: %v\n", time.Until(startTime).Truncate(time.Second))
				time.Sleep(logInterval)
			}

			finalSleep := time.Until(startTime)
			if finalSleep > 0 {
				time.Sleep(finalSleep)
			}
			fmt.Println("Start time reached. Running now...")
		}
	}
	epoch0Timestamp := statedb.NewEpoch0Timestamp("jamtestnet")
	// Set up peers and node
	n, err := node.NewNode(uint16(validatorIndex), secrets[validatorIndex], config.GenesisState, config.GenesisBlock, epoch0Timestamp, peers, peerList, config.DataDir, config.Port)
	if err != nil {
		fmt.Printf("New Node Err:%s", err.Error())
		os.Exit(1)
	}
	n.SetSendTickets(false)
	n.SetServiceDir("/services")
	storage, err := n.GetStorage()
	defer storage.Close()
	if err != nil {
		fmt.Printf("GetStorage Err:%s", err.Error())
		os.Exit(1)
	}
	fmt.Printf("New Node %d started, edkey %v, port%d, time:%s\n", validatorIndex, secrets[validatorIndex].Ed25519Pub, config.Port, time.Now().String())
	timer := time.NewTimer(60 * time.Minute)
	<-timer.C
	fmt.Println("Node has been running for 45 minutes. Shutting down...")
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
			peerAddr := fmt.Sprintf("jam-%d.jamduna.org:%d", i, port)
			/*
			if i == 4 {
				peerAddr = "jam-6.jamduna.org:9900" // poland
			}
			if i == 5 {
				peerAddr = "jamtestnet-6.javajam.io:30300" // javajam
			}
			*/
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

const debug = false

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
