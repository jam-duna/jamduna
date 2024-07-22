package node

import (
	//"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
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
	numNodes = 6
	quicAddr = "localhost:%d" // karura-internal.polkaholic.io:%d
)

type QuicMessage struct {
	MsgType string `json:"msgType"`
	Payload string `json:"payload"`
}

type Node struct {
	id           int
	store        *StateDBStorage
	server       quic.Listener
	peers        []string
	tlsConfig    *tls.Config
	mutex        sync.Mutex
	VMs          map[uint32]*pvm.VM
	connections  map[string]quic.Connection
	streams      map[string]quic.Stream
	connectionMu sync.Mutex
}

func newNode(id int, peers []string) (*Node, error) {
	path := fmt.Sprintf("/var/log/testdb%d", id)
	store, err := NewStateDBStorage(path)
	if err != nil {
		return nil, err
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fmt.Println("Failed to generate private key:", err)
		return nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(1, 0, 0), // Valid for 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		fmt.Println("Failed to create certificate:", err)
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		log.Fatal("Error loading certificate:", err)
	}
	tlsConfig := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		InsecureSkipVerify: true,                                   // For testing purposes only
		NextProtos:         []string{"h3", "http/1.1", "ping/1.1"}, // Enable QUIC and HTTP/3
	}

	addr := fmt.Sprintf(quicAddr, 9000+id)
	fmt.Printf("OPENING %s\n", addr)
	listener, err := quic.ListenAddr(addr, tlsConfig, generateQuicConfig())

	if err != nil {
		fmt.Printf("ERR %v\n", err)
		return nil, err
	}

	node := &Node{
		id:          id,
		store:       store,
		server:      *listener,
		peers:       peers,
		tlsConfig:   tlsConfig,
		connections: make(map[string]quic.Connection),
		streams:     make(map[string]quic.Stream),
	}
	go node.runServer()
	go node.runClient()
	return node, nil
}

func generateQuicConfig() *quic.Config {
	return &quic.Config{
		Allow0RTT:       true,
		KeepAlivePeriod: time.Minute,
	}
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

func (n *Node) handleConnection(conn quic.Connection) {
	peerAddr := conn.RemoteAddr().String()

	n.connectionMu.Lock()
	n.connections[peerAddr] = conn
	n.connectionMu.Unlock()

	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			if quicErr, ok := err.(*quic.ApplicationError); ok && quicErr.ErrorCode == 0 {
				continue
			}
			fmt.Printf("handleConnection: Accept stream error: %v\n", err)
		}
		go n.handleStream(peerAddr, stream)
	}
}

func (n *Node) handleStream(peerAddr string, stream quic.Stream) {
	n.connectionMu.Lock()
	n.streams[peerAddr] = stream
	n.connectionMu.Unlock()

	for {
		buf := make([]byte, 100000)
		numBytes, err := stream.Read(buf)
		if err != nil {
			fmt.Printf("handleStream: Read error: %v\n", err)
		}
		var msg QuicMessage
		err = json.Unmarshal(buf[0:numBytes], &msg)
		if err != nil {
			fmt.Printf("Failed to unmarshal QuicMessage: %v\n", err)
		}
		fmt.Printf("handleStream %d READ %s from %s\n", n.id, msg.MsgType, peerAddr)

		response := []byte("1")
		ok := []byte("0")
		switch msg.MsgType {
		case "ImportDAQuery":
			var query ImportDAQuery
			err := json.Unmarshal([]byte(msg.Payload), &query)
			if err == nil {
				r := ImportDAResponse{Data: [][]byte{[]byte("dummy data")}, Proof: BMTProof{}}
				serializedR, err := json.Marshal(r)
				if err == nil {
					response = serializedR
				}
			}
		case "AuditDAQuery":
			var query AuditDAQuery
			err := json.Unmarshal([]byte(msg.Payload), &query)
			if err == nil {
				r := AuditDAResponse{Data: []byte("dummy data"), Proof: BMTProof{}}
				serializedR, err := json.Marshal(r)
				if err == nil {
					response = serializedR
				}
			}
		case "ImportDAReconstructQuery":
			var query ImportDAReconstructQuery
			err := json.Unmarshal([]byte(msg.Payload), &query)
			if err == nil {
				r := ImportDAReconstructResponse{Data: []byte("dummy data")}
				serializedR, err := json.Marshal(r)
				if err == nil {
					response = serializedR
				}
			}
		case "Ticket":
			var ticket Ticket
			err := json.Unmarshal([]byte(msg.Payload), &ticket)
			if err == nil {
				err = processTicket(ticket)
				if err == nil {
					response = ok
				}
			}
		case "Guarantee":
			var guarantee Guarantee
			err := json.Unmarshal([]byte(msg.Payload), &guarantee)
			if err == nil {
				err = processGuarantee(guarantee)
				if err == nil {
					response = ok
				}
			}
		case "Assurance":
			var assurance Assurance
			err := json.Unmarshal([]byte(msg.Payload), &assurance)
			if err == nil {
				err = processAssurance(assurance)
				if err == nil {
					response = ok
				}
			}
		case "Disputes":
			var disputes Disputes
			err := json.Unmarshal([]byte(msg.Payload), &disputes)
			if err == nil {
				err = processDisputes(disputes)
				if err == nil {
					response = ok
				}
			}
		case "PreimageLookup":
			var preimageLookup PreimageLookup
			err := json.Unmarshal([]byte(msg.Payload), &preimageLookup)
			if err == nil {
				err = processPreimageLookup(preimageLookup)
				if err == nil {
					response = ok
				}
			}
		case "Block":
			var block Block
			err := json.Unmarshal([]byte(msg.Payload), &block)
			if err == nil {
				err = processBlock(block)
				if err == nil {
					response = ok
				}
			}
		case "Announcement":
			var announcement Announcement
			err := json.Unmarshal([]byte(msg.Payload), &announcement)
			if err == nil {
				err = processAnnouncement(announcement)
				if err == nil {
					response = ok
				}
			}
		case "WorkPackage":
			var workPackage WorkPackage
			err := json.Unmarshal([]byte(msg.Payload), &workPackage)
			if err == nil {
				err = processWorkPackage(workPackage)
				if err == nil {
					response = ok
				}
			}
		}

		_, err = stream.Write(response)
		if err != nil {
			log.Println(err)
		}
		fmt.Printf("responded with: %s\n", string(response))
	}
}

func processTicket(ticket Ticket) error {
	// TODO: add to Safrole ticket accumulator
	return nil // Success
}

func processGuarantee(guarantee Guarantee) error {
	// TODO: Store the lookup in a E_G aggregator
	return nil // Success
}

func processAssurance(assurance Assurance) error {
	// TODO: Store the lookup in a E_A aggregator
	return nil // Success
}

func processDisputes(disputes Disputes) error {
	// TODO: Store the lookup in a E_D aggregator
	return nil // Success
}

func processPreimageLookup(preimageLookup PreimageLookup) error {
	// TODO: Store the lookup in a E_P aggregator
	return nil // Success
}

func processBlock(block Block) error {
	// TODO: validate the Block and advance the timeslot
	return nil // Success
}

func processAnnouncement(announcement Announcement) error {
	// TODO: TBD
	return nil // Success
}

func processWorkPackage(workPackage WorkPackage) error {
	// TODO: Incorporate WP chunks
	return nil // Success
}

func (n *Node) makeRequest(peerAddr string, obj interface{}) ([]byte, error) {
	n.connectionMu.Lock()
	conn, exists := n.connections[peerAddr]
	if !exists {
		var err error
		conn, err = quic.DialAddr(context.Background(), peerAddr, n.tlsConfig, generateQuicConfig())
		if err != nil {
			n.connectionMu.Unlock()
			fmt.Printf("-- makeRequest ERR %v peerAddr=%s\n", err, peerAddr)
			return nil, err
		}
		n.connections[peerAddr] = conn
		//fmt.Printf("NEWCONN %d => %s\n", n.id, peerAddr);
	}
	stream, exists := n.streams[peerAddr]
	if !exists {
		var err error
		stream, err = conn.OpenStreamSync(context.Background())
		if err != nil {
			n.connectionMu.Unlock()
			fmt.Printf("-- openstreamsync ERR %v\n", err)
			return nil, err
		}
		n.streams[peerAddr] = stream
		//fmt.Printf("NEWSTREAM %d => %s\n", n.id, peerAddr);
	}
	n.connectionMu.Unlock()

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
		fmt.Printf("HMM2 %v\n", err)
		return nil, err
	}

	_, err = stream.Write(messageData)
	if err != nil {
		fmt.Printf("-- Write ERR %v\n", err)
		return nil, err
	}

	fmt.Printf("-- makeRequest WRITE %s from %d => %s\n", msgType, n.id, peerAddr)

	/*// Read the response from the stream with an expandable buffer
	var buffer bytes.Buffer
	tmp := make([]byte, 100000)
	for {
		nRead, err := stream.Read(tmp)
		buffer.Write(tmp[:nRead])
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			fmt.Printf("-- Read ERR %v\n", err)
			return nil, err
		}
	}
	fmt.Printf("-- makeRequest READ %d bytes from %d => %s\n", buffer.Len(), n.id, peerAddr)
	*/
	return nil, nil
}

func (n *Node) runClient() {
	for {
		time.Sleep(1000 * time.Millisecond)
		peerIndex, _ := rand.Int(rand.Reader, big.NewInt(numNodes))
		if int(peerIndex.Int64()) == n.id {
			continue
		}
		obj := generateRandomObject()
		_, err := n.makeRequest(n.peers[peerIndex.Int64()], obj)
		if err != nil {
			fmt.Printf("runClient request error: %v\n", err)
			continue
		}
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
				ParentHash:        bhash([]byte("parent_hash")),
				PriorStateRoot:    bhash([]byte("prior_state_root")),
				ExtrinsicHash:     bhash([]byte("extrinsic_hash")),
				TimeSlot:          0,
				EpochMark:         &EpochMark{},
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
