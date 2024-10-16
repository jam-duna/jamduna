package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"

	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"os"

	"encoding/json"
	"encoding/pem"
	"fmt"
	"math"
	"reflect"

	"math/big"
	rand0 "math/rand"
	"strings"
	"sync"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/statedb"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"

	"github.com/colorfulnotion/jam/trie"
	"github.com/quic-go/quic-go"
)

const (
	debug    = false
	trace    = false
	numNodes = 6
	quicAddr = "127.0.0.1:%d"
	basePort = 9000
)

const (
	ValidatorFlag   = "VALIDATOR"
	DAFlag          = "DA"
	ValidatorDAFlag = "VALIDATOR&DA"
)

type Node struct {
	id        uint16
	coreIndex uint16

	credential types.ValidatorSecret
	server     quic.Listener
	peers      []string
	peersInfo  map[uint16]*Peer //<ed25519> -> NodeInfo
	//peersAddr  	 map[string]string
	tlsConfig *tls.Config
	mutex     sync.Mutex

	store *storage.StateDBStorage /// where to put this?

	// holds a map of epoch (use entropy to control it) to at most 2 tickets
	selfTickets  map[common.Hash][]types.TicketBucket
	ticketsMutex sync.Mutex
	// this is for audit
	announcementBucket     types.AnnounceBucket
	prevAnnouncementBucket types.AnnounceBucket
	announcementMutex      sync.Mutex
	judgementBucket        types.JudgeBucket
	judgementMutex         sync.Mutex
	// use validator index to lookup the guarantee from the validator in their core
	guaranteeBucket map[common.Hash][]types.GuaranteeReport
	guaranteeMutex  sync.Mutex
	isBadGuarantor  bool
	// this is for assurances
	// use work package hash to lookup the availbility
	assurancesBucket map[common.Hash]types.IsPackageRecieved
	assuranceMutex   sync.Mutex
	// holds a map of the parenthash to the block
	blocks      map[common.Hash]*types.Block
	headers     map[common.Hash]*types.Block
	preimages   map[common.Hash][]byte
	workReports map[common.Hash]types.WorkReport

	blockAnnouncementsCh    chan types.BlockAnnouncement
	ticketsCh               chan types.Ticket
	workPackagesCh          chan types.WorkPackage
	workReportsCh           chan types.WorkReport
	guaranteesCh            chan types.Guarantee
	assurancesCh            chan types.Assurance
	preimageAnnouncementsCh chan types.PreimageAnnouncement
	announcementsCh         chan types.Announcement
	judgementsCh            chan types.Judgement

	// holds a map of the hash to the stateDB
	statedbMap map[common.Hash]*statedb.StateDB
	// holds the tip
	statedb         *statedb.StateDB
	messageChan     chan statedb.Message
	nodeType        string
	dataDir         string
	epoch0Timestamp uint32
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

func (n *Node) String() string {
	return fmt.Sprintf("[N%d]", n.id)
}

func (n *Node) setValidatorCredential(credential types.ValidatorSecret) {
	n.credential = credential
	if false {
		jsonData, err := types.Encode(credential)
		if err != nil {
			fmt.Printf("setValidatorCredential: %v\n", err)
		}
		fmt.Printf("[N%v] credential %s\n", n.id, jsonData)
	}
}

func NewNode(id uint16, credential types.ValidatorSecret, genesisConfig *statedb.GenesisConfig, peers []string, peerList map[uint16]*Peer, dataDir string, port int) (*Node, error) {
	n, err := newNode(id, credential, genesisConfig, peers, peerList, ValidatorFlag, dataDir, port)
	return n, err
}

func newNode(id uint16, credential types.ValidatorSecret, genesisConfig *statedb.GenesisConfig, peers []string, startPeerList map[uint16]*Peer, nodeType string, dataDir string, port int) (*Node, error) {
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	fmt.Printf("[N%v] newNode addr=%s dataDir=%v\n", id, addr, dataDir)

	levelDBPath := fmt.Sprintf("%v/leveldb/", dataDir)
	store, err := storage.NewStateDBStorage(levelDBPath)
	if err != nil {
		return nil, err
	}

	var cert tls.Certificate
	ed25519_priv := ed25519.PrivateKey(credential.Ed25519Secret[:])
	ed25519_pub := ed25519_priv.Public().(ed25519.PublicKey)

	cert, err = generateSelfSignedCert(ed25519_pub, ed25519_priv)
	if err != nil {
		return nil, fmt.Errorf("Error generating self-signed certificate: %v", err)
	}

	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,                                   // For testing purposes only
		NextProtos:         []string{"h3", "http/1.1", "ping/1.1"}, // Enable QUIC and HTTP/3
	}

	//fmt.Printf("[N%v] OPENING %s\n", id, addr)
	listener, err := quic.ListenAddr(addr, tlsConfig, GenerateQuicConfig())

	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}

	messageChan := make(chan statedb.Message, 100)

	node := &Node{
		id:          id,
		coreIndex:   uint16(id % 2), // TODO: NumCores
		store:       store,
		server:      *listener,
		peers:       peers,
		peersInfo:   make(map[uint16]*Peer),
		tlsConfig:   tlsConfig,
		messageChan: messageChan,
		nodeType:    nodeType,

		statedbMap: make(map[common.Hash]*statedb.StateDB),
		blocks:     make(map[common.Hash]*types.Block),
		headers:    make(map[common.Hash]*types.Block),
		preimages:  make(map[common.Hash][]byte),

		selfTickets: make(map[common.Hash][]types.TicketBucket),

		blockAnnouncementsCh:    make(chan types.BlockAnnouncement, 200),
		ticketsCh:               make(chan types.Ticket, 200),
		workPackagesCh:          make(chan types.WorkPackage, 200),
		workReportsCh:           make(chan types.WorkReport, 200),
		guaranteesCh:            make(chan types.Guarantee, 200),
		assurancesCh:            make(chan types.Assurance, 200),
		preimageAnnouncementsCh: make(chan types.PreimageAnnouncement, 200),
		announcementsCh:         make(chan types.Announcement, 200),
		judgementsCh:            make(chan types.Judgement, 200),
		dataDir:                 dataDir,
	}
	for validatorIndex, p := range startPeerList {
		node.peersInfo[validatorIndex] = NewPeer(node, validatorIndex, p.Validator, p.PeerAddr)
	}

	_statedb, err := statedb.NewGenesisStateDB(node.store, genesisConfig)
	if err == nil {
		_statedb.SetID(uint32(id))
		node.addStateDB(_statedb)
	} else {
		fmt.Printf("NewGenesisStateDB ERR %v\n", err)
		return nil, err
	}
	node.setValidatorCredential(credential)
	if genesisConfig != nil && genesisConfig.Epoch0Timestamp > 0 {
		node.epoch0Timestamp = uint32(genesisConfig.Epoch0Timestamp)
	}
	node.store.WriteLog(_statedb.JamState.Snapshot(), 0)

	node.statedb.PreviousGuarantors(true)
	node.statedb.AssignGuarantors(true)

	go node.runServer()
	go node.runClient()
	go node.runMain()
	return node, nil
}

func GenerateQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:       true,
		KeepAlivePeriod: time.Minute,
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
func (n *Node) GetPeerInfoByEd25519(key types.Ed25519Key) (*Peer, error) {
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

func (n *Node) GetCoreCoWorkersPeers(core uint16) (coWorkers []Peer) {
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

func (n *Node) GetCurrValidatorIndex() uint32 {
	return uint32(n.statedb.GetSafrole().GetCurrValidatorIndex(n.GetEd25519Key()))
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

func (n *Node) getPeerByIndex(peerIdx uint16) (*Peer, error) {
	p := n.peersInfo[peerIdx]

	return p, fmt.Errorf("peer not found")
}

func (n *Node) getPeerAddr(peerIdx uint16) (*Peer, error) {
	peer, exist := n.peersInfo[peerIdx]
	if exist {
		return peer, nil
	}
	fmt.Printf("getPeerAddr not found %v\n", peerIdx)
	return nil, fmt.Errorf("peer not found")
}

func (n *Node) addStateDB(_statedb *statedb.StateDB) error {
	if n.statedb == nil || n.statedb.GetBlock() == nil {
		var blkHash common.Hash
		if _statedb.GetBlock() != nil {
			blkHash = _statedb.GetBlock().Hash()
		}
		if debug {
			fmt.Printf("[N%d] addStateDB [%v <- %v] (stateRoot: %v)\n", n.id, _statedb.ParentHash, _statedb.BlockHash, _statedb.StateRoot)
		}
		n.statedb = _statedb
		n.statedbMap[blkHash] = _statedb
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
		if debug {
			fmt.Printf("[N%d] addStateDB TIP %v\n", n.id, _statedb.GetBlock().Hash())
		}
		n.statedb = _statedb
		n.statedbMap[_statedb.GetBlock().Hash()] = _statedb
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

func (n *Node) lookupAddr(addr string) uint16 {

	for validatorIndex, p := range n.peersInfo {
		fmt.Printf("compare %s to %s\n", p.PeerAddr, addr)
		if p.PeerAddr == addr {
			return validatorIndex
		}
	}
	panic(1024)
}

func (n *Node) handleConnection(conn quic.Connection) {
	//remoteAddr := conn.RemoteAddr()
	//localAddr := conn.LocalAddr()
	//fmt.Printf("handleConnection: remoteAddr=%s localAddr=%s\n", remoteAddr, localAddr);
	//peerID := n.lookupAddr(localAddr.String())
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			if quicErr, ok := err.(*quic.ApplicationError); ok && quicErr.ErrorCode == 0 {
				continue
			}
			fmt.Printf("handleConnection: Accept stream error: %v\n", err)
		}
		go n.DispatchIncomingQUICStream(stream)
	}
}

func (n *Node) broadcast(obj interface{}) []byte {
	result := []byte{}
	for id, p := range n.peersInfo {
		if id == n.id {
			continue
		}
		//fmt.Printf("%s BROADCAST PeerID=%v\n", n.String(), p.PeerID)
		objType := reflect.TypeOf(obj)
		switch objType {
		case reflect.TypeOf(types.Ticket{}):
			t := obj.(types.Ticket)
			epoch := uint32(0) // TODO: Shawn
			err := p.SendTicketDistribution(epoch, t, false)
			if err != nil {
				fmt.Printf("SendTicketDistribution ERR %v\n", err)
			}
			break
		case reflect.TypeOf(types.Block{}):
			b := obj.(types.Block)
			slot := uint32(0) // TODO: Shawn
			//fmt.Printf("%s BROADCAST SendBlockAnnouncement to %d: %v\n", n.String(), id, b.Header.Hash())
			err := p.SendBlockAnnouncement(b, slot)
			if err != nil {
				fmt.Printf("SendBlockAnnouncement ERR %v\n", err)
			}
			break

		case reflect.TypeOf(types.Guarantee{}):
			g := obj.(types.Guarantee)

			err := p.SendWorkReportDistribution(g.Report, g.Slot, g.Signatures)
			if err != nil {
				fmt.Printf("SendWorkReportDistribution ERR %v\n", err)
			}

			break
		case reflect.TypeOf(types.Assurance{}):
			a := obj.(types.Assurance)
			err := p.SendAssurance(&a)
			if err != nil {
				fmt.Printf("SendAssurance ERR %v\n", err)
			}
			break
		case reflect.TypeOf(types.Announcement{}):
			a := obj.(types.Announcement)
			coreIndex := a.Core
			workReportHash := a.WorkReportHash
			headerHash := a.HeaderHash
			err := p.SendAuditAnnouncement(workReportHash, headerHash, coreIndex, &a)
			if err != nil {
				fmt.Printf("SendAuditAnnouncement ERR %v\n", err)
			}
			break
		case reflect.TypeOf(types.Judgement{}):
			j := obj.(types.Judgement)
			epoch := uint32(0) // TODO: Shawn
			workReportHash := j.WorkReport.Hash()
			validity := uint8(0)        // TODO: Shawn
			validatorIndex := uint16(0) // TODO: Shawn
			err := p.SendJudgmentPublication(epoch, validatorIndex, validity, workReportHash, j.Signature)
			if err != nil {
				fmt.Printf("SendJudgmentPublication ERR %v\n", err)
			}
			break

		case reflect.TypeOf(types.Preimages{}):
			preimage := obj.(types.Preimages)
			// TODO: William
			//requester := p.Requester
			preimageHash := common.BytesToHash(common.ComputeHash(preimage.Blob))
			_, err := p.SendPreimageRequest(preimageHash)
			if err != nil {
				fmt.Printf("SendPreimageRequest ERR %v\n", err)
			}
			break
		}

	}
	return result
}

func (n *Node) coreBroadcast(obj interface{}) []byte {
	result := []byte{}
	core, err := n.GetSelfCoreIndex()
	if err != nil {
		fmt.Printf("coreBroadcast Error: %v\n", err)
		return nil
	}
	coworker := n.GetCoreCoWorkers(core)
	for id, p := range n.peersInfo {
		for _, worker := range coworker {
			if worker.Ed25519 == p.Validator.Ed25519 {
				//peerIdentifier := peer.Validator.Ed25519.String()
				if id == n.id {
					continue
				}

					objType := reflect.TypeOf(obj)
					switch objType {
					case reflect.TypeOf(types.WorkPackage{}):
						fmt.Printf("coreBroadcast: WorkPackage\n")
						wp := obj.(types.WorkPackage)
						workpackagehashes, segmentRoots, bundle := wp.Split()
						//stub
						bundle = n.encodeWorkPackage(wp)
						work_report_hash, sig, err := p.ShareWorkPackage(core, workpackagehashes, segmentRoots, bundle)
						if err != nil {
							fmt.Printf("ShareWorkPackage ERR in coreBoarcast: %v\n", err)
						}
						validatorIdx := n.statedb.GetSafrole().GetCurrValidatorIndex(p.Validator.Ed25519)
						if validatorIdx == -1 {
							fmt.Printf("coreBroadcast: vidx not found\n")
						}
						work := p.MakeGuaranteeReport(sig, uint16(validatorIdx))
						err = n.PutGuaranteeBucketWithoutReport(work, wp.Hash(), work_report_hash)
						if err != nil {
							fmt.Printf("PutGuaranteeBucket ERR in coreBoarcast: %v\n", err)
						}
					}

				//fmt.Printf("PeerID=%v, peerIdentifier=%v\n", peer.PeerID, peerIdentifier)
				//resp, err := n.makeRequest(peerIdentifier, obj, types.QuicOverallTimeout)

			}
		}

	}
	return result
}

// Helper function to determine if the error is a timeout error
func isTimeoutError(err error) bool {
	// Add more specific error handling as needed
	return strings.Contains(err.Error(), "timeout")
}

func (n *Node) processTicket(ticket types.Ticket) error {
	// Store the ticket in the tip's queued tickets
	s := n.getState()
	s.ProcessIncomingTicket(ticket)
	return nil // Success
}

func (n *Node) processPreimages(preimageLookup types.Preimages) error {
	// TODO: Store the lookup in a E_P aggregator
	s := n.getState()
	s.ProcessIncomingLookup(preimageLookup)
	return nil // Success
}

// Guarantees are sent by a validator working on a core receiving a work package and executing a refine operation
func (n *Node) processGuarantee(guarantee types.Guarantee) error {
	// Store the guarantee in the tip's queued guarantee
	s := n.getState()
	s.ProcessIncomingGuarantee(guarantee)
	return nil // Success
}

func (n *Node) processAssurance(assurance types.Assurance) error {
	// Store the assurance in the tip's queued assurance
	if len(assurance.Signature) == 0 {
		return fmt.Errorf("No assurance signature")
	}
	s := n.getState()
	s.ProcessIncomingAssurance(assurance)
	return nil // Success
}

func (n *Node) dumpstatedbmap() {
	for hash, statedb := range n.statedbMap {
		fmt.Printf("dumpstatedbmap: statedbMap[%v] => statedb (%v<=parent=%v) StateRoot %v\n", hash, statedb.ParentHash, statedb.BlockHash, statedb.StateRoot)
	}
	for hash, blk := range n.blocks {
		fmt.Printf("dumpstatedbmap: blocks[%v] => %v\n", hash, blk.Hash())
	}
}

func randomKey(m map[string]*Peer) string {
	rand0.Seed(time.Now().UnixNano())
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys[rand0.Intn(len(keys))]
}

func (n *Node) fetchBlock(blockHash common.Hash) (*types.Block, error) {
	return nil, fmt.Errorf("fetchBlock - No response")
}

func (n *Node) extendChain() error {
	parenthash := n.statedb.BlockHash
	for {

		ok := false
		for _, b := range n.blocks {
			if b.ParentHash() == parenthash {
				ok = true
				nextBlock := b

				// Measure time taken to apply state transition
				start := time.Now()

				// Apply the block to the tip
				newStateDB, err := statedb.ApplyStateTransitionFromBlock(n.statedb, context.Background(), nextBlock)

				if err != nil {
					fmt.Printf("[N%d] extendChain FAIL %v\n", n.id, err)
					return err
				}

				// Print the elapsed time in milliseconds
				elapsed := time.Since(start).Microseconds()
				if elapsed > 1000000 && trace {
					fmt.Printf("[N%d] extendChain %v <- %v \033[ApplyStateTransitionFromBlock\033[0m took %d ms\n", n.id, common.Str(parenthash), common.Str(nextBlock.Hash()), elapsed/1000)
				}

				// Extend the tip of the chain
				n.addStateDB(newStateDB)
				parenthash = nextBlock.Hash()
				if debug {
					fmt.Printf("[N%d] extendChain addstatedb TIP Now: s:%v<-%v\n", n.id, newStateDB.ParentHash, newStateDB.BlockHash)
				}

				if len(b.Extrinsic.Guarantees) > 0 {
					for _, g := range b.Extrinsic.Guarantees {
							n.assureData(g)
					}
				}

				break
			}
		}

		if !ok {
			// If there is no next block, we're done!
			if debug {
				fmt.Printf("[N%d] extendChain NO further next block %v\n", n.id, parenthash)
			}
			return nil
		}
	}
}

// assureData, given a Guarantee with a AvailabiltySpec within a WorkReport, fetches the bundleShard and segmentShards and stores in ImportDA + AuditDA
func (n *Node) assureData(g types.Guarantee) error {
	wr := g.Report
	if n.coreIndex != wr.CoreIndex {
		spec := wr.AvailabilitySpec
		erasureRoot := spec.ErasureRoot
		bundleLength := spec.BundleLength
		//workPackageHash := spec.WorkPackageHash
		//exportedSegmentRoot := spec.ExportedSegmentRoot
		coreValidator := uint16(0) // TODO: Shawn get the validators for the wr.CoreIndex
		bundleShard, segmentShards, justification, err := n.peersInfo[coreValidator].SendShardRequest(erasureRoot, n.id, false)
		if err != nil {
				fmt.Printf("%s assureData: SendShardRequest %v\n", n.String(), err)
		} else {
			if uint32(len(bundleShard)) == bundleLength {
				segmentIndex := make([]uint16, 0) // TODO: Michael
				segmentShardsI, justificationsI, err := n.peersInfo[coreValidator].SendSegmentShardRequest(erasureRoot, n.id, segmentIndex, false)
				if err != nil {
					fmt.Printf("%s assureData: SendSegmentShardRequest %v\n", n.String(),  err)
				}
				err = n.store.StoreImportDA(erasureRoot, n.id, segmentShardsI, justificationsI)
				if err != nil {
					fmt.Printf("%s assureData: StoreImportDA %v\n", n.String(),  err)
				} else {
					err = n.store.StoreAuditDA(erasureRoot, n.id, bundleShard, segmentShards, justification)
					if err != nil {
						fmt.Printf("%s assureData: storeAuditDA %v\n", n.String(),  err)
					} else {
						a := types.Assurance {
							Anchor: n.statedb.ParentHash,
							//Bitfield: make([types.Avail_bitfield_bytes]byte TODO
							ValidatorIndex: n.id,
						}
						a.Sign(n.credential.Ed25519Secret[:])
						n.broadcast(a)
					}
				}
			}
		}
	}
	return nil
}

// we arrive here when we receive a block from another node
func (n *Node) processBlock(blk *types.Block) error {
	// walk blk backwards, up to the tip, if possible -- but if encountering an unknown parenthash, immediately fetch the block.  Give up if we can't do anything
	b := blk
	n.blocks[b.Hash()] = blk
	n.headers[b.Header.Hash()] = blk
	for {
		if b.ParentHash() == (common.Hash{}) {
			//fmt.Printf("[N%d] processBlock: hit genesis (%v <- %v)\n", n.id, b.ParentHash(), b.Hash())
			break
		} else if n.statedb != nil && b.ParentHash() == n.statedb.BlockHash {
			//fmt.Printf("[N%d] processBlock: hit TIP (%v <- %v)\n", n.id, b.ParentHash(), b.Hash())
			break
		} else {
			var err error
			parentBlock, ok := n.blocks[b.ParentHash()]
			if !ok {
				parentBlock, err = n.fetchBlock(b.ParentHash())
				if err != nil || parentBlock == nil {
					// have to give up right now (could try again though!)
					return err
				}
				// got the parent block, store it in the cache
				if parentBlock.Hash() == blk.ParentHash() {
					fmt.Printf("[N%d] fetchBlock (%v<-%v) Validated --- CACHING\n", n.id, blk.ParentHash(), blk.Hash())
					n.blocks[parentBlock.Hash()] = parentBlock
				} else {
					return nil
				}

			}
			b = parentBlock
		}
	}

	// we got to the tip, now extend the chain, moving the tip forward, applying blocks using blockcache
	n.extendChain()

	currJCE := common.ComputeCurrentJCETime()
	currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
	if blk.Header.EpochMark != nil || (currEpoch == 0 && currPhase == 0) {
		if debug {
			fmt.Printf("[N%d]GenerateTickets currEpoch=%v, currPhase=%v\n", n.id, currEpoch, currPhase)
		}
		n.GenerateTickets()
		n.BroadcastTickets()
	}

	return nil // Success
}

func (n *Node) computeAssuranceBitfield() [1]byte {
	// TODO
	return [1]byte{3}
}

func (n *Node) newAvailabilityJustification(guarantee types.Guarantee) types.AvailabilityJustification {
	return types.AvailabilityJustification{}
}

func (n *Node) processAvailabilityJustification(aj *types.AvailabilityJustification) error {
	// TODO: validate proof
	//ed25519Key := n.GetEd25519Key()
	ed25519Priv := n.GetEd25519Secret()
	assurance := types.Assurance{
		Anchor:         n.statedb.ParentHash,
		Bitfield:       n.computeAssuranceBitfield(),
		ValidatorIndex: uint16(n.id),
		//	Signature: signature,
	}
	assurance.Sign(ed25519Priv)

	n.broadcast(assurance)
	return nil
}

func (n *Node) getImportSegment(treeRoot common.Hash, segmentIndex uint16) (segmentData []byte, err error) {
	// TODO: do you need segmentRoot or segmentsRoot here?
	/*segmentData, err := n.FetchAndReconstructSpecificSegmentData(treeRoot)
	if err != nil {
		return []byte{}, err
	}*/
	return segmentData, nil
}
func (n *Node) GetImportSegments(importsegments []types.ImportSegment) ([][]byte, error) {
	return n.getImportSegments(importsegments)
}

func (n *Node) getImportSegments(importsegments []types.ImportSegment) ([][]byte, error) {
	var imports [][]byte
	for _, s := range importsegments {
		importItem, err := n.getImportSegment(s.TreeRoot, s.Index)
		if err != nil {
			return imports, err
		}
		imports = append(imports, importItem)
	}
	return imports, nil
}

func (n *Node) getPVMStateDB() *statedb.StateDB {
	// TODO: processWorkPackage should provide clear signal on which stateDB to work with. What is this signal?
	// I don't think it should be the tip. workPackage probably requires somekind of blkhash or stateRoot?
	target_statedb := n.statedb.Copy() // stub. need to figure out what to do here
	return target_statedb
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
	case types.AvailabilityJustification:
		return "AvailabilityJustification"
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
		return "Block"
	case types.Block:
		return "Block"
	case types.Announcement:
		return "Announcement"
	case types.WorkPackage:
		return "WorkPackage"
	case types.DistributeECChunk:
		return "DistributeECChunk"
	case types.ECChunkQuery:
		return "ECChunkQuery"
	case types.GuaranteeReport:
		return "GuaranteeReport"
	case *statedb.StateDB:
		return "StateDB"
	case statedb.StateDB:
		return "StateDB"
	case *statedb.JamState:
		return "JamState"
	case statedb.JamState:
		return "JamState"
	case statedb.StateSnapshot:
		return "StateSnapshot"
	case *statedb.StateSnapshot:
		return "StateSnapshot"
	case *statedb.GenesisConfig:
		return "GenesisConfig"
	default:
		return "unknown"
	}
}

const TickTime = 200

func (n *Node) writeDebug(obj interface{}, timeslot uint32) error {
	l := storage.LogMessage{
		Payload:  obj,
		Timeslot: timeslot,
		//TODO:...
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
	//fmt.Printf("!!!! writeDebug msgType=%v, structDir=%v\n", msgType, structDir)
	if msgType != "unknown" {
		epoch, phase := statedb.ComputeEpochAndPhase(timeSlot, n.epoch0Timestamp)
		//currTS := common.ComputeCurrenTS()
		path := fmt.Sprintf("%s/%v_%v", structDir, epoch, phase)
		if epoch == 0 && phase == 0 {
			path = fmt.Sprintf("%s/genesis", structDir)
		}

		if msgType == "Ticket" {
			if ticket, ok := obj.(*types.Ticket); ok {
				// Cast successful, you can now access ticket's methods or fields
				identifier, _ := ticket.TicketID() // Assuming TicketID() is a method of types.Ticket
				path = fmt.Sprintf("%v_%v", path, identifier)
			} else {
				// Handle case where obj is not a *types.Ticket
				return fmt.Errorf("expected types.Ticket but got %T", obj)
			}
		}
		jsonPath := fmt.Sprintf("%s.json", path)
		codecPath := fmt.Sprintf("%s.bin", path)
		//fmt.Printf("%s jsonPath=%v, codecPath=%v\n", msgType, jsonPath, codecPath)

		// Check if the directories exist, if not create them
		if _, err := os.Stat(structDir); os.IsNotExist(err) {
			err := os.MkdirAll(structDir, os.ModePerm)
			if err != nil {
				return fmt.Errorf("Error creating %v directory: %v\n", msgType, err)
			}
		}

		switch v := obj.(type) {
		default:
			jsonEncode, _ := json.MarshalIndent(v, "", "    ")
			codecEncode, err := types.Encode(v)
			if err != nil {
				return fmt.Errorf("Error encoding object: %v\n", err)
			}

			//fmt.Printf("jsonEncode=%s \n", string(jsonEncode))
			//fmt.Printf("codecEncode=%x \n", codecEncode)

			err = os.WriteFile(jsonPath, jsonEncode, 0644)
			if err != nil {
				return fmt.Errorf("Error writing json file: %v\n", err)
			}
			err = os.WriteFile(codecPath, codecEncode, 0644)
			if err != nil {
				return fmt.Errorf("Error writing codec file: %v\n", err)
			}
		}
	}
	return nil
}

func (n *Node) runClient() {

	ticker_pulse := time.NewTicker(TickTime * time.Millisecond)
	defer ticker_pulse.Stop()

	logChan := n.store.GetChan()

	for {
		select {
		case <-ticker_pulse.C:
			if n.GetNodeType() != ValidatorFlag && n.GetNodeType() != ValidatorDAFlag {
				return
			}
			ticketIDs, err := n.GetSelfTicketsIDs()
			if err != nil {
				fmt.Printf("runClient: GetSelfTicketsIDs error: %v\n", err)
			}
			// timeslot mark
			// currJCE := common.ComputeCurrentJCETime()
			currJCE := common.ComputeTimeUnit(types.TimeUnitMode)
			currEpoch, currPhase := n.statedb.GetSafrole().EpochAndPhase(currJCE)
			if currEpoch != -1 {
				if currPhase == 0 {
					n.GenerateTickets()
					n.BroadcastTickets()

				} else if currPhase == types.EpochLength-1 { // you had currPhase == types.EpochLength-1
					// nextEpochFirst-endPhase <= currJCE <= nextEpochFirst
					if debug {
						fmt.Printf("[N%d]GenerateTickets currEpoch=%v, currPhase=%v\n", n.id, currEpoch, currPhase)
					}
					n.GenerateTickets()
					n.BroadcastTickets()

				}
			}
			newBlock, newStateDB, err := n.statedb.ProcessState(n.credential, ticketIDs)
			if err != nil {
				fmt.Printf("[N%d] ProcessState ERROR: %v\n", n.id, err)
				panic(0)
			}
			if newStateDB != nil {
				// we authored a block
				newStateDB.PreviousGuarantors(true)
				newStateDB.AssignGuarantors(true)
				n.addStateDB(newStateDB)
				n.blocks[newBlock.Hash()] = newBlock
				headerHash := newBlock.Header.Hash()
				n.headers[headerHash] = newBlock
				//fmt.Printf("%s BLOCK BROADCASTED: headerHash: %v (%v <- %v)\n", n.String(), headerHash, newBlock.ParentHash(), newBlock.Hash())
				n.broadcast(*newBlock)

				if debug {
					for _, g := range newStateDB.GuarantorAssignments {
						fmt.Printf("[N%d] GUARANTOR ASSIGNMENTS: %v -> core %v \n", n.id, g.Validator.Ed25519.String(), g.CoreIndex)
					}
				}
				timeslot := newStateDB.GetSafrole().Timeslot
				err := n.writeDebug(newBlock, timeslot)
				if err != nil {
					fmt.Printf("writeDebug Block err: %v\n", err)
				}
				err = n.writeDebug(newStateDB.JamState.Snapshot(), timeslot)
				if err != nil {
					fmt.Printf("writeDebug JamState err: %v\n", err)
				}

			}

		case log := <-logChan:
			//fmt.Printf("IM here!!! %v\n", log)
			n.WriteLog(log)
		}
	}
}

func (n *Node) processOutgoingMessage(msg statedb.Message) {
	msgType := msg.MsgType

	// Unmarshal the payload to the appropriate type
	switch msgType {
	case "Ticket":
		var ticket types.Ticket
		payloadBytes, err := types.Encode(msg.Payload)
		if err != nil {
			fmt.Printf("[N%v] Error encoding payload: %v\n", n.id, err)
		}
		decoded, _, err := types.Decode(payloadBytes, reflect.TypeOf(ticket))
		if err != nil {
			fmt.Printf("[N%v] Error decoding payload: %v\n", n.id, err)
		}
		ticket = decoded.(types.Ticket)
		//fmt.Printf("[N%v] Outgoing Ticket: %+v\n", n.id, ticket.TicketID())
		n.broadcast(ticket)
	case "Block":
		var blk types.Block
		payloadBytes, err := types.Encode(msg.Payload)
		if err != nil {
			fmt.Printf("[N%v] Error encoding payload: %v\n", n.id, err)
		}
		decoded, _, err := types.Decode(payloadBytes, reflect.TypeOf(blk))
		if err != nil {
			fmt.Printf("[N%v] Error decoding payload: %v\n", n.id, err)
		}
		blk = decoded.(types.Block)
		//fmt.Printf("[N%v] Outgoing Block: %+v\n", n.id, blk.Hash())
		n.broadcast(blk)
	default:
		fmt.Printf("[N%v] Unhandled message type: %v\n", n.id, msg.MsgType)
	}
}

func generateObject() interface{} {
	return nil
}

// Split the []byte into Hash of fixed size
func SplitBytesIntoHash(data []byte, hashSize int) []common.Hash {
	// Calculate the number of hashs
	numChunks := int(math.Ceil(float64(len(data)) / float64(hashSize)))
	// Make a slice of hashs
	hashs := make([]common.Hash, 0, numChunks)

	// Slice the data into hashs
	for i := 0; i < len(data); i += hashSize {
		end := i + hashSize
		if end > len(data) {
			end = len(data)
		}
		hashs = append(hashs, common.Hash(data[i:end]))
	}
	return hashs
}

func splitHashes(hashes []common.Hash) ([]common.Hash, []common.Hash) {
	// Compute the total number of segments
	totalSegments := len(hashes)
	// Compute the number of page proofs
	pfCount := int(math.Ceil(float64(totalSegments) / 64))

	// The part of the hashes that are segment hashes
	segmentHashs := hashes[:totalSegments-pfCount]

	// The remaining part of the hashes that are page proofs
	pfHashs := hashes[totalSegments-pfCount:]

	return segmentHashs, pfHashs
}

func (n *Node) GetSegmentTreeRoots(erasureRoot common.Hash) ([]common.Hash, error) {
	hashs, err := n.store.ReadKV(erasureRoot)
	if err != nil {
		fmt.Println("Error in FetchWorkPackageAndExportedSegments:", err)
	}
	// fmt.Printf("allHash: %x\n", hashs)
	treeRoot := hashs[32:]

	segmentsECRoots, err := n.store.ReadKV(common.Hash(treeRoot))
	if err != nil {
		return nil, err
	}
	allHash := SplitBytesIntoHash(segmentsECRoots, len(common.Hash{}))
	segmentRoots, _ := splitHashes(allHash)
	return segmentRoots, nil
}

// SplitDataIntoSegmentAndPageProof splits the data into segment and page proof
func SplitDataIntoSegmentAndPageProof(data [][]byte) (segment [][]byte, pageProof [][]byte) {
	totalData := len(data)
	var totalPageProofs int
	var totalSegments int

	// Initial estimate of totalPageProofs
	totalPageProofs = (totalData + 63) / 65

	// Iteratively compute totalPageProofs and totalSegments
	for {
		totalSegments = totalData - totalPageProofs
		newTotalPageProofs := (totalSegments + 63) / 64
		if newTotalPageProofs == totalPageProofs {
			break
		}
		totalPageProofs = newTotalPageProofs
	}

	// Return the segment and its corresponding page proof
	return data[:totalSegments+1], data[totalSegments+1:]
}

// SplitDataIntoSegmentAndPageProof splits the data into segment and page proof
func SplitDataIntoSegmentAndPageProofByIndex(data [][]byte, segmentIndex uint32) (segment []byte, pageProof []byte) {
	totalData := len(data)
	var totalPageProofs int
	var totalSegments int

	// Initial estimate of totalPageProofs
	totalPageProofs = (totalData + 63) / 65

	// Iteratively compute totalPageProofs and totalSegments
	for {
		totalSegments = totalData - totalPageProofs
		newTotalPageProofs := (totalSegments + 63) / 64
		if newTotalPageProofs == totalPageProofs {
			break
		}
		totalPageProofs = newTotalPageProofs
	}

	// Compute the group index
	groupIndex := segmentIndex / 64

	// Compute the page proof position
	pageProofPosition := totalSegments + int(groupIndex)

	// Return the segment and its corresponding page proof
	return data[segmentIndex], data[pageProofPosition]
}
