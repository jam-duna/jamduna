package grandpa

import (
	"reflect"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"
)

/*
CE 152: GRANDPA CatchUp
Catchup Request. This is sent by a voting validator to another validator if the first validator determines that it is behind the other validator by a threshold number of rounds (currently set to 2 rounds). The response includes all votes from the last completed round of the responding validator. Base Hash and Base Number refer to the base, which is a block all vote targets are a descendent of. If the responding voter is unable to send the response it should stop the stream.

Base Hash = Header Hash
Base Number = Slot
Catchup = Round Number ++ len++[Signed Prevote] ++ len++[Signed Precommit] ++ Base Hash ++ Base Number

Validator -> Validator

--> Round Number ++ Set Id
--> FIN
<-- Catchup
<-- FIN
*/

type GrandpaCatchUp struct {
	Round uint64
	SetId uint32
}

func (c *GrandpaCatchUp) String() string {
	return types.ToJSON(c)
}

type CatchUpResponse struct {
	Round            uint64
	SignedPrevotes   []SignedMessage
	SignedPrecommits []SignedMessage
	BaseHash         common.Hash
	BaseNumber       uint32
}

func (c *CatchUpResponse) String() string {
	return types.ToJSON(c)
}

func (c *GrandpaCatchUp) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(c)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (c *GrandpaCatchUp) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(GrandpaCatchUp{}))
	if err != nil {
		return err
	}
	*c = decoded.(GrandpaCatchUp)
	if err != nil {
		return err
	}
	return nil
}

func (c *CatchUpResponse) ToBytes() ([]byte, error) {
	bytes, err := types.Encode(c)
	if err != nil {
		return nil, nil
	}
	return bytes, nil
}

func (c *CatchUpResponse) FromBytes(data []byte) error {
	decoded, _, err := types.Decode(data, reflect.TypeOf(CatchUpResponse{}))
	if err != nil {
		return err
	}
	*c = decoded.(CatchUpResponse)
	if err != nil {
		return err
	}
	return nil
}

func (g *Grandpa) ProcessCatchUpMessage(catchup GrandpaCatchUp) (CatchUpResponse, error) {

	response, err := g.GetCatchUpResponse(catchup.Round)
	if err != nil {
		return CatchUpResponse{}, err
	}

	return response, nil
}

// ProcessCatchUpResponse processes a received catch-up response
// This updates the node's GRANDPA state with the catch-up data
func (g *Grandpa) ProcessCatchUpResponse(response CatchUpResponse, setId uint32) error {

	// Process each prevote in the catch-up response
	for _, prevote := range response.SignedPrevotes {
		vote := GrandpaVote{
			Round:         response.Round,
			SetId:         setId,
			SignedMessage: prevote,
			// SetId would need to be included in the response or tracked separately
		}
		if err := g.ProcessPreVoteMessage(vote); err != nil {
			log.Warn(log.Grandpa, "ProcessCatchUpResponse: failed to process prevote", "err", err)
			// Continue processing other votes even if one fails
		}
	}

	// Process each precommit in the catch-up response
	for _, precommit := range response.SignedPrecommits {
		vote := GrandpaVote{
			Round:         response.Round,
			SignedMessage: precommit,
			SetId:         setId,
			// SetId would need to be included in the response or tracked separately
		}
		if err := g.ProcessPreCommitMessage(vote); err != nil {
			log.Warn(log.Grandpa, "ProcessCatchUpResponse: failed to process precommit", "err", err)
			// Continue processing other votes even if one fails
		}
	}

	log.Trace(log.Grandpa, "ProcessCatchUpResponse: successfully processed catch-up",
		"round", response.Round,
		"prevotes", len(response.SignedPrevotes),
		"precommits", len(response.SignedPrecommits),
		"baseHash", response.BaseHash.String_short(),
		"baseNumber", response.BaseNumber)

	return nil
}
