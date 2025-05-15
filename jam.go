package main

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"

	"fmt"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/node"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/types"
	"github.com/spf13/cobra"

	"os"
	"path/filepath"
	"time"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func main() {

	// cmd
	var rootCmd = &cobra.Command{
		Use:   "./jamduna",
		Short: "JAM DUNA node",
	}
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	var (
		help         bool
		configPath   string
		temp         bool
		version      bool
		pvm_output   string
		pvm_sampling int
		//run flags
		dataPath       string
		chainSpec      string
		Port           int
		RPCPort        int
		validatorIndex int
		network        string
		start_time     string
		GenesisState   string
		logLevel       string

		// run flags that is not supported yet
		pvmBackend  string
		peerID      int
		externalIP  string
		listenIP    string
		rpcListenIP string
		bootnode    string
		telemetry   string

		// run variables
		validatorIndexFlagSet bool
		chainSpecFlagSet      bool
		genesisFileSet        bool
		start_timeFlagSet     bool

		//run-stf flags
		stfFile    string
		outputFile string

		genesisType       = "stf"
		genesis_real_file interface{}
	)

	var (
		helpFlag         = "help"
		logLevelFlag     = "log-level"
		tempFlag         = "temp"
		versionFlag      = "version"
		pvm_outputFlag   = "pvm-output"
		pvm_samplingFlag = "pvm-sampling"

		// run-stf flags
		stfFileFlag    = "intput-file"
		outputFileFlag = "output-file"

		// run flags
		dataPathFlag       = "data-path"
		PortFlag           = "port"
		RPCPortFlag        = "rpc-port"
		startTimeFlag      = "start-time"
		validatorIndexFlag = "dev-validator"
		networkFlag        = "net-spec"
		chainSpecFlag      = "chain"

		//run flags that is not supported yet
		pvmBackendFlag  = "pvm-backend"
		peerIDFlag      = "peer-id"
		externalIPFlag  = "external-ip"
		listenIPFlag    = "listen-ip"
		rpcListenIPFlag = "rpc-listen-ip"
		bootnodeFlag    = "bootnode"
		telemetryFlag   = "telemetry"
	)
	rootCmd.PersistentFlags().BoolVarP(&help, helpFlag, "h", false, "Displays help information about the commands and flags.")
	rootCmd.PersistentFlags().StringVarP(&logLevel, logLevelFlag, "l", "debug", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().BoolVarP(&temp, tempFlag, "t", false, "Use a temporary data directory, removed on exit. Conflicts with data-path")
	rootCmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to the config file")
	rootCmd.PersistentFlags().BoolVarP(&version, versionFlag, "v", false, "Prints the version of the program.")
	rootCmd.PersistentFlags().StringVar(&pvm_output, pvm_outputFlag, "", "For both test-refine and test-stf, generates JSONNL separated execution trace with {step, pc, g, r} params")
	rootCmd.PersistentFlags().IntVarP(&pvm_sampling, pvm_samplingFlag, "s", 1, "If --pvm-output is supplied, only outputs a line when step % pvm-sampling is 0 (default 1)")
	//gen-keys
	var genKeysCmd = &cobra.Command{
		Use:   "gen-keys",
		Short: "Generate keys for validators, pls generate keys for all validators before running the node",
		Run: func(cmd *cobra.Command, args []string) {
			// Generate keys for validators
			_, _, err := GenerateValidatorSecretSet(types.TotalValidators, true, dataPath)
			if err != nil {
				fmt.Printf("Error generating keys: %s", err)
				os.Exit(1)
			}
		},
	}

	// list-keys
	var listKeysCmd = &cobra.Command{
		Use:   "list-keys",
		Short: "List keys for validators",
		Run: func(cmd *cobra.Command, args []string) {
			keysPath := filepath.Join(dataPath, "keys")
			files, err := os.ReadDir(keysPath)
			if err != nil {
				fmt.Printf("Error reading keys directory: %s\n", err)
				os.Exit(1)
			}

			for _, file := range files {
				seedFile := filepath.Join(keysPath, file.Name())
				seed, err := os.ReadFile(seedFile)
				if err != nil {
					fmt.Printf("Error reading seed file: %s\n", err)
					os.Exit(1)
				}
				seed = seed[:32]

				validator, err := generateSelfValidatorPubKey(seed)
				if err != nil {
					fmt.Printf("Error generating validator from seed: %s\n", err)
					os.Exit(1)
				}
				fmt.Println("--------------------------------------------------")
				fmt.Printf("%-14s %s\n", "file:", file.Name())
				fmt.Printf("%-14s %x\n", "seed:", seed)
				fmt.Printf("%-14s %v\n", "ed25519:", validator.Ed25519)
				fmt.Printf("%-14s %s\n", "bandersnatch:", common.BytesToHexStr(validator.Bandersnatch[:]))
				fmt.Printf("%-14s %s\n", "bls:", common.BytesToHexStr(validator.Bls[:]))
				fmt.Printf("%-14s %s\n", "metadata:", common.BytesToHexStr(validator.Metadata[:]))
				fmt.Printf("%-14s %s\n\n", "dns_alt_name:", node.ToSAN(validator.Ed25519[:]))
			}
		},
	}

	var runCmdSTF = &cobra.Command{
		Use:   "test-stf",
		Short: "Run the STF Validation",
		Run: func(cmd *cobra.Command, args []string) {
			// Run the STF Validation
			if stfFile == "" {
				fmt.Println("Error: --file-path is required.")
				os.Exit(1)
			}
			// run the stf validation
			_, err := statedb.ValidateStateTransitionFile(stfFile, dataPath, outputFile)
			if err != nil {
				fmt.Printf("Error running STF Validation: %s", err)
				os.Exit(1)
			} else {
				fmt.Printf("\033[32mSTF Validation passed.File:%s\033[0m\n ", stfFile)
			}

		},
	}

	// test-stf flag used
	runCmdSTF.Flags().StringVarP(&stfFile, stfFileFlag, "f", "", "Specifies the path to the STF file.")
	runCmdSTF.Flags().StringVarP(&dataPath, dataPathFlag, "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Specifies the directory for the blockchain, keystore, and other data.")
	runCmdSTF.Flags().StringVarP(&outputFile, outputFileFlag, "o", "", "Specifies the output file for the STF validation.")

	var testRefineCmd = &cobra.Command{
		Use:   "test-refine",
		Short: "Run the refine test",
		Run: func(cmd *cobra.Command, args []string) {
			// Run the refine test
			fmt.Printf("not implemented yet")
			os.Exit(1)
		},
	}
	// test-refine flag used

	var genSpecCmd = &cobra.Command{
		Use:   "gen-spec",
		Short: "Generate new chain spec from the spec config",
		Run: func(cmd *cobra.Command, args []string) {
			// Generate new chain spec from the spec config
			fmt.Printf("not implemented yet")
			os.Exit(1)
		},
	}
	// gen-spec flag used

	// run node command
	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "Run the JAM DUNA node",
		Run: func(cmd *cobra.Command, args []string) {

			if cmd.Flags().Changed(validatorIndexFlag) {
				validatorIndexFlagSet = true
			}
			if cmd.Flags().Changed(networkFlag) {
				genesisFileSet = true
			}
			if cmd.Flags().Changed(chainSpecFlag) {
				chainSpecFlagSet = true
			}
			if cmd.Flags().Changed(startTimeFlag) {
				start_timeFlagSet = true
			}
			if cmd.Flags().Changed(logLevelFlag) {
				logLevel, _ = cmd.Flags().GetString("log_level")
				if logLevel == "debug" {
					monitor = true
				}
			}
			if cmd.Flags().Changed(dataPathFlag) {
				dataPath, _ = cmd.Flags().GetString(dataPathFlag)
			}
			if cmd.Flags().Changed(RPCPortFlag) {
				node.WSPort = RPCPort
			}
			// check if the flags is invalid or not
			if chainSpecFlagSet && genesisFileSet {
				fmt.Println("Error: --chain and --net_spec cannot be used together.")
				os.Exit(1)
			}
			// print all the flags values
			// use yellow color

			fmt.Printf("Running JAM DUNA node with the following flags:\n")
			fmt.Printf("\033[33mdataPath: %s, Port: %d, RPCPort: %d, validatorIndex: %d, network: %s, chainSpec: %s, logLevel: %s, start_time: %s, GenesisState: %s\033[0m\n", dataPath, Port, RPCPort, validatorIndex, network, chainSpec, logLevel, start_time, GenesisState)

			var err error
			var validators []types.Validator
			var secrets []types.ValidatorSecret
			var selfSecret types.ValidatorSecret
			// Run the JAM DUNA node
			now := time.Now()
			loc := now.Location()
			log.InitLogger(logLevel)
			//log.EnableModule(log.BlockMonitoring)
			var peers []string
			var peerList map[uint16]*node.Peer
			genesis_real_file = network
			if !chainSpecFlagSet {
				//fmt.Printf("AAAAAA")

				fmt.Printf("\033[34mGetting validator port from genesis file...\033[0m\n")
				if genesisFileSet {
					fmt.Printf("Using genesis file: %s\n", network)
				} else {
					fmt.Printf("Using default genesis file: %s\n", network)
				}
				GenesisState = node.GetGenesisFile(network)

				is_local := false
				if !strings.Contains(network, "with_metadata") && network != "" && chainSpec == "" {
					validatorIndex, is_local = setUserPort(Port)
				} else {
					is_local = true
				}
				fmt.Printf("Validator index: %d, is local :%v \n", validatorIndex, is_local)
				fmt.Printf("\033[34mGenerating validator keys...\033[0m\n")
				validators, secrets, err = GenerateValidatorSecretSet(types.TotalValidators, true) // there is no reference data , so we generate it
				if err != nil {
					fmt.Printf("Error: %s", err)
					os.Exit(0)
				}
				peers, peerList, err = generatePeerNetwork(validators, Port, is_local)
				if err != nil {
					fmt.Printf("Error generating peer network: %s", err)
					os.Exit(1)
				}
				selfSecret = secrets[validatorIndex]

				if is_local {
					dataPath = filepath.Join(dataPath, "jam-"+strconv.Itoa(validatorIndex))
				}

			} else { // polkajam mode

				//fmt.Printf("BBBBB")

				genesisType = "chainspec"

				fmt.Printf("\033[34mGetting validator port from chainSpec...\033[0m\n")
				// get peers from chainSpec json file
				chainSpecJsonFile, err := os.Open(chainSpec)
				if err != nil {
					fmt.Printf("Error opening chainSpec json file: %s", err)
					os.Exit(1)
				}
				defer chainSpecJsonFile.Close()
				// unmarshal the json file

				peerList = make(map[uint16]*node.Peer)
				peers = make([]string, 0)
				var chainSpecData node.ChainSpec
				err = json.NewDecoder(chainSpecJsonFile).Decode(&chainSpecData)
				if err != nil {
					fmt.Printf("Error decoding chainSpec json file: %s", err)
					os.Exit(1)
				}
				genesis_real_file = chainSpecData
				validators, err = getValidatorFromChainSpec(chainSpecData)
				for i, bootnode := range chainSpecData.Bootnodes {
					parts := strings.Split(bootnode, "@")
					if len(parts) != 2 {
						log.Crit("invalid bootnode format", "bootnode", bootnode)
					}
					peerID := parts[0]
					addr := parts[1] // e.g. 127.0.0.1:40000

					ip, portStr, err := net.SplitHostPort(addr)
					if err != nil {
						log.Crit("invalid addr", "addr", addr, "err", err)
					}

					fmt.Printf("parsed peerID=%s, ip=%s, port=%s\n", peerID, ip, portStr)

					peerList[uint16(i)] = &node.Peer{
						PeerID:    uint16(i),
						PeerAddr:  addr,
						Validator: validators[i],
					}
				}
				if err != nil {
					fmt.Printf("Error unmarshalling chainSpec json file: %s", err)
					os.Exit(1)
				}
				selfSecret = CheckValidatorInfo(validatorIndex, peerList, dataPath)
				for _, peer := range peerList {
					peers = append(peers, fmt.Sprintf("%v", peer.Validator.Ed25519))
				}
				if validatorIndexFlagSet {
					self_peer := peerList[uint16(validatorIndex)]
					//get the port from the address
					fmt.Printf("setting validatorIndex %d to %s\n", validatorIndex, self_peer.PeerAddr)
					_, portStr, err := net.SplitHostPort(self_peer.PeerAddr)
					if err != nil {
						fmt.Printf("Error converting peer address to port: %s", err)
						os.Exit(1)
					}
					//stoi the port
					Port, err = strconv.Atoi(portStr)
					if err != nil {
						fmt.Printf("Error converting peer address to port: %s", err)
						os.Exit(1)
					}
					fmt.Printf("Port from chainSpec: %d\n", Port)
				}
				dataPath = filepath.Join(dataPath, "jam-"+strconv.Itoa(validatorIndex))
			}

			// to make sure our genesis timestamp is not too far from javajam setup
			if start_timeFlagSet {
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
			epoch0Timestamp := statedb.NewEpoch0Timestamp("jam", start_time)
			// Set up peers and node
			for i := 0; i < len(peerList); i++ {
				//fmt.Printf("!!! Peer %d: %s, key %v\n", i, peerList[uint16(i)].PeerAddr, peerList[uint16(i)].Validator.Ed25519)
			}
			//fmt.Printf("Validator %d: %s\n", validatorIndex, peerList[uint16(validatorIndex)].PeerAddr)

			n, err := node.NewNode(uint16(validatorIndex), selfSecret, genesis_real_file, genesisType, epoch0Timestamp, peers, peerList, dataPath, Port)
			if err != nil {
				fmt.Printf("New Node Err:%s", err.Error())
				os.Exit(1)
			}
			n.SetServiceDir("/services")
			n.WriteDebugFlag = false
			storage, err := n.GetStorage()
			defer storage.Close()
			if err != nil {
				fmt.Printf("GetStorage Err:%s", err.Error())
				os.Exit(1)
			}
			fmt.Printf("New Node %d started, edkey %v, port%d, time:%s. buildVersion=%v\n", validatorIndex, selfSecret.Ed25519Pub, Port, time.Now().String(), n.GetBuild())
			StartRuntimeMonitor(30 * time.Second)
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:

				}
			}
		},
	}

	// data path
	runCmd.Flags().StringVarP(&dataPath, dataPathFlag, "d", filepath.Join(os.Getenv("HOME"), ".jamduna"), "Specifies the directory for the blockchain, keystore, and other data.")
	runCmd.Flags().IntVar(&Port, PortFlag, node.GetJAMNetworkPort(), "Specifies the network listening port.")
	runCmd.Flags().IntVar(&RPCPort, RPCPortFlag, node.GetJAMNetworkWSPort(), "Specifies the RPC listening port.")
	runCmd.Flags().StringVar(&start_time, startTimeFlag, "", "Start time in format: YYYY-MM-DD HH:MM:SS")
	runCmd.Flags().IntVar(&validatorIndex, validatorIndexFlag, 0, "Validator Index (only for development)")
	runCmd.Flags().StringVar(&network, networkFlag, "tiny", "Specifies the genesis state json file.(Only for development)")
	runCmd.Flags().StringVar(&chainSpec, chainSpecFlag, "chainspec.json", `Chain to run. "polkadot", "dev", or the path of a chain spec file`)

	desc := flagDescription("The PVM backend to use", map[string]string{
		"interpreter": "Use a PVM interpreter. Slow, but works everywhere",
		"compiler":    "Use a PVM recompiler. Fast, but is Linux-only",
	})
	runCmd.Flags().StringVar(&pvmBackend, pvmBackendFlag, "interpreter", desc)
	runCmd.Flags().IntVar(&peerID, peerIDFlag, 0, "Peer ID of this node. If not specified, a new peer ID will be generated. The corresponding secret key will not be persisted.")
	runCmd.Flags().StringVar(&externalIP, externalIPFlag, "", "External IP of this node, as used by other nodes to connect. If not specified, this will be guessed.")
	runCmd.Flags().StringVar(&listenIP, listenIPFlag, "", "IP address to listen on. `::` (the default) means all addresses. [default: ::]")
	runCmd.Flags().StringVar(&rpcListenIP, rpcListenIPFlag, "", "IP address for RPC server to listen on. `::` (the default) means all addresses. [default: ::]")
	runCmd.Flags().StringVar(&bootnode, bootnodeFlag, "", "Specify a bootnode")
	runCmd.Flags().StringVar(&telemetry, telemetryFlag, "", " Send data to TART server (JIP-3)")

	// add commands to root
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(genKeysCmd)
	rootCmd.AddCommand(listKeysCmd)
	rootCmd.AddCommand(runCmdSTF)
	rootCmd.AddCommand(testRefineCmd)
	rootCmd.AddCommand(genSpecCmd)
	// parse the persistent flags (Global flags)
	rootCmd.PersistentFlags().Parse(os.Args[1:])
	if version {
		fmt.Printf("Version: %s, Commit: %s, BuildTime: %s\n", Version, Commit, BuildTime)
		os.Exit(0)
	}
	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error executing command:", err)
		os.Exit(1)
	}
	// fmt.Println("JAM DUNA node started")

}

func CheckValidatorInfo(validatorIndex int, peerList map[uint16]*node.Peer, dataPath string) types.ValidatorSecret {
	// get the seed from the data dir
	dataPath = filepath.Join(dataPath, "keys") // store the keys in a subdir
	seedFile := filepath.Join(dataPath, fmt.Sprintf("seed_%d", validatorIndex))
	seed, err := os.ReadFile(seedFile)
	if err != nil {
		fmt.Printf("Error reading seed file: %s", err)
		os.Exit(1)
	}
	seed = seed[:32]
	// generate the validator from the seed
	selfSecrets, err := statedb.InitValidatorSecret(seed, seed, seed, "")
	if selfSecrets.BandersnatchPub != peerList[uint16(validatorIndex)].Validator.Bandersnatch {
		fmt.Printf("Error: seed file does not match the metadata. %s", err)
		os.Exit(1)
	}
	return selfSecrets
}

func generatePeerNetwork(validators []types.Validator, port int, local bool) (peers []string, peerList map[uint16]*node.Peer, err error) {
	peerList = make(map[uint16]*node.Peer)
	if local {
		for i := uint16(0); i < types.TotalValidators; i++ {
			v := validators[i]
			baseport := node.GetJAMNetworkPort()
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
			peerAddr := fmt.Sprintf("%s-%d.jamduna.org:%d", node.GetJAMNetwork(), i, port)
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
func GenerateValidatorSecretSet(numNodes int, save bool, dataDir ...string) ([]types.Validator, []types.ValidatorSecret, error) {

	seeds, _ := generateSeedSet(numNodes)
	validators := make([]types.Validator, numNodes)
	validatorSecrets := make([]types.ValidatorSecret, numNodes)

	for i := 0; i < int(numNodes); i++ {

		seed_i := seeds[i]
		if len(dataDir) != 0 {
			keyDir := dataDir[0]
			keyDir = filepath.Join(keyDir, "keys") // store the keys in a subdir
			// if there is no seed file, create it
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				return validators, validatorSecrets, fmt.Errorf("Failed to create keys directory %s: %v", dataDir, err)
			}
			if save {
				seedFile := filepath.Join(keyDir, fmt.Sprintf("seed_%d", i))

				if _, err := os.Stat(seedFile); os.IsNotExist(err) {
					// create the file
					f, err := os.Create(seedFile)
					if err != nil {
						return validators, validatorSecrets, fmt.Errorf("Failed to create seed file %s", seedFile)
					}
					// write the seed to the file
					_, err = f.Write(seed_i)
					if err != nil {
						return validators, validatorSecrets, fmt.Errorf("Failed to write seed to file %s", seedFile)
					}
					fmt.Printf("Seed file %s created\n", seedFile)
					f.Close()
				}

			}
		}

		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata := ""
		//metadata, _ := generateMetadata(i) // this is NOT used by other teams. somehow we agreed on empty metadata for now

		validator, err := statedb.InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("Failed to init validator %v", i)
		}
		validators[i] = validator

		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := statedb.InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("Failed to init validator secret=%v", i)
		}
		validatorSecrets[i] = validatorSecret
	}

	return validators, validatorSecrets, nil
}
func setUserPort(Port int) (validator_indx int, is_local bool) {
	hostname, err := os.Hostname()
	if err != nil {
		fmt.Println("Error getting current user:", err)
		os.Exit(1)
	}
	userName := hostname
	fmt.Printf("User: %s\n", userName)
	if userName == "rise" || userName == "jam-6" {
		Port = node.GetJAMNetworkPort()
		return 5, false
	}
	if len(userName) >= 4 && (userName[:3] == "jam" || userName[:3] == "dot") {
		number := userName[4:]
		intNum, err := strconv.Atoi(number)
		if err != nil {
			fmt.Println("Error getting the number after jam/dot:", err)
			os.Exit(1)
		}
		fmt.Printf("User: %s, Number: %d\n", userName, intNum)
		Port = node.GetJAMNetworkPort()
		return intNum, false
	} else {
		return Port - node.GetJAMNetworkPort(), true
	}
}
func generateSelfValidatorPubKey(seed []byte) (types.Validator, error) {
	// Generate the validator public key from the seed
	validator, err := statedb.InitValidator(seed, seed, seed, "")
	if err != nil {
		return types.Validator{}, fmt.Errorf("Failed to init validator %v", err)
	}
	return validator, nil
}

func generateSeedSet(ringSize int) ([][]byte, error) {
	ringSet := make([][]byte, ringSize)
	for i := uint32(0); i < uint32(ringSize); i++ {
		seed := make([]byte, 32)
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(seed[j*4:], i)
		}
		ringSet[i] = seed
	}
	return ringSet, nil
}
func StartRuntimeMonitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		var mem runtime.MemStats
		var highest, count int

		// Immediate first report
		const asMB = 1024 * 1024
		runtime.ReadMemStats(&mem)
		count = runtime.NumGoroutine()
		highest = count
		allocMB := mem.Alloc / asMB
		// if mem > 4GB panic

		totalAllocMB := mem.TotalAlloc / asMB
		sysMB := mem.Sys / asMB
		if monitor {
			fmt.Printf("%-22s 🧠 Memory:%4dMB | 💾 TotalAlloc:%4dMB | 📦 Sys:%4dMB | ♻️ GC:%4d | 🧵 Goroutines:%4d\n",
				"[MONITOR New Record]", allocMB, totalAllocMB, sysMB, mem.NumGC, count)
		}

		// Then on every tick
		for range ticker.C {
			runtime.ReadMemStats(&mem)
			count = runtime.NumGoroutine()
			allocMB = mem.Alloc / asMB
			totalAllocMB = mem.TotalAlloc / asMB
			sysMB = mem.Sys / asMB
			if allocMB > 4096 {
				fmt.Printf("❗ Memory usage is too high: %dMB\n", allocMB)
				// print stack trace
				buf := make([]byte, 1<<20)
				stackSize := runtime.Stack(buf, true)
				fmt.Printf("Stack trace:\n%s\n", buf[:stackSize])
				// exit with error code
				fmt.Println("OOM: Out of memory")
				// os.Exit(1)
				dumpHeapProfile(fmt.Sprintf("/tmp/heap_dump_%d.pprof", time.Now().Unix()))
				os.Exit(1)
			}
			label := "[MONITOR]"
			if count > highest {
				highest = count
				label = "[MONITOR New Record]"
			}
			if monitor {
				fmt.Printf("%-22s 🧠 Memory:%4dMB | 💾 TotalAlloc:%4dMB | 📦 Sys:%4dMB | ♻️ GC:%4d | 🧵 Goroutines:%4d\n",
					label, allocMB, totalAllocMB, sysMB, mem.NumGC, count)
			}
		}
	}()
}

var monitor = false

func dumpHeapProfile(filename string) {
	f, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	pprof.Lookup("heap").WriteTo(f, 0)
}

func getValidatorPortFromMetadata(network_file string, validator_idx int) (selfPort uint16, peerList map[uint16]*node.Peer, err error) {
	// Get the metadata from the validator
	path := node.GetGenesisFile(network_file)
	fn := common.GetFilePath(path)
	snapshotRawBytes, err := os.ReadFile(fn)
	var statetransition statedb.StateTransition
	err = json.Unmarshal(snapshotRawBytes, &statetransition)
	if err != nil {
		fmt.Printf("Error unmarshalling JSON: %v\n", err)
		return
	}
	var currValidatorRawBytes []byte
	for _, keyval := range statetransition.PostState.KeyVals {
		if keyval.Key[0] == 0x08 {
			currValidatorRawBytes = keyval.Value
			break
		}
	}
	currValidatorRaw, _, err := types.Decode(currValidatorRawBytes, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return
	}
	currValidators := currValidatorRaw.(types.Validators)
	if validator_idx >= len(currValidators) {
		fmt.Printf("Validator index %d out of range (0-%d)\n", validator_idx, len(currValidators)-1)
		return
	}
	peerList = make(map[uint16]*node.Peer)
	for i := 0; i < len(currValidators); i++ {
		validator := currValidators[i]
		ip_str, port, err := common.ToIPv6Port(validator.Metadata[:])
		if err != nil {
			fmt.Printf("Error converting metadata to IP and port: %s\n", err)
			return 0, nil, err
		}
		fmt.Printf("Validator %d: IP: %s, Port: %d\n", i, ip_str, port)
		if i == validator_idx {
			selfPort = uint16(port)
		}
		peerList[uint16(i)] = &node.Peer{
			PeerID:    uint16(i),
			PeerAddr:  fmt.Sprintf("%s:%d", ip_str, port),
			Validator: validator,
		}
	}
	return selfPort, peerList, nil
}

func getValidatorFromChainSpec(network_file node.ChainSpec) ([]types.Validator, error) {
	// Get the metadata from the validator
	keyvals := network_file.GenesisState
	currValidatorRawBytes := []byte{}
	for _, keyval := range keyvals {
		if keyval.Key[0] == 0x08 {
			currValidatorRawBytes = keyval.Value
			break
		}
	}
	currValidatorRaw, _, err := types.Decode(currValidatorRawBytes, reflect.TypeOf(types.Validators{}))
	if err != nil {
		return nil, err
	}
	currValidators := currValidatorRaw.(types.Validators)
	return currValidators, nil
}
func flagDescription(main string, options map[string]string) string {
	var b strings.Builder
	b.WriteString(main + "\nPossible values:\n")
	for key, val := range options {
		b.WriteString(fmt.Sprintf("  - %-10s %s\n", key+":", val))
	}
	return b.String()
}
