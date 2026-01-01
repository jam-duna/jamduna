package witness

type CommitmentProvider interface {
	CommitmentFor(
		assetID uint32,
		amount Uint128,
		ownerPk, rho, noteRseed [32]byte,
		unlockHeight uint64,
		memoHash [32]byte,
	) ([32]byte, error)
}

type FFICommitmentProvider struct {
	ffi *OrchardFFI
}

func NewFFICommitmentProvider(ffi *OrchardFFI) *FFICommitmentProvider {
	return &FFICommitmentProvider{ffi: ffi}
}

func (p *FFICommitmentProvider) CommitmentFor(
	assetID uint32,
	amount Uint128,
	ownerPk, rho, noteRseed [32]byte,
	unlockHeight uint64,
	memoHash [32]byte,
) ([32]byte, error) {
	if p == nil || p.ffi == nil {
		return [32]byte{}, ErrInvalidInput
	}

	return p.ffi.GenerateCommitment(assetID, amount, ownerPk, rho, noteRseed, unlockHeight, memoHash)
}
