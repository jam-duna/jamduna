package telemetry

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/jam-duna/jamduna/common"
)

// Decoder type for payload decoding functions
type Decoder func(payload []byte) string

// Maps for discriminator to string and decoder lookup
var discriminatorToString = map[int]string{
	// Meta events
	Telemetry_Dropped: "DROPPED",

	// Status events (10-13)
	Telemetry_Status:                  "STATUS",
	Telemetry_Best_Block_Changed:      "BEST_BLOCK_CHANGED",
	Telemetry_Finalized_Block_Changed: "FINALIZED_BLOCK_CHANGED",
	Telemetry_Sync_Status_Changed:     "SYNC_STATUS_CHANGED",

	// Networking events (20-28)
	Telemetry_Connection_Refused: "CONNECTION_REFUSED",
	Telemetry_Connecting_In:      "CONNECTING_IN",
	Telemetry_Connect_In_Failed:  "CONNECT_IN_FAILED",
	Telemetry_Connected_In:       "CONNECTED_IN",
	Telemetry_Connecting_Out:     "CONNECTING_OUT",
	Telemetry_Connect_Out_Failed: "CONNECT_OUT_FAILED",
	Telemetry_Connected_Out:      "CONNECTED_OUT",
	Telemetry_Disconnected:       "DISCONNECTED",
	Telemetry_Peer_Misbehaved:    "PEER_MISBEHAVED",

	// Block events (40-48)
	Telemetry_Authoring:                   "AUTHORING",
	Telemetry_Authoring_Failed:            "AUTHORING_FAILED",
	Telemetry_Authored:                    "AUTHORED",
	Telemetry_Importing:                   "IMPORTING",
	Telemetry_Block_Verification_Failed:   "BLOCK_VERIFICATION_FAILED",
	Telemetry_Block_Verified:              "BLOCK_VERIFIED",
	Telemetry_Block_Execution_Failed:      "BLOCK_EXECUTION_FAILED",
	Telemetry_Block_Executed:              "BLOCK_EXECUTED",
	Telemetry_Accumulate_Result_Available: "ACCUMULATE_RESULT_AVAILABLE",

	// Block announcement events (60-68, 71)
	Telemetry_Block_Announcement_Stream_Opened: "BLOCK_ANNOUNCEMENT_STREAM_OPENED",
	Telemetry_Block_Announcement_Stream_Closed: "BLOCK_ANNOUNCEMENT_STREAM_CLOSED",
	Telemetry_Block_Announced:                  "BLOCK_ANNOUNCED",
	Telemetry_Sending_Block_Request:            "SENDING_BLOCK_REQUEST",
	Telemetry_Receiving_Block_Request:          "RECEIVING_BLOCK_REQUEST",
	Telemetry_Block_Request_Failed:             "BLOCK_REQUEST_FAILED",
	Telemetry_Block_Request_Sent:               "BLOCK_REQUEST_SENT",
	Telemetry_Block_Request_Received:           "BLOCK_REQUEST_RECEIVED",
	Telemetry_Block_Transferred:                "BLOCK_TRANSFERRED",
	Telemetry_Block_Announcement_Malformed:     "BLOCK_ANNOUNCEMENT_MALFORMED",

	// Safrole events (80-84)
	Telemetry_Generating_Tickets:       "GENERATING_TICKETS",
	Telemetry_Ticket_Generation_Failed: "TICKET_GENERATION_FAILED",
	Telemetry_Tickets_Generated:        "TICKETS_GENERATED",
	Telemetry_Ticket_Transfer_Failed:   "TICKET_TRANSFER_FAILED",
	Telemetry_Ticket_Transferred:       "TICKET_TRANSFERRED",

	// Work package/guarantee events (90-113)
	Telemetry_Work_Package_Submission:        "WORK_PACKAGE_SUBMISSION",
	Telemetry_Work_Package_Being_Shared:      "WORK_PACKAGE_BEING_SHARED",
	Telemetry_Work_Package_Failed:            "WORK_PACKAGE_FAILED",
	Telemetry_Duplicate_Work_Package:         "DUPLICATE_WORK_PACKAGE",
	Telemetry_Work_Package_Received:          "WORK_PACKAGE_RECEIVED",
	Telemetry_Authorized:                     "AUTHORIZED",
	Telemetry_Extrinsic_Data_Received:        "EXTRINSIC_DATA_RECEIVED",
	Telemetry_Imports_Received:               "IMPORTS_RECEIVED",
	Telemetry_Sharing_Work_Package:           "SHARING_WORK_PACKAGE",
	Telemetry_Work_Package_Sharing_Failed:    "WORK_PACKAGE_SHARING_FAILED",
	Telemetry_Bundle_Sent:                    "BUNDLE_SENT",
	Telemetry_Refined:                        "REFINED",
	Telemetry_Work_Report_Built:              "WORK_REPORT_BUILT",
	Telemetry_Work_Report_Signature_Sent:     "WORK_REPORT_SIGNATURE_SENT",
	Telemetry_Work_Report_Signature_Received: "WORK_REPORT_SIGNATURE_RECEIVED",
	Telemetry_Guarantee_Built:                "GUARANTEE_BUILT",
	Telemetry_Sending_Guarantee:              "SENDING_GUARANTEE",
	Telemetry_Guarantee_Send_Failed:          "GUARANTEE_SEND_FAILED",
	Telemetry_Guarantee_Sent:                 "GUARANTEE_SENT",
	Telemetry_Guarantees_Distributed:         "GUARANTEES_DISTRIBUTED",
	Telemetry_Receiving_Guarantee:            "RECEIVING_GUARANTEE",
	Telemetry_Guarantee_Receive_Failed:       "GUARANTEE_RECEIVE_FAILED",
	Telemetry_Guarantee_Received:             "GUARANTEE_RECEIVED",
	Telemetry_Guarantee_Discarded:            "GUARANTEE_DISCARDED",

	// Assurance events (120-133)
	Telemetry_Sending_Shard_Request:    "SENDING_SHARD_REQUEST",
	Telemetry_Receiving_Shard_Request:  "RECEIVING_SHARD_REQUEST",
	Telemetry_Shard_Request_Failed:     "SHARD_REQUEST_FAILED",
	Telemetry_Shard_Request_Sent:       "SHARD_REQUEST_SENT",
	Telemetry_Shard_Request_Received:   "SHARD_REQUEST_RECEIVED",
	Telemetry_Shards_Transferred:       "SHARDS_TRANSFERRED",
	Telemetry_Distributing_Assurance:   "DISTRIBUTING_ASSURANCE",
	Telemetry_Assurance_Send_Failed:    "ASSURANCE_SEND_FAILED",
	Telemetry_Assurance_Sent:           "ASSURANCE_SENT",
	Telemetry_Assurance_Distributed:    "ASSURANCE_DISTRIBUTED",
	Telemetry_Assurance_Receive_Failed: "ASSURANCE_RECEIVE_FAILED",
	Telemetry_Assurance_Received:       "ASSURANCE_RECEIVED",
	Telemetry_Context_Available:        "CONTEXT_AVAILABLE",
	Telemetry_Assurance_Provided:       "ASSURANCE_PROVIDED",

	// Bundle recovery events (140-153)
	Telemetry_Sending_Bundle_Shard_Request:   "SENDING_BUNDLE_SHARD_REQUEST",
	Telemetry_Receiving_Bundle_Shard_Request: "RECEIVING_BUNDLE_SHARD_REQUEST",
	Telemetry_Bundle_Shard_Request_Failed:    "BUNDLE_SHARD_REQUEST_FAILED",
	Telemetry_Bundle_Shard_Request_Sent:      "BUNDLE_SHARD_REQUEST_SENT",
	Telemetry_Bundle_Shard_Request_Received:  "BUNDLE_SHARD_REQUEST_RECEIVED",
	Telemetry_Bundle_Shard_Transferred:       "BUNDLE_SHARD_TRANSFERRED",
	Telemetry_Reconstructing_Bundle:          "RECONSTRUCTING_BUNDLE",
	Telemetry_Bundle_Reconstructed:           "BUNDLE_RECONSTRUCTED",
	Telemetry_Sending_Bundle_Request:         "SENDING_BUNDLE_REQUEST",
	Telemetry_Receiving_Bundle_Request:       "RECEIVING_BUNDLE_REQUEST",
	Telemetry_Bundle_Request_Failed:          "BUNDLE_REQUEST_FAILED",
	Telemetry_Bundle_Request_Sent:            "BUNDLE_REQUEST_SENT",
	Telemetry_Bundle_Request_Received:        "BUNDLE_REQUEST_RECEIVED",
	Telemetry_Bundle_Transferred:             "BUNDLE_TRANSFERRED",

	// Segment events (160-178)
	Telemetry_Work_Package_Hash_Mapped:        "WORK_PACKAGE_HASH_MAPPED",
	Telemetry_Segments_Root_Mapped:            "SEGMENTS_ROOT_MAPPED",
	Telemetry_Sending_Segment_Shard_Request:   "SENDING_SEGMENT_SHARD_REQUEST",
	Telemetry_Receiving_Segment_Shard_Request: "RECEIVING_SEGMENT_SHARD_REQUEST",
	Telemetry_Segment_Shard_Request_Failed:    "SEGMENT_SHARD_REQUEST_FAILED",
	Telemetry_Segment_Shard_Request_Sent:      "SEGMENT_SHARD_REQUEST_SENT",
	Telemetry_Segment_Shard_Request_Received:  "SEGMENT_SHARD_REQUEST_RECEIVED",
	Telemetry_Segment_Shards_Transferred:      "SEGMENT_SHARDS_TRANSFERRED",
	Telemetry_Reconstructing_Segments:         "RECONSTRUCTING_SEGMENTS",
	Telemetry_Segment_Reconstruction_Failed:   "SEGMENT_RECONSTRUCTION_FAILED",
	Telemetry_Segments_Reconstructed:          "SEGMENTS_RECONSTRUCTED",
	Telemetry_Segment_Verification_Failed:     "SEGMENT_VERIFICATION_FAILED",
	Telemetry_Segments_Verified:               "SEGMENTS_VERIFIED",
	Telemetry_Sending_Segment_Request:         "SENDING_SEGMENT_REQUEST",
	Telemetry_Receiving_Segment_Request:       "RECEIVING_SEGMENT_REQUEST",
	Telemetry_Segment_Request_Failed:          "SEGMENT_REQUEST_FAILED",
	Telemetry_Segment_Request_Sent:            "SEGMENT_REQUEST_SENT",
	Telemetry_Segment_Request_Received:        "SEGMENT_REQUEST_RECEIVED",
	Telemetry_Segments_Transferred:            "SEGMENTS_TRANSFERRED",

	// Preimage events (190-199)
	Telemetry_Preimage_Announcement_Failed: "PREIMAGE_ANNOUNCEMENT_FAILED",
	Telemetry_Preimage_Announced:           "PREIMAGE_ANNOUNCED",
	Telemetry_Announced_Preimage_Forgotten: "ANNOUNCED_PREIMAGE_FORGOTTEN",
	Telemetry_Sending_Preimage_Request:     "SENDING_PREIMAGE_REQUEST",
	Telemetry_Receiving_Preimage_Request:   "RECEIVING_PREIMAGE_REQUEST",
	Telemetry_Preimage_Request_Failed:      "PREIMAGE_REQUEST_FAILED",
	Telemetry_Preimage_Request_Sent:        "PREIMAGE_REQUEST_SENT",
	Telemetry_Preimage_Request_Received:    "PREIMAGE_REQUEST_RECEIVED",
	Telemetry_Preimage_Transferred:         "PREIMAGE_TRANSFERRED",
	Telemetry_Preimage_Discarded:           "PREIMAGE_DISCARDED",
}

var discriminatorDecoder = map[int]Decoder{
	// Meta events
	Telemetry_Dropped: DecodeDropped,

	// Status events (10-13)
	Telemetry_Status:                  DecodeStatus,
	Telemetry_Best_Block_Changed:      DecodeBestBlockChanged,
	Telemetry_Finalized_Block_Changed: DecodeFinalizedBlockChanged,
	Telemetry_Sync_Status_Changed:     DecodeSyncStatusChanged,

	// Networking events (20-28)
	Telemetry_Connection_Refused: DecodeConnectionRefused,
	Telemetry_Connecting_In:      DecodeConnectingIn,
	Telemetry_Connect_In_Failed:  DecodeConnectInFailed,
	Telemetry_Connected_In:       DecodeConnectedIn,
	Telemetry_Connecting_Out:     DecodeConnectingOut,
	Telemetry_Connect_Out_Failed: DecodeConnectOutFailed,
	Telemetry_Connected_Out:      DecodeConnectedOut,
	Telemetry_Disconnected:       DecodeDisconnected,
	Telemetry_Peer_Misbehaved:    DecodePeerMisbehaved,

	// Block events (40-48)
	Telemetry_Authoring:                   DecodeAuthoring,
	Telemetry_Authoring_Failed:            DecodeAuthoringFailed,
	Telemetry_Authored:                    DecodeAuthored,
	Telemetry_Importing:                   DecodeImporting,
	Telemetry_Block_Verification_Failed:   DecodeBlockVerificationFailed,
	Telemetry_Block_Verified:              DecodeBlockVerified,
	Telemetry_Block_Execution_Failed:      DecodeBlockExecutionFailed,
	Telemetry_Block_Executed:              DecodeBlockExecuted,
	Telemetry_Accumulate_Result_Available: DecodeAccumulateResultAvailable,

	// Block announcement events (60-68, 71)
	Telemetry_Block_Announcement_Stream_Opened: DecodeBlockAnnouncementStreamOpened,
	Telemetry_Block_Announcement_Stream_Closed: DecodeBlockAnnouncementStreamClosed,
	Telemetry_Block_Announced:                  DecodeBlockAnnounced,
	Telemetry_Sending_Block_Request:            DecodeSendingBlockRequest,
	Telemetry_Receiving_Block_Request:          DecodeReceivingBlockRequest,
	Telemetry_Block_Request_Failed:             DecodeBlockRequestFailed,
	Telemetry_Block_Request_Sent:               DecodeBlockRequestSent,
	Telemetry_Block_Request_Received:           DecodeBlockRequestReceived,
	Telemetry_Block_Transferred:                DecodeBlockTransferred,
	Telemetry_Block_Announcement_Malformed:     DecodeBlockAnnouncementMalformed,

	// Safrole events (80-84)
	Telemetry_Generating_Tickets:       DecodeGeneratingTickets,
	Telemetry_Ticket_Generation_Failed: DecodeTicketGenerationFailed,
	Telemetry_Tickets_Generated:        DecodeTicketsGenerated,
	Telemetry_Ticket_Transfer_Failed:   DecodeTicketTransferFailed,
	Telemetry_Ticket_Transferred:       DecodeTicketTransferred,

	// Work package/guarantee events (90-113)
	Telemetry_Work_Package_Submission:        DecodeWorkPackageSubmission,
	Telemetry_Work_Package_Being_Shared:      DecodeWorkPackageBeingShared,
	Telemetry_Work_Package_Failed:            DecodeWorkPackageFailed,
	Telemetry_Duplicate_Work_Package:         DecodeDuplicateWorkPackage,
	Telemetry_Work_Package_Received:          DecodeWorkPackageReceived,
	Telemetry_Authorized:                     DecodeAuthorized,
	Telemetry_Extrinsic_Data_Received:        DecodeExtrinsicDataReceived,
	Telemetry_Imports_Received:               DecodeImportsReceived,
	Telemetry_Sharing_Work_Package:           DecodeSharingWorkPackage,
	Telemetry_Work_Package_Sharing_Failed:    DecodeWorkPackageSharingFailed,
	Telemetry_Bundle_Sent:                    DecodeBundleSent,
	Telemetry_Refined:                        DecodeRefined,
	Telemetry_Work_Report_Built:              DecodeWorkReportBuilt,
	Telemetry_Work_Report_Signature_Sent:     DecodeWorkReportSignatureSent,
	Telemetry_Work_Report_Signature_Received: DecodeWorkReportSignatureReceived,
	Telemetry_Guarantee_Built:                DecodeGuaranteeBuilt,
	Telemetry_Sending_Guarantee:              DecodeSendingGuarantee,
	Telemetry_Guarantee_Send_Failed:          DecodeGuaranteeSendFailed,
	Telemetry_Guarantee_Sent:                 DecodeGuaranteeSent,
	Telemetry_Guarantees_Distributed:         DecodeGuaranteesDistributed,
	Telemetry_Receiving_Guarantee:            DecodeReceivingGuarantee,
	Telemetry_Guarantee_Receive_Failed:       DecodeGuaranteeReceiveFailed,
	Telemetry_Guarantee_Received:             DecodeGuaranteeReceived,
	Telemetry_Guarantee_Discarded:            DecodeGuaranteeDiscarded,

	// Assurance events (120-133)
	Telemetry_Sending_Shard_Request:    DecodeSendingShardRequest,
	Telemetry_Receiving_Shard_Request:  DecodeReceivingShardRequest,
	Telemetry_Shard_Request_Failed:     DecodeShardRequestFailed,
	Telemetry_Shard_Request_Sent:       DecodeShardRequestSent,
	Telemetry_Shard_Request_Received:   DecodeShardRequestReceived,
	Telemetry_Shards_Transferred:       DecodeShardsTransferred,
	Telemetry_Distributing_Assurance:   DecodeDistributingAssurance,
	Telemetry_Assurance_Send_Failed:    DecodeAssuranceSendFailed,
	Telemetry_Assurance_Sent:           DecodeAssuranceSent,
	Telemetry_Assurance_Distributed:    DecodeAssuranceDistributed,
	Telemetry_Assurance_Receive_Failed: DecodeAssuranceReceiveFailed,
	Telemetry_Assurance_Received:       DecodeAssuranceReceived,
	Telemetry_Context_Available:        DecodeContextAvailable,
	Telemetry_Assurance_Provided:       DecodeAssuranceProvided,

	// Bundle recovery events (140-153)
	Telemetry_Sending_Bundle_Shard_Request:   DecodeSendingBundleShardRequest,
	Telemetry_Receiving_Bundle_Shard_Request: DecodeReceivingBundleShardRequest,
	Telemetry_Bundle_Shard_Request_Failed:    DecodeBundleShardRequestFailed,
	Telemetry_Bundle_Shard_Request_Sent:      DecodeBundleShardRequestSent,
	Telemetry_Bundle_Shard_Request_Received:  DecodeBundleShardRequestReceived,
	Telemetry_Bundle_Shard_Transferred:       DecodeBundleShardTransferred,
	Telemetry_Reconstructing_Bundle:          DecodeReconstructingBundle,
	Telemetry_Bundle_Reconstructed:           DecodeBundleReconstructed,
	Telemetry_Sending_Bundle_Request:         DecodeSendingBundleRequest,
	Telemetry_Receiving_Bundle_Request:       DecodeReceivingBundleRequest,
	Telemetry_Bundle_Request_Failed:          DecodeBundleRequestFailed,
	Telemetry_Bundle_Request_Sent:            DecodeBundleRequestSent,
	Telemetry_Bundle_Request_Received:        DecodeBundleRequestReceived,
	Telemetry_Bundle_Transferred:             DecodeBundleTransferred,

	// Segment events (160-178)
	Telemetry_Work_Package_Hash_Mapped:        DecodeWorkPackageHashMapped,
	Telemetry_Segments_Root_Mapped:            DecodeSegmentsRootMapped,
	Telemetry_Sending_Segment_Shard_Request:   DecodeSendingSegmentShardRequest,
	Telemetry_Receiving_Segment_Shard_Request: DecodeReceivingSegmentShardRequest,
	Telemetry_Segment_Shard_Request_Failed:    DecodeSegmentShardRequestFailed,
	Telemetry_Segment_Shard_Request_Sent:      DecodeSegmentShardRequestSent,
	Telemetry_Segment_Shard_Request_Received:  DecodeSegmentShardRequestReceived,
	Telemetry_Segment_Shards_Transferred:      DecodeSegmentShardsTransferred,
	Telemetry_Reconstructing_Segments:         DecodeReconstructingSegments,
	Telemetry_Segment_Reconstruction_Failed:   DecodeSegmentReconstructionFailed,
	Telemetry_Segments_Reconstructed:          DecodeSegmentsReconstructed,
	Telemetry_Segment_Verification_Failed:     DecodeSegmentVerificationFailed,
	Telemetry_Segments_Verified:               DecodeSegmentsVerified,
	Telemetry_Sending_Segment_Request:         DecodeSendingSegmentRequest,
	Telemetry_Receiving_Segment_Request:       DecodeReceivingSegmentRequest,
	Telemetry_Segment_Request_Failed:          DecodeSegmentRequestFailed,
	Telemetry_Segment_Request_Sent:            DecodeSegmentRequestSent,
	Telemetry_Segment_Request_Received:        DecodeSegmentRequestReceived,
	Telemetry_Segments_Transferred:            DecodeSegmentsTransferred,

	// Preimage events (190-199)
	Telemetry_Preimage_Announcement_Failed: DecodePreimageAnnouncementFailed,
	Telemetry_Preimage_Announced:           DecodePreimageAnnounced,
	Telemetry_Announced_Preimage_Forgotten: DecodeAnnouncedPreimageForgotten,
	Telemetry_Sending_Preimage_Request:     DecodeSendingPreimageRequest,
	Telemetry_Receiving_Preimage_Request:   DecodeReceivingPreimageRequest,
	Telemetry_Preimage_Request_Failed:      DecodePreimageRequestFailed,
	Telemetry_Preimage_Request_Sent:        DecodePreimageRequestSent,
	Telemetry_Preimage_Request_Received:    DecodePreimageRequestReceived,
	Telemetry_Preimage_Transferred:         DecodePreimageTransferred,
	Telemetry_Preimage_Discarded:           DecodePreimageDiscarded,
}

// TelemetryServer manages incoming telemetry connections and logs events
type TelemetryServer struct {
	addr     string
	listener net.Listener
	logFile  *os.File
	logger   *log.Logger
	stopped  bool
}

// NewTelemetryServer creates a new telemetry server that will listen on the given address
func NewTelemetryServer(addr, logFilePath string) (*TelemetryServer, error) {
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := log.New(logFile, "", 0) // No prefix, we'll format timestamps ourselves

	return &TelemetryServer{
		addr:    addr,
		logFile: logFile,
		logger:  logger,
	}, nil
}

// Start begins listening for telemetry connections
func (s *TelemetryServer) Start() error {
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", s.addr, err)
	}

	s.listener = listener
	fmt.Printf("Telemetry server listening on %s\n", s.addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			// Check if server was stopped intentionally
			if s.stopped {
				fmt.Printf("Telemetry server stopped\n")
				return nil
			}
			fmt.Printf("Failed to accept connection: %v\n", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

// Stop stops the telemetry server and closes the log file
func (s *TelemetryServer) Stop() error {
	s.stopped = true
	if s.listener != nil {
		s.listener.Close()
	}
	if s.logFile != nil {
		return s.logFile.Close()
	}
	return nil
}

// handleConnection processes a single telemetry client connection
func (s *TelemetryServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	fmt.Printf("New telemetry connection from %s\n", conn.RemoteAddr())

	// Read the initial node information message
	if err := s.readNodeInfo(conn); err != nil {
		fmt.Printf("Failed to read node info: %v\n", err)
		return
	}

	// Process telemetry events
	for {
		if err := s.readAndProcessEvent(conn); err != nil {
			if err == io.EOF {
				fmt.Printf("Telemetry connection from %s closed\n", conn.RemoteAddr())
			} else {
				fmt.Printf("Error reading telemetry event: %v\n", err)
			}
			break
		}
	}
}

// readNodeInfo reads the initial node information message
func (s *TelemetryServer) readNodeInfo(conn net.Conn) error {
	// Read message length
	var length uint32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return fmt.Errorf("failed to read node info length: %w", err)
	}

	// Read message content
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return fmt.Errorf("failed to read node info content: %w", err)
	}

	// Log node info (simplified - just log that we received it)
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000000Z")
	s.logger.Printf("%s|NODE_INFO|%d bytes received from %s", timestamp, length, conn.RemoteAddr())

	return nil
}

// readAndProcessEvent reads a single telemetry event and processes it
func (s *TelemetryServer) readAndProcessEvent(conn net.Conn) error {
	// Read message length (4 bytes, little-endian)
	var length uint32
	if err := binary.Read(conn, binary.LittleEndian, &length); err != nil {
		return err
	}

	// Read message content
	data := make([]byte, length)
	if _, err := io.ReadFull(conn, data); err != nil {
		return fmt.Errorf("failed to read event content: %w", err)
	}

	// Parse timestamp (first 8 bytes, little-endian)
	if len(data) < 9 {
		return fmt.Errorf("event data too short: %d bytes", len(data))
	}

	timestamp := binary.LittleEndian.Uint64(data[:8])
	discriminator := data[8]
	payload := data[9:]

	// Convert JAM timestamp to readable format
	jamTime := common.JceStart.Add(time.Duration(timestamp) * time.Microsecond)
	timeStr := jamTime.UTC().Format("2006-01-02T15:04:05.000000Z")

	// Process the event based on discriminator
	s.processEvent(timeStr, discriminator, payload, conn.RemoteAddr().String())

	return nil
}

// logEvent logs a telemetry event with structured format
func (s *TelemetryServer) logEvent(timestamp string, peerAddr string, eventType string, decodedData string) {
	s.logger.Printf("%s|%s|%s|peer:%s", timestamp, eventType, decodedData, peerAddr)
}

// processEvent handles different event types based on discriminator using lookup maps
func (s *TelemetryServer) processEvent(timestamp string, discriminator byte, payload []byte, peerAddr string) {
	discriminatorInt := int(discriminator)

	// Check if both the event string and decoder function exist for this discriminator
	eventType, hasEventType := discriminatorToString[discriminatorInt]
	decoder, hasDecoder := discriminatorDecoder[discriminatorInt]

	if hasEventType && hasDecoder {
		// Use the decoder function to get the decoded data string
		decodedData := decoder(payload)
		s.logEvent(timestamp, peerAddr, eventType, decodedData)
	} else {
		// Handle unknown events
		s.logEvent(timestamp, peerAddr, "UNKNOWN_EVENT", fmt.Sprintf("discriminator:%d|raw_data:0x%x", discriminator, payload))
	}
}

// DecodeEvent decodes a telemetry event payload using the appropriate decoder.
// Returns the decoded string, or empty string if no decoder exists for this discriminator.
func DecodeEvent(discriminator int, payload []byte) string {
	if decoder, ok := discriminatorDecoder[discriminator]; ok {
		return decoder(payload)
	}
	return ""
}

// GetEventTypeName returns the event type name for a discriminator
func GetEventTypeName(discriminator int) string {
	if name, ok := discriminatorToString[discriminator]; ok {
		return name
	}
	return ""
}
