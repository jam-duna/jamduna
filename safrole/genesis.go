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
	StartTimeslot   uint32
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
	return computeJCETime(currentTime)
}

func RoundUpToNextEpoch(currCJE uint32) uint32 {
	_, currentPhase := EpochAndPhase(currCJE)
	if currentPhase == 0 {
		return currCJE
	}
	return ((currCJE / EpochNumSlots) + 1) * EpochNumSlots
}

// Function to convert JCETime back to the original Unix timestamp
func JCETimeToUnixTimestamp(jceTime uint32) int64 {
	// Define the start of the Jam Common Era
	jceStart := time.Date(2024, time.January, 1, 12, 0, 0, 0, time.UTC)

	// Add the JCE time (in seconds) to the start time
	originalTime := jceStart.Add(time.Duration(jceTime) * time.Second)
	return originalTime.Unix()
}

func getCurrentTimestampRoundedToMinutes(delay uint64) uint64 {
	now := time.Now()
	rounded := now.Round(time.Minute)
	return uint64(rounded.Unix()) + delay
}

// Function to get the current timestamp rounded up to the nearest JCE time with phase = 0 and add a delay in seconds
func getCurrentTimestampRoundedToJCE(delay uint64) uint64 {
	now := time.Now().Unix()
	currCJE := computeJCETime(now + int64(delay))
	roundedCJE := RoundUpToNextEpoch(currCJE)
	roundedUnix := JCETimeToUnixTimestamp(roundedCJE)
	return uint64(roundedUnix)
}

const genesis_delay = 0

func NewGenesisConfig(validators []Validator) GenesisConfig {
	epoch0Timestamp := getCurrentTimestampRoundedToJCE(genesis_delay)
	rawEpoch0JCE := computeJCETime(int64(epoch0Timestamp))

	return GenesisConfig{
		Epoch0Timestamp: epoch0Timestamp,
		StartTimeslot:   rawEpoch0JCE,
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
