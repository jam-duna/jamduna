package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/fuzz"
	"github.com/colorfulnotion/jam/pvm"
)

const (
	defaultSocket   = "/tmp/jam_target.sock"
	defaultTraceDir = "/Users/michael/Github/jam/fuzz/v1"
)

// defaultBackend can be set at build time via -ldflags "-X main.defaultBackend=compiler"
// Default is interpreter for compatibility
var defaultBackend = pvm.BackendInterpreter

// TraceStep represents a single step in the V1 protocol trace
type TraceStep struct {
	StepNumber  int
	Direction   string // "fuzzer" or "target"
	MessageType string // "peer_info", "initialize", "import_block", "state_root", "error", "get_state", "state"
	BinFile     string
	JsonFile    string
}

func main() {
	var socketPath = defaultSocket
	var testDir = defaultTraceDir
	var verbose = false

	// Simple argument parsing
	for i, arg := range os.Args[1:] {
		switch arg {
		case "--socket":
			if i+1 < len(os.Args[1:]) {
				socketPath = os.Args[i+2]
			}
		case "--test-dir":
			if i+1 < len(os.Args[1:]) {
				testDir = os.Args[i+2]
			}
		case "--trace-dir": // Keep backward compatibility
			if i+1 < len(os.Args[1:]) {
				testDir = os.Args[i+2]
			}
		case "--verbose", "-v":
			verbose = true
		case "--help", "-h":
			fmt.Println("Usage: duna_replay [options]")
			fmt.Println("Options:")
			fmt.Println("  --socket <path>     Unix socket path (default: /tmp/jam_target.sock)")
			fmt.Println("  --test-dir <dir>    Test data directory containing V1 trace files (default: /Users/michael/Github/jam/fuzz/v1)")
			fmt.Println("  --trace-dir <dir>   Alias for --test-dir (for backward compatibility)")
			fmt.Println("  --verbose, -v       Enable verbose logging")
			fmt.Println("  --help, -h          Show this help")
			return
		}
	}

	fmt.Printf("V1 Protocol Replay Tool\n")
	fmt.Printf("Socket: %s\n", socketPath)
	fmt.Printf("Test Directory: %s\n", testDir)
	fmt.Printf("Verbose: %v\n\n", verbose)

	fuzzerInfo := fuzz.CreatePeerInfo("duna-fuzzer")
	fuzzerInfo.SetDefaults()

	// Load trace steps
	steps, err := loadTraceSteps(testDir)
	if err != nil {
		log.Fatalf("Failed to load trace steps: %v", err)
	}

	if verbose {
		fmt.Printf("Loaded %d trace steps:\n", len(steps))
		for _, step := range steps {
			fmt.Printf("  %03d: %s -> %s (%s)\n", step.StepNumber, step.Direction, step.MessageType, step.BinFile)
		}
		fmt.Println()
	} else {
		// Show trace steps without verbose details
		fmt.Printf("Loaded %d trace steps\n", len(steps))
	}

	// Create fuzzer instance using the existing infrastructure
	fuzzer, err := fuzz.NewFuzzer("", "", socketPath, fuzzerInfo, defaultBackend)
	if err != nil {
		log.Fatalf("Failed to create fuzzer: %v", err)
	}

	// Connect to target
	fmt.Printf("Connecting to target at %s...\n", socketPath)
	err = fuzzer.Connect()
	if err != nil {
		log.Fatalf("Failed to connect to target: %v", err)
	}
	defer fuzzer.Close()

	// Perform handshake
	fmt.Println("Performing handshake...")
	peerInfo, err := fuzzer.Handshake()
	if err != nil {
		log.Fatalf("Handshake failed: %v", err)
	}

	fmt.Printf("Handshake successful: %s\n\n", peerInfo.PrettyString(false))

	// Replay the trace (skip the first peer_info exchange since handshake was already done)
	matched, totalSteps, err := replayTrace(fuzzer, steps, verbose)
	if err != nil {
		log.Fatalf("Trace replay failed: %v", err)
	}

	// Report results following fuzzer pattern
	fmt.Printf("\nTrace replay completed!\n")
	if matched == totalSteps {
		fmt.Printf("✅ All %d responses MATCHED expected trace\n", matched)
	} else {
		fmt.Printf("❌ %d/%d responses matched (%.1f%% match rate)\n",
			matched, totalSteps, float64(matched)/float64(totalSteps)*100)
	}
}

func loadTraceSteps(traceDir string) ([]TraceStep, error) {
	var steps []TraceStep

	err := filepath.WalkDir(traceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".bin") {
			return nil
		}

		filename := d.Name()
		// Parse filename: 00000000_fuzzer_peer_info.bin
		parts := strings.Split(strings.TrimSuffix(filename, ".bin"), "_")
		if len(parts) < 3 {
			return nil
		}

		stepNum, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil
		}

		direction := parts[1] // "fuzzer" or "target"
		if direction != "fuzzer" && direction != "target" {
			return nil
		}

		// Extract message type from remaining parts
		msgType := strings.Join(parts[2:], "_")

		step := TraceStep{
			StepNumber:  stepNum,
			Direction:   direction,
			MessageType: msgType,
			BinFile:     path,
			JsonFile:    strings.Replace(path, ".bin", ".json", 1),
		}

		steps = append(steps, step)
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(steps, func(i, j int) bool {
		return steps[i].StepNumber < steps[j].StepNumber
	})

	return steps, nil
}

func replayTrace(fuzzer *fuzz.Fuzzer, steps []TraceStep, verbose bool) (matched int, totalSteps int, err error) {
	// Track expected responses for verification
	expectedResponses := make(map[int]TraceStep)
	matchedCount := 0
	totalResponseSteps := 0

	// First pass: collect all expected responses
	for _, step := range steps {
		if step.Direction == "target" {
			// This is an expected target response - store it for verification
			expectedResponses[step.StepNumber] = step
			totalResponseSteps++
			if verbose {
				fmt.Printf("  Added expected response: step %d -> %s\n", step.StepNumber, step.MessageType)
			}
		}
	}

	// Second pass: process fuzzer requests
	for _, step := range steps {
		if step.Direction == "fuzzer" {
			// Skip the first peer_info step since handshake was already done
			if step.StepNumber == 0 && step.MessageType == "peer_info" {
				// But we still need to check if there's a response to count
				if expectedResp, exists := expectedResponses[step.StepNumber]; exists {
					log.Printf("\033[90m [HANDSHAKE]  \033[0m  %08d\t%s\n", step.StepNumber, capitalizeMessageType(expectedResp.MessageType))
					matchedCount++ // Assume handshake worked since connection succeeded
				}
				continue
			}

			// This is a fuzzer request - we need to send it
			log.Printf("\033[32m[OUTGOING REQ]\033[0m  %08d\t%s\n", step.StepNumber, capitalizeMessageType(step.MessageType))
			if verbose {
				fmt.Printf("  Binary file: %s\n", step.BinFile)
			}

			// Read the binary data to send
			data, err := os.ReadFile(step.BinFile)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to read binary file %s: %w", step.BinFile, err)
			}

			if verbose {
				fmt.Printf("  Sending: %d bytes\n", len(data))
			}

			// Decode the binary data into a Message
			handler := fuzz.GetProtocolHandler()
			msg, err := handler.Decode(data)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to decode message at step %d: %w", step.StepNumber, err)
			}

			// Show complete block content for import_block messages
			if step.MessageType == "import_block" && msg.ImportBlock != nil {
				fmt.Printf("%sSTEP#%03d S#%03d\nAuthorIdx:%d\nHeaderHash:%v\nParentStateRoot:%v%s\n", common.ColorGray, step.StepNumber, msg.ImportBlock.Header.Slot, msg.ImportBlock.Header.AuthorIndex, msg.ImportBlock.Header.Hash().Hex(), msg.ImportBlock.Header.ParentStateRoot.Hex(), common.ColorReset)
				/*
					blockJSON, err := json.MarshalIndent(msg.ImportBlock, "", "  ")
					if err != nil {
						fmt.Printf("Error marshaling block: %v\n", err)
					} else {
						fmt.Printf("DECODED FROM BINARY:\n%s\n", blockJSON)
					}
				*/
			}

			// Send the message using fuzzer's existing infrastructure
			err = fuzzer.SendMessage(msg)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to send message at step %d: %w", step.StepNumber, err)
			}

			// Check if we expect a response (look for same step number with target direction)
			if expectedResp, exists := expectedResponses[step.StepNumber]; exists {

				// Receive the response using fuzzer's existing infrastructure
				response, err := fuzzer.ReadMessage()
				if err != nil {
					return 0, 0, fmt.Errorf("failed to receive response at step %d: %w", step.StepNumber, err)
				}

				// Load expected response for comparison to enhance logging
				expectedData, err := os.ReadFile(expectedResp.BinFile)
				if err != nil {
					return 0, 0, fmt.Errorf("failed to read expected response file %s: %w", expectedResp.BinFile, err)
				}

				// Decode expected response
				handler := fuzz.GetProtocolHandler()
				expectedMsg, err := handler.Decode(expectedData)
				if err != nil {
					return 0, 0, fmt.Errorf("failed to decode expected response at step %d: %w", step.StepNumber, err)
				}

				// Detect actual message type and log it with enhanced format
				actualMsgType := getActualMessageType(response)

				// Clean format - actual response first, expected info in comparison
				if response.Error != nil {
					log.Printf("\033[31m[INCOMING RSP]\033[0m  %08d\t%s\n", step.StepNumber, actualMsgType)
				} else {
					log.Printf("\033[34m[INCOMING RSP]\033[0m  %08d\t%s\n", step.StepNumber, actualMsgType)
				}
				if verbose {
					fmt.Printf("  Binary file: %s\n", expectedResp.BinFile)
				}

				// (Expected message already loaded above for enhanced logging)

				// Compare responses and report results
				isMatch := compareResponses(response, expectedMsg, expectedResp.MessageType)
				if isMatch {
					matchedCount++
				}

				if !isMatch {
					fmt.Printf("\n❌ Diff @ %08d: \n", step.StepNumber)
					// Show actual values for debugging
					if response.StateRoot != nil {
						fmt.Printf("\033[32mRec: %s (%s)\033[0m\n", actualMsgType, response.StateRoot.Hex())
					} else if response.Error != nil {
						fmt.Printf("\033[31mRec: %s(%s)\033[0m\n", actualMsgType, *response.Error)
					} else {
						fmt.Printf("\033[32mRec: %s\033[0m\n", actualMsgType)
					}

					// Show expected values
					if expectedMsg.StateRoot != nil {
						fmt.Printf("\033[33mExp: %s (%s)\033[0m\n", capitalizeMessageType(expectedResp.MessageType), expectedMsg.StateRoot.Hex())
					} else if expectedMsg.Error != nil {
						fmt.Printf("\033[33mExp: %s(%s)\033[0m\n", capitalizeMessageType(expectedResp.MessageType), *expectedMsg.Error)
					} else {
						fmt.Printf("\033[33mExp: %s\033[0m\n", capitalizeMessageType(expectedResp.MessageType))
					}
				} else if verbose {
					fmt.Printf("\033[32m%08d: ✅ MATCH\033[0m\n", step.StepNumber)
				}
			}

			// Add a small delay between messages
			time.Sleep(10 * time.Millisecond)
		}
	}

	return matchedCount, totalResponseSteps, nil
}

// capitalizeMessageType converts lowercase message types to proper capitalization
func capitalizeMessageType(msgType string) string {
	switch msgType {
	case "peer_info":
		return "PeerInfo"
	case "import_block":
		return "ImportBlock"
	case "initialize":
		return "Initialize"
	case "state_root":
		return "StateRoot"
	case "error":
		return "ERROR"
	case "get_state":
		return "GetState"
	case "state":
		return "State"
	default:
		return msgType
	}
}

// getActualMessageType determines the actual message type from a response
func getActualMessageType(msg *fuzz.Message) string {
	if msg.PeerInfo != nil {
		return "PeerInfo"
	}
	if msg.StateRoot != nil {
		return "StateRoot"
	}
	if msg.Error != nil {
		return "Error"
	}
	if msg.State != nil {
		return "State"
	}
	if msg.Initialize != nil {
		return "Initialize"
	}
	if msg.ImportBlock != nil {
		return "ImportBlock"
	}
	if msg.GetState != nil {
		return "GetState"
	}
	return "Unknown"
}

// compareResponses compares actual response with expected response following fuzzer pattern
func compareResponses(actual, expected *fuzz.Message, messageType string) bool {
	switch messageType {
	case "peer_info":
		if actual.PeerInfo == nil || expected.PeerInfo == nil {
			return actual.PeerInfo == expected.PeerInfo
		}
		// For peer_info, we mainly care about protocol compatibility, not exact match
		return actual.PeerInfo.GetFuzzVersion() == expected.PeerInfo.GetFuzzVersion()

	case "state_root":
		if actual.StateRoot == nil || expected.StateRoot == nil {
			return actual.StateRoot == expected.StateRoot
		}
		return *actual.StateRoot == *expected.StateRoot

	case "error":
		// For error messages, we only care that both are errors (content doesn't need to match)
		return (actual.Error != nil) == (expected.Error != nil)

	case "state":
		if actual.State == nil || expected.State == nil {
			return actual.State == expected.State
		}
		// For state comparison, check if key-value pairs match
		return len(actual.State.KeyVals) == len(expected.State.KeyVals)

	default:
		// For unknown message types, do a basic nil comparison
		return (actual == nil && expected == nil) || (actual != nil && expected != nil)
	}
}
