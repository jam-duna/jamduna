package witness

type OrchardWallet interface {
	NullifierFor(ownerPk, rho, commitment [32]byte) ([32]byte, error)
	ProofFor(extrinsicType ExtrinsicType, inputData []byte) (proof, publicInputs []byte, err error)
}

type SpendKeyProvider interface {
	SpendKeyFor(ownerPk [32]byte) ([32]byte, error)
}

type FFIWallet struct {
	ffi              *OrchardFFI
	spendKeyProvider SpendKeyProvider
}

func NewFFIWallet(ffi *OrchardFFI, spendKeyProvider SpendKeyProvider) *FFIWallet {
	return &FFIWallet{
		ffi:              ffi,
		spendKeyProvider: spendKeyProvider,
	}
}

func (w *FFIWallet) NullifierFor(ownerPk, rho, commitment [32]byte) ([32]byte, error) {
	if w == nil || w.ffi == nil || w.spendKeyProvider == nil {
		return [32]byte{}, ErrInvalidInput
	}

	skSpend, err := w.spendKeyProvider.SpendKeyFor(ownerPk)
	if err != nil {
		return [32]byte{}, err
	}
	defer zeroBytes32(&skSpend)

	return w.ffi.GenerateNullifier(skSpend, rho, commitment)
}

func (w *FFIWallet) ProofFor(extrinsicType ExtrinsicType, inputData []byte) ([]byte, []byte, error) {
	if w == nil || w.ffi == nil {
		return nil, nil, ErrInvalidInput
	}

	proof, publicInputs, err := w.ffi.GenerateProofWithPublicInputs(extrinsicType, inputData)
	if err != nil {
		return nil, nil, err
	}

	return proof, publicInputs, nil
}

type deterministicSpendKeyProvider struct{}

func (deterministicSpendKeyProvider) SpendKeyFor(ownerPk [32]byte) ([32]byte, error) {
	skSpend := ownerPk
	for i := range skSpend {
		skSpend[i] ^= 0xA5
	}
	return skSpend, nil
}

func newDeterministicWallet(ffi *OrchardFFI) *FFIWallet {
	return NewFFIWallet(ffi, deterministicSpendKeyProvider{})
}

func zeroBytes32(value *[32]byte) {
	for i := range value {
		value[i] = 0
	}
}
