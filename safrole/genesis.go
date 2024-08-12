package safrole

import (
	//"fmt"
	// "github.com/colorfulnotion/jam/scale"
	//"encoding/binary"

	//"github.com/colorfulnotion/jam/common"
	//"golang.org/x/crypto/blake2b"
	"encoding/json"
	"io/ioutil"
	"time"
)

// GenesisConfig is the key data "genesis" structure loaded and saved at the start of every binary run
type GenesisConfig struct {
	Epoch0Timestamp uint64
	Authorities     []Validator
}

type GenesisAuthorities struct {
	Authorities []Validator
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

func NewGenesisConfig(validators []Validator) GenesisConfig {
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
