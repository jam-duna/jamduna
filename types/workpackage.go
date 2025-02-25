package types

import (
	"encoding/json"
	"fmt"
	"reflect"

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
	// $u$ - an authorization code hash
	AuthorizationCodeHash common.Hash `json:"authorization_code_hash"`
	// $p$ - a parameterization blob
	ParameterizationBlob []byte `json:"parameterization_blob"`
	// $x$ - context
	RefineContext RefineContext `json:"context"`
	// $w$ - a sequence of work items
	WorkItems []WorkItem `json:"items"`
}

// WorkPackageBundle represents a work package.
type WorkPackageBundle struct {
	WorkPackage       WorkPackage       `json:"p"` // P: workPackage
	ExtrinsicData     ExtrinsicsBlobs   `json:"x"` // X: extrinsic data for some workitem argument w
	ImportSegmentData [][][]byte        `json:"s"` // M: import segment data, previouslly called m (each of segment is size of W_E*W_S)
	Justification     [][][]common.Hash `json:"j"` // J: justifications of segment data build using CDT
}

func (b *WorkPackageBundle) Validate() error {
	//0.6.2 14.2
	work_package := b.WorkPackage
	if len(work_package.WorkItems) < 1 {
		return fmt.Errorf("WorkPackageBundle must have at least one WorkItem")
	}
	if len(work_package.WorkItems) > MaxWorkItemsPerPackage {
		return fmt.Errorf("WorkPackageBundle has too many WorkItems")
	}
	// 0.6.2 14.4
	total_exports := 0
	total_imports := 0
	for _, work_item := range work_package.WorkItems {
		total_exports += int(work_item.ExportCount)
		total_imports += len(work_item.ImportedSegments)
	}
	if total_exports > MaxManifestEntries {
		return fmt.Errorf("WorkPackageBundle has too many exports")
	}
	if total_imports > MaxManifestEntries {
		return fmt.Errorf("WorkPackageBundle has too many imports")
	}
	// 0.6.2 14.5
	data_lens := 0
	data_lens += len(work_package.Authorization)
	data_lens += len(work_package.ParameterizationBlob)
	for _, work_item := range work_package.WorkItems {
		data_lens += work_item.GetTotalDataLength()
	}
	if data_lens > MaxEncodedWorkPackageSize {
		return fmt.Errorf("WorkPackageBundle has too much data")
	}
	// 0.6.2 14.6
	Gas_a := uint64(0)
	Gas_r := uint64(0)
	for _, work_item := range work_package.WorkItems {
		Gas_a += work_item.AccumulateGasLimit
		Gas_r += work_item.RefineGasLimit
	}
	if Gas_a > AccumulationGasAllocation {
		return fmt.Errorf("WorkPackageBundle has too much accumulate gas")
	}
	if Gas_r > RefineGasAllocation {
		return fmt.Errorf("WorkPackageBundle has too much refine gas")
	}
	return nil
}

func (b *WorkPackageBundle) Bytes() []byte {
	encoded, err := Encode(b)
	if err != nil {
		return nil
	}
	return encoded
}

func (b *WorkPackageBundle) String() string {
	jsonByte, _ := json.Marshal(b)
	return string(jsonByte)
}

func (b *WorkPackageBundle) PackageHash() common.Hash {
	return b.WorkPackage.Hash()
}

func (b *WorkPackageBundle) Package() WorkPackage {
	return b.WorkPackage
}

func WorkPackageBundleFromBytes(data []byte) (*WorkPackageBundle, error) {
	var b WorkPackageBundle
	decoded, _, err := Decode(data, reflect.TypeOf(WorkPackageBundle{}))
	if err != nil {
		return nil, err
	}
	b = decoded.(WorkPackageBundle)
	return &b, nil
}

type Authorizer struct {
	CodeHash common.Hash `json:"code_hash"`
	Params   []byte      `json:"params"`
}

func (a *WorkPackage) String() string {
	enc, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

// Bytes returns the bytes of the Assurance
func (a *WorkPackage) Bytes() []byte {
	encode, err := Encode(a)
	if err != nil {
		return nil
	}
	return encode
}

func (a *WorkPackage) Hash() common.Hash {
	data := a.Bytes()
	if data == nil {
		// Handle the error case
		return common.Hash{}
	}
	return common.Blake2Hash(data)
}

func (a *WorkPackage) UnmarshalJSON(data []byte) error {
	var s struct {
		Authorization         string        `json:"authorization"`
		AuthCodeHost          uint32        `json:"auth_code_host"`
		AuthorizationCodeHash common.Hash   `json:"authorization_code_hash"`
		ParameterizationBlob  []byte        `json:"parameterization_blob"`
		RefineContext         RefineContext `json:"context"`
		WorkItems             []WorkItem    `json:"items"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	a.Authorization = common.FromHex(s.Authorization)
	a.AuthCodeHost = s.AuthCodeHost
	a.AuthorizationCodeHash = s.AuthorizationCodeHash
	a.ParameterizationBlob = s.ParameterizationBlob
	a.RefineContext = s.RefineContext
	a.WorkItems = s.WorkItems

	return nil
}

func (a *Authorizer) UnmarshalJSON(data []byte) error {
	var s struct {
		CodeHash common.Hash `json:"code_hash"`
		Params   string      `json:"params"`
	}
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	a.CodeHash = s.CodeHash
	a.Params = common.FromHex(s.Params)

	return nil
}

func (a WorkPackage) MarshalJSON() ([]byte, error) {
	// Convert Authorization from []byte to hex string
	authorization := common.HexString(a.Authorization)

	return json.Marshal(&struct {
		Authorization         string        `json:"authorization"`
		AuthCodeHost          uint32        `json:"auth_code_host"`
		AuthorizationCodeHash common.Hash   `json:"authorization_code_hash"`
		ParameterizationBlob  []byte        `json:"parameterization_blob"`
		RefineContext         RefineContext `json:"context"`
		WorkItems             []WorkItem    `json:"items"`
	}{
		Authorization:         authorization,
		AuthCodeHost:          a.AuthCodeHost,
		AuthorizationCodeHash: a.AuthorizationCodeHash,
		ParameterizationBlob:  a.ParameterizationBlob,
		RefineContext:         a.RefineContext,
		WorkItems:             a.WorkItems,
	})
}

func (a Authorizer) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		CodeHash common.Hash `json:"code_hash"`
		Params   string      `json:"params"`
	}{
		CodeHash: a.CodeHash,
		Params:   common.HexString(a.Params),
	})
}
