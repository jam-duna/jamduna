package types

type Empower struct {
	M uint32
	A uint32
	V uint32
}

type Designate struct {
	V []byte
}

type Assign struct {
	Core uint32
	C    []byte
}

type AddTransfer struct {
	M []byte
	A uint64
	G uint64
	D uint32
}

func (a *AddTransfer) Bytes() []byte {
	enc, err := Encode(a)
	if err != nil {
		return nil
	}

	return enc
}
