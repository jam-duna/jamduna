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

/*
type NewService struct {
	C []byte
	L uint32
	B uint64
	G uint64
	M uint64
	I uint32 // new service_index
}
*/

type NewService struct {
	S uint32
	A ServiceAccount
}

type UpgradeService struct {
	C []byte
	G uint64
	M uint64
}

type AddTransfer struct {
	M []byte
	A uint64
	G uint64
	D uint32
}
