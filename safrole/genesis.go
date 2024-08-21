package safrole

import (
	//"fmt"
	// "github.com/colorfulnotion/jam/scale"
	//"encoding/binary"
	"fmt"
	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/types"

	//"golang.org/x/crypto/blake2b"
	"encoding/json"
	"io/ioutil"
	"time"
)

// GenesisConfig is the key data "genesis" structure loaded and saved at the start of every binary run
type GenesisConfig struct {
	Epoch0Timestamp uint64
	Authorities     []types.Validator
}

type GenesisAuthorities struct {
	Authorities []types.Validator
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

func ComputeCurrentJCETime() uint32 {
	currentTime := time.Now().Unix()
	return uint32(currentTime) // computeJCETime(currentTime)
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
	return GenesisConfig{
		Epoch0Timestamp: uint64(6 * ((now + 12) / 6)),
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

func InitValidator(bandersnatch_seed, ed25519_seed []byte) (types.Validator, error) {
	validator := types.Validator{}
	banderSnatch_pub, _, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, _, err := bandersnatch.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validator, fmt.Errorf("Failed to init Ed25519 Key")
	}
	validator.Ed25519 = common.BytesToHash(ed25519_pub)
	validator.Bandersnatch = common.BytesToHash(banderSnatch_pub)
	//fmt.Printf("validator %v\n", validator)
	return validator, nil
}

func InitValidatorSecret(bandersnatch_seed, ed25519_seed []byte) (types.ValidatorSecret, error) {
	validatorSecret := types.ValidatorSecret{}
	banderSnatch_pub, banderSnatch_priv, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init BanderSnatch Key")
	}
	ed25519_pub, ed25519_priv, err := bandersnatch.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("Failed to init Ed25519 Key")
	}
	validatorSecret.Ed25519Secret = ed25519_priv
	validatorSecret.Ed25519Pub = common.BytesToHash(ed25519_pub)

	validatorSecret.BandersnatchSecret = banderSnatch_priv
	validatorSecret.BandersnatchPub = common.BytesToHash(banderSnatch_pub)
	//fmt.Printf("validatorSecret %v\n", validatorSecret)
	return validatorSecret, nil
}
