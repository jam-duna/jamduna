package types

type ConfigJamBlocks struct {
	Mode        string
	HTTP        string
	QUIC        string
	Verbose     bool
	NumBlocks   int
	InvalidRate int
	Statistics  int
	Endpoint    string
	Network     string
}
