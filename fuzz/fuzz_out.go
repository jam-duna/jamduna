package fuzz

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/colorfulnotion/jam/statedb"
)

const (
	ChallengeTimeOut = 10 * time.Second
)

type StateTransitionQA struct {
	Mutated bool
	Error   error
	STF     *statedb.StateTransition
}

type JamError struct {
	Error string `json:"error"`
}

type StateTransitionResponse struct {
	PostState *statedb.StateSnapshotRaw
	JamError  *JamError
	Mutated   bool `json:"mutated"`
}

func (s *StateTransitionResponse) String() string {
	jsonEncode, _ := json.Marshal(s)
	return string(jsonEncode)
}

func (stca StateTransitionQA) ToChallenge() statedb.StateTransitionChallenge {
	c := statedb.StateTransitionChallenge{
		PreState: stca.STF.PreState,
		Block:    stca.STF.Block,
	}
	return c
}

/*
[200] StatusOK
Return post StateSnapshotRaw: {"state_root": "0x12..34", "keyvals": [{"key": "0x12..34", "value": "0x12..34", "struct_type": "0x12..34", "metadata": "0x12..34"}]}

[406] StatusNotAcceptable
return jamError in JSON body: {"error": "Epoch marker expected"}
*/

func (f *Fuzzer) SendStateTransitionChallenge(endpoint string, challenge statedb.StateTransitionChallenge) (stResp *StateTransitionResponse, respStatus bool, nonJamError error) {
	respNotOK := false // Assume error until proven otherwise
	respOK := true     // For valid 200 & 406 responses only

	jsonData, err := json.Marshal(challenge)
	if err != nil {
		return nil, respNotOK, fmt.Errorf("failed to marshal STF data: %v", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, respNotOK, fmt.Errorf("failed to open HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: ChallengeTimeOut}
	resp, err := client.Do(req)
	if err != nil {
		return nil, respNotOK, fmt.Errorf("failed to send HTTP request: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// [200 OK] Solver: stc -> valid
		var snapshot statedb.StateSnapshotRaw
		if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
			return nil, respNotOK, fmt.Errorf("failed to decode challenge response: %v", err)
		}
		postOriginalStateSnapshotResp := StateTransitionResponse{
			PostState: &snapshot,
			Mutated:   false,
		}
		return &postOriginalStateSnapshotResp, respOK, nil

	case http.StatusNotAcceptable:
		// [406 Not Acceptable] Solver: stc -> jamError
		var jamError JamError
		if decodeErr := json.NewDecoder(resp.Body).Decode(&jamError); decodeErr != nil {
			return nil, respNotOK, fmt.Errorf("failed to decode JamError: %v", decodeErr)
		}
		postFuzzedStateSnapshotResp := StateTransitionResponse{
			Mutated:  true,
			JamError: &jamError,
		}
		return &postFuzzedStateSnapshotResp, respOK, nil

	default:
		return nil, respNotOK, fmt.Errorf("unexpected response code %d", resp.StatusCode)
	}
}
