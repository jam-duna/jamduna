package node

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/colorfulnotion/jam/common"
	grandpa "github.com/colorfulnotion/jam/grandpa"
	"github.com/colorfulnotion/jam/types"
	"github.com/quic-go/quic-go"
)

/*
CE 149: GRANDPA CatchUp
Catchup Request. This is sent by a voting validator to another validator if the first validator
determines that it is behind the other validator by a threshold number of rounds (currently set to
2 rounds). The response includes all votes from the last completed round of the responding validator.
Base Hash and Base Number refer to the base, which is a block all vote targets are a descendent of.
If the responding voter is unable to send the response it should stop the stream.

Base Hash = Header Hash
Base Number = Slot
Catchup = Round Number ++ len++[Signed Prevote] ++ len++[Signed Precommit] ++ Base Hash ++ Base Number

Validator -> Validator

--> Round Number ++ Set Id
--> FIN
<-- Catchup
<-- FIN
*/

// CatchUpRequest represents the request sent by a validator that is behind
type CatchUpRequest struct {
	RoundNumber uint64 // Round number
	SetID       uint64 // Authority set ID
}

// ToBytes encodes a CatchUpRequest to bytes
func (r *CatchUpRequest) ToBytes() []byte {
	var data []byte

	// Round Number (variable-length encoding)
	data = append(data, types.E(r.RoundNumber)...)

	// Set Id (variable-length encoding)
	data = append(data, types.E(r.SetID)...)

	return data
}

// FromBytes decodes a CatchUpRequest from bytes
func (r *CatchUpRequest) FromBytes(data []byte) error {
	offset := 0

	// Round Number
	roundNumber, bytesRead := types.DecodeE(data[offset:])
	if bytesRead == 0 {
		return fmt.Errorf("failed to decode round number")
	}
	r.RoundNumber = roundNumber
	offset += int(bytesRead)

	if offset >= len(data) {
		return fmt.Errorf("insufficient data for set ID")
	}

	// Set Id
	setID, bytesRead := types.DecodeE(data[offset:])
	if bytesRead == 0 {
		return fmt.Errorf("failed to decode set ID")
	}
	r.SetID = setID

	return nil
}

// SignedVote represents a signed prevote or precommit
// This matches SignMessage from grandpa/type.go
type SignedVote struct {
	Message   grandpa.Message        // The vote message (stage + vote)
	Signature types.Ed25519Signature // Ed25519 signature
	VoterIdx  uint64                 // Voter index in authority set
}

// ToBytes encodes a SignedVote to bytes
func (v *SignedVote) ToBytes() ([]byte, error) {
	return types.Encode(v)
}

// FromBytes decodes a SignedVote from bytes
func (v *SignedVote) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, nil)
	if err != nil {
		return err
	}
	*v = decoded.(SignedVote)
	return nil
}

// CatchUpResponse represents the catchup response with all votes from the last completed round
type CatchUpResponse struct {
	RoundNumber      uint64       // Round number
	SignedPrevotes   []SignedVote // All prevotes from the round
	SignedPrecommits []SignedVote // All precommits from the round
	BaseHash         common.Hash  // Base block hash (all vote targets descend from this)
	BaseNumber       uint32       // Base block slot number
}

// ToBytes encodes a CatchUpResponse to bytes
// Format: Round Number ++ len++[Signed Prevote] ++ len++[Signed Precommit] ++ Base Hash ++ Base Number
func (r *CatchUpResponse) ToBytes() ([]byte, error) {
	var data []byte

	// Round Number (variable-length encoding)
	data = append(data, types.E(r.RoundNumber)...)

	// len++[Signed Prevote]
	data = append(data, types.E(uint64(len(r.SignedPrevotes)))...)
	for _, prevote := range r.SignedPrevotes {
		voteBytes, err := prevote.ToBytes()
		if err != nil {
			return nil, fmt.Errorf("failed to encode prevote: %w", err)
		}
		data = append(data, voteBytes...)
	}

	// len++[Signed Precommit]
	data = append(data, types.E(uint64(len(r.SignedPrecommits)))...)
	for _, precommit := range r.SignedPrecommits {
		voteBytes, err := precommit.ToBytes()
		if err != nil {
			return nil, fmt.Errorf("failed to encode precommit: %w", err)
		}
		data = append(data, voteBytes...)
	}

	// Base Hash (32 bytes)
	data = append(data, r.BaseHash.Bytes()...)

	// Base Number (4 bytes, little-endian)
	baseNumBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(baseNumBytes, r.BaseNumber)
	data = append(data, baseNumBytes...)

	return data, nil
}

// FromBytes decodes a CatchUpResponse from bytes
func (r *CatchUpResponse) FromBytes(data []byte) error {
	offset := 0

	// Round Number
	roundNumber, bytesRead := types.DecodeE(data[offset:])
	if bytesRead == 0 {
		return fmt.Errorf("failed to decode round number")
	}
	r.RoundNumber = roundNumber
	offset += int(bytesRead)

	// len++[Signed Prevote]
	numPrevotes, bytesRead := types.DecodeE(data[offset:])
	if bytesRead == 0 {
		return fmt.Errorf("failed to decode prevotes length")
	}
	offset += int(bytesRead)

	r.SignedPrevotes = make([]SignedVote, numPrevotes)
	for i := uint64(0); i < numPrevotes; i++ {
		var vote SignedVote
		// Note: This assumes fixed-size encoding. Adjust if variable-length.
		if err := vote.FromBytes(data[offset:]); err != nil {
			return fmt.Errorf("failed to decode prevote %d: %w", i, err)
		}
		r.SignedPrevotes[i] = vote
		// TODO: Calculate actual bytes consumed based on encoding
		offset += 100 // Placeholder - needs actual size calculation
	}

	// len++[Signed Precommit]
	numPrecommits, bytesRead := types.DecodeE(data[offset:])
	if bytesRead == 0 {
		return fmt.Errorf("failed to decode precommits length")
	}
	offset += int(bytesRead)

	r.SignedPrecommits = make([]SignedVote, numPrecommits)
	for i := uint64(0); i < numPrecommits; i++ {
		var vote SignedVote
		if err := vote.FromBytes(data[offset:]); err != nil {
			return fmt.Errorf("failed to decode precommit %d: %w", i, err)
		}
		r.SignedPrecommits[i] = vote
		// TODO: Calculate actual bytes consumed based on encoding
		offset += 100 // Placeholder - needs actual size calculation
	}

	// Base Hash (32 bytes)
	if offset+32 > len(data) {
		return fmt.Errorf("insufficient data for base hash")
	}
	r.BaseHash = common.BytesToHash(data[offset : offset+32])
	offset += 32

	// Base Number (4 bytes, little-endian)
	if offset+4 > len(data) {
		return fmt.Errorf("insufficient data for base number")
	}
	r.BaseNumber = binary.LittleEndian.Uint32(data[offset : offset+4])

	return nil
}

// SendCatchUpRequest sends a catchup request to a peer (requester side)
func (p *Peer) SendCatchUpRequest(ctx context.Context, roundNumber uint64, setID uint64) (*CatchUpResponse, error) {
	code := uint8(CE149_Catchup)
	stream, err := p.openStream(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("openStream[CE149_Catchup] failed: %w", err)
	}
	defer stream.Close()

	// Create request: Round Number ++ Set Id
	request := CatchUpRequest{
		RoundNumber: roundNumber,
		SetID:       setID,
	}
	requestBytes := request.ToBytes()

	// Send request
	if err := sendQuicBytes(ctx, stream, requestBytes, p.PeerID, code); err != nil {
		return nil, fmt.Errorf("sendQuicBytes[CE149_Catchup] request failed: %w", err)
	}

	// Receive response: Catchup
	responseBytes, err := receiveQuicBytes(ctx, stream, p.PeerID, code)
	if err != nil {
		return nil, fmt.Errorf("receiveQuicBytes[CE149_Catchup] response failed: %w", err)
	}

	// Decode response
	var response CatchUpResponse
	if err := response.FromBytes(responseBytes); err != nil {
		return nil, fmt.Errorf("failed to decode catchup response: %w", err)
	}

	return &response, nil
}

// onCatchUpRequest handles incoming catchup requests (responder side)
func (n *Node) onCatchUpRequest(ctx context.Context, stream quic.Stream, msg []byte, peerID uint16) error {
	defer stream.Close()

	// Decode request: Round Number ++ Set Id
	var request CatchUpRequest
	if err := request.FromBytes(msg); err != nil {
		return fmt.Errorf("failed to decode catchup request: %w", err)
	}

	// Get catchup data from GRANDPA state
	response, err := n.GetCatchUpData(request.RoundNumber, request.SetID)
	if err != nil {
		// If unable to send response, stop the stream (as per spec)
		return fmt.Errorf("failed to get catchup data: %w", err)
	}

	// Encode response
	responseBytes, err := response.ToBytes()
	if err != nil {
		return fmt.Errorf("failed to encode catchup response: %w", err)
	}

	// Send response: Catchup
	if err := sendQuicBytes(ctx, stream, responseBytes, peerID, uint8(CE149_Catchup)); err != nil {
		return fmt.Errorf("sendQuicBytes[CE149_Catchup] response failed: %w", err)
	}

	return nil
}

// GetCatchUpData retrieves catchup data for the specified round and authority set
// This should return all votes from the last completed round
func (n *NodeContent) GetCatchUpData(roundNumber uint64, setID uint64) (*CatchUpResponse, error) {
	// TODO: Implement actual lookup in GRANDPA state
	// This should retrieve:
	// - All signed prevotes from the specified round
	// - All signed precommits from the specified round
	// - The base block (hash and slot) that all vote targets descend from

	// For now, return an error indicating this needs implementation
	return nil, fmt.Errorf("GetCatchUpData not yet implemented")
}
