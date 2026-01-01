package types

// NodeClient defines the interface for builder communication with JAM nodes
// This replaces direct statedb dependencies in the builder architecture
type NodeClient interface {
	// Service management
	GetService(serviceID uint32) (*ServiceAccount, bool, error)

	// State access
	ReadGlobalDepth(serviceIndex uint32) (uint8, error)
	GetWitnessCount() int

	// Bundle execution
	BuildBundle(
		workPackage WorkPackage,
		extrinsics []ExtrinsicsBlobs,
		coreIndex uint16,
		authTokens []AuthorizeCode,
		pvmBackend string,
	) (*WorkPackage, *WorkReport, error)
}

// AuthorizeCode represents authorization code for JAM services
type AuthorizeCode struct {
	PackageMetaData   []byte
	AuthorizationCode []byte
}

// Encode encodes the authorization code
func (a *AuthorizeCode) Encode() ([]byte, error) {
	metadata := string(a.PackageMetaData)
	encoded_data := CombineMetadataAndCode(metadata, a.AuthorizationCode)
	return encoded_data, nil
}

// Decode decodes the authorization code
func (a *AuthorizeCode) Decode(data []byte) error {
	metadata, authcode := SplitMetadataAndCode(data)
	a.PackageMetaData = []byte(metadata)
	a.AuthorizationCode = authcode
	return nil
}