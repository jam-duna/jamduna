package statedb

import (
	"fmt"

	"encoding/json"
	"io/ioutil"
	"time"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"
)

// GenesisConfig is the key data "genesis" structure loaded and saved at the start of every binary run
type GenesisConfig struct {
	Epoch0Timestamp uint64            `json:"epoch0_timestamp,omitempty"`
	Authorities     []types.Validator `json:"authorities,omitempty"`
}

type GenesisAuthorities struct {
	Authorities []types.Validator
}

func InitStateFromSnapshot(s *StateSnapshot) (j *JamState) {
	j = NewJamState()
	// TODO
	return j
}

// 6.4.1 Startup parameters
func InitGenesisState(genesisConfig *GenesisConfig) (j *JamState) {
	j = NewJamState()

	// shift starting condition by one phase to make space for 0_0
	genesisTimeslot := uint32(genesisConfig.Epoch0Timestamp - types.PeriodSecond)
	fmt.Printf("InitGenesisState genesisTimeslot=%v\n", genesisTimeslot)

	j.SafroleState.EpochFirstSlot = uint32(genesisConfig.Epoch0Timestamp)
	j.SafroleState.Timeslot = genesisTimeslot
	j.SafroleState.BlockNumber = -1 // also not ok

	validators := genesisConfig.Authorities
	vB := []byte{}
	for _, v := range validators {
		vB = append(vB, v.Bytes()...)
	}
	j.SafroleState.PrevValidators = validators
	j.SafroleState.CurrValidators = validators
	j.SafroleState.NextValidators = validators
	j.SafroleState.DesignedValidators = validators

	/*
		The on-chain randomness is initialized after the genesis block construction.
		The first buffer entry is set as the Blake2b hash of the genesis block,
		each of the other entries is set as the Blake2b hash of the previous entry.
	*/

	// j.SafroleState.Entropy = make([]common.Hash, types.EntropySize)
	j.SafroleState.Entropy[0] = common.BytesToHash(common.ComputeHash(vB))                                //BLAKE2B hash of the genesis block#0
	j.SafroleState.Entropy[1] = common.BytesToHash(common.ComputeHash(j.SafroleState.Entropy[0].Bytes())) //BLAKE2B of Current
	j.SafroleState.Entropy[2] = common.BytesToHash(common.ComputeHash(j.SafroleState.Entropy[1].Bytes())) //BLAKE2B of EpochN1
	j.SafroleState.Entropy[3] = common.BytesToHash(common.ComputeHash(j.SafroleState.Entropy[2].Bytes())) //BLAKE2B of EpochN2
	j.SafroleState.TicketsOrKeys.Keys, _ = j.SafroleState.ChooseFallBackValidator()
	j.DisputesState = Psi_state{}
	//j.AvailabilityAssignments = make([types.TotalCores]*Rho_state, 0)

	return j
}

// The current time expressed in seconds after the start of the Jam Common Era. See section 4.4
func computeJCETime(unixTimestamp int64) uint32 {
	// Define the start of the Jam Common Era
	jceStart := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

	// Convert the Unix timestamp to a Time object
	currentTime := time.Unix(unixTimestamp, 0).UTC()

	// Calculate the difference in seconds
	diff := currentTime.Sub(jceStart)
	return uint32(diff.Seconds())
}

// Function to convert JCETime back to the original Unix timestamp
func JCETimeToUnixTimestamp(jceTime uint32) int64 {
	// Define the start of the Jam Common Era
	jceStart := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

	// Add the JCE time (in seconds) to the start time
	originalTime := jceStart.Add(time.Duration(jceTime) * time.Second)
	return originalTime.Unix()
}

func NewGenesisConfig(validators []types.Validator) GenesisConfig {
	now := time.Now().Unix()
	epoch0Timestamp := uint64(6 * ((now + 12 + types.SecondsPerSlot) / 6))
	fmt.Printf("!!!NewGenesisConfig epoch0Timestamp: %v\n", epoch0Timestamp)
	return GenesisConfig{
		Epoch0Timestamp: epoch0Timestamp,
		Authorities:     validators,
	}
}

// writeGenesisConfig writes the genesis configuration to a JSON file
func writeGenesisConfig(filePath string, config *GenesisAuthorities) error {
	// Marshal the GenesisAuthorities struct into JSON
	bytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Write the JSON data to the specified file
	err = ioutil.WriteFile(filePath, bytes, 0644)
	if err != nil {
		return err
	}
	return nil
}

// readGenesisConfig reads the genesis configuration from a JSON file
func readGenesisConfig(filePath string) (*GenesisAuthorities, error) {
	// Read the JSON data from the specified file
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Unmarshal the JSON data into the GenesisAuthorities struct
	var config GenesisAuthorities
	err = json.Unmarshal(bytes, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (g *GenesisConfig) String() string {
	enc, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		// Handle the error according to your needs.
		return fmt.Sprintf("Error marshaling JSON: %v", err)
	}
	return string(enc)
}

func InitValidator(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata string) (types.Validator, error) {
	validator := types.Validator{}
	banderSnatch_pub, _, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, _, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init Ed25519 Key")
	}
	bls_pub, _, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init BanderSnatch Key")
	}

	validator.Ed25519 = ed25519_pub
	copy(validator.Bandersnatch[:], banderSnatch_pub.Bytes())
	copy(validator.Metadata[:], []byte(metadata))
	copy(validator.Bls[:], bls_pub.Bytes())
	return validator, nil
}

func InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata string) (types.ValidatorSecret, error) {
	validatorSecret := types.ValidatorSecret{}
	banderSnatch_pub, banderSnatch_priv, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, ed25519_priv, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init Ed25519 Key")
	}
	bls_pub, bls_priv, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init BLS Key")
	}

	copy(validatorSecret.Ed25519Secret[:], ed25519_priv[:])
	validatorSecret.Ed25519Pub = ed25519_pub

	validatorSecret.BandersnatchSecret = banderSnatch_priv.Bytes()
	validatorSecret.BandersnatchPub = types.BandersnatchKey(banderSnatch_pub)

	copy(validatorSecret.BlsSecret[:], bls_priv.Bytes())
	copy(validatorSecret.BlsPub[:], bls_pub.Bytes())

	copy(validatorSecret.Metadata[:], []byte(metadata))
	return validatorSecret, nil
}
