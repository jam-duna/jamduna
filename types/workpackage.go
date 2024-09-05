package types

import (
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
	Authorization []byte `json:"authorization"`
	// $h$ - the index of the service which hosts the authorization code
	AuthCodeHost uint32 `json:"auth_code_host"`
	// $c$ - an authorization code hash
	Authorizer Authorizer `json:"authorizer"`
	// $x$ - context
	RefineContext RefineContext `json:"context"`
	// $i$ - a sequence of work items
	WorkItems []WorkItem `json:"items"`
}

// The workpackage is an ordered collection of workitems
type ASWorkPackage struct {
	ImportSegments []ASWorkItem
	Extrinsic      []byte
}

// for codec
type CWorkPackage struct {
	// $j$ - a simple blob acting as an authorization token
	Authorization []byte `json:"authorization"`
	// $h$ - the index of the service which hosts the authorization code
	AuthCodeHost uint32 `json:"auth_code_host"`
	// $c$ - an authorization code hash
	Authorizer Authorizer `json:"authorizer"`
	// $x$ - context
	RefineContext RefineContext `json:"context"`
	// $i$ - a sequence of work items
	WorkItems []CWorkItem `json:"items"`
}

type SWorkPackage struct {
	Authorization string        `json:"authorization"`
	AuthCodeHost  uint32        `json:"auth_code_host"`
	Authorizer    SAuthorizer   `json:"authorizer"`
	RefineContext RefineContext `json:"context"`
	WorkItems     []SWorkItem   `json:"items"`
}

type Authorizer struct {
	CodeHash common.Hash `json:"code_hash"`
	Params   []byte      `json:"params"`
}

type SAuthorizer struct {
	CodeHash common.Hash `json:"code_hash"`
	Params   string      `json:"params"`
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

func (s *SWorkPackage) Deserialize() (CWorkPackage, error) {
	authorization := common.FromHex(s.Authorization)
	auth_code_host := s.AuthCodeHost
	authorizer, err := s.Authorizer.Deserialize()
	if err != nil {
		return CWorkPackage{}, err
	}
	refine_context := s.RefineContext
	work_items := make([]CWorkItem, len(s.WorkItems))
	for i, item := range s.WorkItems {
		work_items[i], err = item.Deserialize()
		if err != nil {
			return CWorkPackage{}, err
		}
	}

	return CWorkPackage{
		Authorization: authorization,
		AuthCodeHost:  auth_code_host,
		Authorizer:    authorizer,
		RefineContext: refine_context,
		WorkItems:     work_items,
	}, nil
}

func (s *SAuthorizer) Deserialize() (Authorizer, error) {
	params := common.FromHex(s.Params)

	return Authorizer{
		CodeHash: s.CodeHash,
		Params:   params,
	}, nil
}
