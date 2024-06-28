package pvm

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/blake2b"
)

// JAMEnvironment defines the interface for host calls
type JAMEnvironment interface {
	// A.6 pg 38
	InvokeHostCall(opcode byte, operands []byte, vm *VM) (bool, error)
	// B.2 pg 39
	IsAuthorized(p, c byte) (byte, error) // Is-Authorized invocation
	// B.3 pg 39
	Refine(input RefineInput) (byte, []byte, error)
	// B.4 pg 40
	Accumulate(input AccumulateInput) (byte, error)
	// B.5 pg 41
	OnTransfer(input OnTransferInput) (byte, error)
}

// JAMEnv conforms to the JAMEnvironment interface
type JAMEnv struct {
	ξ uint64
}

// Hash function using Blake2b
func bhash(data []byte) []byte {
	hash := blake2b.Sum256(data)
	return hash[:]
}

func (env *JAMEnv) IsAuthorized(p, c byte) (uint32, error) {
	// Implement the Is-Authorized logic
	// Return the amount of gas remaining or an error code
	if p == 0 {
		return LOW, nil // Example logic
	}
	return OK, nil
}

func (env *JAMEnv) InvokeHostCall(opcode byte, operands []byte, vm *VM) (bool, error) {
	// Implement the logic for handling host calls
	// Return true if the call results in a halt condition, otherwise false
	switch opcode {
	case 0xFF: // Example opcode for a host call
		// Perform the host call logic
		return true, nil // Return true to indicate a halt condition
	// Add more cases for different host call opcodes here...
	default:
		return false, fmt.Errorf("unknown host call opcode: %d", opcode)
	}
}

type RefineInput struct {
	s uint32
}

type AccumulateInput struct {
	δ uint32
}

type OT struct {
	sb uint32
}

type OnTransferInput struct {
	sc uint32
	s  OT
	t  []uint32
}

func (env *JAMEnv) Refine(input RefineInput) (uint32, []byte, error) {
	// Implement the Refine Invocation logic
	if input.s == BAD {
		return BAD, nil, errors.New("bad refine invocation")
	}
	if input.s > S {
		return BIG, nil, errors.New("refine invocation too big")
	}
	// Perform the actual refine logic and return the result
	output := []byte("refined_data")
	return OK, output, nil
}

func (env *JAMEnv) Accumulate(input AccumulateInput) (uint32, error) {
	// Implement the Accumulate Invocation logic based on the provided specification
	// Handle the accumulation logic here
	if input.δ == BAD {
		return BAD, errors.New("accumulation failed")
	}
	// Perform the actual accumulation logic and return the result
	return OK, nil
}

func (env *JAMEnv) OnTransfer(input OnTransferInput) (byte, error) {
	// Implement the On-Transfer Invocation logic based on the provided specification
	// If sc is empty or t is empty, return s
	/*if input.sc == 0 || len(input.t) == 0 {
		return input.s.sb, nil
	}

	// Update the service account state as specified in the function definition
	for _, ret := range input.t {
		input.s.sb += ret
	}
	*/
	// Perform further processing as per the spec
	return OK, nil
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func (env *JAMEnv) getBytes(o uint32, numBytes uint32) []byte {
	return []byte{}
}

func (env *JAMEnv) getValue(k []byte) ([]byte, error) {
	return []byte{}, nil
}

func (env *JAMEnv) deleteKey(k []byte) error {
	return nil
}

func (env *JAMEnv) setBytes(v []byte, b0 uint32, numBytes int) error {
	return nil
}

func (env *JAMEnv) containsKey(h []byte, z []byte) bool {
	return false
}

func (env *JAMEnv) setKey(k []byte, v []byte, b0 uint32, numBytes int) error {
	return nil
}

func (env *JAMEnv) addKey(k []byte, v []byte) error {
	return nil
}

// Lookup preimage
func (env *JAMEnv) Lookup(h0 uint32, b0 uint32, bz uint32) uint32 {
	h := env.getBytes(h0, 32)
	v, err := env.getValue(h)
	if err != nil {
		return NONE
	}
	// copy v to b0 ... +bz
	env.setBytes(v, b0, min(int(bz), len(v)))
	return uint32(len(v))
}

func (env *JAMEnv) E4(s []byte) []byte {
	return []byte{}
}

// Read Storage
func (env *JAMEnv) Read(s uint32, ko, kz, bo, bz uint32) uint32 {
	blob := env.getBytes(ko, kz)
	k := bhash(append(env.E4([]byte{}), blob...))
	v, err := env.getValue(k)
	if err != nil {
		return NONE
	}
	// return OOB
	env.setBytes(v, bo, int(bz))
	return uint32(len(v))
}

// Write Storage
func (env *JAMEnv) Write(ko, kz, vo, vz uint32) uint32 {
	blob := env.getBytes(ko, kz)
	k := bhash(append(env.E4([]byte{}), blob...))
	if vz == 0 {
		env.deleteKey(k)
	} else {
		env.setKey(k, env.getBytes(vo, vz), vo, int(vz))
	}
	// return FULL if at > ab
	return vz
}

type Service struct {
	tc uint32
	tb uint32
	tt uint32
	tg uint32
	tm uint32
	tl uint32
	ti uint32
}

func (env *JAMEnv) getService(service uint32) (*Service, error) {
	return &Service{}, nil
}

// Information-on-Service
func (env *JAMEnv) Info(service uint32, o uint32) uint32 {
	s, err := env.getService(service)
	if err != nil {
		return NONE
	}
	m := env.ScaleEncode([]interface{}{s.tc, s.tb, s.tt, s.tg, s.tm, s.tl, s.ti})
	env.setBytes(m, o, len(m))

	return OK
}

func (env *JAMEnv) ScaleEncode(interface{}) []byte {
	return []byte{}
}

// Empower updates
func (env *JAMEnv) Empower(m, a, v uint32) uint32 {
	// Set (x'p)_m, (x'p)_a, (x'p)_v
	return OK
}

// Assign Core x_c[i]
func (env *JAMEnv) Assign(ξ, ω0 uint32, i uint32) uint32 {
	// let c = memory from o+32i ... +32
	// copy c into x
	return OK
}

// Designate validators
func (env *JAMEnv) Designate(o uint32, i uint32) uint32 {
	// b := env.getBytes(o+176i, 176)
	return OK
}

// Checkpoint gets Gas-remaining
func (env *JAMEnv) Checkpoint() (uint32, uint32) {
	ξ := env.ξ
	return uint32(ξ % (1 << 32)), uint32(ξ / (1 << 32))
}

// New service
func (env *JAMEnv) New(o, l, gl, gh, ml, mh uint32) uint32 {
	// let c = copy o... +32
	// put 'g' and 'm' together
	// g := uint64(gh)<<32 + uint64(gl)
	// m := uint64(mh)<<32 + uint64(ml)
	// init service with bump
	// return CASH if balance insufficient
	// decrease balance of service?
	return OK
}

// Upgrade service
func (env *JAMEnv) Upgrade(o, gh, gl, mh, ml uint32) uint32 {
	// put 'g' and 'm' together
	// g := uint64(gh)<<32 + uint64(gl)
	// m := uint64(mh)<<32 + uint64(ml)
	return OK
}

// Transfer host call
func (env *JAMEnv) Transfer(d, al, ah, gl, gh, o uint32) uint32 {
	// a := uint64(ah)<<32 + uint64(al)
	// g := uint64(gh)<<32 + uint64(gl)
	// m := env.Decode(env.getBytes(o, M))
	return OK
}

// Quit Service
func (env *JAMEnv) Quit(d, o uint32) uint32 {
	// a := (xs[len(xs)-1] - xs[0]) + o
	// g := env.ξ
	// return WHO
	// return LOW
	return OK
}

// Solicit preimage
func (env *JAMEnv) Solicit(o, z uint32) uint32 {
	h := env.getBytes(o, 32)
	if !env.containsKey(h, env.getBytes(z, 32)) {
		env.addKey(h, env.getBytes(z, 32))
	}
	// return HUH
	// return FULL
	return OK
}

// Forget preimage
func (env *JAMEnv) Forget(o, z uint32) uint32 {
	// Determine 'h' based on the specification
	h := env.getBytes(o, 32)
	env.deleteKey(h)
	// return HUH
	return OK
}

// Make-PVM -- Accumulate
func (env *JAMEnv) Invoke() uint32 {
	// does not have it
	return OK
}

// HistoricalLookup determines whether the preimage of some hash h was available for lookup by some service account a at some timeslot t, and if so, provide its preimage
func (env *JAMEnv) HistoricalLookup(h common.Hash) uint32 {
	/* v, historical, err := env.fetch_DA(h)
	if err != nil {
		return NONE, nil,
	} */
	return OK
}

// Import Segment
func (env *JAMEnv) Import(i common.Hash) uint32 {
	/*v, h, err := env.fetch_DA(i)
	if err != nil {
		return NONE
	} */
	return OK
}

// Export segment host-call
func (env *JAMEnv) Export(i []uint32) (uint32, [][]byte) {
	var x [][]byte
	// return OOB
	// if len(e) >= WX return FULL
	// where ... const WX = 1024
	return OK, x
}

// Not sure but this is running a machine at instruction i? using bytes from po to pz
func (env *JAMEnv) Machine(po uint32, pz uint32, i uint32) uint32 {
	return OK
}

func (env *JAMEnv) Peek(n, a, b, l uint32) uint32 {
	// return WHO
	return OK
}

func (env *JAMEnv) Poke(n, a, b, l uint32) uint32 {
	// get m[n]
	/*u, err := env.getMKey(n, b, l)
	if err != nil {
		return OOB
	}
	env.setBytes(u, a, int(l)) */
	return OK
}

// Kickoff
func (env *JAMEnv) InvokeR(n, o uint32) uint32 {
	/*g, err := env.DecodeE8(o)
	if err != nil {
		return OOB
	}
	w, err := env.DecodeE4(o+8, 4*int(n)+4)
	if err != nil {
		return OOB
	}
	// get m[n], which
	m, err := env.getM(n)
	if err != nil {
		return OOB
	}
	*/
	// func NewVM(program *Program, ramSize int, regSize int) *VM

	//pvm := NewVM(m, int(n), g, w)
	//c := pvm.Execute()
	return OK
}

// Expunge PVM
func (env *JAMEnv) Expunge(n uint32) uint32 {
	// return WHO
	return OK
}
