package safrole

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Define the structure for the JSON data
type SInput struct {
	Slot       int          `json:"slot"`
	Entropy    string       `json:"entropy"`
	Extrinsics []SExtrinsic `json:"extrinsics"`
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
	//should we expect extra here?
}
type SSafroleAccumulator struct {
	Id      string `json:"id"`
	Attempt int    `json:"attempt"`
}

type SState struct {
	Timeslot           int                   `json:"timeslot"`
	Entropy            []string              `json:"entropy"`
	PrevValidators     []SValidator          `json:"prev_validators"`
	CurrValidators     []SValidator          `json:"curr_validators"`
	NextValidators     []SValidator          `json:"next_validators"`
	DesignedValidators []SValidator          `json:"designed_validators"`
	TicketsAccumulator []SSafroleAccumulator `json:"tickets_accumulator"`
	TicketsOrKeys      struct {
		Keys []string `json:"keys"`
	} `json:"tickets_or_keys"`
	TicketsVerifierKey string `json:"tickets_verifier_key"`
}

type SEpochMark struct {
	Entropy    string   `json:"entropy"`
	Validators []string `json:"validators"`
}

type SOutput struct {
	Ok struct {
		EpochMark   SEpochMark            `json:"epoch_mark"`
		TicketsMark []SSafroleAccumulator `json:"tickets_mark"`
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
func (sa *SafroleAccumulator) serialize() SSafroleAccumulator {
	return SSafroleAccumulator{
		Id:      sa.Id.Hex(),
		Attempt: sa.Attempt,
	}
}

// SSafroleAccumulator to SafroleAccumulator
func (ssa *SSafroleAccumulator) deserialize() (SafroleAccumulator, error) {
	return SafroleAccumulator{
		Id:      common.HexToHash(ssa.Id),
		Attempt: ssa.Attempt,
	}, nil
}

// State to SState
func (s *State) serialize() SState {
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
		TicketsOrKeys: struct {
			Keys []string `json:"keys"`
		}{
			Keys: keys,
		},
		TicketsVerifierKey: common.Bytes2Hex(s.TicketsVerifierKey),
	}
}

// SState to State
func (ss *SState) deserialize() (State, error) {
	entropy := make([]common.Hash, len(ss.Entropy))
	for idx, e := range ss.Entropy {
		if len(e) != 66 {
			return State{}, fmt.Errorf("invalid entropy length got %d expected 66", len(e))
		}
		entropy[idx] = common.HexToHash(e)
	}

	prevValidators := make([]Validator, len(ss.PrevValidators))
	for idx, sValidator := range ss.PrevValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return State{}, err
		}
		prevValidators[idx] = validator
	}

	currValidators := make([]Validator, len(ss.CurrValidators))
	for idx, sValidator := range ss.CurrValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return State{}, err
		}
		currValidators[idx] = validator
	}

	nextValidators := make([]Validator, len(ss.NextValidators))
	for idx, sValidator := range ss.NextValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return State{}, err
		}
		nextValidators[idx] = validator
	}

	designedValidators := make([]Validator, len(ss.DesignedValidators))
	for idx, sValidator := range ss.DesignedValidators {
		validator, err := sValidator.deserialize()
		if err != nil {
			return State{}, err
		}
		designedValidators[idx] = validator
	}

	ticketsAccumulator := make([]SafroleAccumulator, len(ss.TicketsAccumulator))
	for idx, sAccumulator := range ss.TicketsAccumulator {
		accumulator, err := sAccumulator.deserialize()
		if err != nil {
			return State{}, err
		}
		ticketsAccumulator[idx] = accumulator
	}

	keys := make([]common.Hash, len(ss.TicketsOrKeys.Keys))
	for idx, key := range ss.TicketsOrKeys.Keys {
		if len(key) != 66 {
			return State{}, fmt.Errorf("invalid key length - got %d expected %d", len(key), 66)
		}
		keys[idx] = common.HexToHash(key)
	}

	ticketsVerifierKey, _ := hexutil.Decode(ss.TicketsVerifierKey)
	if len(ticketsVerifierKey) != TicketsVerifierKeyInBytes {
		return State{}, fmt.Errorf("invalid tickets verifier key length - got %d expected %d", len(ticketsVerifierKey), TicketsVerifierKeyInBytes)
	}

	return State{
		Timeslot:           ss.Timeslot,
		Entropy:            entropy,
		PrevValidators:     prevValidators,
		CurrValidators:     currValidators,
		NextValidators:     nextValidators,
		DesignedValidators: designedValidators,
		TicketsAccumulator: ticketsAccumulator,
		TicketsOrKeys: struct {
			Keys []common.Hash `json:"keys"`
		}{
			Keys: keys,
		},
		TicketsVerifierKey: ticketsVerifierKey,
	}, nil
}

// Serialize function for Output
func (o *Output) serialize() SOutput {
	epochMark := SEpochMark{
		Entropy: common.Bytes2Hex(o.Ok.EpochMark.Entropy),
	}

	for _, validator := range o.Ok.EpochMark.Validators {
		epochMark.Validators = append(epochMark.Validators, common.Bytes2Hex(validator))
	}

	ticketsMark := make([]SSafroleAccumulator, len(o.Ok.TicketsMark))
	for i, ticket := range o.Ok.TicketsMark {
		ticketsMark[i] = ticket.serialize()
	}

	return SOutput{
		Ok: struct {
			EpochMark   SEpochMark            `json:"epoch_mark"`
			TicketsMark []SSafroleAccumulator `json:"tickets_mark"`
		}{
			EpochMark:   epochMark,
			TicketsMark: ticketsMark,
		},
	}
}

// Deserialize function for SOutput
func (so *SOutput) deserialize() (Output, error) {
	entropy, _ := hexutil.Decode(so.Ok.EpochMark.Entropy)
	validators := make([][]byte, len(so.Ok.EpochMark.Validators))
	for i, validator := range so.Ok.EpochMark.Validators {
		decoded, _ := hexutil.Decode(validator)
		validators[i] = decoded
	}

	ticketsMark := make([]SafroleAccumulator, len(so.Ok.TicketsMark))
	for i, ticket := range so.Ok.TicketsMark {
		accumulator, err := ticket.deserialize()
		if err != nil {
			return Output{}, err
		}
		ticketsMark[i] = accumulator
	}

	return Output{
		Ok: struct {
			EpochMark   EpochMark            `json:"epoch_mark"`
			TicketsMark []SafroleAccumulator `json:"tickets_mark"`
		}{
			EpochMark: EpochMark{
				Entropy:    entropy,
				Validators: validators,
			},
			TicketsMark: ticketsMark,
		},
	}, nil
}

func equalOutput(o1 SOutput, o2 SOutput) bool {
	// TODO
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
		return fmt.Errorf("ticketsaccumulator mismatch on length")
	}
	if s2.TicketsVerifierKey != s1.TicketsVerifierKey {
		return fmt.Errorf("TicketsVerifierKey mismatch on value")
	}

	return nil
}
