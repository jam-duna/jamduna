package telemetry

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

func TestTelemetry(t *testing.T) {
	// Set a 3 second timeout for the entire test
	timeout := time.After(3 * time.Second)
	done := make(chan bool)

	go func() {
		// Create temporary log file
		logFile, err := os.CreateTemp("", "telemetry_test_*.log")
		if err != nil {
			t.Fatalf("Failed to create temp log file: %v", err)
		}
		// Log file will be kept for inspection at: %s
		t.Logf("Log file created at: %s", logFile.Name())
		logFile.Close()

		// Set up telemetry server
		server, err := NewTelemetryServer("localhost:0", logFile.Name())
		if err != nil {
			t.Fatalf("Failed to create telemetry server: %v", err)
		}
		defer server.Stop()

		// Start server in background
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := server.Start(); err != nil {
				t.Errorf("Server failed to start: %v", err)
			}
		}()

		// Give server time to start
		time.Sleep(100 * time.Millisecond)

		// Get server address from listener
		serverAddr := server.listener.Addr().String()

		// Create telemetry client
		host, port, err := parseHostPort(serverAddr)
		if err != nil {
			t.Fatalf("Failed to parse server address: %v", err)
		}
		client := NewTelemetryClient(host, port)

		// Create test node info
		nodeInfo := NodeInfo{
			JAMParameters:     []byte("test_params"),
			GenesisHeaderHash: common.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
			PeerID:            [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32},
			PeerAddress:       [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1}, // IPv4-mapped IPv6 for 127.0.0.1
			PeerPort:          8080,
			NodeFlags:         1, // PVM recompiler
			NodeName:          "TestNode",
			NodeVersion:       "1.0.0",
			GrayPaperVersion:  "0.7.1",
			Note:              "Test telemetry node",
		}

		// Connect client to server
		if err := client.Connect(nodeInfo); err != nil {
			t.Fatalf("Failed to connect client to server: %v", err)
		}
		defer client.Close()

		// Send test events for each discriminator in the map using TelemetryClient methods
		expectedEvents := make(map[string]bool)

		for discriminator, eventType := range discriminatorToString {
			// Send event using appropriate TelemetryClient method
			if err := sendTestEvent(client, discriminator); err != nil {
				t.Errorf("Failed to send event %s (%d): %v", eventType, discriminator, err)
				continue
			}

			// Track expected event
			expectedEvents[eventType] = false
		}

		// Give time for events to be processed and written to log
		time.Sleep(500 * time.Millisecond)

		// Stop server to ensure all logs are flushed
		server.Stop()
		wg.Wait()

		// Read and parse log file
		logContent, err := os.ReadFile(logFile.Name())
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}

		// Parse log lines and verify events
		lines := strings.Split(string(logContent), "\n")
		receivedEvents := make(map[string]int)

		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}

			// Parse log line format: timestamp|event_type|decoded_data|peer:address
			parts := strings.Split(line, "|")
			if len(parts) < 2 {
				continue
			}

			eventType := parts[1]

			// Skip NODE_INFO events as they're not part of our test
			if eventType == "NODE_INFO" {
				continue
			}

			receivedEvents[eventType]++
		}

		// Verify we received exactly one log line for each expected event
		for eventType := range expectedEvents {
			count, found := receivedEvents[eventType]
			if !found {
				t.Errorf("Expected event %s was not found in log", eventType)
			} else if count != 1 {
				t.Errorf("Expected exactly 1 log line for event %s, got %d", eventType, count)
			}
		}

		// Verify we didn't receive any unexpected events
		for eventType, count := range receivedEvents {
			if _, expected := expectedEvents[eventType]; !expected {
				t.Errorf("Unexpected event %s found in log (%d times)", eventType, count)
			}
		}

		t.Logf("Successfully tested %d telemetry events", len(expectedEvents))
		done <- true
	}()

	// Wait for either completion or timeout
	select {
	case <-done:
		// Test completed successfully
	case <-timeout:
		t.Fatal("Test timed out after 3 seconds")
	}
}

// sendTestEvent sends a test event using the appropriate TelemetryClient method
func sendTestEvent(client *TelemetryClient, discriminator int) error {
	// Create test data
	testPeerID := [32]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	testHash := common.Hash{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	testAddress := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 127, 0, 0, 1} // IPv4-mapped IPv6 for 127.0.0.1
	testEventID := client.GetEventID()

	// Create test structures
	testBlockOutline := BlockOutline{
		HeaderHash: testHash,
	}
	testWorkPackageOutline := WorkPackageOutline{
		WorkPackageHash: testHash,
	}

	switch discriminator {
	// Meta events
	case Telemetry_Dropped:
		client.DroppedEvents(12345, 5)
		return nil

	// Status events (10-13)
	case Telemetry_Status:
		client.Status(10, 5, 3, []byte{1, 2, 3, 4}, 100, 1000, 50, 500)
		return nil
	case Telemetry_Best_Block_Changed:
		client.BestBlockChanged(12345, testHash)
		return nil
	case Telemetry_Finalized_Block_Changed:
		client.FinalizedBlockChanged(12345, testHash)
		return nil
	case Telemetry_Sync_Status_Changed:
		client.SyncStatusChanged(true)
		return nil

	// Networking events (20-28)
	case Telemetry_Connection_Refused:
		client.ConnectionRefused(testAddress, 8080)
		return nil
	case Telemetry_Connecting_In:
		client.ConnectingIn(testAddress, 8080)
		return nil
	case Telemetry_Connect_In_Failed:
		client.ConnectInFailed(testEventID, "test error")
		return nil
	case Telemetry_Connected_In:
		client.ConnectedIn(testEventID, testPeerID)
		return nil
	case Telemetry_Connecting_Out:
		client.ConnectingOut(testPeerID, testAddress, 8080)
		return nil
	case Telemetry_Connect_Out_Failed:
		client.ConnectOutFailed(testEventID, "test error")
		return nil
	case Telemetry_Connected_Out:
		client.ConnectedOut(testEventID)
		return nil
	case Telemetry_Disconnected:
		var connectionSide byte = 0
		client.Disconnected(testPeerID, &connectionSide, "test disconnect")
		return nil
	case Telemetry_Peer_Misbehaved:
		client.PeerMisbehaved(testPeerID, "test misbehavior")
		return nil

	// Block events (40-48)
	case Telemetry_Authoring:
		client.Authoring(12345, testHash)
		return nil
	case Telemetry_Authoring_Failed:
		client.AuthoringFailed(testEventID, "test failure")
		return nil
	case Telemetry_Authored:
		client.Authored(testEventID, testBlockOutline)
		return nil
	case Telemetry_Importing:
		client.Importing(12345, testBlockOutline)
		return nil
	case Telemetry_Block_Verification_Failed:
		client.BlockVerificationFailed(testEventID, "test failure")
		return nil
	case Telemetry_Block_Verified:
		client.BlockVerified(testEventID)
		return nil
	case Telemetry_Block_Execution_Failed:
		client.BlockExecutionFailed(testEventID, "test failure")
		return nil
	case Telemetry_Block_Executed:
		client.BlockExecuted(testEventID, []ServiceAccumulateCost{})
		return nil
	case Telemetry_Accumulate_Result_Available:
		client.AccumulateResultAvailable(12345, testHash)
		return nil

	// Block announcement events (60-68, 71)
	case Telemetry_Block_Announcement_Stream_Opened:
		client.BlockAnnouncementStreamOpened(testPeerID, 0)
		return nil
	case Telemetry_Block_Announcement_Stream_Closed:
		client.BlockAnnouncementStreamClosed(testPeerID, 0, "test close")
		return nil
	case Telemetry_Block_Announced:
		client.BlockAnnounced(testPeerID, 0, 12345, testHash)
		return nil
	case Telemetry_Sending_Block_Request:
		client.SendingBlockRequest(testPeerID, testHash, 0, 10)
		return nil
	case Telemetry_Receiving_Block_Request:
		client.ReceivingBlockRequest(testPeerID)
		return nil
	case Telemetry_Block_Request_Failed:
		client.BlockRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Block_Request_Sent:
		client.BlockRequestSent(testEventID)
		return nil
	case Telemetry_Block_Request_Received:
		client.BlockRequestReceived(testEventID, testHash, 0, 10)
		return nil
	case Telemetry_Block_Transferred:
		client.BlockTransferred(testEventID, 12345, testBlockOutline, true)
		return nil
	case Telemetry_Block_Announcement_Malformed:
		client.BlockAnnouncementMalformed(testPeerID, "test malformed")
		return nil

	// Safrole events (80-84)
	case Telemetry_Generating_Tickets:
		client.GeneratingTickets(12345)
		return nil
	case Telemetry_Ticket_Generation_Failed:
		client.TicketGenerationFailed(testEventID, "test failure")
		return nil
	case Telemetry_Tickets_Generated:
		client.TicketsGenerated(testEventID, [][32]byte{testHash})
		return nil
	case Telemetry_Ticket_Transfer_Failed:
		client.TicketTransferFailed(testPeerID, 0, false, "test failure")
		return nil
	case Telemetry_Ticket_Transferred:
		client.TicketTransferred(testPeerID, 0, false, 12345, 1, testHash)
		return nil

	// Work package/guarantee events (90-113)
	case Telemetry_Work_Package_Submission:
		client.WorkPackageSubmission(testEventID, testPeerID, testWorkPackageOutline)
		return nil
	case Telemetry_Work_Package_Being_Shared:
		client.WorkPackageBeingShared(testPeerID)
		return nil
	case Telemetry_Work_Package_Failed:
		client.WorkPackageFailed(testEventID, "test failure")
		return nil
	case Telemetry_Duplicate_Work_Package:
		client.DuplicateWorkPackage(testEventID, 0, testWorkPackageOutline.WorkPackageHash)
		return nil
	case Telemetry_Work_Package_Received:
		client.WorkPackageReceived(testEventID, testWorkPackageOutline)
		return nil
	case Telemetry_Authorized:
		testCost := IsAuthorizedCost{TotalGasUsed: 1000, TotalTimeNs: 5000, LoadCompileTimeNs: 100, HostCallsGasUsed: 50, HostCallsTimeNs: 200}
		client.Authorized(testEventID, testCost)
		return nil
	case Telemetry_Extrinsic_Data_Received:
		client.ExtrinsicDataReceived(testEventID)
		return nil
	case Telemetry_Imports_Received:
		client.ImportsReceived(testEventID)
		return nil
	case Telemetry_Sharing_Work_Package:
		client.SharingWorkPackage(testEventID, testPeerID, testWorkPackageOutline)
		return nil
	case Telemetry_Work_Package_Sharing_Failed:
		client.WorkPackageSharingFailed(testEventID, testPeerID, "test failure")
		return nil
	case Telemetry_Bundle_Sent:
		client.BundleSent(testEventID, testPeerID)
		return nil
	case Telemetry_Refined:
		testRefineCosts := []RefineCost{{TotalGasUsed: 500, TotalTimeNs: 2000, LoadCompileTimeNs: 100, HistoricalLookupGas: 25, HistoricalLookupTimeNs: 50}}
		client.Refined(testEventID, testRefineCosts)
		return nil
	case Telemetry_Work_Report_Built:
		testWorkReportOutline := WorkReportOutline{
			WorkReportHash: testHash,
			BundleSize:     1024,
			ErasureRoot:    testHash,
			SegmentsRoot:   testHash,
		}
		client.WorkReportBuilt(testEventID, testWorkReportOutline)
		return nil
	case Telemetry_Work_Report_Signature_Sent:
		client.WorkReportSignatureSent(testEventID)
		return nil
	case Telemetry_Work_Report_Signature_Received:
		client.WorkReportSignatureReceived(testEventID, testPeerID, testHash)
		return nil

	case Telemetry_Guarantee_Built:
		testGuaranteeOutline := GuaranteeOutline{
			WorkReportHash: testHash,
			Slot:           12345,
			Guarantors:     []uint16{0, 1, 2},
		}
		client.GuaranteeBuilt(testEventID, testGuaranteeOutline)
		return nil
	case Telemetry_Sending_Guarantee:
		client.SendingGuarantee(testEventID, testPeerID)
		return nil
	case Telemetry_Guarantee_Send_Failed:
		client.GuaranteeSendFailed(testEventID, "test failure")
		return nil
	case Telemetry_Guarantee_Sent:
		client.GuaranteeSent(testEventID)
		return nil
	case Telemetry_Guarantees_Distributed:
		client.GuaranteesDistributed(testEventID)
		return nil
	case Telemetry_Receiving_Guarantee:
		client.ReceivingGuarantee(testPeerID)
		return nil
	case Telemetry_Guarantee_Receive_Failed:
		client.GuaranteeReceiveFailed(testEventID, "test failure")
		return nil
	case Telemetry_Guarantee_Received:
		testGuaranteeOutline := GuaranteeOutline{
			WorkReportHash: testHash,
			Slot:           12345,
			Guarantors:     []uint16{0, 1, 2},
		}
		client.GuaranteeReceived(testEventID, testGuaranteeOutline)
		return nil
	case Telemetry_Guarantee_Discarded:
		testGuaranteeOutline := GuaranteeOutline{
			WorkReportHash: testHash,
			Slot:           12345,
			Guarantors:     []uint16{0, 1, 2},
		}
		client.GuaranteeDiscarded(testGuaranteeOutline, 1)
		return nil

	// Assurance events (120-133)
	case Telemetry_Sending_Shard_Request:
		client.SendingShardRequest(testPeerID, testHash, 0)
		return nil
	case Telemetry_Receiving_Shard_Request:
		client.ReceivingShardRequest(testPeerID)
		return nil
	case Telemetry_Shard_Request_Failed:
		client.ShardRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Shard_Request_Sent:
		client.ShardRequestSent(testEventID)
		return nil
	case Telemetry_Shard_Request_Received:
		client.ShardRequestReceived(testEventID, testHash, 0)
		return nil
	case Telemetry_Shards_Transferred:
		client.ShardsTransferred(testEventID)
		return nil
	case Telemetry_Distributing_Assurance:
		client.DistributingAssurance(testHash, []byte{1, 2, 3, 4})
		return nil
	case Telemetry_Assurance_Send_Failed:
		client.AssuranceSendFailed(testEventID, testPeerID, "test failure")
		return nil
	case Telemetry_Assurance_Sent:
		client.AssuranceSent(testEventID, testPeerID)
		return nil
	case Telemetry_Assurance_Distributed:
		client.AssuranceDistributed(testEventID)
		return nil
	case Telemetry_Assurance_Receive_Failed:
		client.AssuranceReceiveFailed(testPeerID, "test failure")
		return nil
	case Telemetry_Assurance_Received:
		client.AssuranceReceived(testPeerID, testHash)
		return nil
	case Telemetry_Context_Available:
		testSpec := types.AvailabilitySpecifier{WorkPackageHash: testHash, BundleLength: 1024, ErasureRoot: testHash, ExportedSegmentRoot: testHash, ExportedSegmentLength: 100}
		client.ContextAvailable(testHash, 0, 12345, testSpec)
		return nil
	case Telemetry_Assurance_Provided:
		testAssurance := types.Assurance{Anchor: testHash, Bitfield: [1]byte{0xFF}, ValidatorIndex: 0}
		client.AssuranceProvided(testAssurance)
		return nil

	// Bundle recovery events (140-153)
	case Telemetry_Sending_Bundle_Shard_Request:
		client.SendingBundleShardRequest(testEventID, testPeerID, 0)
		return nil
	case Telemetry_Receiving_Bundle_Shard_Request:
		client.ReceivingBundleShardRequest(testPeerID)
		return nil
	case Telemetry_Bundle_Shard_Request_Failed:
		client.BundleShardRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Bundle_Shard_Request_Sent:
		client.BundleShardRequestSent(testEventID)
		return nil
	case Telemetry_Bundle_Shard_Request_Received:
		client.BundleShardRequestReceived(testEventID, testHash, 0)
		return nil
	case Telemetry_Bundle_Shard_Transferred:
		client.BundleShardTransferred(testEventID)
		return nil
	case Telemetry_Reconstructing_Bundle:
		client.ReconstructingBundle(testEventID, false)
		return nil
	case Telemetry_Bundle_Reconstructed:
		client.BundleReconstructed(testEventID)
		return nil
	case Telemetry_Sending_Bundle_Request:
		client.SendingBundleRequest(testEventID, testPeerID)
		return nil
	case Telemetry_Receiving_Bundle_Request:
		client.ReceivingBundleRequest(testPeerID)
		return nil
	case Telemetry_Bundle_Request_Failed:
		client.BundleRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Bundle_Request_Sent:
		client.BundleRequestSent(testEventID)
		return nil
	case Telemetry_Bundle_Request_Received:
		client.BundleRequestReceived(testEventID, testHash)
		return nil
	case Telemetry_Bundle_Transferred:
		client.BundleTransferred(testEventID)
		return nil

	// Segment events (160-178)
	case Telemetry_Work_Package_Hash_Mapped:
		client.WorkPackageHashMapped(testEventID, testHash, testHash)
		return nil
	case Telemetry_Segments_Root_Mapped:
		client.SegmentsRootMapped(testEventID, testHash, testHash)
		return nil
	case Telemetry_Sending_Segment_Shard_Request:
		testSegmentRequest := SegmentShardRequest{ImportSegmentID: 0, ShardIndex: 0}
		client.SendingSegmentShardRequest(testEventID, testPeerID, false, []SegmentShardRequest{testSegmentRequest})
		return nil
	case Telemetry_Receiving_Segment_Shard_Request:
		client.ReceivingSegmentShardRequest(testPeerID, false)
		return nil
	case Telemetry_Segment_Shard_Request_Failed:
		client.SegmentShardRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Segment_Shard_Request_Sent:
		client.SegmentShardRequestSent(testEventID)
		return nil
	case Telemetry_Segment_Shard_Request_Received:
		client.SegmentShardRequestReceived(testEventID, 1)
		return nil
	case Telemetry_Segment_Shards_Transferred:
		client.SegmentShardsTransferred(testEventID)
		return nil
	case Telemetry_Reconstructing_Segments:
		client.ReconstructingSegments(testEventID, []uint16{0, 1}, false)
		return nil
	case Telemetry_Segment_Reconstruction_Failed:
		client.SegmentReconstructionFailed(testEventID, "test failure")
		return nil
	case Telemetry_Segments_Reconstructed:
		client.SegmentsReconstructed(testEventID)
		return nil
	case Telemetry_Segment_Verification_Failed:
		client.SegmentVerificationFailed(testEventID, []uint16{0}, "test failure")
		return nil
	case Telemetry_Segments_Verified:
		client.SegmentsVerified(testEventID, []uint16{0, 1})
		return nil
	case Telemetry_Sending_Segment_Request:
		client.SendingSegmentRequest(testEventID, testPeerID, []uint16{0, 1})
		return nil
	case Telemetry_Receiving_Segment_Request:
		client.ReceivingSegmentRequest(testPeerID)
		return nil
	case Telemetry_Segment_Request_Failed:
		client.SegmentRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Segment_Request_Sent:
		client.SegmentRequestSent(testEventID)
		return nil
	case Telemetry_Segment_Request_Received:
		client.SegmentRequestReceived(testEventID, 0)
		return nil
	case Telemetry_Segments_Transferred:
		client.SegmentsTransferred(testEventID)
		return nil

	// Preimage events (190-199)
	case Telemetry_Preimage_Announcement_Failed:
		client.PreimageAnnouncementFailed(testPeerID, 0, "test failure")
		return nil
	case Telemetry_Preimage_Announced:
		client.PreimageAnnounced(testPeerID, 0, 1, testHash, 1024)
		return nil
	case Telemetry_Announced_Preimage_Forgotten:
		client.AnnouncedPreimageForgotten(1, testHash, 1024, 1)
		return nil
	case Telemetry_Sending_Preimage_Request:
		client.SendingPreimageRequest(testPeerID, testHash)
		return nil
	case Telemetry_Receiving_Preimage_Request:
		client.ReceivingPreimageRequest(testPeerID)
		return nil
	case Telemetry_Preimage_Request_Failed:
		client.PreimageRequestFailed(testEventID, "test failure")
		return nil
	case Telemetry_Preimage_Request_Sent:
		client.PreimageRequestSent(testEventID)
		return nil
	case Telemetry_Preimage_Request_Received:
		client.PreimageRequestReceived(testEventID, testHash)
		return nil
	case Telemetry_Preimage_Transferred:
		client.PreimageTransferred(testEventID, 1024)
		return nil
	case Telemetry_Preimage_Discarded:
		client.PreimageDiscarded(testHash, 1024, 1)
		return nil

	default:
		// For events we haven't implemented yet, skip them to avoid test failure
		// In a real implementation, all events should have corresponding methods
		return fmt.Errorf("no test method implemented for discriminator %d", discriminator)
	}
}

// parseHostPort parses "host:port" string
func parseHostPort(addr string) (string, string, error) {
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid address format: %s", addr)
	}
	return parts[0], parts[1], nil
}
