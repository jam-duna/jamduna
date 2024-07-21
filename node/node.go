package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	//"encoding/binary"
	"encoding/json"
	//"crypto/elliptic"
	//"crypto/ecdsa"
	//"crypto/x509"
	//"crypto/x509/pkix"
	//"encoding/pem"
	"fmt"
	"github.com/colorfulnotion/jam/pvm"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quic-go/quic-go"
	"golang.org/x/crypto/blake2b"
	"log"
	"math/big"
	"sync"
	"time"
)

// Hash function using Blake2b
func bhash(data []byte) common.Hash {
	hash := blake2b.Sum256(data)
	return common.BytesToHash(hash[:])
}

const (
	numNodes   = 6
	numKeys    = 100
	quicAddr   = "localhost:%d"
	nodePrefix = "node"
)

type QuicMessage struct {
	MsgType string `json:"msgType"`
	Payload string `json:"payload"`
}

type Node struct {
	id     int
	store  *StateDBStorage
	server quic.Listener
	peers  []string
	mutex  sync.Mutex
	VMs    map[uint32]*pvm.VM
}

func newNode(id int, peers []string) (*Node, error) {
	path := fmt.Sprintf("path/to/testdb%d", id)
	store, err := NewStateDBStorage(path)
	if err != nil {
		return nil, err
	}

	for i := 1; i <= numKeys; i++ {
		key := common.BytesToHash([]byte(fmt.Sprintf("%d", i)))
		value := []byte(fmt.Sprintf("%s%d", nodePrefix, id))
		err := store.WriteKV(key, value)
		if err != nil {
			return nil, err
		}
	}

	addr := fmt.Sprintf(quicAddr, 9000+id)
	listener, err := quic.ListenAddr(addr, generateTLSConfig(), generateQuicConfig())

	if err != nil {
		return nil, err
	}

	node := &Node{
		id:     id,
		store:  store,
		server: *listener,
		peers:  peers,
	}
	go node.runServer()
	return node, nil
}

func generateTLSConfig() *tls.Config {
	// Use a dummy TLS configuration with InsecureSkipVerify set to true
	return &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{"quic-echo-example"},
	}
}

func (n *Node) runServer() {
	for {
		conn, err := n.server.Accept(context.Background())
		if err != nil {
			log.Println("Server accept error:", err)
			continue
		}
		go n.handleConnection(conn)
	}
}

func (n *Node) handleConnection(conn quic.Connection) {
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			log.Println("Accept stream error:", err)
			return
		}
		go n.handleStream(stream)
	}
}

func (n *Node) handleStream(stream quic.Stream) {
	defer stream.Close()

	buf := new(bytes.Buffer)
	buf.ReadFrom(stream)
	var msg QuicMessage
	err := json.Unmarshal(buf.Bytes(), &msg)
	if err != nil {
		log.Println("Failed to unmarshal QuicMessage:", err)
		return
	}

	var response interface{}
	switch msg.MsgType {
	case "ImportDAQuery":
		var query ImportDAQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err != nil {
			log.Println("Failed to unmarshal ImportDAQuery:", err)
			return
		}
		response = ImportDAResponse{Data: [][]byte{[]byte("dummy data")}, Proof: BMTProof{}}
	case "AuditDAQuery":
		var query AuditDAQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err != nil {
			log.Println("Failed to unmarshal AuditDAQuery:", err)
			return
		}
		response = AuditDAResponse{Data: []byte("dummy data"), Proof: BMTProof{}}
	case "ImportDAReconstructQuery":
		var query ImportDAReconstructQuery
		err := json.Unmarshal([]byte(msg.Payload), &query)
		if err != nil {
			log.Println("Failed to unmarshal ImportDAReconstructQuery:", err)
			return
		}
		response = ImportDAReconstructResponse{Data: []byte("dummy data")}
	case "Ticket":
		var ticket Ticket
		err := json.Unmarshal([]byte(msg.Payload), &ticket)
		if err != nil {
			log.Println("Failed to unmarshal Ticket:", err)
			return
		}
		err = processTicket(ticket)
	case "Guarantee":
		var guarantee Guarantee
		err := json.Unmarshal([]byte(msg.Payload), &guarantee)
		if err != nil {
			log.Println("Failed to unmarshal Guarantee:", err)
			return
		}
		err = processGuarantee(guarantee)
	case "Assurance":
		var assurance Assurance
		err := json.Unmarshal([]byte(msg.Payload), &assurance)
		if err != nil {
			log.Println("Failed to unmarshal Assurance:", err)
			return
		}
		err = processAssurance(assurance)
	case "Disputes":
		var disputes Disputes
		err := json.Unmarshal([]byte(msg.Payload), &disputes)
		if err != nil {
			log.Println("Failed to unmarshal Disputes:", err)
			return
		}
		err = processDisputes(disputes)
	case "PreimageLookup":
		var preimageLookup PreimageLookup
		err := json.Unmarshal([]byte(msg.Payload), &preimageLookup)
		if err != nil {
			log.Println("Failed to unmarshal PreimageLookup:", err)
			return
		}
		err = processPreimageLookup(preimageLookup)
	case "Block":
		var block Block
		err := json.Unmarshal([]byte(msg.Payload), &block)
		if err != nil {
			log.Println("Failed to unmarshal Block:", err)
			return
		}
		err = processBlock(block)
	case "Announcement":
		var announcement Announcement
		err := json.Unmarshal([]byte(msg.Payload), &announcement)
		if err != nil {
			log.Println("Failed to unmarshal Announcement:", err)
			return
		}
		err = processAnnouncement(announcement)
	case "WorkPackage":
		var workPackage WorkPackage
		err := json.Unmarshal([]byte(msg.Payload), &workPackage)
		if err != nil {
			log.Println("Failed to unmarshal WorkPackage:", err)
			return
		}
		err = processWorkPackage(workPackage)
	default:
		response = "OK"
	}

	if err != nil {
		response = 1 // Error
	} else {
		response = 0 // Success
	}

	responseData, err := json.Marshal(response)
	if err != nil {
		log.Println("Failed to marshal response:", err)
		return
	}

	_, err = stream.Write(responseData)
	if err != nil {
		log.Println("Failed to write response:", err)
		return
	}
}

func processTicket(ticket Ticket) error {
	// Add your logic here
	return nil // Success
}

func processGuarantee(guarantee Guarantee) error {
	// Add your logic here
	return nil // Success
}

func processAssurance(assurance Assurance) error {
	// Add your logic here
	return nil // Success
}

func processDisputes(disputes Disputes) error {
	// Add your logic here
	return nil // Success
}

func processPreimageLookup(preimageLookup PreimageLookup) error {
	// Add your logic here
	return nil // Success
}

func processBlock(block Block) error {
	// Add your logic here
	return nil // Success
}

func processAnnouncement(announcement Announcement) error {
	// Add your logic here
	return nil // Success
}

func processWorkPackage(workPackage WorkPackage) error {
	// Add your logic here
	return nil // Success
}

func (n *Node) makeRequest(peerAddr string, obj interface{}) ([]byte, error) {
	msgType := ""
	switch obj.(type) {
	case ImportDAQuery:
		msgType = "ImportDAQuery"
	case AuditDAQuery:
		msgType = "AuditDAQuery"
	case ImportDAReconstructQuery:
		msgType = "ImportDAReconstructQuery"
	case Ticket:
		msgType = "Ticket"
	case Guarantee:
		msgType = "Guarantee"
	case Assurance:
		msgType = "Assurance"
	case Disputes:
		msgType = "Disputes"
	case PreimageLookup:
		msgType = "PreimageLookup"
	case Block:
		msgType = "Block"
	case Announcement:
		msgType = "Announcement"
	case WorkPackage:
		msgType = "WorkPackage"
	default:
		return nil, fmt.Errorf("unsupported type")
	}

	payload, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}

	quicMessage := QuicMessage{
		MsgType: msgType,
		Payload: string(payload),
	}

	messageData, err := json.Marshal(quicMessage)
	if err != nil {
		return nil, err
	}

	conn, err := quic.DialAddr(context.Background(), peerAddr, generateTLSConfig(), generateQuicConfig())
	if err != nil {
		return nil, err
	}
	defer conn.CloseWithError(0, "client closing connection")

	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	_, err = stream.Write(messageData)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(stream)
	return buf.Bytes(), nil
}

func (n *Node) runClient() {
	for {
		time.Sleep(1 * time.Second)
		peerIndex, _ := rand.Int(rand.Reader, big.NewInt(numNodes))
		if int(peerIndex.Int64()) == n.id {
			continue
		}

		obj := generateRandomObject()

		value, err := n.makeRequest(n.peers[peerIndex.Int64()], obj)
		if err != nil {
			log.Println("Client request error:", err)
			continue
		}

		// For demonstration purposes, we just log the response
		log.Printf("Received response: %s\n", string(value))
	}
}

func generateRandomObject() interface{} {
	types := []string{
		"ImportDAQuery", "AuditDAQuery", "ImportDAReconstructQuery", "Ticket",
		"Guarantee", "Assurance", "Disputes", "PreimageLookup", "Block",
		"Announcement", "WorkPackage",
	}

	randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(types))))
	selectedType := types[randomIndex.Int64()]

	switch selectedType {
	case "ImportDAQuery":
		return ImportDAQuery{
			SegmentRoot:    bhash([]byte("segment_root")),
			SegmentIndex:   0,
			ProofRequested: true,
		}
	case "AuditDAQuery":
		return AuditDAQuery{
			SegmentRoot:    bhash([]byte("segment_root")),
			Index:          0,
			ProofRequested: true,
		}
	case "ImportDAReconstructQuery":
		return ImportDAReconstructQuery{
			SegmentRoot: bhash([]byte("segment_root")),
		}
	case "Ticket":
		var signature [ExtrinsicSignatureInBytes]byte
		copy(signature[:], "signature")
		return Ticket{
			Attempt:   1,
			Signature: signature,
		}
	case "Guarantee":
		return Guarantee{
			WorkReport:  WorkReport{},
			TimeSlot:    1,
			Credentials: []GuaranteeCredential{},
		}
	case "Assurance":
		return Assurance{
			ParentHash:     bhash([]byte("parent_hash")),
			Bitstring:      []byte("bitstring"),
			ValidatorIndex: 1,
			Signature:      Ed25519Signature{},
		}
	case "Disputes":
		return Disputes{
			Verdicts: []Verdict{
				{WorkReportHash: bhash([]byte("work_report_hash")), Epoch: 1},
			},
			Culprits: []Culprit{
				{R: PublicKey{}, K: PublicKey{}, Signature: Ed25519Signature{}},
			},
			Faults: []Fault{
				{R: PublicKey{}, K: PublicKey{}, V: PublicKey{}, Signature: Ed25519Signature{}},
			},
		}
	case "PreimageLookup":
		return PreimageLookup{
			ServiceIndex: 0,
			Data:         []byte("lookup"),
		}
	case "Block":
		return Block{
			Header: Header{
				ParentHash:     bhash([]byte("parent_hash")),
				PriorStateRoot: bhash([]byte("prior_state_root")),
				ExtrinsicHash:  bhash([]byte("extrinsic_hash")),
				TimeSlot:       0,
				EpochMark:      &EpochMark{},
				//WinningTickets:    &SafroleAccumulator{},
				JudgementsMarkers: &JudgementMarker{},
				BlockAuthorKey:    0,
				VRFSignature:      []byte("vrf_signature"),
				BlockSeal:         []byte("block_seal"),
			},
			Extrinsic: Extrinsic{},
		}
	case "Announcement":
		return Announcement{
			Signature: Ed25519Signature{},
		}
	case "WorkPackage":
		return WorkPackage{
			AuthorizationToken: []byte("exampleToken"),
			ServiceIndex:       0,
			AuthorizationCode:  bhash([]byte("authorization_code")),
			ParamBlob:          []byte("param_blob"),
			Context:            []byte("context"),
			WorkItems:          []WorkItem{},
		}
	default:
		return nil
	}
}

func generateQuicConfig() *quic.Config {
	return &quic.Config{}
}
