package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/rpc"

	"sync/atomic"

	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	rand0 "math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/trie"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/qlog"
)

const (
	// immediate-term: bundle=WorkPackage.Bytes(); short-term: bundle=WorkPackageBundle.Bytes() without justification; medium-term= same with proofs; long-term: push method
	module       = "n_mod"   // General Node Ops
	debugDA      = "da_mod"  // DA
	debugG       = "g_mod"   // Guaranteeing
	debugT       = "t_mod"   // Tickets/Safrole
	debugP       = "p_mod"   // Preimages
	debugA       = "a_mod"   // Assurances
	debugAudit   = "ad_mod"  // Audit
	debugGrandpa = "gp_mod"  // Guaranteeing
	debugBlock   = "blk_mod" // Block
	debugStream  = "q_mod"
	numNodes     = types.TotalValidators
	quicAddr     = "127.0.0.1:%d"
	godMode      = false
	Grandpa      = false
	Audit        = false
	revalidate   = false // turn off for production (or publication of traces)

	paranoidVerification = true  // turn off for production
	writeJAMPNTestVector = false // turn on true when generating JAMNP test vectors only
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

// var bootstrap_auth_codehash = common.Hash(common.FromHex("0x397c392ad076df2b8f9e1522cb3554178e41200ba389a4fa4aab141a560202a2"))

var test_prereq = false               // Test Prerequisites Enabled
var pvm_authoring_log_enabled = false // PVM Authoring Log Enabled
const (
	ValidatorFlag   = "VALIDATOR"
	ValidatorDAFlag = "VALIDATOR&DA"
)

type NodeContent struct {
	id                   uint16
	node_type            string
	node_name            string
	command_chan         chan string
	peersInfo            map[uint16]*Peer                 //<validatorIndex> -> Peer
	UP0_HandshakeChan    map[uint16]chan JAMSNP_Handshake //<validatorIndex> -> chan
	UP0_HandshakeMu      sync.Mutex
	UP0_stream           map[uint16]quic.Stream //<validatorIndex> -> stream (self initiated)
	UP0_streamMu         sync.Mutex
	blockAnnouncementsCh chan JAMSNP_BlockAnnounce

	server quic.Listener

	epoch0Timestamp uint32
	// Jamweb
	hub       *Hub
	tlsConfig *tls.Config
	store     *storage.StateDBStorage
	// holds a map of the hash to the stateDB
	statedbMap      map[common.Hash]*statedb.StateDB
	statedbMapMutex sync.Mutex
	// holds a map of the parenthash to the block
	blocks      map[common.Hash]*types.Block
	blocksMutex sync.Mutex
	// holds the tip
	statedb      *statedb.StateDB
	statedbMutex sync.Mutex
	headers      map[common.Hash]*types.Block
	headersMutex sync.Mutex
	// Track the number of opened streams
	openedStreamsMu   sync.Mutex
	openedStreams     map[quic.Stream]struct{}
	dataHashStreamsMu sync.Mutex
	dataHashStreams   map[common.Hash][]quic.Stream

	workReports      map[common.Hash]types.WorkReport
	workReportsMutex sync.Mutex
	workReportsCh    chan types.WorkReport
	workPackagesCh   chan types.WorkPackage

	preimages      map[common.Hash][]byte // preimageLookup -> preimageBlob
	preimagesMutex sync.Mutex

	workPackageQueue sync.Map

	chunkMap            sync.Map
	chunkBox            map[common.Hash][][]byte
	loaded_services_dir string
	block_tree          *types.BlockTree
	nodeSelf            *Node

	RPC_Client        []*rpc.Client
	new_timeslot_chan chan uint32
}

func NewNodeContent(id uint16, store *storage.StateDBStorage) NodeContent {
	return NodeContent{
		id:                   id,
		store:                store,
		command_chan:         make(chan string, 2000), // temporary
		peersInfo:            make(map[uint16]*Peer),
		UP0_stream:           make(map[uint16]quic.Stream),
		statedbMap:           make(map[common.Hash]*statedb.StateDB),
		dataHashStreams:      make(map[common.Hash][]quic.Stream),
		blockAnnouncementsCh: make(chan JAMSNP_BlockAnnounce, 2000),
		blocks:               make(map[common.Hash]*types.Block),
		headers:              make(map[common.Hash]*types.Block),
		workPackagesCh:       make(chan types.WorkPackage, 2000),
		workReportsCh:        make(chan types.WorkReport, 2000),
		preimages:            make(map[common.Hash][]byte),
		workPackageQueue:     sync.Map{},
		new_timeslot_chan:    make(chan uint32, 1),
	}
}

type Node struct {
	NodeContent

	AuditNodeType  string
	credential     types.ValidatorSecret
	peers          []string
	extrinsic_pool *types.ExtrinsicPool

	grandpa *grandpa.Grandpa
	// holds a map of epoch (use entropy to control it) to at most 2 tickets
	selfTickets  map[common.Hash][]types.TicketBucket
	ticketsMutex sync.Mutex
	sendTickets  bool // when mode=fallback this is false, otherwise is true

	// this is for audit

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

	guaranteesCh            chan types.Guarantee
	assurancesCh            chan types.Assurance
	preimageAnnouncementsCh chan types.PreimageAnnouncement
	announcementsCh         chan types.Announcement
	judgementsCh            chan types.Judgement
	auditingCh              chan *statedb.StateDB // use this to trigger auditing, block hash

	// grandpa input channels
	grandpaPreVoteMessageCh   chan grandpa.VoteMessage
	grandpaPreCommitMessageCh chan grandpa.VoteMessage
	grandpaPrimaryMessageCh   chan grandpa.VoteMessage
	grandpaCommitMessageCh    chan grandpa.CommitMessage

	waitingAnnouncements      []types.Announcement
	waitingAnnouncementsMutex sync.Mutex
	waitingJudgements         []types.Judgement
	waitingJudgementsMutex    sync.Mutex

	nodeType string
	dataDir  string

	// DA testing only
	lastHash       common.Hash
	currentHash    common.Hash
	announcement   bool
	announcementMu sync.RWMutex

	// DA Debugging
	totalConnections     int64
	totalIncomingStreams int64
	connectedPeers       map[uint16]bool

	// JamBlocks testing only
	JAMBlocksEndpoint string
	JAMBlocksPort     uint16

	// god mode
	godCh        *chan uint32
	timeslotUsed map[uint32]bool

	// JCE
	currJCE       uint32
	jce_timestamp map[uint32]time.Time
	currJCEMutex  sync.Mutex
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

func generateSelfSignedCert(ed25519_pub ed25519.PublicKey, ed25519_priv ed25519.PrivateKey) (tls.Certificate, error) {
	// Create a self-signed certificate
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Example Org"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, ed25519_pub, ed25519_priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// PEM encode the certificate
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// Convert a generated ed25519 key into a PEM block
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(ed25519_priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	// PEM encode the private key
	privKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKeyBytes})

	// Load the certificate
	return tls.X509KeyPair(certPEM, privKeyPEM)
}

func (n *NodeContent) String() string {
	return fmt.Sprintf("[N%d]", n.id)
}

func (n *Node) setValidatorCredential(credential types.ValidatorSecret) {
	n.credential = credential
	if false {
		jsonData, err := types.Encode(credential)
		if err != nil {
			log.Crit(module, "setValidatorCredential", "err", err)
		}
		log.Info(module, "[N%v] credential %s\n", n.id, jsonData)
	}
}

func (n *Node) setGodCh(c *chan uint32) {
	n.godCh = c
}

func (n *Node) sendGodTimeslotUsed(timeslot uint32) {
	if n.godCh == nil {
		return
	}

	*n.godCh <- timeslot
}

func (n *Node) receiveGodTimeslotUsed(timeslot uint32) {
	if n.godCh == nil {
		return
	}
	n.timeslotUsed[timeslot] = true
}

func (n *Node) checkGodTimeslotUsed(timeslot uint32) bool {
	if n.godCh == nil {
		return false
	}

	_, ok := n.timeslotUsed[timeslot]
	return ok
}

func loadStateSnapshot(filePath string) (statedb.StateSnapshotRaw, error) {
	snapshotRawBytes, err := os.ReadFile(filePath)
	if err != nil {
		return statedb.StateSnapshotRaw{}, fmt.Errorf("error reading JSON file %s: %v", filePath, err)
	}

	var stateSnapshotRaw statedb.StateSnapshotRaw
	err = json.Unmarshal(snapshotRawBytes, &stateSnapshotRaw)
	if err != nil {
		return statedb.StateSnapshotRaw{}, fmt.Errorf("error unmarshaling JSON file %s: %v", filePath, err)
	}

	return stateSnapshotRaw, nil
}

func GetGenesisFile(network string) (string, string) {
	return fmt.Sprintf("/chainspecs/traces/genesis-%s.json", network), fmt.Sprintf("/chainspecs/blocks/genesis-%s.bin", network)
}

func createNode(id uint16, credential types.ValidatorSecret, genesisStateFile string, genesisBlockFile string, epoch0Timestamp uint32, peers []string, peerList map[uint16]*Peer, dataDir string, port int, flag string) (*Node, error) {
	return newNode(id, credential, genesisStateFile, genesisBlockFile, epoch0Timestamp, peers, peerList, flag, dataDir, port)
}

func NewNode(id uint16, credential types.ValidatorSecret, genesisStateFile string, genesisBlockFile string, epoch0Timestamp uint32, peers []string, peerList map[uint16]*Peer, dataDir string, port int) (*Node, error) {
	return createNode(id, credential, genesisStateFile, genesisBlockFile, epoch0Timestamp, peers, peerList, dataDir, port, ValidatorFlag)
}

func NewNodeDA(id uint16, credential types.ValidatorSecret, genesisStateFile string, genesisBlockFile string, epoch0Timestamp uint32, peers []string, peerList map[uint16]*Peer, dataDir string, port int) (*Node, error) {
	return createNode(id, credential, genesisStateFile, genesisBlockFile, epoch0Timestamp, peers, peerList, dataDir, port, ValidatorDAFlag)
}

func newNode(id uint16, credential types.ValidatorSecret, genesisStateFile string, genesisBlockFile string, epoch0Timestamp uint32, peers []string, startPeerList map[uint16]*Peer, nodeType string, dataDir string, port int) (*Node, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	//log.Info(module, fmt.Sprintf("[N%v]", id), "addr", addr, "dataDir", dataDir)

	//REQUIRED FOR CAPTURING JOBID. DO NOT DELETE THIS LINE!!
	fmt.Printf("[N%v] addr=%v, dataDir=%v\n", id, addr, dataDir)

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
		return nil, fmt.Errorf("Error generating self-signed certificate: %v", err)
	}
	node := &Node{
		NodeContent: NewNodeContent(id, store),
		peers:       peers,
		clients:     make(map[string]string),
		nodeType:    nodeType,

		extrinsic_pool: types.NewExtrinsicPool(),

		auditingMap:     make(map[common.Hash]*statedb.StateDB),
		announcementMap: make(map[common.Hash]*types.TrancheAnnouncement),
		judgementMap:    make(map[common.Hash]*types.JudgeBucket),
		judgementWRMap:  make(map[common.Hash]common.Hash),

		selfTickets:      make(map[common.Hash][]types.TicketBucket),
		assurancesBucket: make(map[common.Hash]bool),
		delaysend:        make(map[common.Hash]int),

		ticketsCh:               make(chan types.Ticket, 200),
		guaranteesCh:            make(chan types.Guarantee, 200),
		assurancesCh:            make(chan types.Assurance, 200),
		preimageAnnouncementsCh: make(chan types.PreimageAnnouncement, 200),
		announcementsCh:         make(chan types.Announcement, 200),
		judgementsCh:            make(chan types.Judgement, 200),
		auditingCh:              make(chan *statedb.StateDB, 200),

		grandpaPreVoteMessageCh:   make(chan grandpa.VoteMessage, 200),
		grandpaPreCommitMessageCh: make(chan grandpa.VoteMessage, 200),
		grandpaPrimaryMessageCh:   make(chan grandpa.VoteMessage, 200),
		grandpaCommitMessageCh:    make(chan grandpa.CommitMessage, 200),

		sendTickets: true,

		timeslotUsed: make(map[uint32]bool),
		godCh:        nil,

		dataDir: dataDir,

		connectedPeers: make(map[uint16]bool),
	}
	node.node_name = fmt.Sprintf("jam-%d", id)
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		ClientAuth:         tls.RequireAnyClientCert,
		InsecureSkipVerify: true,
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			return &tls.Config{
				Certificates:       []tls.Certificate{cert},
				ClientAuth:         tls.RequireAnyClientCert,
				InsecureSkipVerify: true,
				VerifyPeerCertificate: func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
					if len(rawCerts) == 0 {
						return fmt.Errorf("no client certificate provided")
					}
					cert, err := x509.ParseCertificate(rawCerts[0])
					if err != nil {
						return fmt.Errorf("failed to parse client certificate: %v", err)
					}
					pubKey, ok := cert.PublicKey.(ed25519.PublicKey)
					if !ok {
						return fmt.Errorf("client public key is not Ed25519")
					}
					node.clientsMutex.Lock()
					node.clients[info.Conn.RemoteAddr().String()] = hex.EncodeToString(pubKey)
					node.clientsMutex.Unlock()
					return nil
				},
				NextProtos: []string{"h3", "http/1.1", "ping/1.1"},
			}, nil
		},
		NextProtos: []string{"h3", "http/1.1", "ping/1.1"},
	}
	node.tlsConfig = tlsConfig

	listener, err := quic.ListenAddr(addr, tlsConfig, GenerateQuicConfig())
	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}
	node.server = *listener

	for validatorIndex, p := range startPeerList {
		node.peersInfo[validatorIndex] = NewPeer(node, validatorIndex, p.Validator, p.PeerAddr)
	}

	block := statedb.NewBlockFromFile(genesisBlockFile)
	_statedb, err := statedb.NewStateDBFromSnapshotRawFile(node.store, genesisStateFile)
	_statedb.Block = block
	_statedb.HeaderHash = block.Header.Hash()
	if err == nil {
		_statedb.SetID(uint16(id))
		node.addStateDB(_statedb)
		node.StoreBlock(block, id, false)
	} else {
		fmt.Printf("NewGenesisStateDB ERR %v\n", err)
		return nil, err
	}
	node.setValidatorCredential(credential)
	node.epoch0Timestamp = epoch0Timestamp
	if nodeType == ValidatorDAFlag {
		validators, _, err := generateValidatorNetwork()
		if err != nil {
			return nil, err
		}
		node.statedb.GetSafrole().NextValidators = validators
		node.statedb.GetSafrole().CurrValidators = validators
	}

	validators := node.statedb.GetSafrole().NextValidators
	if len(validators) == 0 {
		panic("newNode No validators")
	}
	go node.runServer()
	if nodeType != ValidatorDAFlag {
		go node.runJCE()
		//go node.runFasterJCE()
		// go node.runJCEManually()
		go node.runClient()
		go node.runMain()
		go node.runPreimages()
		go node.runBlocksTickets()
		go node.runReceiveBlock()
		go node.StartRPCServer(port)
		go node.runWPQueue()
		if Audit {
			go node.runAudit() // disable this to pause FetchWorkPackageBundle, if we disable this grandpa will not work
		}
		host_name, _ := os.Hostname()
		if id == 0 || host_name[:4] == "jam-" {
			go node.runJamWeb(uint16(port+1000)+id, port)
		}

	}
	return node, nil
}

func GenerateQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:                  true,
		KeepAlivePeriod:            1 * time.Second,
		MaxIdleTimeout:             60 * time.Second,
		MaxIncomingUniStreams:      2 * 1023 * 1023,
		MaxIncomingStreams:         2 * 1023 * 1023,
		MaxStreamReceiveWindow:     100 * 1024 * 1024,
		MaxConnectionReceiveWindow: 500 * 1024 * 1024,
		Tracer:                     qlog.DefaultConnectionTracer,
	}
}

func getConnKey(identifier string, incoming bool) string {
	if incoming {
		return fmt.Sprintf("%v-in", identifier)
	} else {
		return fmt.Sprintf("%v-out", identifier)
	}
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

func (n *Node) GetCoreIndexFromEd25519Key(key types.Ed25519Key) (uint16, error) {
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

// func (n *Node) GetCoreWorkersPeers(core uint16)(workers []Peer) {
// 	workers = make([]Peer, 0)
// 	for _, assignment := range n.statedb.GuarantorAssignments {
// 		if assignment.CoreIndex == core {
// 			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
// 			if err == nil {
// 				workers = append(workers, *peer.Clone())
// 			}
// 		}
// 	}
// 	return workers
// }

// this function will return the core workers of that core
func (n *NodeContent) GetCoreCoWorkersPeers(core uint16) (coWorkers []Peer) {
	coWorkers = make([]Peer, 0)
	for _, assignment := range n.statedb.GuarantorAssignments {
		if assignment.CoreIndex == core {
			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
			if err == nil {
				coWorkers = append(coWorkers, *peer.Clone())
			}
		}
	}
	return coWorkers
}

func (n *Node) GetCoreCoWorkerPeersByStateDB(core uint16, using_statedb *statedb.StateDB) (coWorkers []Peer) {
	coWorkers = make([]Peer, 0)
	for _, assignment := range using_statedb.GuarantorAssignments {
		if assignment.CoreIndex == core {
			peer, err := n.GetPeerInfoByEd25519(assignment.Validator.Ed25519)
			if err == nil {
				coWorkers = append(coWorkers, *peer.Clone())
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

func (n *Node) getTrie() *trie.MerkleTree {
	s := n.getState()
	return s.GetTrie()
}

func (n *NodeContent) getPeerByIndex(peerIdx uint16) (*Peer, error) {
	p := n.peersInfo[peerIdx]
	// check if peer exists
	if p != nil {
		return p, nil
	}
	return nil, fmt.Errorf("peer %v not found", peerIdx)
}

func (n *Node) getPeerAddr(peerIdx uint16) (*Peer, error) {
	peer, exist := n.peersInfo[peerIdx]
	if exist {
		return peer, nil
	}
	fmt.Printf("getPeerAddr not found %v\n", peerIdx)
	return nil, fmt.Errorf("peer not found")
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
		return nil
	}
	if _statedb.GetBlock() == nil {
		fmt.Printf("addStateDB: NO BLOCK!!! %v\n", _statedb)
		panic(0)
	}
	if n.statedb.GetBlock() == nil {
		fmt.Printf("node statedb: NO BLOCK!!! %v\n", n.statedb)
		panic(0)
	}
	if _statedb.GetBlock().TimeSlot() > n.statedb.GetBlock().TimeSlot() {
		n.statedb = _statedb
		n.statedbMap[_statedb.GetHeaderHash()] = _statedb
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
	remoteAddr := conn.RemoteAddr().String()

	n.clientsMutex.Lock()
	pubKey, ok := n.clients[remoteAddr]
	n.clientsMutex.Unlock()

	if !ok {
		fmt.Printf("handleConnection: UNKNOWN remoteAddr=%s unknown\n", remoteAddr)
		return
	}

	validatorIndex, ok := n.lookupPubKey(pubKey)
	if !ok {
		fmt.Printf("handleConnection: Non-Validator pubkey %s from remoteAddr=%s\n", pubKey, remoteAddr)
		validatorIndex = 9999
		// remoteAddr change the port to 13000
		// see how many number from the end
		host, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			panic(err)
		}
		newAddr := net.JoinHostPort(host, "13370")
		fmt.Printf("change port to %s\n", newAddr)
		if _, ok := n.peersInfo[validatorIndex]; !ok {
			n.peersInfo[validatorIndex] = NewPeer(n, uint16(validatorIndex), types.Validator{}, newAddr)
		} else {
			for {
				validatorIndex++
				if _, ok := n.peersInfo[validatorIndex]; !ok {
					n.peersInfo[validatorIndex] = NewPeer(n, uint16(validatorIndex), types.Validator{}, newAddr)
					break
				}
			}
		}
	}

	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Error(debugDA, "AcceptStream", "n", n.id, "validatorIndex", validatorIndex, "err", err)
			break
		}
		atomic.AddInt64(&n.totalIncomingStreams, 1)
		go func() {
			defer func() {
				atomic.AddInt64(&n.totalIncomingStreams, -1)
			}()
			n.DispatchIncomingQUICStream(stream, validatorIndex)
		}()
	}
}

func (n *Node) broadcast(obj interface{}) []byte {
	result := []byte{}
	objType := reflect.TypeOf(obj)
	var wg sync.WaitGroup
	for id, p := range n.peersInfo {

		if id > types.TotalValidators {
			switch objType {
			case reflect.TypeOf(types.Block{}):
				b := obj.(types.Block)
				up0_stream, err := p.GetOrInitBlockAnnouncementStream()
				if err != nil {
					log.Error(debugStream, "GetOrInitBlockAnnouncementStream", "n", n.String(), "err", err)
				}
				block_a_bytes, err := n.GetBlockAnnouncementBytes(b)
				if err != nil {
					log.Error(debugStream, "GetBlockAnnouncementBytes", "n", n.String(), "err", err)
				}
				err = sendQuicBytes(up0_stream, block_a_bytes)
				if err != nil {
					if id > types.TotalValidators {
						n.UP0_streamMu.Lock()
						delete(n.UP0_stream, id)
						n.UP0_streamMu.Unlock()
						delete(n.peersInfo, id)
					}
					log.Error(debugStream, "SendBlockAnnouncement:sendQuicBytes", "n", n.String(), "err", err)
				}
			}
			continue
		}

		if id == n.id {
			if objType == reflect.TypeOf(types.Assurance{}) {
				a := obj.(types.Assurance)
				n.assurancesCh <- a
				continue
			} else if objType == reflect.TypeOf(types.PreimageAnnouncement{}) {
				continue
			} else if objType == reflect.TypeOf(grandpa.VoteMessage{}) || objType == reflect.TypeOf(grandpa.CommitMessage{}) {
				continue
			} else {
				continue
			}
		}

		wg.Add(1)
		go func(id uint16, p *Peer) {
			defer wg.Done()
			switch objType {
			case reflect.TypeOf(types.Ticket{}):
				t := obj.(types.Ticket)
				epoch := uint32(0) // TODO: Shawn
				err := p.SendTicketDistribution(epoch, t, false)
				if err != nil {
					log.Error(debugStream, "SendTicketDistribution", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(types.Block{}):
				b := obj.(types.Block)
				up0_stream, err := p.GetOrInitBlockAnnouncementStream()
				if err != nil {
					log.Error(debugStream, "GetOrInitBlockAnnouncementStream", "n", n.String(), "err", err)
				}
				block_a_bytes, err := n.GetBlockAnnouncementBytes(b)
				if err != nil {
					log.Error(debugStream, "GetBlockAnnouncementBytes", "n", n.String(), "err", err)
				}
				err = sendQuicBytes(up0_stream, block_a_bytes)
				if err != nil {
					log.Error(debugStream, "SendBlockAnnouncement:sendQuicBytes", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(types.Guarantee{}):
				g := obj.(types.Guarantee)
				err := p.SendWorkReportDistribution(g.Report, g.Slot, g.Signatures)
				if err != nil {
					log.Error(debugStream, "SendWorkReportDistribution", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(types.Assurance{}):
				a := obj.(types.Assurance)
				err := p.SendAssurance(&a)
				if err != nil {
					log.Error(debugStream, "SendAssurance", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(JAMSNPAuditAnnouncementWithProof{}):
				a := obj.(JAMSNPAuditAnnouncementWithProof)
				err := p.SendAuditAnnouncement(&a)
				if err != nil {
					log.Error(debugStream, "SendAuditAnnouncement", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(types.Judgement{}):
				j := obj.(types.Judgement)
				epoch := uint32(0) // TODO: Shawn
				err := p.SendJudgmentPublication(epoch, j)
				if err != nil {
					log.Error(debugStream, "SendJudgmentPublication", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(types.PreimageAnnouncement{}):
				preimageAnnouncement := obj.(types.PreimageAnnouncement)
				err := p.SendPreimageAnnouncement(&preimageAnnouncement)
				if err != nil {
					log.Error(debugStream, "SendPreimageAnnouncement", "n", n.String(), "err", err)
				}
			case reflect.TypeOf([]DADistributeECChunk{}):
				distributeECChunks := obj.([]DADistributeECChunk)
				err := p.SendDistributionECChunks(distributeECChunks[id])
				if err != nil {
					log.Error(debugStream, "SendDistributionECChunks", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(grandpa.VoteMessage{}):
				vote := obj.(grandpa.VoteMessage)
				err := p.SendVoteMessage(vote)
				if err != nil {
					log.Error(debugStream, "SendVoteMessage", "n", n.String(), "err", err)
				}
			case reflect.TypeOf(grandpa.CommitMessage{}):
				commit := obj.(grandpa.CommitMessage)
				err := p.SendCommitMessage(commit)
				if err != nil {
					log.Error(debugStream, "SendCommitMessage", "n", n.String(), "err", err)
				}
			}
		}(id, p)
	}

	wg.Wait()
	return result
}

// Helper function to determine if the error is a timeout error
func isTimeoutError(err error) bool {
	// Add more specific error handling as needed
	return strings.Contains(err.Error(), "timeout")
}

func (n *Node) dumpstatedbmap() {
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()

	for hash, statedb := range n.statedbMap {
		fmt.Printf("dumpstatedbmap: statedbMap[%v] => statedb (%v<=parent=%v) StateRoot %v\n", hash, statedb.ParentHeaderHash, statedb.HeaderHash, statedb.StateRoot)
	}

	n.blocksMutex.Lock()
	defer n.blocksMutex.Unlock()
	for hash, blk := range n.blocks {
		fmt.Printf("dumpstatedbmap: blocks[%v] => %v\n", hash, blk.Hash())
	}
}

func (n *NodeContent) getStateDBByHeaderHash(headerHash common.Hash) (statedb *statedb.StateDB, ok bool) {
	n.statedbMapMutex.Lock()
	defer n.statedbMapMutex.Unlock()
	statedb, ok = n.statedbMap[headerHash]
	return statedb, ok
}

func randomKey(m map[string]*Peer) string {
	rand0.Seed(time.Now().UnixNano())
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys[rand0.Intn(len(keys))]
}

func (n *Node) fetchBlocks(headerHash common.Hash, direction uint8, maximumBlocks uint32) (*[]types.Block, error) {
	requests_original := make([]CE128_request, types.TotalValidators)
	for i := range types.TotalValidators {
		requests_original[i] = CE128_request{
			ShardIndex:    uint16(i),
			HeaderHash:    headerHash,
			Direction:     direction,
			MaximumBlocks: maximumBlocks,
		}
	}
	requests := make([]interface{}, types.TotalValidators)
	for i, req := range requests_original {
		requests[i] = req
	}

	resps, err := n.makeRequests(requests, 1, time.Duration(3*maximumBlocks)*time.Second, time.Duration(6*maximumBlocks)*time.Second)
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

func (n *Node) extendChain() error {
	parentheaderhash := n.statedb.HeaderHash
	for {

		ok := false
		for _, b := range n.blocks {
			if b.GetParentHeaderHash() == parentheaderhash {
				// TODO: add span here to capture ApplyStateTransitionFromBlock

				ok = true
				nextBlock := b

				// Apply the block to the tip
				recoveredStateDB := n.statedb.Copy()
				recoveredStateDB.RecoverJamState(nextBlock.Header.ParentStateRoot)
				newStateDB, err := statedb.ApplyStateTransitionFromBlock(recoveredStateDB, context.Background(), nextBlock, "extendChain")
				if err != nil {
					fmt.Printf("[N%d] extendChain FAIL %v\n", n.id, err)
					return err
				}
				newStateDB.GetAllKeyValues()
				n.clearQueueUsingBlock(nextBlock.Extrinsic.Guarantees)
				newStateDB.SetAncestor(nextBlock.Header, recoveredStateDB)

				// current we always dump state transitions for every node
				go func() {
					st := buildStateTransitionStruct(recoveredStateDB, nextBlock, newStateDB)
					err = n.writeDebug(st, nextBlock.TimeSlot()) // StateTransition
					if err != nil {
						log.Error(module, "writeDebug", "err", err)
					}
					err = n.writeDebug(nextBlock, nextBlock.TimeSlot()) // Blocks
					if err != nil {
						log.Error(module, "writeDebug", "err", err)
					}
					err = n.writeDebug(newStateDB.JamState.Snapshot(&st.PostState), nextBlock.TimeSlot()) // StateSnapshot
					if err != nil {
						log.Error(module, "writeDebug", "err", err)
					}
				}()

				// Extend the tip of the chain
				n.addStateDB(newStateDB)
				n.assureNewBlock(b)

				parentheaderhash = nextBlock.Header.Hash()

				n.statedbMapMutex.Lock()
				if Audit {
					n.auditingCh <- n.statedbMap[n.statedb.HeaderHash].Copy()
				}
				n.statedbMapMutex.Unlock()

				IsTicketSubmissionClosed := n.statedb.GetSafrole().IsTicketSubmissionClosed(n.statedb.GetTimeslot())
				n.extrinsic_pool.RemoveUsedExtrinsicFromPool(b, n.statedb.GetSafrole().Entropy[2], IsTicketSubmissionClosed)
				break
			}

		}

		if !ok {
			// If there is no next block, we're done!
			return nil
		}
	}
}

func (n *Node) assureNewBlock(b *types.Block) error {
	if len(b.Extrinsic.Guarantees) > 0 {
		for _, g := range b.Extrinsic.Guarantees {
			err := n.StoreWorkReport(g.Report)
			if err != nil {
				log.Error(debugDA, "assureData:StoreWorkReport", "n", n.String(), "err", err)
			}
			n.assureData(g)
		}
	}

	a, numCores, err := n.generateAssurance(b.Header.Hash())
	if err != nil {
		return err
	}
	if numCores == 0 {
		return nil
	}

	log.Debug(debugA, "assureNewBlock:Broadcasting assurance", "n", n.String(), "bitfield", a.Bitfield)
	go n.broadcast(a)
	return nil
}

// we arrive here when we receive a block from another node
func (n *Node) processBlock(blk *types.Block) error {
	// walk blk backwards, up to the tip, if possible -- but if encountering an unknown parenthash, immediately fetch the block.  Give up if we can't do anything
	b := blk
	n.StoreBlock(blk, n.id, false)
	n.cacheBlock(blk)
	n.cacheHeaders(b.Header.Hash(), blk)
	// Sometimes this loop will get in deadlock
	for {
		if b.GetParentHeaderHash() == (common.Hash{}) {
			break
		} else if n.statedb != nil && b.GetParentHeaderHash() == n.statedb.HeaderHash {
			break
		} else {

			parentBlock, ok := n.cacheBlockRead(b.GetParentHeaderHash())
			if !ok {
				parentBlocks, err := n.fetchBlocks(b.GetParentHeaderHash(), 1, 1)
				if err != nil || parentBlocks == nil {
					// have to give up right now (could try again though!)
					return err
				} else {
					parentBlock = &(*parentBlocks)[0]
				}
				// got the parent block, store it in the cache
				if parentBlock.GetParentHeaderHash() == blk.GetParentHeaderHash() {
					n.StoreBlock(parentBlock, n.id, false)
					n.cacheBlock(parentBlock)
				} else {
					return nil
				}

			}
			b = parentBlock
		}
	}

	// we got to the tip, now extend the chain, moving the tip forward, applying blocks using blockcache
	err := n.extendChain()
	if err != nil {
		return err
	}
	return nil // Success
}

func setupSegmentsShards(segmentLen int) (segmentShards [][][]byte) {
	// setup proper arr for reconstruction
	segmentShards = make([][][]byte, segmentLen)
	for j := 0; j < segmentLen; j++ {
		for shardIdx := uint16(0); shardIdx < types.TotalValidators; shardIdx++ {
			segmentShards[j] = make([][]byte, types.TotalValidators)
		}
	}
	return segmentShards
}

// reconstructSegments uses CE139 and CAN use CE140 upon failure
// We continuily use erasureRoot to ask the question
func (n *NodeContent) reconstructSegments(si *SpecIndex) (segments [][]byte, justifications [][]common.Hash, err error) {
	requests_original := make([]CE139_request, types.TotalValidators)

	// this will track the proofpages
	proofpages := []uint16{}

	// ask for the indices, and tally the proofpages needed to fetch justifications
	allsegmentindices := make([]uint16, len(si.Indices))
	for i, idx := range si.Indices {
		allsegmentindices[i] = idx
		if idx >= si.WorkReport.AvailabilitySpec.ExportedSegmentLength {
			return segments, justifications, fmt.Errorf("requested index %d exceeds availability spec export count %d", idx, si.WorkReport.AvailabilitySpec.ExportedSegmentLength)
		}
		p := idx / 64 // each proofpage is 64 segments
		if !slices.Contains(proofpages, p) {
			proofpages = append(proofpages, p)
		}
	}
	for _, p := range proofpages {
		allsegmentindices = append(allsegmentindices, si.WorkReport.AvailabilitySpec.ExportedSegmentLength+p)
	}
	for i := range types.TotalValidators {
		requests_original[i] = CE139_request{
			ErasureRoot:    si.WorkReport.AvailabilitySpec.ErasureRoot,
			SegmentIndices: allsegmentindices,
			ShardIndex:     uint16(i),
		}
	}
	requests := make([]interface{}, types.TotalValidators)
	for i, req := range requests_original {
		requests[i] = req
	}
	responses, err := n.makeRequests(requests, types.ECPieceSize/2, time.Duration(3)*time.Second, time.Duration(10)*time.Second)
	if err != nil {
		fmt.Printf("Error in fetching import segments By ErasureRoot: %v\n", err)
		return segments, justifications, err
	}

	shards := make([][]byte, types.TotalCores)
	indexes := make([]uint32, types.TotalCores)
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
	fmt.Printf("!!! reconstructSegments: %d segments, %d shards, %d chunkSize\n", len(allsegmentindices), numShards, chunkSize)
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
	pageproofs := allsegments[indicesLen:]
	segmentsonly := allsegments[0:indicesLen]
	justifications = make([][]common.Hash, indicesLen)
	for i, segmentIndex := range si.Indices {
		pageSize := 1 << trie.PageFixedDepth
		pageIdx := int(segmentIndex) / pageSize
		pagedProofByte := pageproofs[pageIdx]
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
			log.Error(debugDA, "cdttree:VerifyCDTJustificationX", "derived_globalRoot_j0", derived_globalRoot_j0)
			return segments, justifications, err
		} else {
			log.Trace(debugDA, "cdttree:VerifyCDTJustificationX Justified", "ExportedSegmentRoot", common.BytesToHash(si.WorkReport.AvailabilitySpec.ExportedSegmentRoot[:]))
		}
		justifications[i] = fullJustification
	}
	if len(segmentsonly) != indicesLen {
		panic(123444)
	}
	fmt.Printf("reconstructSegments: %d segments, %d justifications\n", len(segmentsonly), len(justifications))
	return segmentsonly, justifications, nil
}

// HERE we are in a AUDITING situation, if verification fails, we can still execute the work package by using CE140?
func (n *NodeContent) reconstructPackageBundleSegments(erasureRoot common.Hash, blength uint32, segmentRootLookup types.SegmentRootLookup) (workPackageBundle types.WorkPackageBundle, err error) {
	requests_original := make([]CE138_request, types.TotalValidators)
	for i := range types.TotalValidators {
		requests_original[i] = CE138_request{
			ErasureRoot: erasureRoot,
			ShardIndex:  uint16(i),
		}
	}
	requests := make([]interface{}, types.TotalValidators)
	for i, req := range requests_original {
		requests[i] = req
	}

	responses, err := n.makeRequests(requests, types.ECPieceSize/2, time.Duration(3)*time.Second, time.Duration(10)*time.Second)
	if err != nil {
		fmt.Printf("Error in fetching bundle segments makeRequests: %v\n", err)
		return types.WorkPackageBundle{}, err
	}
	bundleShards := make([][]byte, types.TotalCores)
	indexes := make([]uint32, types.TotalCores)
	numShards := 0
	for _, resp := range responses {
		daResp, ok := resp.(CE138_response)
		if !ok {
			log.Warn(module, "reconstructPackageBundleSegments: Error in convert bundle segments CE138_response", "n", n.id, "len(BundleShard)", len(daResp.BundleShard), "daResp.BundleShard", daResp.BundleShard)
		}
		if numShards < types.TotalCores {
			encodedPath := daResp.Justification
			decodedPath, _ := common.DecodeJustification(encodedPath, types.NumECPiecesPerSegment)
			bClub := common.Blake2Hash(daResp.BundleShard)
			sClub := daResp.SClub
			leaf := append(bClub.Bytes(), sClub.Bytes()...)
			log.Debug(module, "!!!! reconstructPackageBundleSegments: leaf", "leaf", fmt.Sprintf("%x", leaf), "erasureRoot", erasureRoot, "shardIndex", daResp.ShardIndex, "decodedPath", fmt.Sprintf("%x", decodedPath))
			verified, _ := VerifyWBTJustification(types.TotalValidators, erasureRoot, uint16(daResp.ShardIndex), leaf, decodedPath)
			if verified {
				bundleShards[numShards] = daResp.BundleShard
				indexes[numShards] = uint32(daResp.ShardIndex)
				numShards++
			} else {
				log.Crit(module, "reconstructPackageBundleSegments:VerifyWBTJustification", "erasureRoot", erasureRoot, "shardIndex", daResp.ShardIndex, "leaf", fmt.Sprintf("%x", leaf), "decodedPath", fmt.Sprintf("%x", decodedPath))
			}
		}
	}
	encodedBundle, err := bls.Decode(bundleShards, types.TotalValidators, indexes, int(blength))
	if err != nil {
		log.Error(debugDA, "decode: Error in fetching bundle segments decode", "err", err)
	}

	workPackageBundleRaw, _, err := types.Decode(encodedBundle, reflect.TypeOf(types.WorkPackageBundle{}))
	if err != nil {
		log.Error(debugDA, "reconstructPackageBundleSegments:Decode", "err", err)
		return
	}
	workPackageBundle = workPackageBundleRaw.(types.WorkPackageBundle)

	// IMPORTANT: VerifyBundle checks all imported segments against the justifications contained within the bundle, which hash up to the segmentRoot hashes in the segmentRootLookup
	verified, verifyErr := n.VerifyBundle(&workPackageBundle, segmentRootLookup)
	if verifyErr != nil || !verified {
		log.Warn(module, "executeWorkPackageBundle: VerifyBundle failed", "err", verifyErr)
		// TODO: reconstruct the imported segments
	}
	return workPackageBundle, nil
}

func (n *NodeContent) getPVMStateDB() *statedb.StateDB {
	// refine's executeWorkPackage is a statelessish process
	target_statedb := n.statedb.Copy()
	return target_statedb
}

func (n *NodeContent) AddPreimage(preimageByte []byte) (codeHash common.Hash) {
	codeHash = common.Blake2Hash(preimageByte)
	n.nodeSelf.preimagesMutex.Lock()
	defer n.nodeSelf.preimagesMutex.Unlock()
	n.nodeSelf.preimages[codeHash] = preimageByte
	return codeHash
}

// -----Custom methods for tiny QUIC EC experiment-----

func getStructType(obj interface{}) string {
	v := reflect.TypeOf(obj)

	// If the type is a pointer, get the underlying element type
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Get the type name (if available) and the package path
	structType := v.Name()
	pkgPath := v.PkgPath()

	// If there's no name, handle unexported types differently
	if structType == "" {
		structType = fmt.Sprintf("%v", v) // Fallback to full type description
	}

	// Check if there's a package path (means it's an exported type)
	if pkgPath != "" {
		parts := strings.Split(structType, ".")
		structType = strings.ToLower(parts[len(parts)-1])
	} else {
		structType = strings.ToLower(structType)
	}

	fmt.Printf("!!!!getStructType=%v\n", structType)
	return structType
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
	case DA_announcement:
		return "DA_announcement"
	case DA_request:
		return "DA_request"
	case DA_response:
		return "DA_response"
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

func (n *Node) writeDebug(obj interface{}, timeslot uint32) error {
	l := storage.LogMessage{
		Payload:  obj,
		Timeslot: timeslot,
	}
	return n.WriteLog(l)
}

func (n *Node) WriteLog(logMsg storage.LogMessage) error {
	//msgType := getStructType(obj)
	obj := logMsg.Payload
	timeSlot := logMsg.Timeslot
	msgType := getMessageType(obj)
	if msgType != "unknown" {
	}
	dataDir := fmt.Sprintf("%s/data", n.dataDir)
	structDir := fmt.Sprintf("%s/%vs", dataDir, msgType)

	// Check if the directories exist, if not create them
	if _, err := os.Stat(structDir); os.IsNotExist(err) {
		err := os.MkdirAll(structDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating %v directory: %v\n", msgType, err)
		}
	}

	if msgType != "unknown" {
		epoch, phase := statedb.ComputeEpochAndPhase(timeSlot, n.epoch0Timestamp)
		path := fmt.Sprintf("%s/%v_%03d", structDir, epoch, phase)
		if epoch == 0 && phase == 0 {
			path = fmt.Sprintf("%s/genesis", structDir)
		}
		types.SaveObject(path, obj)
	}
	return nil
}

func WriteSTFLog(stf *statedb.StateTransition, timeslot uint32, dataDir string) error {
	dataDir = fmt.Sprintf("%s", dataDir)
	structDir := fmt.Sprintf("%s/%vs", dataDir, "state_transition")

	// Check if the directories exist, if not create them
	if _, err := os.Stat(structDir); os.IsNotExist(err) {
		err := os.MkdirAll(structDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("Error creating %v directory: %v\n", "state_transition", err)
		}
	}

	epoch, phase := statedb.ComputeEpochAndPhase(timeslot, 0)
	path := fmt.Sprintf("%s/%v_%03d", structDir, epoch, phase)
	if epoch == 0 && phase == 0 {
		path = fmt.Sprintf("%s/genesis", structDir)
	}
	types.SaveObject(path, stf)
	return nil
}

func (n *Node) runJCE() {
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
	const tickInterval = 100
	ticker_pulse := time.NewTicker(tickInterval * time.Millisecond)
	defer ticker_pulse.Stop()

	const blockInterval = 2000
	block_pulse := time.NewTicker(blockInterval * time.Millisecond)
	defer block_pulse.Stop()
	for {
		select {
		case <-ticker_pulse.C:
			prevJCE := n.GetCurrJCE()
			currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
			if prevJCE <= types.EpochLength {
				n.SetCurrJCE(currJCE)
				//do something with it
			}
		case <-block_pulse.C:
			prevJCE := n.GetCurrJCE()
			if prevJCE > types.EpochLength {
				currJCE := prevJCE + 1
				n.SetCurrJCE(currJCE)
			}
		}
	}
}

func (n *Node) runJCEManually() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:

		case <-n.new_timeslot_chan:
			n.SetCurrJCE(CurrentSlot)
		}
	}
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
	if (prevJCE >= 0) && (prevJCE >= currJCE) {
		return
	}
	//fmt.Printf("Node %d: Update CurrJCE %d\n", n.id, currJCE)
	n.currJCE = currJCE
	if n.jce_timestamp == nil {
		n.jce_timestamp = make(map[uint32]time.Time)
	}
	n.jce_timestamp[currJCE] = time.Now()
}

func (n *Node) monitorCommandChannel() {
	for {
		time.Sleep(5 * time.Second)
		log.Info(module, "chan monitoring", "command_chan", fmt.Sprintf("Diagnostic: command_chan has %d pending commands", len(n.command_chan)))
	}
}

func (n *Node) runClient() {
	ticker_pulse := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_pulse.Stop()

	logChan := n.store.GetChan()
	n.statedb.GetSafrole().EpochFirstSlot = n.epoch0Timestamp / types.SecondsPerSlot

	//go n.monitorCommandChannel()

	for {
		select {

		case <-ticker_pulse.C:
			if n.GetNodeType() != ValidatorFlag && n.GetNodeType() != ValidatorDAFlag {
				return
			}
			// timeslot mark
			currJCE := n.GetCurrJCE()
			currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
			// bandersnatch is time consuming
			if currEpoch != -1 {
				if currPhase == 0 {
					n.GenerateTickets()
					n.BroadcastTickets()

				} else if currPhase == types.EpochLength-1 { // you had currPhase == types.EpochLength-1
					// nextEpochFirst-endPhase <= currJCE <= nextEpochFirst
					n.GenerateTickets()
					n.BroadcastTickets()
				}
			}
			//if n.checkGodTimeslotUsed(currJCE) {
			//	return
			//}
			ticketIDs, err := n.GetSelfTicketsIDs(currPhase)
			if err != nil {
				fmt.Printf("runClient: GetSelfTicketsIDs error: %v\n", err)
			}
			n.statedbMutex.Lock()
			newBlock, newStateDB, err := n.statedb.ProcessState(currJCE, n.credential, ticketIDs, n.extrinsic_pool)

			n.statedbMutex.Unlock()
			if err != nil {
				fmt.Printf("[N%d] ProcessState ERROR: %v\n", n.id, err)
				continue
			}

			if newStateDB != nil {

				if n.checkGodTimeslotUsed(currJCE) {
					fmt.Printf("%s could author but blocked by god\n", n.String())
					return
				}
				n.sendGodTimeslotUsed(currJCE)
				// we authored a block
				oldstate := n.statedb
				newStateDB.SetAncestor(newBlock.Header, oldstate)

				n.addStateDB(newStateDB)
				n.StoreBlock(newBlock, n.id, true)
				n.cacheBlock(newBlock)

				headerHash := newBlock.Header.Hash()
				n.cacheHeaders(headerHash, newBlock)
				go n.broadcast(*newBlock)
				go func() {
					timeslot := newStateDB.GetSafrole().Timeslot
					err := n.writeDebug(newBlock, timeslot)
					if err != nil {
						log.Error(module, "runClient:writeDebug", "err", err)
					}
					s := n.statedb
					allStates := s.GetAllKeyValues()
					ok, err := s.CompareStateRoot(allStates, newBlock.Header.ParentStateRoot)
					if !ok || err != nil {
						log.Crit(module, "CompareStateRoot", "err", err)
					}

					st := buildStateTransitionStruct(oldstate, newBlock, newStateDB)
					err = n.writeDebug(st, timeslot) // StateTransition
					if err != nil {
						log.Error(module, "runClient:writeDebug", "err", err)
					}

					if revalidate {
						err = statedb.CheckStateTransition(n.store, st, s.AncestorSet)
						if err != nil {
							log.Crit(module, "runClient:CheckStateTransition", "err", err)
						}
						log.Trace(module, "Validated state transition")
					}

					// store StateSnapshot
					err = n.writeDebug(newStateDB.JamState.Snapshot(&(st.PostState)), timeslot) // StateSnapshot
					if err != nil {
						log.Error(module, "runClient:writeDebug", "err", err)
					}
				}()

				// Author is assuring the new block, resulting in a broadcast assurance with anchor = newBlock.Hash()
				n.assureNewBlock(newBlock)
				if Audit {
					n.auditingCh <- newStateDB.Copy()
				}
				IsTicketSubmissionClosed := n.statedb.GetSafrole().IsTicketSubmissionClosed(n.statedb.GetTimeslot())
				n.extrinsic_pool.RemoveUsedExtrinsicFromPool(newBlock, newStateDB.GetSafrole().Entropy[2], IsTicketSubmissionClosed)

			}

		case log := <-logChan:
			go n.WriteLog(log)
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

// write_jamnp_test_vector writes binary and JSON test vectors
func write_jamnp_test_vector(ce string, typ string, testVectorName string, vBytes []byte, v interface{}) {
	if writeJAMPNTestVector == false {
		return
	}
	dir := fmt.Sprintf("/tmp/jamnp/%s", ce)
	err := os.MkdirAll(dir, 0755) // Ensure the directory exists
	if err != nil {
		fmt.Printf("Failed to create directory %s: %v\n", dir, err)
		return
	}

	// Write .bin file with vBytes
	fnBin := filepath.Join(dir, fmt.Sprintf("%s-%s.bin", testVectorName, typ))
	err = os.WriteFile(fnBin, vBytes, 0644)
	if err != nil {
		fmt.Printf("Failed to write binary file %s: %v\n", fnBin, err)
	}

	// Write .json file with v if not nil
	if v != nil {
		fnJSON := filepath.Join(dir, fmt.Sprintf("%s-%s.json", testVectorName, typ))
		jsonData := types.ToJSON(v)
		err = os.WriteFile(fnJSON, []byte(jsonData), 0644)
		if err != nil {
			fmt.Printf("Failed to write JSON file %s: %v\n", fnJSON, err)
		}
	}
}

func (n *Node) jamnp_test_vector(ce string, testVectorName string, b []byte, obj interface{}) {
	write_jamnp_test_vector(ce, "response", testVectorName, b, obj)
}

func GenerateValidatorNetwork() (validators []types.Validator, secrets []types.ValidatorSecret, err error) {
	validators, secrets, err = generateValidatorNetwork()
	return validators, secrets, err
}

func generateValidatorNetwork() (validators []types.Validator, secrets []types.ValidatorSecret, err error) {
	return statedb.GenerateValidatorSecretSet(numNodes)
}

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
