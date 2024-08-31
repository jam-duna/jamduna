package types

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/colorfulnotion/jam/common"
)

/*
14.3. Packages and Items.  A work-package includes: (See Equation 174):
* ${\bf j}$ - a simple blob acting as an authorization token
* $h$ - the index of the service which hosts the authorization code h
* $c$ - an authorization code hash
* ${\bf p}$ - a parameterization blob
* $x$ - context
* ${\bf i}$ - a sequence of work items
*/

// WorkPackage represents a work package.
type WorkPackage struct {
	// $j$ - a simple blob acting as an authorization token
	AuthorizationToken []byte `json:"authorization_token"`
	// $h$ - the index of the service which hosts the authorization code
	ServiceIndex int `json:"service_index"`
	// $c$ - an authorization code hash
	AuthorizationCode common.Hash `json:"authorization_code"`
	// $p$ - a parameterization blob
	ParamBlob []byte `json:"param_blob"`
	// $x$ - context
	Context RefinementContext `json:"context"`
	// $i$ - a sequence of work items
	WorkItems []WorkItem `json:"work_items"`
}

type SWorkPackage struct {
	// $j$ - a simple blob acting as an authorization token
	AuthorizationToken string `json:"authorization_token"`
	// $h$ - the index of the service which hosts the authorization code
	ServiceIndex int `json:"service_index"`
	// $c$ - an authorization code hash
	AuthorizationCode common.Hash `json:"authorization_code"`
	// $p$ - a parameterization blob
	ParamBlob string `json:"param_blob"`
	// $x$ - context
	Context RefinementContext `json:"context"`
	// $i$ - a sequence of work items
	WorkItems []SWorkItem `json:"work_items"`
}

// Bytes returns the bytes of the Assurance
func (a *WorkPackage) Bytes() []byte {
	enc, err := json.Marshal(a)
	if err != nil {
		// Handle the error according to your needs.
		fmt.Println("Error marshaling JSON:", err)
		return nil
	}
	return enc
}

func (a *WorkPackage) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.BytesToHash(common.ComputeHash(data))
}

func (s *SWorkPackage) Deserialize() (WorkPackage, error) {
	authorizationToken, err := hex.DecodeString(s.AuthorizationToken)
	if err != nil {
		return WorkPackage{}, fmt.Errorf("failed to decode authorization token: %v", err)
	}

	paramBlob, err := hex.DecodeString(s.ParamBlob)
	if err != nil {
		return WorkPackage{}, fmt.Errorf("failed to decode parameterization blob: %v", err)
	}

	workItems := make([]WorkItem, len(s.WorkItems))
	for i, si := range s.WorkItems {
		item, err := si.Deserialize()
		if err != nil {
			return WorkPackage{}, err
		}
		workItems[i] = item
	}

	return WorkPackage{
		AuthorizationToken: authorizationToken,
		ServiceIndex:       s.ServiceIndex,
		AuthorizationCode:  s.AuthorizationCode,
		ParamBlob:          paramBlob,
		Context:            s.Context,
		WorkItems:          workItems,
	}, nil
}
