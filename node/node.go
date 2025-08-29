package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/rpc"
	"runtime"
	"runtime/pprof"

	"sync/atomic"

	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	rand0 "math/rand"
	"net"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/chainspecs"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
)

const (
	JCEManual  = "manual"
	JCESimple  = "simple"
	JCEDefault = "normal"
	JCEFast    = "fast"
	JCEAUTO    = "auto"
)

const (
	isWriteSnapshot        = false
	isWriteTransition      = true
	isWriteBundleFollower  = true
	isWriteBundleGuarantor = true
	isWriteBundleAuditor   = true
)

const (
	enableInit  = false
	numNodes    = types.TotalValidators
	quicAddr    = "127.0.0.1:%d"
	Grandpa     = false
	GrandpaEasy = true
	Audit       = false
	CE138_test  = false
	CE129_test  = false // turn on for testing CE129
	revalidate  = false // turn off for production (or publication of traces)

	paranoidVerification = false // turn off for production
	writeJAMPNTestVector = false // turn on true when generating JAMNP test vectors only

	// GOAL: centralize use of context timeout parameters here, avoid hard
	TinyTimeout                = 2000 * time.Millisecond
	MiniTimeout                = 300 * time.Second // TEMPORARY == revert back to 3s
	SmallTimeout               = 6 * time.Second
	NormalTimeout              = 900 * time.Second
	MediumTimeout              = 10 * time.Second
	LargeTimeout               = 12 * time.Second
	VeryLargeTimeout           = 600 * time.Second
	RefineTimeout              = 36 * time.Second        // 36
	RefineAndAccumalateTimeout = (36 + 60) * time.Second // 96

	useCompiler        = true
	fudgeFactorJCE     = 1
	DefaultChannelSize = 200
)

var auth_code_bytes, _ = os.ReadFile(common.GetFilePath(statedb.BootStrapNullAuthFile))

var auth_code = statedb.AuthorizeCode{
	PackageMetaData:   []byte("bootstrap"),
	AuthorizationCode: auth_code_bytes,
}

var auth_code_encoded_bytes, _ = auth_code.Encode()

var auth_code_hash = common.Blake2Hash(auth_code_encoded_bytes) //pu
var auth_code_hash_hash = common.Blake2Hash(auth_code_hash[:])  //pa
var bootstrap_auth_codehash = auth_code_hash

type NodeContent struct {
	id                   uint16
	node_name            string
	AuditFlag            bool
	command_chan         chan string
	peersInfo            map[uint16]*Peer                 //<validatorIndex> -> Peer
	UP0_HandshakeChan    map[uint16]chan JAMSNP_Handshake //<validatorIndex> -> chan
	UP0_HandshakeMu      sync.Mutex
	UP0_stream           map[uint16]quic.Stream //<validatorIndex> -> stream (self initiated)
	UP0_streamMu         sync.Mutex
	blockAnnouncementsCh chan JAMSNP_BlockAnnounce
	ba_checker           *BlockAnnouncementChecker

	server quic.Listener

	pvmBackend      string
	epoch0Timestamp uint64
	// Jamweb
	hub             *Hub
	tlsConfig       *tls.Config
	clientTLSConfig *tls.Config
	store           *storage.StateDBStorage
	// holds a map of the hash to the stateDB
	statedbMap      map[common.Hash]*statedb.StateDB
	statedbMapMutex sync.Mutex
	// holds a map of the parenthash to the block
	blocks map[common.Hash]*types.Block

	// holds the tip
	statedb      *statedb.StateDB
	statedbMutex sync.Mutex
	// Track the number of opened streams
	dataHashStreams map[common.Hash][]quic.Stream

	workReports      map[common.Hash]types.WorkReport
	workReportsMutex sync.Mutex
	workReportsCh    chan types.WorkReport
	workPackagesCh   chan types.WorkPackage

	extrinsic_pool *types.ExtrinsicPool

	servicesMap   map[uint32]*types.ServiceSummary
	servicesMutex sync.Mutex

	workPackageQueue sync.Map

	loaded_services_dir string
	block_tree          *types.BlockTree
	nodeSelf            *Node

	RPC_Client        []*rpc.Client
	new_timeslot_chan chan uint32

	jceManagerMutex sync.Mutex
	jceManager      *ManualJCEManager
}

func NewNodeContent(id uint16, store *storage.StateDBStorage, pvmBackend string) NodeContent {
	//fmt.Printf("[N%v] NewNodeContent pvmBackend: %s\n", id, pvmBackend)
	return NodeContent{
		id:                   id,
		store:                store,
		command_chan:         make(chan string, DefaultChannelSize), // temporary
		peersInfo:            make(map[uint16]*Peer),
		UP0_stream:           make(map[uint16]quic.Stream),
		statedbMap:           make(map[common.Hash]*statedb.StateDB),
		dataHashStreams:      make(map[common.Hash][]quic.Stream),
		blockAnnouncementsCh: make(chan JAMSNP_BlockAnnounce, DefaultChannelSize),
		ba_checker:           InitBlockAnnouncementChecker(),
		blocks:               make(map[common.Hash]*types.Block),
		workPackagesCh:       make(chan types.WorkPackage, DefaultChannelSize),
		workReportsCh:        make(chan types.WorkReport, DefaultChannelSize),
		servicesMap:          make(map[uint32]*types.ServiceSummary),
		workPackageQueue:     sync.Map{},
		new_timeslot_chan:    make(chan uint32, 1),
		extrinsic_pool:       types.NewExtrinsicPool(),
		pvmBackend:           pvmBackend,
	}
}

func (n *Node) Clean(block_hashes []common.Hash) {
	n.statedbMapMutex.Lock()
	for _, block_hash := range block_hashes {
		log.Trace(log.B, "runReceiveBlock: unused_blocks", "n", n.String(), "block_hash", block_hash)
		//TOCHECK
		delete(n.statedbMap, block_hash)

		n.ba_checker.Clear(block_hash)

		// audit
		n.cleanUselessAudit(block_hash)
	}
	n.statedbMapMutex.Unlock()

	// cleaning process
	n.ticketsMutex.Lock()
	for entropy := range n.selfTickets {
		find := false
		for _, safrole_entropy := range n.statedb.GetSafrole().Entropy {
			if bytes.Equal(entropy[:], safrole_entropy[:]) {
				find = true
				break
			}
		}
		if !find {
			log.Trace(log.Node, "runReceiveBlock: cleaning ticket", "n", n.String(), "entropy", entropy)
			delete(n.selfTickets, entropy)
		}
	}
	n.ticketsMutex.Unlock()

}

type Node struct {
	NodeContent
	IsSync   bool
	IsSyncMu sync.RWMutex

	author_status string

	commitHash    string
	AuditNodeType string
	credential    types.ValidatorSecret
	peers         []string

	latest_block_mutex sync.Mutex
	latest_block       *JAMSNP_BlockInfo

	grandpa *grandpa.Grandpa
	// holds a map of epoch (use entropy to control it) to at most 2 tickets
	selfTickets   map[common.Hash][]types.TicketBucket
	ticketsMutex  sync.Mutex
	sendTickets   bool // when mode=fallback this is false, otherwise is true
	resendTickets bool

	auditingMap      map[common.Hash]*statedb.StateDB // headerHash -> stateDB
	auditingMapMutex sync.RWMutex

	announcementMap      map[common.Hash]*types.TrancheAnnouncement // announcement [headerHash -> [wr_hash]]
	announcementMapMutex sync.RWMutex

	judgementMap      map[common.Hash]*types.JudgeBucket // headerHash -> JudgeBucket
	judgementMapMutex sync.RWMutex
	//judgementBucket types.JudgeBucket
	judgementWRMap      map[common.Hash]common.Hash // wr_hash -> headerHash. TODO: shawn to update this
	judgementWRMapMutex sync.Mutex

	clients      map[string]string
	clientsMutex sync.Mutex

	// assurances state: are this node assuring the work package bundle/segments?
	assurancesBucket map[common.Hash]bool
	assuranceMutex   sync.Mutex
	delaysend        map[common.Hash]int // delaysend is a map of workpackagehash to the number of times it has been delayed

	ticketsCh chan types.Ticket

	guaranteesCh    chan types.Guarantee
	assurancesCh    chan types.Assurance
	announcementsCh chan types.Announcement
	judgementsCh    chan types.Judgement
	auditingCh      chan *statedb.StateDB // use this to trigger auditing, block hash

	// grandpa input channels
	grandpaPreVoteMessageCh   chan grandpa.VoteMessage
	grandpaPreCommitMessageCh chan grandpa.VoteMessage
	grandpaPrimaryMessageCh   chan grandpa.VoteMessage
	grandpaCommitMessageCh    chan grandpa.CommitMessage

	waitingAnnouncements      map[common.Hash][]types.Announcement
	waitingAnnouncementsMutex sync.Mutex
	waitingJudgements         map[common.Hash][]types.Judgement
	waitingJudgementsMutex    sync.Mutex

	nodeType string
	dataDir  string

	// DA Debugging
	totalIncomingStreams int64
	connectedPeers       map[uint16]bool

	// JamBlocks testing only
	JAMBlocksEndpoint string
	JAMBlocksPort     uint16

	// JCE
	jceMode             string
	currJCE             uint32 // the JCE to be processed
	completedJCE        uint32 // the JCE that the node has finished processing
	currJCEMutex        sync.Mutex
	completedJCEMutex   sync.Mutex
	jce_timestamp       map[uint32]time.Time
	jce_timestamp_mutex sync.Mutex

	stop_receive_blk    chan string
	restart_receive_blk chan string
	WriteDebugFlag      bool
}

func (n *Node) GetIsSync() bool {
	n.IsSyncMu.RLock()
	defer n.IsSyncMu.RUnlock()
	return n.IsSync
}
func (n *Node) SetIsSync(isSync bool, why string) {
	n.IsSyncMu.Lock()
	defer n.IsSyncMu.Unlock()
	if !isSync {
		log.Info(log.B, "SetIsSync", "n", n.String(), "isSync", isSync, "reason", why)
	}
	n.IsSync = isSync
}

func (n *Node) GetLatestBlockInfo() *JAMSNP_BlockInfo {
	// n.latest_block_mutex.Lock()
	// defer n.latest_block_mutex.Unlock()
	return n.latest_block
}

func (n *Node) SetLatestBlockInfo(block *JAMSNP_BlockInfo, where string) {
	// n.latest_block_mutex.Lock()
	// defer n.latest_block_mutex.Unlock()
	log.Debug(log.B, "SetLatestBlockInfo", "n", n.String(), "slot", block.Slot, "block_hash", block.HeaderHash.Hex(), "where", where)
	n.latest_block = block

}

func GenerateWorkPackageTraceID(wp types.WorkPackage) string {
	wpHashBytes := wp.Hash().Bytes()
	wpHashHex := hex.EncodeToString(wpHashBytes)

	if len(wpHashHex) > 2 && wpHashHex[:2] == "0x" {
		wpHashHex = wpHashHex[2:]
	}

	// TraceID is 16 bytes, so we need to trim the hash to 16 bytes
	if len(wpHashHex) == 64 {
		wpHashHex = wpHashHex[:16] + wpHashHex[48:]

	}
	return wpHashHex
}

/*
A Tip StateDB is held in the node structure
When a block comes in, we validate whether the block identified by parenthash and its extrinsics gets to this block.
  if it does, we put it into map[blockHash]*StateDB
  if its the latest timeslot, we update the tip
When a block is authored, we take the latest block identified by some parenthash and its extrinsics and get a new block.
  we update the tip
*/

func generateSelfSignedCert(pub ed25519.PublicKey, priv ed25519.PrivateKey) (tls.Certificate, error) {
	san := common.ToSAN(pub)
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Example Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),

		DNSNames: []string{san},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})
	return tls.X509KeyPair(certPEM, keyPEM)
}

func (n *NodeContent) String() string {
	return fmt.Sprintf("[N%d]", n.id)
}

func (n *Node) setValidatorCredential(credential types.ValidatorSecret) {
	n.credential = credential
	if false {
		jsonData, err := types.Encode(credential)
		if err != nil {
			log.Crit(log.Node, "setValidatorCredential", "err", err)
		}
		log.Info(log.Node, "[N%v] credential %s\n", n.id, jsonData)
	}
}

func createNode(id uint16, credential types.ValidatorSecret, chainspec *chainspecs.ChainSpec, pvmBackend string, epoch0Timestamp uint64, peers []string, peerList map[uint16]*Peer, dataDir string, port int, jceMode string) (*Node, error) {
	return newNode(id, credential, chainspec, pvmBackend, epoch0Timestamp, peers, peerList, dataDir, port, jceMode)
}

func PrintSpec(chainspec *chainspecs.ChainSpec) error {
	levelDBPath := "/tmp/xxx"
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		return err
	}
	stateTransition := &statedb.StateTransition{}
	stateTransition.PreState.KeyVals = chainspec.GenesisState
	stateTransition.PreState.StateRoot = common.Hash{}
	stateTransition.PostState.KeyVals = chainspec.GenesisState
	stateTransition.PostState.StateRoot = common.Hash{}
	header, _, err := types.Decode(chainspec.GenesisHeader, reflect.TypeOf(types.BlockHeader{}))
	if err != nil {
		return err
	}
	stateTransition.Block.Header = header.(types.BlockHeader)

	_statedb, err := statedb.NewStateDBFromStateTransition(store, stateTransition)
	if err != nil {
		return err
	}
	fmt.Printf("Spec: %s\n", _statedb.JamState.String())
	return nil
}
func NewNode(id uint16, credential types.ValidatorSecret, chainspec *chainspecs.ChainSpec, pvmBackend string, epoch0Timestamp uint64, peers []string, peerList map[uint16]*Peer, dataDir string, port int) (*Node, error) {
	return createNode(id, credential, chainspec, pvmBackend, epoch0Timestamp, peers, peerList, dataDir, port, JCEDefault)
}

func StandardizePVMBackend(pvm_mode string) string {
	mode := strings.ToUpper(pvm_mode)
	var pvmBackend string
	switch mode {
	case "INTERPRETER":
		pvmBackend = pvm.BackendInterpreter
	case "COMPILER", "RECOMPILER", "X86":
		if runtime.GOOS == "linux" {
			pvmBackend = pvm.BackendCompiler
		} else {
			log.Warn(log.Node, fmt.Sprintf("COMPILER Not Supported. Defaulting to interpreter"))
		}
	case "SANDBOX", "RECOMPILER_SANDBOX", "COMPILER_SANDBOX":
		pvmBackend = pvm.BackendSandbox
	default:
		log.Warn(log.Node, fmt.Sprintf("Unknown PVM mode [%s], defaulting to interpreter", pvm_mode))
		pvmBackend = pvm.BackendInterpreter
	}
	return pvmBackend
}

func (n *Node) SetPVMBackend(pvm_mode string) {
	pvmBackend := StandardizePVMBackend(pvm_mode)
	n.pvmBackend = pvmBackend
	log.Info(log.Node, fmt.Sprintf("PVM Backend: [%s]", pvmBackend))
}

func newNode(id uint16, credential types.ValidatorSecret, chainspec *chainspecs.ChainSpec, pvmBackend string, epoch0Timestamp uint64, peers []string, startPeerList map[uint16]*Peer, dataDir string, port int, jceMode string) (*Node, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	log.Info(log.Node, fmt.Sprintf("NewNode [N%v]", id), "spec", chainspec.ID, "addr", addr, "dataDir", dataDir)
	//REQUIRED FOR CAPTURING JOBID. DO NOT DELETE THIS LINE!!
	fmt.Printf("[N%v] addr=%v, dataDir=%v\n", id, addr, dataDir) //REQUIRED FOR CAPTURING JOBID. DO NOT DELETE THIS LINE!!
	//REQUIRED FOR CAPTURING JOBID. DO NOT DELETE THIS LINE!!

	levelDBPath := fmt.Sprintf("%v/leveldb/%d/", dataDir, port)
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		return nil, fmt.Errorf("NewStateDBStorage[port:%d] Err %v", port, err)
	}
	store.NodeID = id
	var cert tls.Certificate
	ed25519_priv := ed25519.PrivateKey(credential.Ed25519Secret[:])
	ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)

	cert, err = generateSelfSignedCert(ed25519_pub, ed25519_priv)
	if err != nil {
		return nil, fmt.Errorf("error generating self-signed certificate: %v", err)
	}
	node := &Node{
		NodeContent: NewNodeContent(id, store, StandardizePVMBackend(pvmBackend)), // mkcheck
		IsSync:      true,
		peers:       peers,
		clients:     make(map[string]string),

		auditingMap:     make(map[common.Hash]*statedb.StateDB),
		announcementMap: make(map[common.Hash]*types.TrancheAnnouncement),
		judgementMap:    make(map[common.Hash]*types.JudgeBucket),
		judgementWRMap:  make(map[common.Hash]common.Hash),

		selfTickets:      make(map[common.Hash][]types.TicketBucket),
		assurancesBucket: make(map[common.Hash]bool),
		delaysend:        make(map[common.Hash]int),

		ticketsCh:       make(chan types.Ticket, DefaultChannelSize),
		guaranteesCh:    make(chan types.Guarantee, DefaultChannelSize),
		assurancesCh:    make(chan types.Assurance, DefaultChannelSize),
		announcementsCh: make(chan types.Announcement, DefaultChannelSize),
		judgementsCh:    make(chan types.Judgement, DefaultChannelSize),
		auditingCh:      make(chan *statedb.StateDB, DefaultChannelSize),

		grandpaPreVoteMessageCh:   make(chan grandpa.VoteMessage, DefaultChannelSize),
		grandpaPreCommitMessageCh: make(chan grandpa.VoteMessage, DefaultChannelSize),
		grandpaPrimaryMessageCh:   make(chan grandpa.VoteMessage, DefaultChannelSize),
		grandpaCommitMessageCh:    make(chan grandpa.CommitMessage, DefaultChannelSize),

		sendTickets:   true,
		resendTickets: false, // activate this when you want to resend tickets

		dataDir: dataDir,

		connectedPeers: make(map[uint16]bool),
		WriteDebugFlag: true,
	}
	node.NodeContent.nodeSelf = node
	var _statedb *statedb.StateDB

	stateTransition := &statedb.StateTransition{}
	stateTransition.PreState.KeyVals = chainspec.GenesisState
	stateTransition.PreState.StateRoot = common.Hash{}
	stateTransition.PostState.KeyVals = chainspec.GenesisState
	stateTransition.PostState.StateRoot = common.Hash{}
	header, _, err := types.Decode(chainspec.GenesisHeader, reflect.TypeOf(types.BlockHeader{}))
	if err != nil {
		return nil, fmt.Errorf("Decode genesis header Err %v", err)
	}
	stateTransition.Block.Header = header.(types.BlockHeader)

	_statedb, err = statedb.NewStateDBFromStateTransition(node.store, stateTransition)
	if err != nil {
		return nil, fmt.Errorf("NewStateDBFromStateTransition Err %v", err)
	}

	block := _statedb.Block
	if block == nil {
		return nil, fmt.Errorf("NewStateDBFromStateTransition block is nil")
	}

	err = node.StoreBlock(block, id, false)
	if err != nil {
		log.Error(log.Node, "StoreBlock", "err", err)
		return nil, err
	}
	finalizedBlock, FinalizedOk, err := node.GetFinalizedBlockInternal()
	if err != nil || !FinalizedOk {
		FinalizedOk = true
		log.Info(log.Node, "GetFinalizedBlock", "block_hash", block.Header.HeaderHash().Hex())
		node.NodeContent.block_tree = types.NewBlockTree(&types.BT_Node{
			Parent:    nil,
			Block:     block,
			Height:    0,
			Finalized: true,
			Applied:   true,
		})
	} else {
		log.Info(log.Node, "NewBlockTree111", "block_hash", finalizedBlock.Header.HeaderHash().Hex())
		node.NodeContent.block_tree = types.NewBlockTree(&types.BT_Node{
			Parent:    nil,
			Block:     finalizedBlock,
			Height:    0,
			Finalized: true,
			Applied:   true,
		})
	}
	node.commitHash = common.GetCommitHash()
	fmt.Printf("[N%v] running on buildV: %s\n", id, node.GetBuild())

	genesisBlockHash = block.Header.HeaderHash()
	//jamnp-s/V/H/builder. Here V is the protocol version, 0, and H is the first 8 nibbles of the hash of the chain's genesis header, in lower-case hexadecimal.
	x := hex.EncodeToString(block.Header.HeaderHash().Bytes()[:4])
	alpn_builder := "jamnp-s/0/" + x + "/builder"
	alpn := "jamnp-s/0/" + x

	node.node_name = fmt.Sprintf("%s-%d", GetJAMNetwork(), id)
	log.Info(log.Node, "ALPN configuration",
		"genesis_hash", block.Header.HeaderHash().Hex(),
		"alpn", alpn,
		"alpn_builder", alpn_builder,
	)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAnyClientCert,
		NextProtos:   []string{alpn, alpn_builder},
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			remoteAddr := info.Conn.RemoteAddr().String()
			return &tls.Config{
				Certificates:       []tls.Certificate{cert},
				ClientAuth:         tls.RequireAnyClientCert,
				NextProtos:         []string{alpn, alpn_builder},
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
					if len(rawCerts) == 0 {
						err := fmt.Errorf("no client certificate provided")
						log.Error(log.Node, "VerifyPeerCertificate", "remoteAddr", remoteAddr, "err", err)
						return err
					}
					cert, err := x509.ParseCertificate(rawCerts[0])
					if err != nil {
						err := fmt.Errorf("failed to parse client certificate: %v", err)
						log.Error(log.Node, "VerifyPeerCertificate", "remoteAddr", remoteAddr, "err", err)
						return err
					}
					pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
					if !ok {
						err := fmt.Errorf("client public key is not Ed25519")
						log.Error(log.Node, "VerifyPeerCertificate", "remoteAddr", remoteAddr, "err", err)
						return err
					}
					expectedSAN := common.ToSAN(pubKey)
					if len(cert.DNSNames) != 1 || cert.DNSNames[0] != expectedSAN {
						dnsNameBytes := []byte(cert.DNSNames[0])
						dnsNameHex := hex.EncodeToString(dnsNameBytes)
						err := fmt.Errorf("SAN mismatch: expected %s %v, got %v pub key %s", expectedSAN, pubKey, cert.DNSNames, dnsNameHex)
						log.Error(log.Node, "VerifyPeerCertificate", "remoteAddr", remoteAddr, "err", err)
						return err
					}
					node.clientsMutex.Lock()
					node.clients[remoteAddr] = hex.EncodeToString(pubKey)
					node.clientsMutex.Unlock()
					log.Trace(log.Node, "VerifyPeerCertificate", "remoteAddr", remoteAddr, "pubKey", hex.EncodeToString(pubKey))
					return nil
				},
			}, nil
		},
	}
	node.tlsConfig = tlsConfig
	// put the white list to client
	node.clientsMutex.Lock()
	for _, p := range startPeerList {
		node.clients[p.PeerAddr] = hex.EncodeToString(p.Validator.Ed25519[:])
	}
	node.clientsMutex.Unlock()
	clientTLS := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				err := fmt.Errorf("no server certificate provided")
				log.Error(log.Node, "VerifyPeerCertificate2", "err", err)
				return err
			}
			cert, err := x509.ParseCertificate(rawCerts[0])
			if err != nil {
				err := fmt.Errorf("failed to parse server certificate: %v", err)
				log.Error(log.Node, "VerifyPeerCertificate2", "err", err)
				return err
			}
			pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
			if !ok {
				err := fmt.Errorf("server public key is not Ed25519")
				log.Error(log.Node, "VerifyPeerCertificate2", "err", err)
				return err
			}
			expectedSAN := common.ToSAN(pubKey)
			if len(cert.DNSNames) != 1 || cert.DNSNames[0] != expectedSAN {
				san := cert.DNSNames[0]
				sanBody := san[1:] // strip the "e" prefix
				b32 := base32.StdEncoding.WithPadding(base32.NoPadding)
				decodedPubKey, err := b32.DecodeString(strings.ToUpper(sanBody)) // base32 expects uppercase
				if err != nil {
					log.Error(log.Node, "Base32DecodeSAN", "san", san, "err", err)
				}
				err = fmt.Errorf("SAN mismatch: expected %s %v, got %v pub key %s", expectedSAN, pubKey, cert.DNSNames, fmt.Sprintf("%x", decodedPubKey))
				log.Error(log.Node, "VerifyPeerCertificate2", "pubKey", pubKey, "expectedSAN", expectedSAN, "cert.DNSNames", cert.DNSNames, "err", err)
				return err
			}
			log.Trace(log.Node, "VerifyPeerCertificate2 SUCCESS", "expectedSAN", expectedSAN, "cert.DNSNames", cert.DNSNames)
			return nil
		},
		NextProtos: []string{alpn, alpn_builder},
	}
	node.clientTLSConfig = clientTLS
	log.Info(log.Node, "ListenAddr", "addr", addr)
	listener, err := quic.ListenAddr(addr, tlsConfig, GenerateQuicConfig())
	if err != nil {
		log.Error(log.Node, "quic.ListenAddr", "err", err)
		return nil, err
	}
	node.server = *listener

	for validatorIndex, p := range startPeerList {
		node.peersInfo[validatorIndex] = NewPeer(node, validatorIndex, p.Validator, p.PeerAddr)
		// DISABLED FOR NOW
		if enableInit {
			if validatorIndex != id && FinalizedOk {
				_, err = node.peersInfo[validatorIndex].GetOrInitBlockAnnouncementStream(context.Background())
				if err != nil {
					log.Error(log.Node, "GetOrInitBlockAnnouncementStream", "err", err)
				}
			}
		}
	}
	_statedb.HeaderHash = block.Header.Hash()
	if FinalizedOk {
		finalizedBlock = block
		recoveredStateDB := _statedb.Copy()
		recoveredStateDB.RecoverJamState(finalizedBlock.Header.ParentStateRoot) // it don't even know if it got the correct state
		recoveredStateDB.UnsetPosteriorEntropy()
		recoveredStateDB.StateRoot = finalizedBlock.Header.ParentStateRoot
		recoveredStateDB.Block = finalizedBlock.Copy()
	}
	genesisBlockHash = block.Header.Hash()
	if err == nil {
		_statedb.SetID(uint16(id))
		node.addStateDB(_statedb)
	} else {
		fmt.Printf("NewGenesisStateDB ERR %v\n", err)
		return nil, err
	}

	validators := node.statedb.GetSafrole().NextValidators
	if len(validators) == 0 {
		return nil, fmt.Errorf("newNode No validators")
	}

	go node.runServer()

	node.setValidatorCredential(credential)
	node.epoch0Timestamp = epoch0Timestamp

	// DISABLED FOR NOW
	if enableInit && !node.GetIsSync() {
		ctx, cancel := context.WithTimeout(context.Background(), VeryLargeTimeout)
		defer cancel()
		randomselectedPeer := rand0.Intn(len(node.peersInfo))
		for randomselectedPeer == int(id) {
			randomselectedPeer = rand0.Intn(len(node.peersInfo))
		}
		peer := node.peersInfo[uint16(randomselectedPeer)]
		last_finalized := node.block_tree.GetLastFinalizedBlock()
		block_header_hash := last_finalized.Block.Header.HeaderHash()
		blocks, err := peer.GetMultiBlocks(block_header_hash, ctx)
		if err != nil {
			log.Error(log.Node, "GetMultiBlocks", "err", err, "hash", block_header_hash)
			return nil, err
		}
		if len(blocks) == 0 {
			log.Error(log.Node, "GetMultiBlocks", "blocks", blocks)
			return nil, fmt.Errorf("GetMultiBlocks: no blocks")
		}
		for _, block := range blocks {
			err := node.processBlock(&block)
			if err != nil {
				log.Error(log.Node, "processBlock", "err", err)
				return nil, err
			}
			log.Info(log.Node, "newNode:processBlock", "block_hash", block.Header.HeaderHash().Hex())
		}
		log.Info(log.Node, "newNode:extendChain", "block_hash", blocks[len(blocks)-1].Header.HeaderHash().Hex())
		node.extendChain(ctx)
	}

	go node.runAuthoring()
	go node.runGuarantees()
	go node.runAssurances()
	go node.runWorkReports()
	go node.runBlocksTickets()
	go node.runReceiveBlock()
	go node.StartRPCServer(int(id))
	go node.RunRPCCommand()
	go node.runWPQueue()
	if Audit {
		node.AuditFlag = true
		go node.runAudit() // disable this to pause FetchWorkPackageBundle, if we disable this grandpa will not work
		go node.runAuditAnnouncementJudgement()
	} else {
		node.AuditFlag = false
	}
	// we need to organize the /ws usage to avoid conflicts
	if node.id == 5 { // HACK
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go node.runJamWeb(context.Background(), wg, uint16(19800)+id, port) // TODO: setup default WS
		go func() {
			wg.Wait()
			log.Info("jamweb", "Node 0", "shutdown complete")
		}()
	}

	node.jceMode = jceMode
	node.runJCE()

	return node, nil
}

func (n *Node) runJCE() {
	mode := n.jceMode
	switch mode {
	case JCEManual, JCESimple:
		initialJCE := uint32(11)
		n.SetCompletedJCE(initialJCE)
		go n.runJCEManually()
	case JCEFast:
		go n.runFasterJCE()
	case JCEAUTO, JCEDefault:
		go n.runJCEDefault()
	default:
		log.Error(log.Node, "runJCE", "mode", mode, "err", fmt.Errorf("unknown mode"))
		return
	}
	//fmt.Printf("[N%v] runJCE %v mode\n", n.id, mode)
}

func GenerateQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:                  true,
		KeepAlivePeriod:            1 * time.Second,
		MaxIdleTimeout:             20 * time.Second,
		MaxIncomingUniStreams:      5000, // TODO
		MaxIncomingStreams:         5000,
		MaxStreamReceiveWindow:     8 * 1024 * 1024,
		MaxConnectionReceiveWindow: 200 * 1024 * 1024,
		Tracer:                     qlog.DefaultConnectionTracer,
	}
}

func (n *Node) GetBuild() string {
	info := n.commitHash
	//TODO: add extra info here
	return info
}

// use ed25519 key to get peer info
func (n *NodeContent) GetPeerInfoByEd25519(key types.Ed25519Key) (*Peer, error) {
	for _, peer := range n.peersInfo {
		if peer.Validator.Ed25519 == key {
			return peer, nil
		}
	}
	return nil, fmt.Errorf("peer not found")
}

func RunGrandpaGraphServer(watchNode *Node, basePort uint16) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	block_graph_server := types.NewGraphServer(basePort)
	go block_graph_server.StartServer()
	for {
		time.Sleep(10 * time.Millisecond)
		select {
		case <-ticker.C:
			block_graph_server.Update(watchNode.block_tree)
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func (n *Node) GetSegments(importedSegments []types.ImportSegment) (raw_segments [][]byte, err error) {
	workReportSearchMap := make(map[common.Hash]*SpecIndex)
	erasureRootIndex := make(map[common.Hash]*SpecIndex)

	// Step 1: resolve WorkReport for each RequestedHash
	for _, importedSegment := range importedSegments {
		si, ok := workReportSearchMap[importedSegment.RequestedHash]
		if !ok {
			si = n.WorkReportSearch(importedSegment.RequestedHash)
			if si == nil {
				return nil, fmt.Errorf("WorkReportSearch(%s) not found", importedSegment.RequestedHash)
			}
			workReportSearchMap[importedSegment.RequestedHash] = si
		}

		erasureRoot := si.WorkReport.AvailabilitySpec.ErasureRoot
		oldSi, exists := erasureRootIndex[erasureRoot]
		if exists {
			oldSi.AddIndex(importedSegment.Index)
		} else {
			si.AddIndex(importedSegment.Index)
			erasureRootIndex[erasureRoot] = si
		}
	}

	// Step 2: reconstruct segments for each erasure root
	segmentDataMap := make(map[common.Hash][][]byte)
	for erasureRoot, si := range erasureRootIndex {
		segments, _, err := n.reconstructSegments(si)
		if err != nil {
			return nil, fmt.Errorf("reconstructSegments failed for erasureRoot %s: %v", erasureRoot, err)
		}
		segmentDataMap[erasureRoot] = segments
	}

	// Step 3: map back to original import order
	raw_segments = make([][]byte, len(importedSegments))
	for i, importedSegment := range importedSegments {
		si := workReportSearchMap[importedSegment.RequestedHash]
		erasureRoot := si.WorkReport.AvailabilitySpec.ErasureRoot
		segments := segmentDataMap[erasureRoot]
		for j, idx := range si.Indices {
			if idx == importedSegment.Index {
				raw_segments[i] = segments[j]
				break
			}
		}
	}
	return raw_segments, nil
}

func (n *Node) GetSegmentsByRequestedHash(RequestedHash common.Hash, count int) (raw_segments [][]byte, err error) {

	ExportedSegmentLength := count

	importedSegments := make([]types.ImportSegment, 0)
	for i := 0; i < int(ExportedSegmentLength); i++ {
		importedSegment := types.ImportSegment{
			RequestedHash: RequestedHash,
			Index:         uint16(i),
		}
		importedSegments = append(importedSegments, importedSegment)
	}

	raw_segments, err = n.GetSegments(importedSegments)
	if err != nil {
		return nil, fmt.Errorf("GetSegments failed: %v", err)
	}
	return raw_segments, nil
}

func (n *Node) GetWorkReport(requestedHash common.Hash) (wr *types.WorkReport, err error) {
	specIndex := n.WorkReportSearch(requestedHash)
	if specIndex == nil {
		return nil, fmt.Errorf("GetWorkReport: WorkReportSearch(%s) not found", requestedHash.Hex())
	}
	wr = &(specIndex.WorkReport)
	log.Debug(log.Node, "GetWorkReport", "requestedHash", requestedHash.Hex(), "wr_hash", wr.Hash().Hex())
	return wr, nil
}

func (n *Node) GetService(serviceIndex uint32) (sa *types.ServiceAccount, ok bool, err error) {
	return n.getState().GetService(serviceIndex)
}

func (n *Node) GetServiceStorage(serviceIndex uint32, k []byte) ([]byte, bool, error) {
	return n.getState().GetTrie().GetServiceStorage(serviceIndex, k)
}

func (n *Node) SubmitAndWaitForPreimage(ctx context.Context, serviceIndex uint32, preimage []byte) (err error) {
	errCh := make(chan error, 1)
	preimageHash := common.Blake2Hash(preimage)

	log.Info(log.Node, "SubmitAndWaitForPreimage SUBMITTED", "id", fmt.Sprintf("%d", serviceIndex), "preimageHash", preimageHash, "len", len(preimage))

	// Submit preimage
	n.AddPreimageToPool(serviceIndex, preimage)
	// Announce it everyone else with CE142 (and they will request it with CE143, which will be available in the pool from the above)
	err = n.BroadcastPreimageAnnouncement(serviceIndex, preimageHash, uint32(len(preimage)), preimage)
	if err != nil {
		log.Error(log.Node, "SubmitAndWaitForPreimage ERR", "err", err)
		return err
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			errCh <- fmt.Errorf("*SubmitAndWaitForPreimage*: context canceled or timed out (serviceID=%d, h=%s, l=%d)", serviceIndex, preimageHash, len(preimage))
			return nil
		case <-ticker.C:
			time_slots, ok, err := n.statedb.ReadServicePreimageLookup(serviceIndex, preimageHash, uint32(len(preimage)))
			if err != nil {
				return err
			}
			if len(time_slots) > 0 && ok {
				log.Info(log.Node, "SubmitAndWaitForPreimage ON-CHAIN", "id", fmt.Sprintf("%d", serviceIndex), "preimageHash", preimageHash, "len", len(preimage))
				return nil
			}
		}
	}
}

const (
	maxRobustTries = 4
)

// RobustSubmitAndWaitForWorkPackages will retry SubmitAndWaitForWorkPackages up to 4 times
func RobustSubmitAndWaitForWorkPackages(ctx context.Context, n JNode, reqs []*WorkPackageRequest) (*types.WorkReport, error) {
	var lastErr error
	for attempt := 1; attempt <= maxRobustTries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), RefineTimeout)
		defer cancel()

		hashes, err := n.SubmitAndWaitForWorkPackages(ctx, reqs)
		if err == nil {
			wr, err := n.GetWorkReport(hashes[0])
			if err != nil {
				log.Error(log.Node, "GetWorkReport ERR", "err", err)
				return nil, fmt.Errorf("GetWorkReport failed: %w", err)
			}
			return wr, nil
		}
		lastErr = err
		log.Warn(log.Node, "RobustSubmitAndWaitForWorkPackages", "attempt", attempt, "err", err)
		// small backoff between retries
		time.Sleep(5 * time.Second)
	}

	return nil, fmt.Errorf("all retries failed after %d attempts: %w", maxRobustTries, lastErr)
}
func (n *Node) SubmitAndWaitForWorkPackages(ctx context.Context, reqs []*WorkPackageRequest) ([]common.Hash, error) {
	log.Info(log.Node, "Node SubmitAndWaitForWorkPackages", "reqLen", len(reqs))
	workPackageHashes := make([]common.Hash, len(reqs))
	accumulated := make(map[common.Hash]bool)
	identifierToIndex := make(map[string]int)

	// Initialize refine context and identifier map
	refineCtx := n.getRefineContext()
	for i, req := range reqs {
		identifierToIndex[req.Identifier] = i
		rc := refineCtx.Clone()
		req.WorkPackage.RefineContext = *rc
	}

	// Populate prerequisite hashes
	for _, req := range reqs {
		if len(req.Prerequisites) == 0 {
			continue
		}
		prereqHashes := make([]common.Hash, 0, len(req.Prerequisites))
		for _, prereqID := range req.Prerequisites {
			if idx, ok := identifierToIndex[prereqID]; ok {
				prereqHashes = append(prereqHashes, reqs[idx].WorkPackage.Hash())
			} else {
				log.Warn(log.Node, "Unknown prerequisite identifier", "identifier", prereqID)
			}
		}
		req.WorkPackage.RefineContext.Prerequisites = prereqHashes
	}

	// Compute hashes and track accumulation status
	for i, req := range reqs {
		hash := req.WorkPackage.Hash()
		workPackageHashes[i] = hash
		accumulated[hash] = false
	}

	// Submit each work package to a random peer on the assigned core
	for _, req := range reqs {
		fmt.Printf("Submitting work package: %s\n", req.WorkPackage.String())
		n.SubmitWPSameCore(req.WorkPackage, req.ExtrinsicsBlobs)
		log.Info(log.Node, "Work package submitted", "identifier", req.Identifier, "hash", workPackageHashes[identifierToIndex[req.Identifier]].Hex())
	}

	// Wait for accumulation
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for accumulatedCount := 0; accumulatedCount < len(reqs); {
		select {
		case <-ctx.Done():
			return workPackageHashes, ctx.Err()
		case <-ticker.C:
			for j := types.EpochLength - 1; j > 0; j-- {
				history := n.statedb.JamState.AccumulationHistory[j]
				for _, hash := range history.WorkPackageHash {
					if seen, exists := accumulated[hash]; exists && !seen {
						accumulated[hash] = true
						accumulatedCount++
						log.Info(log.Node, "Work package accumulated", "hash", hash.Hex(), "count", accumulatedCount)
					}
				}
			}
		}
	}

	log.Info(log.Node, "All work packages accumulated")
	return workPackageHashes, nil
}

func (n *Node) SubmitAndWaitForWorkPackage(ctx context.Context, wp *WorkPackageRequest) (common.Hash, error) {
	//fmt.Printf("NODE SubmitAndWaitForWorkPackage %s\n", wp.WorkPackage.Hash())
	wp.WorkPackage.RefineContext = n.getRefineContext()
	err := n.SubmitWPSameCore(wp.WorkPackage, wp.ExtrinsicsBlobs)
	if err != nil {
		log.Error(log.Node, "SubmitAndWaitForWorkPackage", "err", err, "id", wp.Identifier)
		return common.Hash{}, fmt.Errorf("SubmitAndWaitForWorkPackage: %w", err)
	}
	workPackageHash := wp.WorkPackage.Hash()
	log.Info(log.Node, "SubmitAndWaitForWorkPackage SUBMITTED", "id", wp.Identifier, "workpackageHash", workPackageHash.Hex())

	jceManager, _ := n.GetJCEManager()
	if jceManager != nil {
		jceManager.SendWP(workPackageHash)
	}
	initialJCE := n.GetCurrJCE()
	refineDone := false

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return workPackageHash, ctx.Err()
		case <-ticker.C:
			recentBlocks := n.statedb.JamState.RecentBlocks.B_H
			accumulationHistory := n.statedb.JamState.AccumulationHistory

			if jceManager != nil {
				if c15, e := types.Encode(accumulationHistory); e == nil {
					jceManager.UpdateAccumulationState(c15)
				}
				if c3, e := types.Encode(recentBlocks); e == nil {
					jceManager.UpdateRefineState(c3)
				}
			}

			currJCE := n.GetCurrJCE()
			if !refineDone {
				for i := len(recentBlocks) - 1; i >= 0; i-- {
					for _, info := range recentBlocks[i].Reported {
						if info.WorkPackageHash == workPackageHash {
							//fmt.Printf("[***JCE=%d] WP %s Refine Complete\n", currJCE, workPackageHash)
							refineDone = true
						}
					}
				}
			}

			for i := len(accumulationHistory) - 1; i >= 0; i-- {
				for _, h := range accumulationHistory[i].WorkPackageHash {
					if h == workPackageHash {
						log.Info(log.Node, "SubmitAndWaitForWorkPackage ACCUMULATED", "id", wp.Identifier, "workpackageHash", workPackageHash.Hex())
						return workPackageHash, nil
					}
				}
			}

			if currJCE-initialJCE >= types.RecentHistorySize {
				return workPackageHash, fmt.Errorf("SubmitAndWaitForWorkPackage: expired after %d JCEs", types.RecentHistorySize)
			}
		}
	}
}
func (n *NodeContent) SetJCEManager(jceManager *ManualJCEManager) (err error) {
	n.jceManagerMutex.Lock()
	defer n.jceManagerMutex.Unlock()
	if n.jceManager != nil {
		err = fmt.Errorf("jceManager already set")
		log.Error(log.Node, "SetJCEManager", "err", err)
		return err
	}
	n.jceManager = jceManager
	return nil
}

func (n *NodeContent) GetJCEManager() (jceManager *ManualJCEManager, err error) {
	n.jceManagerMutex.Lock()
	defer n.jceManagerMutex.Unlock()
	if n.jceManager != nil {
		jceManager = n.jceManager
		return jceManager, nil
	}
	return nil, nil
}

func (n *Node) GetBandersnatchSecret() []byte {
	return n.credential.BandersnatchSecret
}

func (n *Node) GetSelfCoreIndex() (uint16, error) {
	if len(n.statedb.GuarantorAssignments) == 0 {
		return 0, fmt.Errorf("NO ASSIGNMENTS")
	}
	for _, assignment := range n.statedb.GuarantorAssignments {
		if assignment.Validator.GetEd25519Key() == n.GetEd25519Key() {
			return assignment.CoreIndex, nil
		}
	}
	return 0, fmt.Errorf("core index not found")
}

func (n *Node) GetPrevCoreIndex() (uint16, error) {
	assignments := n.statedb.PreviousGuarantorAssignments
	if len(assignments) == 0 {
		return 0, fmt.Errorf("NO ASSIGNMENTS")
	}
	for _, assignment := range assignments {
		if assignment.Validator.GetEd25519Key() == n.GetEd25519Key() {
			return assignment.CoreIndex, nil
		}
	}
	return 0, fmt.Errorf("core index not found")
}

func (n *NodeContent) GetCoreIndexFromEd25519Key(key types.Ed25519Key) (uint16, error) {
	assignments := n.statedb.GuarantorAssignments
	for _, assignment := range assignments {
		if assignment.Validator.GetEd25519Key() == key {
			return assignment.CoreIndex, nil
		}
	}
	return 0, fmt.Errorf("core index not found")
}

func (n *Node) GetCoreCoWorkers(coreIndex uint16) []types.Validator {
	coWorkers := make([]types.Validator, 0)
	for _, assignment := range n.statedb.GuarantorAssignments {
		if types.Ed25519Key(n.credential.Ed25519Pub) == assignment.Validator.Ed25519 {
			continue
		}
		if assignment.CoreIndex == coreIndex {
			coWorkers = append(coWorkers, assignment.Validator)
		}
	}
	return coWorkers
}

func (n *NodeContent) GetEd25519Key() types.Ed25519Key {
	return n.nodeSelf.credential.Ed25519Pub
}

func (n *NodeContent) SubmitWPSameCore(wp types.WorkPackage, extrinsicsBlobs types.ExtrinsicsBlobs) (err error) {
	workPackageHash := wp.Hash()
	var coreIndex uint16

	// map the WALL CLOCK time to the coreIndex of THIS node and calculate all validator assignments
	// NOTE: we use +1 to avoid the current JCE, we want the next one
	slot := common.GetWallClockJCE(fudgeFactorJCE)
	_, assignments := n.statedb.CalculateAssignments(slot)
	for _, assignment := range assignments {
		if types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) == n.GetEd25519Key() {
			coreIndex = assignment.CoreIndex
		}
	}
	peers := make([]uint16, 0)
	for _, assignment := range assignments {
		if assignment.CoreIndex == coreIndex {
			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
			if err != nil {

			} else {
				peers = append(peers, peer.PeerID)
			}

		}
	}
	log.Info(log.G, "SubmitWPSameCore SUBMISSION Start", "NODE", n.id, "validators", peers, "coreIndex", coreIndex, "slot", slot)

	// if we want to process it ourselves, this should be true
	allowSelfSubmission := false
	if allowSelfSubmission {
		n.workPackageQueue.Store(workPackageHash, &WPQueueItem{
			workPackage:        wp,
			coreIndex:          coreIndex,
			extrinsics:         extrinsicsBlobs,
			addTS:              time.Now().Unix(),
			nextAttemptAfterTS: time.Now().Unix(),
			slot:               slot, // IMPORTANT: this will be used as guarantee.Slot
		})
		log.Info(log.G, "SubmitWPSameCore SUBMISSION SELF", "coreIndex", coreIndex)
		return nil
	}
	// now we can send to the other 2 nodes
	for _, assignment := range assignments {
		if assignment.CoreIndex == coreIndex {
			if types.Ed25519Key(assignment.Validator.Ed25519.PublicKey()) != n.GetEd25519Key() {
				pubkey := assignment.Validator.Ed25519
				peer, err := n.GetPeerInfoByEd25519(pubkey)
				if err != nil {
					log.Error(log.Node, "SubmitWPSameCore GetPeerInfoByEd25519", "err", err, "pubkey", pubkey)
				} else {
					err = peer.SendWorkPackageSubmission(context.Background(), wp, extrinsicsBlobs, coreIndex)
					if err != nil {
						log.Error(log.Node, "SubmitWPSameCore SendWorkPackageSubmission", "err", err, "pubkey", pubkey)
					} else {
						// we only want to process ONE
						return nil
					}
				}
			}
		}
	}
	return fmt.Errorf("SubmitWPSameCore: no peers found for coreIndex %d", coreIndex)
}

// this function will return the core workers of that core
func (n *NodeContent) GetCoreCoWorkersPeers(core uint16) (coWorkers []*Peer) {
	coWorkers = make([]*Peer, 0)
	for _, assignment := range n.statedb.GuarantorAssignments {
		if assignment.CoreIndex == core {
			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
			if err == nil {
				coWorkers = append(coWorkers, peer.Clone())
			}
		}
	}
	return coWorkers
}

// GetCoreCoWorkerPeersByStateDB returns the list of *Peer to iterate over them
func (n *Node) GetCoreCoWorkerPeersByStateDB(core uint16, using_statedb *statedb.StateDB) (coWorkers []*Peer) {
	coWorkers = make([]*Peer, 0)
	for _, assignment := range using_statedb.GuarantorAssignments {
		if assignment.CoreIndex == core {
			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
			if err == nil {
				coWorkers = append(coWorkers, peer)
			}
		}
	}
	return coWorkers
}

func (n *Node) GetCurrValidatorIndex() uint32 {
	return uint32(n.statedb.GetSafrole().GetCurrValidatorIndex(n.GetEd25519Key()))
}
func (n *Node) GetSafrole() *statedb.SafroleState {
	return n.statedb.GetSafrole()
}

func (n *Node) GetPrevValidatorIndex() uint32 {
	return uint32(n.statedb.GetSafrole().GetPrevValidatorIndex(n.GetEd25519Key()))
}

func (n *Node) GetNodeType() string {
	return n.nodeType
}

func (n *Node) getState() *statedb.StateDB {
	return n.statedb
}

func (n *NodeContent) getPeerByIndex(peerIdx uint16) (*Peer, error) {
	p := n.peersInfo[peerIdx]
	// check if peer exists
	if p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("peer %v not found", peerIdx)
}

func (n *NodeContent) updateServiceMap(statedb *statedb.StateDB, b *types.Block) error {
	stats := statedb.JamState.ValidatorStatistics
	for s, stats := range stats.ServiceStatistics {
		summ, ok := n.servicesMap[s]
		if !ok {
			summ = &types.ServiceSummary{
				ServiceID:          s,
				ServiceName:        "",
				LastRefineSlot:     0,
				LastAccumulateSlot: 0,
			}
			if s == 0 {
				summ.ServiceName = "bootstrap" // temp hack
			}
			n.servicesMap[s] = summ
		}
		slot := b.Header.Slot
		if stats.AccumulateGasUsed > 0 {
			summ.LastAccumulateSlot = slot
		}
		if stats.RefineGasUsed > 0 {
			summ.LastRefineSlot = slot
		}
		summ.Statistics = &stats
	}
	for _, p := range b.Extrinsic.Preimages {
		s := p.Requester
		service, ok, err := statedb.GetService(uint32(s))
		if err == nil && ok {
			//check if the p.Blob hash is the requesters codehash before doing this serviceName
			codeHash := common.Blake2Hash(p.Blob)
			if service.CodeHash == codeHash {
				metadata, _ := types.SplitMetadataAndCode(p.Blob)
				if len(metadata) > 0 {
					_, ok := n.servicesMap[s]
					if !ok {
						summ := &types.ServiceSummary{
							ServiceID:          s,
							ServiceName:        metadata,
							LastRefineSlot:     0,
							LastAccumulateSlot: 0,
						}
						n.servicesMap[s] = summ
					} else {
						if len(n.servicesMap[s].ServiceName) == 0 {
							n.servicesMap[s].ServiceName = metadata
						}
					}
				}
			}
		}
	}
	return nil
}

func (n *NodeContent) AddStateDB(_statedb *statedb.StateDB) error {
	return n.addStateDB(_statedb)
}

func (n *NodeContent) addStateDB(_statedb *statedb.StateDB) error {
	n.statedbMutex.Lock()
	n.statedbMapMutex.Lock()
	defer n.statedbMutex.Unlock()
	defer n.statedbMapMutex.Unlock()

	if n.statedb == nil || n.statedb.GetBlock() == nil {
		var headerHash common.Hash
		if _statedb.GetBlock() != nil {
			headerHash = _statedb.GetHeaderHash()
		}
		n.statedb = _statedb
		n.statedbMap[headerHash] = _statedb
		log.Debug(log.B, "addStateDB", "statedb", n.statedb.GetHeaderHash().Hex())
		return nil
	}
	if _statedb.GetBlock() == nil {
		return fmt.Errorf("addStateDB: NO BLOCK")
	}
	if _statedb.GetBlock().TimeSlot() > n.statedb.GetBlock().TimeSlot() { // not nessary  && _statedb.GetBlock().GetParentHeaderHash() == n.statedb.GetBlock().Header.Hash()
		if !(_statedb.GetBlock().GetParentHeaderHash() == n.statedb.GetBlock().Header.Hash()) {
			log.Error(log.B, "addStateDB Warning:newStateDB's Parent is not current StateDB", "n", n.String(), "new_statedb", _statedb.GetHeaderHash().Hex(), "new_statedb_slot", _statedb.GetBlock().TimeSlot(), "current_statedb", n.statedb.GetHeaderHash().Hex(), "current_statedb_slot", n.statedb.GetBlock().TimeSlot())
			return nil
		}
		n.statedb = _statedb
		n.statedbMap[_statedb.GetHeaderHash()] = _statedb
		log.Debug(log.B, "addStateDB", "statedb", n.statedb.GetHeaderHash().Hex())
	} else {
		log.Warn(log.B, "addStateDB", "statedb", _statedb.GetHeaderHash().Hex(), "statedb2", n.statedb.GetHeaderHash().Hex())
	}
	return nil
}

func (n *Node) GetEd25519Key() types.Ed25519Key {
	return types.Ed25519Key(n.credential.Ed25519Pub)
}

func (n *Node) GetEd25519Secret() []byte {
	return n.credential.Ed25519Secret[:]
}

func (n *Node) ResetPeer(peerIdentifier string) {
	/*
	   inConnKey := getConnKey(peerIdentifier, false)
	   outcomingConnKey := getConnKey(peerIdentifier, false)
	   n.connectionMu.Lock()
	   defer n.connectionMu.Unlock()
	   delete(n.connections, outcomingConnKey)
	   delete(n.streams, outcomingConnKey)
	   delete(n.connections, inConnKey)
	   delete(n.streams, inConnKey)
	   peer, exist := n.peersInfo[peerIdentifier]

	   	if exist {
	   		peer.RemoteAddr = peer.PeerAddr
	   	}
	*/
}

func (n *Node) runServer() {
	for {
		conn, err := n.server.Accept(context.Background())
		if err != nil {
			fmt.Printf("runServer: Server accept error: %v\n", err)
			continue
		}
		go n.handleConnection(conn)
	}
}

func (n *Node) lookupPubKey(pubKey string) (uint16, bool) {
	hpubKey := fmt.Sprintf("0x%s", pubKey)
	for validatorIndex, p := range n.peersInfo {
		peerPubKey := p.Validator.Ed25519.String()
		if peerPubKey == hpubKey {
			return validatorIndex, true
		}
	}

	return 0, false
}

func (n *Node) handleConnection(conn quic.Connection) {
	defer conn.CloseWithError(0, "closing connection")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	remoteAddr := conn.RemoteAddr().String()
	log.Trace(log.Node, "handleConnection", "remoteAddr", remoteAddr)
	host, port, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		log.Warn(log.Node, "handleConnection", "remoteAddr", remoteAddr, "host", host, "port", port)
		return
	}
	n.clientsMutex.Lock()
	pubKey, ok := n.clients[remoteAddr]
	n.clientsMutex.Unlock()

	if !ok {
		log.Warn(log.Node, "handleConnection DROPPING - not found in n.client", "remoteAddr", remoteAddr)
		return
	}

	validatorIndex, ok := n.lookupPubKey(pubKey)
	if !ok {
		log.Info(log.Node, "handleConnection - found n.clients but unknown pubkey", "remoteAddr", remoteAddr, "port", port, "pubKey", pubKey)
		validatorIndex = 9999
		// remoteAddr change the port to 13000
		// see how many number from the end
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			log.Warn(log.Node, "handleConnection", "remoteAddr", remoteAddr, "host", host, "port", port)
			return
		}
		newAddr := net.JoinHostPort(host, "13370")
		if _, ok := n.peersInfo[validatorIndex]; !ok {
			n.peersInfo[validatorIndex] = NewPeer(n, uint16(validatorIndex), types.Validator{}, newAddr)
			log.Debug(log.Quic, "handleConnection: Non-Validator peer", "validatorIndex", validatorIndex, "pubKey", pubKey, "remoteAddr", remoteAddr, "newAddr", newAddr)
		} else {
			for {
				validatorIndex++
				if _, ok := n.peersInfo[validatorIndex]; !ok {
					n.peersInfo[validatorIndex] = NewPeer(n, uint16(validatorIndex), types.Validator{}, newAddr)
					log.Debug(log.Quic, "handleConnection: Non-Validator peer", "validatorIndex", validatorIndex, "pubKey", pubKey, "remoteAddr", remoteAddr, "newAddr", newAddr)
					break
				}
			}
		}

	} else {
		log.Trace(log.Node, "handleConnection - KNOWN pubkey", "validatorIndex", validatorIndex, "remoteAddr", remoteAddr, "port", port, "pubKey", pubKey)
		if n.peersInfo[validatorIndex].conn == nil {
			n.peersInfo[validatorIndex].connectionMu.Lock()
			n.peersInfo[validatorIndex].conn = conn
			n.peersInfo[validatorIndex].connectionMu.Unlock()
		}
	}

	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			log.Trace(log.DA, "AcceptStream", "n", n.id, "validatorIndex", validatorIndex, "err", err)
			if stream != nil {
				stream.Close()
			}
			break
		}
		atomic.AddInt64(&n.totalIncomingStreams, 1)

		go func(stream quic.Stream) {
			defer func() {
				if r := recover(); r != nil {
					log.Error(log.Node, "Recovered from panic in QUIC stream handler", "err", r)
					// save stack info to /tmp/panic.txt
					buf := make([]byte, 1<<16)
					n := runtime.Stack(buf, true)
					// if file is not exist, create it
					f, err := os.OpenFile("/tmp/panic.txt", os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0644)
					if err != nil {
						log.Error(log.Node, "Failed to open /tmp/panic.txt", "err", err)
						return
					}
					defer f.Close()
					_, err = f.Write(buf[:n])
					if err != nil {
						log.Error(log.Node, "Failed to write to /tmp/panic.txt", "err", err)
						return
					}

				}
				atomic.AddInt64(&n.totalIncomingStreams, -1)
			}()

			streamCtx, cancel := context.WithTimeout(context.Background(), NormalTimeout)
			defer cancel()

			err := n.DispatchIncomingQUICStream(streamCtx, stream, validatorIndex)
			if err != nil {
				log.Warn(log.DA, "DispatchIncomingQUICStream", "n", n.id, "validatorIndex", validatorIndex, "err", err)
			}

		}(stream)
	}
}

func isGridNeighbor(vIdx, vIdx2 uint16) bool {
	W := uint16(2)
	if types.TotalValidators > 6 {
		if types.TotalValidators == 1023 {
			W = uint16(31)
		}
	}
	if sameRow := vIdx/W == vIdx2/W; sameRow {
		return true
	}
	if sameCol := vIdx%W == vIdx2%W; sameCol {
		return true
	}
	return false
}

func (n *Node) getEpoch() uint32 {
	return n.statedb.GetSafrole().GetEpoch()
}

// broadcast sends the object to all peers
// TODO: Use worker pools to limit concurrent goroutines to like a few hundred at most
func (n *Node) broadcast(ctxParent context.Context, obj interface{}) {
	objType := reflect.TypeOf(obj)

	for id, p := range n.peersInfo {

		if id == n.id {
			switch objType {
			case reflect.TypeOf(types.Assurance{}):
				a := obj.(types.Assurance)
				select {
				case n.assurancesCh <- a:
					// successfully sent
				default:
					log.Warn(log.Node, "broadcast: assurancesCh full, dropping Assurance", "assurance", a)
				}
			}
			continue
		}

		peerID := id
		peer := p
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Error(log.Node, "broadcast error", "id", id, "err", r)
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), SmallTimeout)
			defer cancel()

			switch objType {
			case reflect.TypeOf(types.Ticket{}):
				t := obj.(types.Ticket)
				epoch := n.getEpoch()
				if err := peer.SendTicketDistribution(ctx, epoch, t, false); err != nil {
					log.Warn(log.Quic, "SendTicketDistribution", "n", n.String(), "->p", peer.PeerID, "err", err)
				}
			case reflect.TypeOf(JAMSNP_BlockAnnounce{}):
				b := obj.(JAMSNP_BlockAnnounce)
				h := b.Header.Hash()

				//TODO: what's the difference between block and block announcement case?
				if !isGridNeighbor(n.id, id) {
					log.Trace(log.Quic, "Skip Block Broadcast AAAA - NOT GRID NEIGHBOR", "n(self)", n.String(), "peer.ID", id)
					return
				}

				if p.IsKnownHash(h) || n.ba_checker.CheckAndSet(h, peerID) {
					return
				}
				p.AddKnownHash(h)
				//log.Info(log.Node, "broadcast-BlockAnnouncement", "h", h.String(), "n", n.String(), "peerID", peerID)
				up0_stream, err := peer.GetOrInitBlockAnnouncementStream(context.Background())
				if err != nil {
					log.Warn(log.Quic, "GetOrInitBlockAnnouncementStream", "n", n.String(), "->p", peer.PeerID, "err", err)
					return
				}
				block_a_bytes := b.ToBytes()
				err = sendQuicBytes(ctx, up0_stream, block_a_bytes, peerID, UP0_BlockAnnouncement)
				if err != nil {
					log.Warn(log.Quic, "SendBlockAnnouncement:sendQuicBytes (broadcast)", "n", n.String(), "err", err)
				}

			case reflect.TypeOf(types.Guarantee{}):
				g := obj.(types.Guarantee)
				if err := peer.SendWorkReportDistribution(ctx, g.Report, g.Slot, g.Signatures); err != nil {
					log.Warn(log.Quic, "SendWorkReportDistribution", "n", n.String(), "err", err)
					return
				}
			case reflect.TypeOf(types.Assurance{}):
				a := obj.(types.Assurance)
				if err := peer.SendAssurance(ctx, &a); err != nil {
					log.Warn(log.Quic, "SendAssurance", "n", n.String(), "err", err)
					return
				}
			case reflect.TypeOf(JAMSNPAuditAnnouncementWithProof{}):
				a := obj.(JAMSNPAuditAnnouncementWithProof)
				tranche := a.Announcement.Tranche
				log.Debug(log.Audit, "SendAuditAnnouncement", "n", n.String(), "tranche", tranche, "peerID", peerID)
				if tranche == 0 {
					s0 := a.EvidenceTranche0
					if len(s0) >= 4 && binary.BigEndian.Uint32(s0[0:4]) == 0 {
						panic(fmt.Errorf("tranche0 evidence is empty in Announce"))
					}
					if err := peer.SendAuditAnnouncement(ctx, a.Announcement, a.EvidenceTranche0); err != nil {
						log.Warn(log.Quic, "SendAuditAnnouncement", "n", n.String(), "tranche0", tranche, "err", err)
						return
					}
				} else {
					if err := peer.SendAuditAnnouncement(ctx, a.Announcement, a.EvidenceTrancheN); err != nil {
						log.Warn(log.Quic, "SendAuditAnnouncement", "n", n.String(), "tranche", tranche, "err", err)
						return
					}
				}

			case reflect.TypeOf(types.Judgement{}):
				j := obj.(types.Judgement)
				if p.IsKnownHash(j.Hash()) {
					return
				}
				if !isGridNeighbor(n.id, id) {
					log.Trace(log.Quic, "Skip CE 145 Judgement Broadcast - NOT GRID NEIGHBOR", "n(self)", n.String(), "peer.ID", id)
					return
				}
				p.AddKnownHash(j.Hash())
				epoch := n.getEpoch()
				if err := peer.SendJudgmentPublication(ctx, epoch, j); err != nil {
					log.Warn(log.Quic, "SendJudgmentPublication", "n", n.String(), "err", err)
					return
				}
			case reflect.TypeOf(types.PreimageAnnouncement{}):
				announcement := obj.(types.PreimageAnnouncement)
				h := announcement.PreimageHash
				if p.IsKnownHash(h) {
					return
				}
				if !isGridNeighbor(n.id, id) {
					log.Trace(log.Quic, "Skip CE 142 PreimageAnnouncement Broadcast - NOT GRID NEIGHBOR", "n(self)", n.String(), "peer.ID", id)
					return
				}
				//log.Trace(log.Node, "broadcast-PreimageAnnouncement", "h", h.String(), "n", n.String(), "peerID", peerID)
				if err := peer.SendPreimageAnnouncement(ctx, &announcement); err != nil {
					log.Warn(log.Quic, "SendPreimageAnnouncement", "n", n.String(), "err", err)
					return
				}
			case reflect.TypeOf(grandpa.VoteMessage{}):
				vote := obj.(grandpa.VoteMessage)
				if err := peer.SendVoteMessage(ctx, vote); err != nil {
					log.Warn(log.Quic, "SendVoteMessage", "n", n.String(), "err", err)
					return
				}
			case reflect.TypeOf(grandpa.CommitMessage{}):
				commit := obj.(grandpa.CommitMessage)
				if err := peer.SendCommitMessage(ctx, commit); err != nil {
					log.Warn(log.Quic, "SendCommitMessage", "n", n.String(), "err", err)
					return
				}
			}
		}()
	}
}

func (n *NodeContent) getStateDBByHeaderHash(headerHash common.Hash) (statedb *statedb.StateDB, ok bool) {
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()
	statedb, ok = n.statedbMap[headerHash]
	return statedb, ok
}

func (n *Node) fetchBlocks(headerHash common.Hash, direction uint8, maximumBlocks uint32) (*[]types.Block, error) {
	requests_original := make([]CE128_request, types.TotalValidators)
	for i := range types.TotalValidators {
		requests_original[i] = CE128_request{
			HeaderHash:    headerHash,
			Direction:     direction,
			MaximumBlocks: maximumBlocks,
		}
	}
	requests := make(map[uint16]interface{})
	for i, req := range requests_original {
		requests[uint16(i)] = req
	}

	resps, err := n.makeRequests(requests, 1, SmallTimeout, LargeTimeout)
	if err != nil {
		return nil, err
	}
	for _, resp := range resps {
		ce128_Resp, ok := resp.(CE128_response)
		if !ok {
			// ignore such response
			continue
		}

		if len(ce128_Resp.Blocks) >= 0 && ce128_Resp.HeaderHash == headerHash {
			return &ce128_Resp.Blocks, nil
		}
	}
	return nil, fmt.Errorf("fetchBlocks - No response")
}

func (n *Node) extendChain(ctx context.Context) error {
	if n.block_tree == nil {
		return nil
	}
	start := time.Now()
	n.statedbMutex.Lock()
	mutexElapsed := common.ElapsedStr(start)

	latestStateDB := n.statedb
	if latestStateDB.Block == nil {
		n.statedbMutex.Unlock()
		log.Warn(log.B, "extendChain", "SyncState", "latestStateDB.Block is nil")
		return nil
	}
	currentHash := n.block_tree.GetLastFinalizedBlock().Block.Header.Hash()
	// if current block tree is forked, we need to apply from the common ancestor
	currNode, ok := n.block_tree.GetBlockNode(currentHash)
	if len(n.block_tree.GetLeafs()) > 1 {

		leafs := n.block_tree.GetLeafs()
		leafs_slice := make([]*types.BT_Node, 0)
		for _, leaf := range leafs {
			if leaf.Applied {
				continue
			}
			leafs_slice = append(leafs_slice, leaf)
		}
		if len(leafs_slice) > 1 {
			currNode = n.block_tree.GetCommonAncestor(leafs_slice[0], leafs_slice[1])
		}
	}
	blocktreeElapsed := common.ElapsedStr(start)
	n.statedbMutex.Unlock()

	if !ok {
		// Handle edge case: maybe we're at genesis
		if n.block_tree.Root.Block.Header.ParentHeaderHash == currentHash {
			currNode = n.block_tree.Root
		}
		if currNode == nil {
			currNode = n.block_tree.Root
		}
		if !currNode.Applied {
			if err := n.ApplyBlock(ctx, currNode); err != nil {
				log.Error(log.B, "SyncState", "ApplyFirstBlock", "block", currNode.Block.Header.Hash().String_short(), "err", err)
				return fmt.Errorf("extendChain: ApplyFirstBlock failed: %w", err)
			}
		}
	}

	// Traverse and apply all descendants
	if err := n.applyChildrenRecursively(ctx, currNode); err != nil {
		header := currNode.Block.Header
		log.Error(log.Node, "SyncState", "applyChildren", err, "header", header.Hash().String_short(), "slot", header.Slot, "author", header.AuthorIndex)
		return err
	}
	applyBlockElapsed := common.ElapsedStr(start)
	log.Debug(log.B, "extendChain internal elapsed", "n", n.String(),
		"mutexElapsed", mutexElapsed,
		"blocktreeElapsed", blocktreeElapsed,
		"applyBlockElapsed", applyBlockElapsed,
	)
	//TODO
	return nil
}
func (n *Node) getCE129(nodeIndex uint16, headerHash common.Hash) {
	var startKey [31]byte
	var endKey [31]byte
	for i := range 31 {
		startKey[i] = 0x00
		endKey[i] = 0xff
	}
	n.peersInfo[nodeIndex].SendStateRequest(context.TODO(), headerHash, startKey, endKey, 10000000)
}

func (n *Node) applyChildrenRecursively(ctx context.Context, node *types.BT_Node) error {
	for _, child := range node.Children {
		start := time.Now()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if child.Applied {
			if err := n.applyChildrenRecursively(ctx, child); err != nil {
				return err
			}
			continue
		}
		if err := n.ApplyBlock(ctx, child); err != nil {
			return fmt.Errorf("applyChildrenRecursively: ApplyBlock failed for %v: %w", child.Block.Header.Hash(), err)
		}
		if err := n.applyChildrenRecursively(ctx, child); err != nil {
			return err
		}
		applyChildrenElapsed := common.ElapsedStr(start)
		if applyChildrenElapsed > 1*time.Second {
			log.Debug(log.B, "applyChildrenRecursively elapsed", "n", n.String(),
				"child", child.Block.Header.Hash().String_short(),
				"applyChildrenElapsed", applyChildrenElapsed,
			)
		}
	}
	return nil
}

func (n *Node) ApplyBlock(ctx context.Context, nextBlockNode *types.BT_Node) error {
	nextBlock := nextBlockNode.Block
	// if !n.appliedFirstBlock {
	// 	if nextBlock.Header.ParentHeaderHash == genesisBlockHash {
	// 		n.appliedFirstBlock = true
	// 	}
	// }
	// 1. Prepare recovered state from parent
	start := time.Now()
	recoveredStateDB := n.statedb.Copy()
	recoveredStateDB.RecoverJamState(nextBlock.Header.ParentStateRoot) // it don't even know if it got the correct state
	recoveredStateDB.UnsetPosteriorEntropy()
	recoveredStateDB.StateRoot = nextBlock.Header.ParentStateRoot
	recoveredStateDB.Block = nextBlock
	recoverElapsed := time.Since(start)

	var used_entropy common.Hash
	if nextBlock.EpochMark() != nil {
		used_entropy = nextBlock.EpochMark().TicketsEntropy
	} else {
		used_entropy = recoveredStateDB.GetSafrole().Entropy[2]
	}
	start = time.Now()
	valid_tickets := n.extrinsic_pool.GetTicketIDPairFromPool(used_entropy)
	newStateDB, err := statedb.ApplyStateTransitionFromBlock(recoveredStateDB, ctx, nextBlock, valid_tickets, n.pvmBackend)
	stateTransitionElapsed := common.ElapsedStr(start)
	if err != nil {
		fmt.Printf("[N%d] extendChain FAIL %v\n", n.id, err)
		return fmt.Errorf("ApplyStateTransitionFromBlock failed: %w", err)
	}
	start = time.Now()
	// newStateDB.GetAllKeyValues()
	newStateDB.Block = nextBlock
	newStateDB.SetAncestor(nextBlock.Header, recoveredStateDB)
	n.clearQueueUsingBlock(nextBlock.Extrinsic.Guarantees)

	// 2. Update services for new state
	n.updateServiceMap(newStateDB, nextBlock)
	updateServiceElapsed := common.ElapsedStr(start)
	// 3. Async write of debug state
	go func() {
		st := buildStateTransitionStruct(recoveredStateDB, nextBlock, newStateDB)
		//log.Debug(log.B, "Storing ImportBlock", "n", n.String(), "h", nextBlock.Header.Hash().String_short(), "blk", nextBlock.Str())
		if isWriteTransition {
			if err := n.writeDebug(st, nextBlock.TimeSlot(), true); err != nil {
				log.Error(log.Node, "writeDebug: StateTransition", "err", err)
			}
		}
		if isWriteSnapshot {
			if err := n.writeDebug(newStateDB.JamState.Snapshot(&st.PostState, newStateDB.GetStateUpdates()), nextBlock.TimeSlot(), false); err != nil {
				log.Error(log.Node, "writeDebug: Snapshot", "err", err)
			}
		}
	}()
	start = time.Now()
	// 4. Extend the chain
	err = n.addStateDB(newStateDB)
	if err != nil {
		log.Error(log.B, "ApplyBlock: addStateDB failed", "n", n.String(), "err", err)
		return fmt.Errorf("addStateDB failed: %w", err)
	}
	addStateDBElapsed := common.ElapsedStr(start)
	// 5. Finalization logic
	start = time.Now()
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()
	mini_peers := 2
	latest_block_info := n.GetLatestBlockInfo()
	nextBlockNode.Applied = true
	currEpoch, currPhase := newStateDB.JamState.SafroleState.EpochAndPhase(nextBlock.Header.Slot)

	mode := "safrole"
	if newStateDB.JamState.SafroleState.GetEpochT() == 0 {
		mode = "fallback"
	}
	log.Info(log.B, "Imported Block", // "n", n.String(),
		"mode", mode,
		"author", nextBlock.Header.AuthorIndex,
		"p", nextBlock.Header.ParentHeaderHash.String_short(),
		"h", common.Str(nextBlock.Header.Hash()),
		"e'", currEpoch, "m'", currPhase,
		"len(γ_a')", len(newStateDB.JamState.SafroleState.NextEpochTicketsAccumulator),
		"blk", nextBlock.Str(),
	)
	// TODO: write finalizd block kv here

	if CE129_test {
		go n.getCE129(nextBlock.Header.AuthorIndex, nextBlock.Header.Hash())
	}

	if newStateDB.GetSafrole().GetTimeSlot() != nextBlock.Header.Slot {
		panic("ApplyBlock: TimeSlot mismatch")
	}
	if latest_block_info == nil {
		log.Info(log.B, "ApplyBlock: latest_block_info is nil", "n", n.String())
		return nil
	}
	if nextBlock.Header.Hash() == latest_block_info.HeaderHash {
		n.extrinsic_pool.ForgetPreimages(newStateDB.GetForgets())
		if len(n.UP0_stream) > mini_peers {
			log.Trace(log.Quic, "ApplyBlock: UP0_stream", "n", n.String(), "len", len(n.UP0_stream))
			n.SetIsSync(true, "syncing")
		}
		go func() {
			assure_ctx, cancel := context.WithTimeout(context.Background(), NormalTimeout)
			defer cancel()
			if err := n.assureNewBlock(assure_ctx, nextBlock, newStateDB); err != nil {
				log.Error(log.A, "ApplyBlock: assureNewBlock failed", "n", n.String(), "err", err)
			}
		}()

		// MK: NOT sure if this is the proper place to set this completedJCE
		log.Debug(log.Node, "ApplyBlock: SetCompletedJCE !!!!", "n", n.String(), "slot", nextBlock.Header.Slot)
		n.SetCompletedJCE(nextBlock.Header.Slot)

		if n.AuditFlag {
			if snap, ok := n.statedbMap[n.statedb.HeaderHash]; ok {
				select {
				case n.auditingCh <- snap.Copy():
					log.Debug(log.Audit, "ApplyBlock: auditingCh", "n", n.String(), "slot", nextBlock.Header.Slot)
				default:
					log.Warn(log.Node, "auditingCh full, skipping audit")
				}
			}
		}

		if CE138_test {
			fmt.Printf("CE138_test: START\n")
			for _, workReport := range n.statedb.AvailableWorkReport {
				spec := workReport.AvailabilitySpec
				coreIndex := workReport.CoreIndex
				workPackageHash := spec.WorkPackageHash

				workPackageBundle, err := n.reconstructPackageBundleSegments(spec.ErasureRoot, spec.BundleLength, workReport.SegmentRootLookup, coreIndex, spec.ExportedSegmentLength)
				if err != nil {
					log.Error(log.Audit, "FetchWorkPackageBundle:reconstructPackageBundleSegments", "err", err)
					continue
				}
				if workPackageBundle.PackageHash() != workPackageHash {
					log.Error(log.Audit, "auditWorkReport:FetchWorkPackageBundle package mismatch")
					continue
				}
				wr, _, pvmElapsed, _, err := n.executeWorkPackageBundle(uint16(workReport.CoreIndex), workPackageBundle, workReport.SegmentRootLookup, n.statedb.GetTimeslot(), false)
				if err != nil {
					log.Error(log.Audit, "auditWorkReport:executeWorkPackageBundle", "err", err)
					continue
				}
				if reflect.DeepEqual(wr, workReport) {
					log.Info(log.DA, "reconstructPackageBundleSegments: WorkReport matches", "n", n.String(),
						"coreIndex", coreIndex,
						"workPackageHash", workPackageHash.String_short(),
						"pvmElapsed", pvmElapsed,
						"workReport", workReport.Hash())
				} else {
					log.Error(log.DA, "reconstructPackageBundleSegments: WorkReport mismatch", "n", n.String(),
						"coreIndex", coreIndex,
						"workPackageHash", workPackageHash.String_short(),
						"pvmElapsed", pvmElapsed,
						"workReport", workReport.Hash(),
					)

				}
			}
		}

		if CE138_test && false {
			for _, workReport := range n.statedb.AvailableWorkReport {
				spec := workReport.AvailabilitySpec
				coreIndex := workReport.CoreIndex
				workPackageHash := spec.WorkPackageHash

				workPackageBundle, err := n.reconstructPackageBundleSegments(spec.ErasureRoot, spec.BundleLength, workReport.SegmentRootLookup, coreIndex, spec.ExportedSegmentLength)
				if err != nil {
					log.Error(log.Audit, "FetchWorkPackageBundle:reconstructPackageBundleSegments", "err", err)
					continue
				}
				if workPackageBundle.PackageHash() != workPackageHash {
					log.Error(log.Audit, "auditWorkReport:FetchWorkPackageBundle package mismatch")
					continue
				}
				wr, _, pvmElapsed, _, err := n.executeWorkPackageBundle(uint16(workReport.CoreIndex), workPackageBundle, workReport.SegmentRootLookup, n.statedb.GetTimeslot(), false)
				if err != nil {
					log.Error(log.Audit, "auditWorkReport:executeWorkPackageBundle", "err", err)
					continue
				}
				if reflect.DeepEqual(wr, workReport) {
					log.Info(log.DA, "reconstructPackageBundleSegments: WorkReport matches", "n", n.String(),
						"coreIndex", coreIndex,
						"workPackageHash", workPackageHash.String_short(),
						"pvmElapsed", pvmElapsed,
						"workReport", workReport.Hash())
				} else {
					log.Error(log.DA, "reconstructPackageBundleSegments: WorkReport mismatch", "n", n.String(),
						"coreIndex", coreIndex,
						"workPackageHash", workPackageHash.String_short(),
						"pvmElapsed", pvmElapsed,
						"workReport", workReport.Hash(),
					)

				}
			}
		}
	} else {
		log.Info(log.Quic, "ApplyBlock: latest_block not equal to nextBlock", "n", n.String(), "latest_block", latest_block_info.HeaderHash.String_short(), "nextBlock", nextBlock.Header.Hash().String_short())
	}

	// 6. Cleanup used extrinsics
	isClosed := n.statedb.GetSafrole().IsTicketSubmissionClosed(n.statedb.GetTimeslot())
	n.extrinsic_pool.RemoveUsedExtrinsicFromPool(nextBlock, n.statedb.GetSafrole().Entropy[2], isClosed)
	finalElapsed := common.ElapsedStr(start)
	log.Debug(log.B, "ApplyBlock elapsed", "n", n.String(),
		"recoverElapsed", recoverElapsed,
		"stateTransitionElapsed", stateTransitionElapsed,
		"updateServiceElapsed", updateServiceElapsed,
		"addStateDBElapsed", addStateDBElapsed,
		"finalElapsed", finalElapsed,
	)

	if recoverElapsed > 500*time.Millisecond || stateTransitionElapsed > 500*time.Millisecond || updateServiceElapsed > 500*time.Millisecond || addStateDBElapsed > 500*time.Millisecond || finalElapsed > 500*time.Millisecond {
		if !cpu_flag {
			cpu_flag = true
			go func() {
				log.Info(log.Node, "CPU profile START!!!!!", "n", n.String())
				f, err := os.Create("/tmp/recover_slow_cpu.pprof")
				if err != nil {
					log.Warn(log.Node, "Create CPU profile", "err", err)
					return
				}
				defer f.Close()

				if err := pprof.StartCPUProfile(f); err != nil {
					log.Warn(log.Node, "Start CPU profile", "err", err)
					return
				}
				time.Sleep(5 * time.Minute)
				pprof.StopCPUProfile()
			}()
		}
	}
	return nil
}

var cpu_flag = false

func (n *Node) assureNewBlock(ctx context.Context, b *types.Block, sdb *statedb.StateDB) error {
	if n.hub != nil {
		finalizedBlockNode := n.block_tree.GetLastFinalizedBlock()
		finalizedBlock := finalizedBlockNode.Block
		bestBlockNode := n.block_tree.GetLastAuditedBlock()
		bestBlock := bestBlockNode.Block
		go n.hub.ReceiveLatestBlock(finalizedBlock, bestBlock, sdb, false)
	}
	assureStart := time.Now()

	if len(b.Extrinsic.Guarantees) > 0 {
		var wg sync.WaitGroup
		errCh := make(chan error, len(b.Extrinsic.Guarantees))

		for _, g := range b.Extrinsic.Guarantees {
			// First, store the work report (independent of assurance)
			if err := n.StoreWorkReport(g.Report); err != nil {
				log.Error(log.DA, "assureNewBlock: StoreWorkReport failed", "n", n.String(), "err", err)
			}

			wg.Add(1)
			go func(g types.Guarantee) {
				defer wg.Done()

				if ctx.Err() != nil {
					errCh <- ctx.Err()
					return
				}

				if err := n.assureData(ctx, g); err != nil {
					log.Error(log.DA, "assureNewBlock: assureData failed", "n", n.String(), "err", err)
					errCh <- err
				}
			}(g)
		}

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			return fmt.Errorf("assureNewBlock cancelled: %w", ctx.Err())
		case err := <-errCh:
			return fmt.Errorf("assureNewBlock encountered error: %w", err)
		case <-done:
			// All assurances completed successfully
		}
	}

	// Context check before continuing to assure
	if ctx.Err() != nil {
		return ctx.Err()
	}
	a, numCores := n.generateAssurance(b.Header.Hash(), b.TimeSlot())
	if numCores == 0 {
		return nil
	}
	n.broadcast(ctx, a) // via CE141
	n.Telemetry(log.MsgTypeAssurance, a, "elapsed", common.Elapsed(assureStart), "codec_encoded", types.EncodeAsHex(a))

	return nil
}

// we arrive here when we receive a block from another node
func (n *Node) processBlock(blk *types.Block) error {
	// walk blk backwards, up to the tip, if possible -- but if encountering an unknown parenthash, immediately fetch the block.  Give up if we can't do anything
	n.StoreBlock(blk, n.id, false)
	err := n.cacheBlock(blk)
	// Sometimes this loop will get in deadlock

	return err
}

// reconstructSegments uses CE139 and CAN use CE140 upon failure
// We continuily use erasureRoot to ask the question
func (n *NodeContent) reconstructSegments(si *SpecIndex) (segments [][]byte, justifications [][]common.Hash, err error) {
	requests_original := make([]CE139_request, types.TotalValidators)

	// this will track the proofpages
	proofpages := []uint16{}
	segmentsPerPageProof := uint16(64)

	// ask for the indices, and tally the proofpages needed to fetch justifications
	allsegmentindices := make([]uint16, len(si.Indices))
	for i, idx := range si.Indices {
		allsegmentindices[i] = idx
		if idx >= si.WorkReport.AvailabilitySpec.ExportedSegmentLength {
			return segments, justifications, fmt.Errorf("requested index %d exceeds availability spec export count %d", idx, si.WorkReport.AvailabilitySpec.ExportedSegmentLength)
		}
		p := idx / segmentsPerPageProof
		if !slices.Contains(proofpages, p) {
			proofpages = append(proofpages, p)
		}
	}
	for _, p := range proofpages {
		allsegmentindices = append(allsegmentindices, si.WorkReport.AvailabilitySpec.ExportedSegmentLength+p)
	}
	for validatorIdx := range types.TotalValidators {
		requests_original[validatorIdx] = CE139_request{
			ErasureRoot:    si.WorkReport.AvailabilitySpec.ErasureRoot,
			SegmentIndices: allsegmentindices,
			CoreIndex:      uint16(si.WorkReport.CoreIndex),
			ShardIndex:     ComputeShardIndex(uint16(si.WorkReport.CoreIndex), uint16(validatorIdx)),
		}
	}
	requests := make(map[uint16]interface{})
	for validatorIdx, req := range requests_original {
		requests[uint16(validatorIdx)] = req
	}
	responses, err := n.makeRequests(requests, types.RecoveryThreshold, SmallTimeout, LargeTimeout)
	if err != nil {
		fmt.Printf("Error in fetching import segments By ErasureRoot: %v\n", err)
		return segments, justifications, err
	}

	shards := make([][]byte, types.RecoveryThreshold)
	indexes := make([]uint32, types.RecoveryThreshold)
	numShards := 0
	for _, resp := range responses {
		daResp, ok := resp.(CE139_response)
		if !ok {
			fmt.Printf("Error in convert import segments CE139_response: %v\n", err)
		}
		if numShards < len(indexes) {
			indexes[numShards] = uint32(daResp.ShardIndex)
			shards[numShards] = daResp.SegmentShards // this is actually multiple segment shards
			if false {
				fmt.Printf("%s EC response shardindex=%d raw=%x hash=%s (%d bytes)\n", n.String(), daResp.ShardIndex,
					daResp.SegmentShards, common.Blake2Hash(daResp.SegmentShards), len(daResp.SegmentShards))
			}
			numShards++
		}
	}
	chunkSize := types.NumECPiecesPerSegment * 2 // MKTODO: SegmentSize / W_E * 2
	//	fmt.Printf("!!! reconstructSegments: %d segments, %d shards, %d chunkSize\n", len(allsegmentindices), numShards, chunkSize)
	rawshards := make([][]byte, len(indexes))
	numsegments := len(allsegmentindices)
	allsegments := make([][]byte, numsegments) // note that the last few are actually pageproofs
	for s := 0; s < numsegments; s++ {
		for i := 0; i < numShards; i++ {
			rawshards[i] = shards[i][s*chunkSize : (s+1)*chunkSize]
		}
		recoveredSegment, err := bls.Decode(rawshards, types.TotalValidators, indexes, types.SegmentSize)
		if err != nil {
			fmt.Printf("Error in fetching import segments decode: %v\n", err)
			allsegments[s] = nil // ???
		} else {
			allsegments[s] = recoveredSegment
		}
	}

	// j - justifications  (14.14) J(W in I)

	indicesLen := len(si.Indices)
	pageproofs := allsegments[indicesLen:] // problematic
	clonedProofs := make([][]byte, len(pageproofs))
	for i := range pageproofs {
		// allocate a fresh slice and copy
		cloned := make([]byte, len(pageproofs[i]))
		copy(cloned, pageproofs[i])
		clonedProofs[i] = cloned
	}
	segmentsonly := allsegments[0:indicesLen] // problematic
	clonedSegs := make([][]byte, len(segmentsonly))
	for i := range segmentsonly {
		cloned := make([]byte, len(segmentsonly[i]))
		copy(cloned, segmentsonly[i])
		clonedSegs[i] = cloned
	}
	justifications = make([][]common.Hash, indicesLen)
	for i, segmentIndex := range si.Indices {
		pageSize := 1 << trie.PageFixedDepth
		pageIdx := int(segmentIndex) / pageSize
		pagedProofByte := clonedProofs[pageIdx]
		// Decode the proof back to segments and verify
		decodedData, _, err := types.Decode(pagedProofByte, reflect.TypeOf(types.PageProof{}))
		if err != nil {
			fmt.Printf("Failed to decode page proof: %v", err)
		}
		recoveredPageProof := decodedData.(types.PageProof)
		//fmt.Printf("recoveredPageProof: %v\n", recoveredPageProof)
		subTreeIdx := int(segmentIndex) % pageSize
		fullJustification, err := trie.PageProofToFullJustification(pagedProofByte, pageIdx, subTreeIdx)
		if err != nil {
			fmt.Printf("fullJustification len: %d, PageProofToFullJustification ERR: %v.", len(fullJustification), err)
		}
		leafHash := recoveredPageProof.LeafHashes[subTreeIdx]
		derived_globalRoot_j0 := trie.VerifyCDTJustificationX(leafHash.Bytes(), int(segmentIndex), fullJustification, 0)
		if common.BytesToHash(derived_globalRoot_j0) != common.BytesToHash(si.WorkReport.AvailabilitySpec.ExportedSegmentRoot[:]) {
			log.Error(log.DA, "cdttree:VerifyCDTJustificationX", "derived_globalRoot_j0", common.BytesToHash(derived_globalRoot_j0), "ExportedSegmentRoot", si.WorkReport.AvailabilitySpec.ExportedSegmentRoot)
			return segments, justifications, err
		} else {
			log.Debug(log.DA, "cdttree:VerifyCDTJustificationX Justified", "ExportedSegmentRoot", common.BytesToHash(si.WorkReport.AvailabilitySpec.ExportedSegmentRoot[:]))
		}
		justifications[i] = fullJustification
	}
	if len(clonedSegs) != indicesLen {
		log.Error(log.DA, "reconstructSegments", "l", len(clonedSegs), "l2", len(justifications))
	}

	return clonedSegs, justifications, nil
}

// HERE we are in a AUDITING situation, if verification fails, we can still execute the work package by using CE140?
func (n *NodeContent) reconstructPackageBundleSegments(erasureRoot common.Hash, blength uint32, segmentRootLookup types.SegmentRootLookup, coreIndex uint, exportedSegmentLength uint16) (types.WorkPackageBundle, error) {

	// Prepare requests to validators
	requestsOriginal := make([]CE138_request, types.TotalValidators)
	for validatorIdx := range requestsOriginal {
		requestsOriginal[validatorIdx] = CE138_request{
			ErasureRoot: erasureRoot,
			CoreIndex:   uint16(coreIndex),
			ShardIndex:  ComputeShardIndex(uint16(coreIndex), uint16(validatorIdx)),
		}
	}

	requests := make(map[uint16]interface{}, len(requestsOriginal))
	for validatorIdx, req := range requestsOriginal {
		requests[uint16(validatorIdx)] = req
	}
	// calling fetchall
	if attemptReconstruction {
		n.FetchAllBundleAndSegmentShards(uint16(coreIndex), erasureRoot, exportedSegmentLength, true)
	}

	// Fetch shard responses
	responses, err := n.makeRequests(requests, types.RecoveryThreshold, SmallTimeout, LargeTimeout)
	if err != nil {
		log.Error(log.Node, "reconstructPackageBundleSegments: makeRequests failed", "err", err)
		return types.WorkPackageBundle{}, fmt.Errorf("makeRequests failed: %w", err)
	}

	// Collect valid bundle shards
	bundleShards := make([][]byte, types.RecoveryThreshold)
	indexes := make([]uint32, types.RecoveryThreshold)
	numShards := 0
	for _, resp := range responses {
		daResp, ok := resp.(CE138_response)
		if !ok {
			log.Warn(log.Node, "reconstructPackageBundleSegments: invalid CE138_response conversion", "response", resp)
			continue
		}

		if numShards >= types.RecoveryThreshold {
			break
		}

		decodedPath, decodeErr := common.DecodeJustification(daResp.Justification, types.NumECPiecesPerSegment)
		if decodeErr != nil {
			log.Warn(log.Node, "reconstructPackageBundleSegments: DecodeJustification failed", "err", decodeErr)
			continue
		}

		bClub := common.Blake2Hash(daResp.BundleShard)
		sClub := daResp.SClub
		leaf := common.BuildBundleSegment(bClub, sClub)
		verified, _ := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(daResp.ShardIndex), leaf, decodedPath, "reconstructPackageBundleSegments")
		if verified {
			bundleShards[numShards] = daResp.BundleShard
			indexes[numShards] = uint32(daResp.ShardIndex)
			numShards++
			log.Info(log.Node, "reconstructPackageBundleSegments: shard verified", "len", len(daResp.BundleShard), "shardIndex", daResp.ShardIndex, "bundleShard", fmt.Sprintf("%x", daResp.BundleShard))
		} else {
			log.Warn(log.Node, "reconstructPackageBundleSegments: shard verification failed", "callerIdx", n.id, "shardIndex", daResp.ShardIndex)
		}
		log.Debug(log.Node, "reconstructPackageBundleSegments: shard received", "shardIndex", daResp.ShardIndex, "bundleShard", fmt.Sprintf("%x", daResp.BundleShard))
	}

	// Check if enough shards were collected
	if numShards < types.RecoveryThreshold {
		log.Error(log.Node, "reconstructPackageBundleSegments: insufficient valid shards", "have", numShards, "need", types.TotalCores)
		return types.WorkPackageBundle{}, fmt.Errorf("insufficient valid shards: have %d, need %d", numShards, types.TotalCores)
	}

	// Attempt to decode the full bundle
	log.Debug(log.Audit, "reconstructPackageBundleSegments: Decoding bundle1", "callerIdx", n.id, "shardIndex", indexes[:numShards])
	encodedBundle, err := bls.Decode(bundleShards[:numShards], types.TotalValidators, indexes[:numShards], int(blength))
	if err != nil {
		log.Error(log.Node, "reconstructPackageBundleSegments: Decode failed", "err", err)
		return types.WorkPackageBundle{}, fmt.Errorf("decode failed: %w", err)
	}

	log.Info(log.Audit, "reconstructPackageBundleSegments: bundle EC decoded", "shards", indexes[:numShards], "encodedBundle", fmt.Sprintf("%x", encodedBundle))
	workPackageBundleRaw, _, err := types.Decode(encodedBundle, reflect.TypeOf(types.WorkPackageBundle{}))
	if err != nil {
		log.Error(log.Node, "reconstructPackageBundleSegments: Decode into WorkPackageBundle failed", "err", err)
		return types.WorkPackageBundle{}, fmt.Errorf("decode WorkPackageBundle failed: %w", err)
	}

	workPackageBundle, ok := workPackageBundleRaw.(types.WorkPackageBundle)
	if !ok {
		log.Error(log.Node, "reconstructPackageBundleSegments: casting to WorkPackageBundle failed")
		return types.WorkPackageBundle{}, fmt.Errorf("failed to cast to WorkPackageBundle")
	}

	// IMPORTANT: Verify the reconstructed bundle against the segment root lookup
	verified, verifyErr := n.VerifyBundle(&workPackageBundle, segmentRootLookup)
	if verifyErr != nil {
		log.Warn(log.Node, "reconstructPackageBundleSegments: VerifyBundle errored", "err", verifyErr)
		return types.WorkPackageBundle{}, fmt.Errorf("verify bundle failed: %w", verifyErr)
	}
	if !verified {
		log.Warn(log.Node, "reconstructPackageBundleSegments: bundle verification failed")
		return types.WorkPackageBundle{}, fmt.Errorf("bundle verification failed")
	}

	return workPackageBundle, nil
}

func (n *NodeContent) getPVMStateDB() *statedb.StateDB {
	// refine's executeWorkPackage is a statelessish process
	target_statedb := n.statedb.Copy()
	return target_statedb
}

// AddPreimageToPool adds a new preimage to the extrinsic pool NO VALIDATION IS REQUIRED
func (n *NodeContent) AddPreimageToPool(serviceID uint32, preimage []byte) (err error) {
	// here we check that it has been solicited (or new)
	n.extrinsic_pool.AddPreimageToPool(types.Preimages{
		Requester: serviceID,
		Blob:      preimage,
	})
	return nil
}

func getMessageType(obj interface{}) string {
	switch obj.(type) {
	case types.BlockQuery:
		return "BlockQuery"
	case types.Guarantee:
		return "Guarantee"
	case types.Assurance:
		return "Assurance"
	case types.Judgement:
		return "Judgement"
	case types.Preimages:
		return "Preimages"
	case types.Ticket:
		return "Ticket"
	case *types.Ticket:
		return "Ticket"
	case *types.Block:
		return "block"
	case types.Block:
		return "block"
	case types.Announcement:
		return "Announcement"
	case types.WorkPackage:
		return "WorkPackage"
	case types.DistributeECChunk:
		return "DistributeECChunk"
	case types.ECChunkQuery:
		return "ECChunkQuery"
	case *statedb.StateDB:
		return "StateDB"
	case statedb.StateDB:
		return "StateDB"
	case *types.WorkPackageBundleSnapshot:
		return "bundle_snapshot"
	case types.WorkPackageBundleSnapshot:
		return "bundle_snapshot"
	case *statedb.JamState:
		return "JamState"
	case statedb.JamState:
		return "JamState"
	case statedb.StateSnapshot:
		return "state_snapshot"
	case *statedb.StateSnapshot:
		return "state_snapshot"
	case statedb.StateSnapshotRaw:
		return "Trace"
	case *statedb.StateSnapshotRaw:
		return "Trace"
	case statedb.StateTransition:
		return "state_transition"
	case *statedb.StateTransition:
		return "state_transition"
	case CE128_request:
		return "CE128_request"
	case CE128_response:
		return "CE128_response"
	case CE138_request:
		return "CE138_request"
	case CE138_response:
		return "CE138_response"
	case CE139_request:
		return "CE139_request"
	case CE139_response:
		return "CE139_response"
	default:
		return "unknown"
	}
}

const TickTime = 200

func (n *Node) PrintGuarantorAssignments() {
	for _, assign := range n.statedb.GuarantorAssignments {
		vid := n.statedb.GetSafrole().GetCurrValidatorIndex(assign.Validator.GetEd25519Key())
		fmt.Printf("v%d->c%v\n", vid, assign.CoreIndex)
	}
}
func (n *Node) SetSendTickets(sendTickets bool) {
	n.sendTickets = sendTickets
}

func (n *Node) writeLogWithDescription(obj interface{}, timeslot uint32, description string, withJSON bool) error {
	if !n.WriteDebugFlag {
		return nil
	}
	l := storage.LogMessage{
		Payload:     obj,
		Timeslot:    timeslot,
		Description: description,
	}
	return n.WriteLog(l, withJSON)
}

func (n *Node) writeDebug(obj interface{}, timeslot uint32, withJSON bool) error {
	if !n.WriteDebugFlag {
		return nil
	}
	l := storage.LogMessage{
		Payload:  obj,
		Timeslot: timeslot,
	}
	return n.WriteLog(l, withJSON)
}

func (n *Node) WriteLog(logMsg storage.LogMessage, withJSON bool) error {
	//msgType := getStructType(obj)
	obj := logMsg.Payload
	timeSlot := logMsg.Timeslot
	description := logMsg.Description
	msgType := getMessageType(obj)

	dataDir := fmt.Sprintf("%s/data", n.dataDir)
	structDir := fmt.Sprintf("%s/%vs", dataDir, msgType)

	// Check if the directories exist, if not create them
	if _, err := os.Stat(structDir); os.IsNotExist(err) {
		err := os.MkdirAll(structDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("error creating %v directory: %v", msgType, err)
		}
	}

	if msgType != "unknown" {

		epoch, phase := statedb.ComputeEpochAndPhase(timeSlot, n.epoch0Timestamp)

		path := fmt.Sprintf("%s/%08d", structDir, timeSlot)
		if epoch == 0 && phase == 0 {
			path = fmt.Sprintf("%s/genesis", structDir)
		}
		if description != "" {
			path = fmt.Sprintf("%s/%08d_%s", structDir, timeSlot, description)
		}

		err := types.SaveObject(path, obj, withJSON)
		if err != nil {
			panic(err)
		}
	}
	return nil
}

func WriteSTFLog(stf *statedb.StateTransition, timeslot uint32, dataDir string, withJSON bool) error {

	structDir := fmt.Sprintf("%s/%vs", dataDir, "state_transition")

	// Check if the directories exist, if not create them
	if _, err := os.Stat(structDir); os.IsNotExist(err) {
		err := os.MkdirAll(structDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("error creating %v directory: %v", "state_transition", err)
		}
	}

	epoch, phase := statedb.ComputeEpochAndPhase(timeslot, 0)
	path := fmt.Sprintf("%s/%v_%03d", structDir, epoch, phase)
	if epoch == 0 && phase == 0 {
		path = fmt.Sprintf("%s/genesis", structDir)
	}
	types.SaveObject(path, stf, withJSON)
	return nil
}

func (n *Node) runJCEDefault() {
	const tickInterval = 100
	ticker_pulse := time.NewTicker(tickInterval * time.Millisecond)
	defer ticker_pulse.Stop()
	for {
		select {
		case <-ticker_pulse.C:
			currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
			n.SetCurrJCE(currJCE)
			//do something with it
		}
	}
}

func (n *Node) runFasterJCE() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	blockTicker := time.NewTicker(6000 * time.Millisecond)
	defer blockTicker.Stop()

	for {
		select {
		case <-ticker.C:
			prevJCE := n.GetCurrJCE()
			if prevJCE <= types.EpochLength {
				currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
				n.SetCurrJCE(currJCE)
				// do something with it
			} else {
				//n.GenerateTickets(prevJCE)
				//n.BroadcastTickets(prevJCE)
			}
		case <-blockTicker.C:
			prevJCE := n.GetCurrJCE()
			if prevJCE > types.EpochLength {
				n.SetCurrJCE(prevJCE + 1)
			}
		}
	}
}

func (n *Node) runJCEManually() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			prevJCE := n.GetCurrJCE()
			if prevJCE >= types.EpochLength && prevJCE < types.EpochLength*2 {
				n.GenerateTickets(prevJCE)
				n.BroadcastTickets(prevJCE)
			}
		case newJCE := <-n.new_timeslot_chan:
			time.Sleep(1500 * time.Millisecond)
			n.SetCurrJCE(newJCE)
		}
	}
}

func (n *Node) ValidateJCE(receivedJCE uint32) bool {
	if n.jceMode != JCEDefault {
		// Accept any JCE in manual/non-default mode
		return true
	}
	// Only validate JCE in default mode where JCE is determinisitically computed via unixTimestamp
	realJCE := common.ComputeTimeUnit(types.TimeUnitMode)
	jceDiff := int32(realJCE) - int32(receivedJCE)

	if jceDiff > 0 && realJCE-receivedJCE >= UP0LowerBound {
		// receivedJCE is more than UP0LowerBound slot behind. ignore
		log.Warn(log.Node, "ValidateJCE Failed: received block is unreasonanly far behind", "currJCE", realJCE, "receivedJCE", receivedJCE, "diff", jceDiff)
		return false
	}
	if jceDiff < 0 && receivedJCE-realJCE >= UP0UpperBound {
		// receivedJCE is more than UP0UpperBound slot ahead. ignore
		log.Warn(log.Node, "ValidateJCE Failed: received block is unreasonanly far ahead", "currJCE", realJCE, "receivedJCE", receivedJCE, "diff", jceDiff)
		return false
	}
	return true
}

func (n *Node) GetCurrJCE() uint32 {
	n.currJCEMutex.Lock()
	defer n.currJCEMutex.Unlock()
	return n.currJCE
}

func (n *Node) SetCurrJCE(currJCE uint32) {
	n.currJCEMutex.Lock()
	defer n.currJCEMutex.Unlock()
	prevJCE := n.currJCE
	if prevJCE > currJCE {
		log.Error(log.Node, "Invalid JCE: currJCE is less than previous JCE", "prevJCE", prevJCE, "currJCE", currJCE)
		return
	}
	//fmt.Printf("Node %d: Update CurrJCE %d\n", n.id, currJCE)
	n.currJCE = currJCE
	if prevJCE == currJCE {
		return // set only once
	}
	n.jce_timestamp_mutex.Lock()
	if n.jce_timestamp == nil {
		n.jce_timestamp = make(map[uint32]time.Time)
	}
	// for non-normal-jce case
	if n.epoch0Timestamp != 0 {
		n.jce_timestamp[currJCE] = time.Now()
	} else { // for normal-jce case
		n.jce_timestamp[currJCE] = time.Unix(int64(n.GetSlotTimestamp(currJCE)), 0)
	}
	n.jce_timestamp_mutex.Unlock()

	if n.sendTickets {
		stateslot := n.statedb.GetSafrole().Timeslot
		currEpoch, _ := n.statedb.GetSafrole().EpochAndPhase(stateslot)
		_, realPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)

		if currEpoch > 0 && (realPhase == 5) { // } || realPhase == types.EpochLength) {
			// nextEpochFirst-endPhase <= currJCE <= nextEpochFirst
			n.GenerateTickets(stateslot)
			n.BroadcastTickets(stateslot)
		}
	}

}

func (n *Node) SetCompletedJCE(completedCurrJCE uint32) {
	n.completedJCEMutex.Lock()
	defer n.completedJCEMutex.Unlock()
	prevCompletedJCE := n.completedJCE
	if (prevCompletedJCE > 0) && (prevCompletedJCE > completedCurrJCE) && n.jceMode != JCEDefault {
		log.Error(log.Node, "Invalid JCE: currJCE is less than previous JCE", "prevCompletedJCE", prevCompletedJCE, "completedCurrJCE", completedCurrJCE)
		return
	}
	//fmt.Printf("Node %d: Update completed JCE %d\n", n.id, completedCurrJCE)
	n.completedJCE = completedCurrJCE
}

func (n *Node) GetCompletedJCE() uint32 {
	n.completedJCEMutex.Lock()
	defer n.completedJCEMutex.Unlock()
	return n.completedJCE
}

func (nc *NodeContent) SendNewJCE(targetJCE uint32) error {
	if nc.new_timeslot_chan == nil {
		return fmt.Errorf("new_timeslot_chan is nil, cannot send new timeslot")
	}
	nc.new_timeslot_chan <- targetJCE
	return nil
}

func runWithTimeout[T any](f func() (T, error), timeout time.Duration) (T, error) {
	var zero T
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultCh := make(chan struct {
		val T
		err error
	})

	go func() {
		val, err := f()
		select {
		case resultCh <- struct {
			val T
			err error
		}{val, err}:
		case <-ctx.Done():
			// timeout already happened; abandon sending
		}
	}()

	select {
	case result := <-resultCh:
		return result.val, result.err
	case <-ctx.Done():
		return zero, fmt.Errorf("timeout after %s", timeout)
	}
}

func (n *Node) runAuthoring() {
	tickerPulse := time.NewTicker(TickTime * time.Millisecond)
	defer tickerPulse.Stop()

	logChan := n.store.GetChan()
	n.statedb.GetSafrole().EpochFirstSlot = uint32(n.epoch0Timestamp / types.SecondsPerSlot)
	lastAuthorizableJCE := uint32(0)

	for {
		select {
		case <-tickerPulse.C:

			if !n.GetIsSync() {
				n.author_status = "not sync"
				continue
			}
			if n.GetLatestBlockInfo() != nil && n.statedb.HeaderHash != n.GetLatestBlockInfo().HeaderHash {
				log.Debug(log.Node, "runAuthoring: HeaderHash not equal", "n", n.String(), "statedb.HeaderHash", n.statedb.HeaderHash.String_short(), "latestBlockInfo.HeaderHash", n.GetLatestBlockInfo().HeaderHash.String_short())
				continue
			}

			currJCE := n.GetCurrJCE()

			_, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
			epochChange := n.statedb.GetSafrole().EpochChanged(currJCE)
			ticketIDs, _ := n.GetSelfTicketsIDs(currPhase, epochChange)
			if currJCE == lastAuthorizableJCE {
				n.author_status = "authorized"
				continue
			}
			lastAuthorizableJCE = currJCE
			if currJCE%types.EpochLength == 1 {
				slotMap := n.statedb.GetSafrole().GetGonnaAuthorSlot(currJCE, n.credential.BandersnatchPub.Hash(), ticketIDs)
				slotMapjson, err := json.Marshal(slotMap)
				if err != nil {
					log.Error(log.Node, "runAuthoring: Marshal error", "err", err)
				}
				log.Debug(log.B, "runAuthoring: Gonna Author Slot", "n", n.String(), "slotMap", string(slotMapjson))
			}
			makeblock_start := time.Now()

			type processResult struct {
				isAuthorized bool
				newBlock     *types.Block
				newStateDB   *statedb.StateDB
			}
			result, err := runWithTimeout(func() (processResult, error) {
				n.statedbMutex.Lock()
				defer n.statedbMutex.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), MediumTimeout)
				defer cancel()
				stProcessState := time.Now()
				isAuthorized, newBlock, newStateDB, err := n.statedb.ProcessState(ctx, currJCE, n.credential, ticketIDs, n.extrinsic_pool, n.pvmBackend)
				if err != nil {
					log.Error(log.Node, "ProcessState", "err", err)
					return processResult{}, err
				}

				elapsed := time.Since(stProcessState)
				if elapsed > time.Second {
					log.Info(log.Node, "ProcessState", "isAuthorized", isAuthorized, "elapsed", elapsed)
				}
				return processResult{isAuthorized, newBlock, newStateDB}, nil
			}, MediumTimeout)

			if err != nil {
				log.Error(log.Node, "runAuthoring: ProcessState error", "n", n.String(), "err", err)
				continue
			}

			isAuthorizedBlockBuilder := result.isAuthorized
			newBlock := result.newBlock
			newStateDB := result.newStateDB

			if !isAuthorizedBlockBuilder {
				log.Trace(log.B, "runAuthoring: Not Authorized", "n", n.String(), "JCE", currJCE)
				n.author_status = "not authoring"
				continue
			}

			log.Debug(log.B, "runAuthoring: Authoring Block", "n", n.String(), "JCE", currJCE)
			n.author_status = "authoring"
			if newStateDB == nil {
				log.Warn(log.Node, "runAuthoring: ProcessState newStateDB is nil", "n", n.String())
				continue
			}
			oldstate := n.statedb
			newStateDB.SetAncestor(newBlock.Header, oldstate)

			n.addStateDB(newStateDB)
			n.StoreBlock(newBlock, n.id, true)
			n.processBlock(newBlock)

			n.authorTelemetry(newBlock, newStateDB, common.Elapsed(makeblock_start))
			n.SetLatestBlockInfo(
				&JAMSNP_BlockInfo{
					HeaderHash: newBlock.Header.Hash(),
					Slot:       newBlock.Header.Slot,
				},
				"runAuthoring:ProcessState",
			)
			n.author_status = "authoring:broadcasting"
			nodee, ok := n.block_tree.GetBlockNode(newBlock.Header.Hash())
			if !ok {
				log.Warn(log.Node, "runAuthoring: GetBlockNode not found", "hash", newBlock.Header.Hash().String_short(), "n", n.String())
				continue
			}
			nodee.Applied = true
			np_blockAnnouncement, err := n.GetJAMSNPBlockAnnouncementFromHeader(newBlock.Header)
			if err != nil {
				log.Error(log.Node, "runAuthoring: GetJAMSNPBlockAnnouncementFromHeader", "err", err)
				continue
			}
			n.broadcast(context.Background(), np_blockAnnouncement)
			log.Debug(log.Node, "runAuthoring: broadcast", "n", n.String(), "slot", newBlock.Header.Slot)
			go func() {
				timeslot := newStateDB.GetSafrole().Timeslot
				s := n.statedb
				allStates := s.GetAllKeyValues()
				ok, err := s.CompareStateRoot(allStates, newBlock.Header.ParentStateRoot)
				if !ok || err != nil {
					log.Crit(log.Node, "CompareStateRoot", "err", err)
				}
				st := buildStateTransitionStruct(oldstate, newBlock, newStateDB)
				if isWriteTransition {
					if err := n.writeDebug(st, timeslot, true); err != nil {
						log.Error(log.Node, "runAuthoring:writeDebug", "err", err)
					}
				}
				if revalidate {
					if err := statedb.CheckStateTransition(n.store, st, s.AncestorSet, n.pvmBackend); err != nil {
						log.Crit(log.Node, "runAuthoring:CheckStateTransition", "err", err)
						panic(fmt.Sprintf("CheckStateTransition failed: %v", err))
					} else {
						log.Info(log.Node, "runAuthoring:CheckStateTransition", "revalidate", revalidate, "status", "ok")
					}
				}
				if isWriteSnapshot {
					if err := n.writeDebug(newStateDB.JamState.Snapshot(&(st.PostState), newStateDB.GetStateUpdates()), timeslot, true); err != nil {
						log.Error(log.Node, "runAuthoring:writeDebug", "err", err)
					}
				}
			}()
			n.author_status = "authoring:broadcasted"
			assureCtx, cancelAssure := context.WithTimeout(context.Background(), NormalTimeout)
			go func() {
				defer cancelAssure()
				n.assureNewBlock(assureCtx, newBlock, newStateDB)
			}()

			n.extrinsic_pool.ForgetPreimages(newStateDB.GetForgets())

			log.Debug(log.Node, "runAuthoring:ProcessState Proposer !!!!", "n", n.String(), "slot", newBlock.Header.Slot)
			n.SetCompletedJCE(newBlock.Header.Slot)

			if n.AuditFlag {
				select {
				case n.auditingCh <- newStateDB.Copy():
				default:
					log.Warn(log.Node, "auditingCh full, dropping state")
				}
			}
			n.author_status = "authorizing:finished"
			IsClosed := n.statedb.GetSafrole().IsTicketSubmissionClosed(n.statedb.GetTimeslot())
			n.extrinsic_pool.RemoveUsedExtrinsicFromPool(newBlock, newStateDB.GetSafrole().Entropy[2], IsClosed)
		case log := <-logChan:
			go n.WriteLog(log, true)
		}
	}
}

func (n *Node) Telemetry(msgType uint8, obj interface{}, tags ...interface{}) {
	if false {
		sender_id := n.GetEd25519Key().String()
		log.Telemetry(msgType, sender_id, obj, tags...)

	}
}

func (n *Node) authorTelemetry(b *types.Block, newStateDB *statedb.StateDB, elapsed uint32) {
	n.Telemetry(log.MsgTypeBlock, b, "elapsed", elapsed, "codec_encoded", types.EncodeAsHex(b))
	for _, p := range b.Extrinsic.Preimages {
		n.Telemetry(log.MsgTypePreimage, p, "metadata", fmt.Sprintf("blob_len=%v|h=%v|s=%v", len(p.Blob), p.Hash(), p.Requester))
	}
	n.Telemetry(log.MsgTypeStatistics, newStateDB.JamState.ValidatorStatistics, "codec_encoded", types.EncodeAsHex(newStateDB.JamState.ValidatorStatistics))
	serviceUpdates := newStateDB.GetStateUpdates().GetServiceUpdates()
	for _, upd := range serviceUpdates {
		if upd != nil && upd.ServiceInfo != nil {
			if upd.ServiceInfo.Info.NewAccount {
				info := upd.ServiceInfo.Info
				n.Telemetry(log.MsgTypeNewService, upd.ServiceInfo.Info, "codec_encoded", types.EncodeAsHex(info))
			}
		}
	}
}

func BuildStateTransitionStruct(oldStateDB *statedb.StateDB, newBlock *types.Block, newStateDB *statedb.StateDB) *statedb.StateTransition {
	return buildStateTransitionStruct(oldStateDB, newBlock, newStateDB)
}

func buildStateTransitionStruct(oldStateDB *statedb.StateDB, newBlock *types.Block, newStateDB *statedb.StateDB) *statedb.StateTransition {

	st := statedb.StateTransition{
		PreState: statedb.StateSnapshotRaw{
			KeyVals:   oldStateDB.GetAllKeyValues(),
			StateRoot: oldStateDB.StateRoot,
		},
		Block: *newBlock,
		PostState: statedb.StateSnapshotRaw{
			KeyVals:   newStateDB.GetAllKeyValues(),
			StateRoot: newStateDB.StateRoot,
		},
	}

	return &st
}

func GenerateValidatorNetwork() (validators []types.Validator, secrets []types.ValidatorSecret, err error) {
	validators, secrets, err = generateValidatorNetwork()
	return validators, secrets, err
}

func generateValidatorNetwork() (validators []types.Validator, secrets []types.ValidatorSecret, err error) {
	return statedb.GenerateValidatorSecretSet(numNodes)
}
