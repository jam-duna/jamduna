package safrole

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	//github.com/ethereum/go-ethereum/common"
	"github.com/colorfulnotion/jam/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Define the structure for the JSON data
type SInput struct {
	Slot       int          `json:"slot"`
	Entropy    string       `json:"entropy"`
	Extrinsics []SExtrinsic `json:"extrinsic"`
}

type SValidator struct {
	Ed25519      string `json:"ed25519"`
	Bandersnatch string `json:"bandersnatch"`
	Bls          string `json:"bls"`
	Metadata     string `json:"metadata"`
}

type SExtrinsic struct {
	Attempt   int    `json:"attempt"`
	Signature string `json:"signature"`
}
type SSafroleAccumulator struct {
	Id      string `json:"id"`
	Attempt int    `json:"attempt"`
}

type STicketsOrKeys struct {
	Tickets []STicketBody `json:"tickets,omitempty"`
	Keys    []string      `json:"keys,omitempty"`
}

type SState struct {
	Timeslot           int                   `json:"tau"`
	Entropy            []string              `json:"eta"`
	PrevValidators     []SValidator          `json:"lambda"`
	CurrValidators     []SValidator          `json:"kappa"`
	NextValidators     []SValidator          `json:"gamma_k"`
	DesignedValidators []SValidator          `json:"iota"`
	TicketsAccumulator []SSafroleAccumulator `json:"gamma_a"`
	TicketsOrKeys      STicketsOrKeys        `json:"gamma_s"`
	TicketsVerifierKey string                `json:"gamma_z"`
}

type SEpochMark struct {
	Entropy    string   `json:"entropy"`
	Validators []string `json:"validators"`
}

type SOutput struct {
	Ok *struct {
		EpochMark   *SEpochMark            `json:"epoch_mark"`
		TicketsMark []*SSafroleAccumulator `json:"tickets_mark"`
	} `json:"ok"`
}

// Serialization and deserialization functions

// Extrinsic to SExtrinsic
func (e *Extrinsic) serialize() SExtrinsic {
	return SExtrinsic{
		Attempt:   e.Attempt,
		Signature: common.Bytes2Hex(e.Signature[:]),
	}
}

// SExtrinsic to Extrinsic
func (se *SExtrinsic) deserialize() (Extrinsic, error) {
	signature, err := hexutil.Decode(se.Signature)
	if err != nil {
		return Extrinsic{}, fmt.Errorf("failed to decode signature: %w", err)
	}
	if len(signature) != ExtrinsicSignatureInBytes {
		return Extrinsic{}, fmt.Errorf("invalid signature length, got %d expected %d", len(signature), ExtrinsicSignatureInBytes)
	}

	var sigArray [ExtrinsicSignatureInBytes]byte
	copy(sigArray[:], signature)

	return Extrinsic{
		Attempt:   se.Attempt,
		Signature: sigArray,
	}, nil
}

// Validator to SValidator
func (v *Validator) serialize() SValidator {
	return SValidator{
		Ed25519:      v.Ed25519.Hex(),
		Bandersnatch: v.Bandersnatch.Hex(),
		Bls:          hexutil.Encode(v.Bls[:]),
		Metadata:     hexutil.Encode(v.Metadata[:]),
	}
}

// SValidator to Validator
func (sv *SValidator) deserialize() (Validator, error) {
	if len(sv.Ed25519) != 66 {
		return Validator{}, fmt.Errorf("invalid ed25519 length, got %d expected %d", len(sv.Ed25519), 66)
	}
	if len(sv.Bandersnatch) != 66 {
		return Validator{}, fmt.Errorf("invalid bandersnatch length, got %d expected %d", len(sv.Bandersnatch), 66)
	}
	bls, _ := hexutil.Decode(sv.Bls)
	if len(bls) > 0 && len(bls) != BlsSizeInBytes {
		return Validator{}, fmt.Errorf("invalid bls length, got %d expected %d", len(bls), BlsSizeInBytes)
	}

	var blsArray [BlsSizeInBytes]byte
	copy(blsArray[:], sv.Bls)

	var metadataArray [MetadataSizeInBytes]byte
	copy(metadataArray[:], sv.Metadata)

	return Validator{
		Ed25519:      common.HexToHash(sv.Ed25519),
		Bandersnatch: common.HexToHash(sv.Bandersnatch),
		Bls:          blsArray,
		Metadata:     metadataArray,
	}, nil
}

// Input to SInput
func (i *Input) serialize() SInput {
	extrinsics := make([]SExtrinsic, len(i.Extrinsics))
	for idx, extrinsic := range i.Extrinsics {
		extrinsics[idx] = extrinsic.serialize()
	}
	return SInput{
		Slot:       i.Slot,
		Entropy:    i.Entropy.Hex(),
		Extrinsics: extrinsics,
	}
}

// SInput to Input
func (si *SInput) deserialize() (Input, error) {
	extrinsics := make([]Extrinsic, len(si.Extrinsics))
	for idx, sExtrinsic := range si.Extrinsics {
		extrinsic, err := sExtrinsic.deserialize()
		if err != nil {
			return Input{}, err
		}
		extrinsics[idx] = extrinsic
	}

	return Input{
		Slot:       si.Slot,
		Entropy:    common.HexToHash(si.Entropy),
		Extrinsics: extrinsics,
	}, nil
}

// SafroleAccumulator to SSafroleAccumulator
func (sa *TicketBody) serialize() SSafroleAccumulator {
	return SSafroleAccumulator{
		Id:      sa.Id.Hex(),
		Attempt: int(sa.Attempt),
	}
}

// SSafroleAccumulator to SafroleAccumulator
func (ssa *SSafroleAccumulator) deserialize() (TicketBody, error) {
	return TicketBody{
		Id:      common.HexToHash(ssa.Id),
		Attempt: uint8(ssa.Attempt),
	}, nil
}

type STicketBody struct {
	Id      string `json:"id"`
	Attempt uint8  `json:"attempt"`
}

func (s *SafroleState) Serialize() SState {
	return s.serialize()
}

// State to SState
func (s *SafroleState) serialize() SState {
	entropy := make([]string, len(s.Entropy))
	for idx, e := range s.Entropy {
		entropy[idx] = e.Hex()
	}

	prevValidators := make([]SValidator, len(s.PrevValidators))
	for idx, validator := range s.PrevValidators {
		prevValidators[idx] = validator.serialize()
	}

	currValidators := make([]SValidator, len(s.CurrValidators))
	for idx, validator := range s.CurrValidators {
		currValidators[idx] = validator.serialize()
	}

	nextValidators := make([]SValidator, len(s.NextValidators))
	for idx, validator := range s.NextValidators {
		nextValidators[idx] = validator.serialize()
	}

	designedValidators := make([]SValidator, len(s.DesignedValidators))
	for idx, validator := range s.DesignedValidators {
		designedValidators[idx] = validator.serialize()
	}

	ticketsAccumulator := make([]SSafroleAccumulator, len(s.TicketsAccumulator))
	for idx, accumulator := range s.TicketsAccumulator {
		ticketsAccumulator[idx] = accumulator.serialize()
	}

	tickets := make([]STicketBody, len(s.TicketsOrKeys.Tickets))
	for idx, ticket := range s.TicketsOrKeys.Tickets {
		tickets[idx] = STicketBody{
			Id:      ticket.Id.Hex(),
			Attempt: ticket.Attempt,
		}
	}

	keys := make([]string, len(s.TicketsOrKeys.Keys))
	for idx, key := range s.TicketsOrKeys.Keys {
		keys[idx] = key.Hex()
	}

	return SState{
		Timeslot:           s.Timeslot,
		Entropy:            entropy,
		PrevValidators:     prevValidators,
		CurrValidators:     currValidators,
		NextValidators:     nextValidators,
		DesignedValidators: designedValidators,
		TicketsAccumulator: ticketsAccumulator,
		TicketsOrKeys: STicketsOrKeys{
			Tickets: tickets,
			Keys:    keys,
		},
		TicketsVerifierKey: common.Bytes2Hex(s.TicketsVerifierKey),
	}
}

// SState to State
// func (ss *SState) deserialize() (SafroleState, error) {
// 	if len(ss.Entropy) != 4 {
// 		return SafroleState{}, fmt.Errorf("invalid entropy length got %d expected 4", len(ss.Entropy))
// 	}
// 	entropy := make([]common.Hash, len(ss.Entropy))
// 	for idx, e := range ss.Entropy {
// 		if len(e) != 66 {
// 			return SafroleState{}, fmt.Errorf("invalid entropy length got %d expected 66", len(e))
// 		}
// 		entropy[idx] = common.HexToHash(e)
// 	}
//
// 	prevValidators := make([]Validator, len(ss.PrevValidators))
// 	for idx, sValidator := range ss.PrevValidators {
// 		validator, err := sValidator.deserialize()
// 		if err != nil {
// 			return SafroleState{}, err
// 		}
// 		prevValidators[idx] = validator
// 	}
//
// 	currValidators := make([]Validator, len(ss.CurrValidators))
// 	for idx, sValidator := range ss.CurrValidators {
// 		validator, err := sValidator.deserialize()
// 		if err != nil {
// 			return SafroleState{}, err
// 		}
// 		currValidators[idx] = validator
// 	}
//
// 	nextValidators := make([]Validator, len(ss.NextValidators))
// 	for idx, sValidator := range ss.NextValidators {
// 		validator, err := sValidator.deserialize()
// 		if err != nil {
// 			return SafroleState{}, err
// 		}
// 		nextValidators[idx] = validator
// 	}
//
// 	designedValidators := make([]Validator, len(ss.DesignedValidators))
// 	for idx, sValidator := range ss.DesignedValidators {
// 		validator, err := sValidator.deserialize()
// 		if err != nil {
// 			return SafroleState{}, err
// 		}
// 		designedValidators[idx] = validator
// 	}
//
// 	ticketsAccumulator := make([]SafroleAccumulator, len(ss.TicketsAccumulator))
// 	for idx, sAccumulator := range ss.TicketsAccumulator {
// 		accumulator, err := sAccumulator.deserialize()
// 		if err != nil {
// 			return SafroleState{}, err
// 		}
// 		ticketsAccumulator[idx] = accumulator
// 	}
//
// 	keys := make([]common.Hash, len(ss.TicketsOrKeys.Keys))
// 	for idx, key := range ss.TicketsOrKeys.Keys {
// 		if len(key) != 66 {
// 			return SafroleState{}, fmt.Errorf("invalid key length - got %d expected %d", len(key), 66)
// 		}
// 		keys[idx] = common.HexToHash(key)
// 	}
//
// 	ticketsVerifierKey, _ := hexutil.Decode(ss.TicketsVerifierKey)
// 	if len(ticketsVerifierKey) != TicketsVerifierKeyInBytes {
// 		return SafroleState{}, fmt.Errorf("invalid tickets verifier key length - got %d expected %d", len(ticketsVerifierKey), TicketsVerifierKeyInBytes)
// 	}
//
// 	return SafroleState{
// 		Timeslot:           ss.Timeslot,
// 		Entropy:            entropy,
// 		PrevValidators:     prevValidators,
// 		CurrValidators:     currValidators,
// 		NextValidators:     nextValidators,
// 		DesignedValidators: designedValidators,
// 		TicketsAccumulator: ticketsAccumulator,
// 		TicketsOrKeys: struct {
// 			Keys []common.Hash `json:"keys"`
// 		}{
// 			Keys: keys,
// 		},
// 		TicketsVerifierKey: ticketsVerifierKey,
// 	}, nil
// }

func (ss *SState) deserialize() (SafroleState, error) {
	if len(ss.Entropy) != 4 {
		return SafroleState{}, fmt.Errorf("invalid entropy length got %d expected 4", len(ss.Entropy))
	}
	entropy := make([]common.Hash, len(ss.Entropy))
	for idx, e := range ss.Entropy {
		if len(e) != 66 {
			return SafroleState{}, fmt.Errorf("invalid entropy length got %d expected 66", len(e))
		}
		entropy[idx] = common.HexToHash(e)
	}

	prevValidators := make([]Validator, len(ss.PrevValidators))
	for idx, sValidator := range ss.PrevValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return SafroleState{}, err
		}
		prevValidators[idx] = validator
	}

	currValidators := make([]Validator, len(ss.CurrValidators))
	for idx, sValidator := range ss.CurrValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return SafroleState{}, err
		}
		currValidators[idx] = validator
	}

	nextValidators := make([]Validator, len(ss.NextValidators))
	for idx, sValidator := range ss.NextValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return SafroleState{}, err
		}
		nextValidators[idx] = validator
	}

	designedValidators := make([]Validator, len(ss.DesignedValidators))
	for idx, sValidator := range ss.DesignedValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return SafroleState{}, err
		}
		designedValidators[idx] = validator
	}

	ticketsAccumulator := make([]TicketBody, len(ss.TicketsAccumulator))
	for idx, sAccumulator := range ss.TicketsAccumulator {
		accumulator, err := sAccumulator.deserialize()
		if err != nil {
			return SafroleState{}, err
		}
		ticketsAccumulator[idx] = accumulator
	}

	tickets := make([]*TicketBody, len(ss.TicketsOrKeys.Tickets))
	for idx, sTicket := range ss.TicketsOrKeys.Tickets {
		if len(sTicket.Id) != 66 {
			return SafroleState{}, fmt.Errorf("invalid ticket id length - got %d expected %d", len(sTicket.Id), 66)
		}
		tickets[idx] = &TicketBody{
			Id:      common.HexToHash(sTicket.Id),
			Attempt: sTicket.Attempt,
		}
	}

	keys := make([]common.Hash, len(ss.TicketsOrKeys.Keys))
	for idx, key := range ss.TicketsOrKeys.Keys {
		if len(key) != 66 {
			return SafroleState{}, fmt.Errorf("invalid key length - got %d expected %d", len(key), 66)
		}
		keys[idx] = common.HexToHash(key)
	}

	ticketsVerifierKey, _ := hexutil.Decode(ss.TicketsVerifierKey)
	if len(ticketsVerifierKey) != TicketsVerifierKeyInBytes {
		return SafroleState{}, fmt.Errorf("invalid tickets verifier key length - got %d expected %d", len(ticketsVerifierKey), TicketsVerifierKeyInBytes)
	}

	return SafroleState{
		Timeslot:           ss.Timeslot,
		Entropy:            entropy,
		PrevValidators:     prevValidators,
		CurrValidators:     currValidators,
		NextValidators:     nextValidators,
		DesignedValidators: designedValidators,
		TicketsAccumulator: ticketsAccumulator,
		TicketsOrKeys: TicketsOrKeys{
			Tickets: tickets,
			Keys:    keys,
		},
		TicketsVerifierKey: ticketsVerifierKey,
	}, nil
}

// Serialize function for Output

// Serialize function for Output
func (o *Output) serialize() SOutput {
	var epochMark *SEpochMark
	var ticketsMark []*SSafroleAccumulator

	if o.Ok != nil {
		if o.Ok.EpochMark != nil {
			epochMark = &SEpochMark{
				Entropy: o.Ok.EpochMark.Entropy.Hex(),
			}

			for _, validator := range o.Ok.EpochMark.Validators {
				epochMark.Validators = append(epochMark.Validators, validator.Hex())
			}
		}

		if o.Ok.TicketsMark != nil {
			tm := make([]*SSafroleAccumulator, len(o.Ok.TicketsMark))
			for i, ticket := range o.Ok.TicketsMark {
				tm[i] = &SSafroleAccumulator{
					Id:      ticket.Id.String(),
					Attempt: int(ticket.Attempt),
				}
			}
			ticketsMark = tm
		}
		return SOutput{
			Ok: &struct {
				EpochMark   *SEpochMark            `json:"epoch_mark"`
				TicketsMark []*SSafroleAccumulator `json:"tickets_mark"`
			}{
				EpochMark:   epochMark,
				TicketsMark: ticketsMark,
			},
		}
	} else {
		return SOutput{
			Ok: nil,
		}

	}

}

// Deserialize function for SOutput
func (so *SOutput) deserialize() (Output, error) {
	var output Output

	if so.Ok != nil {
		var epochMark *EpochMark
		var ticketsMark []*TicketBody

		if so.Ok.EpochMark != nil {
			entropy, err := hex.DecodeString(so.Ok.EpochMark.Entropy)
			if err != nil {
				return Output{}, err
			}

			validators := make([]common.Hash, len(so.Ok.EpochMark.Validators))
			for i, validator := range so.Ok.EpochMark.Validators {
				decoded, err := hex.DecodeString(validator)
				if err != nil {
					return Output{}, err
				}
				validators[i] = common.BytesToHash(decoded)
			}

			epochMark = &EpochMark{
				Entropy:    common.BytesToHash(entropy),
				Validators: validators,
			}
		}

		if so.Ok.TicketsMark != nil {
			tm := make([]*TicketBody, len(so.Ok.TicketsMark))
			for i, ticket := range so.Ok.TicketsMark {
				id, _ := hex.DecodeString(ticket.Id)
				tm[i] = &TicketBody{
					Id:      common.BytesToHash(id),
					Attempt: uint8(ticket.Attempt),
				}
			}
			ticketsMark = tm
		}

		output.Ok = &struct {
			EpochMark   *EpochMark    `json:"epoch_mark"`
			TicketsMark []*TicketBody `json:"tickets_mark"`
		}{
			EpochMark:   epochMark,
			TicketsMark: ticketsMark,
		}
	}

	return output, nil
}

func equalOutput(o1 SOutput, o2 SOutput) bool {
	o1JSON, _ := json.MarshalIndent(o1, "", "  ")
	o2JSON, _ := json.MarshalIndent(o2, "", "  ")
	if !bytes.Equal(o1JSON, o2JSON) {
		return false
	}
	return true
}

// Implement the equalEpochMark function
func equalEpochMark(e1, e2 SEpochMark) bool {
	return e1.Entropy == e2.Entropy && compareStringSlices(e1.Validators, e2.Validators)
}

// Implement the equalTicketsMark function
func equalTicketsMark(t1, t2 []SSafroleAccumulator) bool {
	if len(t1) != len(t2) {
		return false
	}
	for i := range t1 {
		if t1[i] != t2[i] {
			return false
		}
	}
	return true
}

// Helper function to compare two slices of strings
func compareStringSlices(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func equalState(s1 SState, s2 SState) error {
	if s2.Timeslot != s1.Timeslot {
		return fmt.Errorf("timeslot mismatch expected %d, got %d", s2.Timeslot, s1.Timeslot)
	}
	if len(s2.Entropy) != len(s1.Entropy) {
		return fmt.Errorf("entropy mismatch on len")
	}
	for i, s := range s2.Entropy {
		if s1.Entropy[i] != s {
			return fmt.Errorf("entropy mismatch on value %d: expected %s, got %s", i, s2.Entropy[i], s1.Entropy[i])
		}
	}
	if len(s2.TicketsAccumulator) != len(s1.TicketsAccumulator) {
		return fmt.Errorf("ticketsaccumulator mismatch on length s2 has %d, s1 has %d", len(s2.TicketsAccumulator), len(s1.TicketsAccumulator))
	}
	for i, t2 := range s2.TicketsAccumulator {
		t1 := s1.TicketsAccumulator[i]
		if t1.Id != t2.Id {
			return fmt.Errorf("ticketsaccumulator mismatch on item %d: s2 has %s, s1 has %s", i, t1.Id, t2.Id)
		}
	}

	if s2.TicketsVerifierKey != s1.TicketsVerifierKey {
		// return fmt.Errorf("TicketsVerifierKey mismatch on value")
	}

	return nil
}
