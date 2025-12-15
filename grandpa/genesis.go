package grandpa

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"

	"github.com/colorfulnotion/jam/bandersnatch"
	"github.com/colorfulnotion/jam/bls"
	"github.com/colorfulnotion/jam/types"
)

func GenerateSeedSet(ringSize int) ([][]byte, error) {
	ringSet := make([][]byte, ringSize)
	for i := uint32(0); i < uint32(ringSize); i++ {
		seed := make([]byte, 32)
		for j := 0; j < 8; j++ {
			binary.LittleEndian.PutUint32(seed[j*4:], i)
		}
		ringSet[i] = seed
	}
	return ringSet, nil
}

func InitValidator(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata []byte) (types.Validator, error) {
	validator := types.Validator{}
	banderSnatch_pub, _, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validator, fmt.Errorf("failed to init BanderSnatch Key")
	}
	ed25519_pub, _, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validator, fmt.Errorf("failed to init Ed25519 Key")
	}
	bls_pub, _, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validator, fmt.Errorf("failed to init BanderSnatch Key")
	}

	validator.Ed25519 = ed25519_pub
	copy(validator.Bandersnatch[:], banderSnatch_pub.Bytes())
	copy(validator.Metadata[:], metadata)
	copy(validator.Bls[:], bls_pub.Bytes())
	return validator, nil
}

func InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed []byte, metadata []byte) (types.ValidatorSecret, error) {
	validatorSecret := types.ValidatorSecret{}
	banderSnatch_pub, banderSnatch_priv, err := bandersnatch.InitBanderSnatchKey(bandersnatch_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("failed to init BanderSnatch Key")
	}
	ed25519_pub, ed25519_priv, err := types.InitEd25519Key(ed25519_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("failed to init Ed25519 Key")
	}
	bls_pub, bls_priv, err := bls.InitBLSKey(bls_seed)
	if err != nil {
		return validatorSecret, fmt.Errorf("failed to init BLS Key")
	}

	copy(validatorSecret.Ed25519Secret[:], ed25519_priv[:])
	validatorSecret.Ed25519Pub = ed25519_pub

	validatorSecret.BandersnatchSecret = banderSnatch_priv.Bytes()
	validatorSecret.BandersnatchPub = types.BandersnatchKey(banderSnatch_pub)

	copy(validatorSecret.BlsSecret[:], bls_priv.Bytes())
	copy(validatorSecret.BlsPub[:], bls_pub.Bytes())

	copy(validatorSecret.Metadata[:], metadata)
	return validatorSecret, nil
}

func GenerateValidatorSecretSetToPath(numNodes int, save bool, dataDir ...string) ([]types.Validator, []types.ValidatorSecret, error) {

	seeds, _ := GenerateSeedSet(numNodes)
	validators := make([]types.Validator, numNodes)
	validatorSecrets := make([]types.ValidatorSecret, numNodes)

	for i := 0; i < int(numNodes); i++ {

		seed_i := seeds[i]
		if len(dataDir) != 0 {
			keyDir := dataDir[0]
			keyDir = filepath.Join(keyDir, "keys") // store the keys in a subdir
			// if there is no seed file, create it
			if err := os.MkdirAll(keyDir, 0700); err != nil {
				return validators, validatorSecrets, fmt.Errorf("failed to create keys directory %s: %v", dataDir, err)
			}
			if save {
				seedFile := filepath.Join(keyDir, fmt.Sprintf("seed_%d", i))

				if _, err := os.Stat(seedFile); os.IsNotExist(err) {
					// create the file
					f, err := os.Create(seedFile)
					if err != nil {
						return validators, validatorSecrets, fmt.Errorf("failed to create seed file %s", seedFile)
					}
					// write the seed to the file
					_, err = f.Write(seed_i)
					if err != nil {
						return validators, validatorSecrets, fmt.Errorf("failed to write seed to file %s", seedFile)
					}
					fmt.Printf("Seed file %s created\n", seedFile)
					f.Close()
				}

			}
		}

		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata := []byte{}
		//metadata, _ := generateMetadata(i) // this is NOT used by other teams. somehow we agreed on empty metadata for now

		validator, err := InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator %v", i)
		}
		validators[i] = validator

		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator secret=%v", i)
		}
		validatorSecrets[i] = validatorSecret
	}

	return validators, validatorSecrets, nil
}

func GenerateValidatorSecretSet(numNodes int) ([]types.Validator, []types.ValidatorSecret, error) {

	seeds, _ := GenerateSeedSet(numNodes)
	validators := make([]types.Validator, numNodes)
	validatorSecrets := make([]types.ValidatorSecret, numNodes)

	for i := 0; i < int(numNodes); i++ {

		seed_i := seeds[i]
		bandersnatch_seed := seed_i
		ed25519_seed := seed_i
		bls_seed := seed_i
		metadata := []byte{}
		//metadata, _ := generateMetadata(i) // this is NOT used by other teams. somehow we agreed on empty metadata for now

		validator, err := InitValidator(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator %v", i)
		}
		validators[i] = validator

		//bandersnatch_seed, ed25519_seed, bls_seed
		validatorSecret, err := InitValidatorSecret(bandersnatch_seed, ed25519_seed, bls_seed, metadata)
		if err != nil {
			return validators, validatorSecrets, fmt.Errorf("failed to init validator secret=%v", i)
		}
		validatorSecrets[i] = validatorSecret
	}

	return validators, validatorSecrets, nil
}

func GenerateValidatorPubKeyFromSeed(seed []byte) (types.Validator, error) {
	// Generate the validator public key from the seed
	validator, err := InitValidator(seed, seed, seed, []byte{})
	if err != nil {
		return types.Validator{}, fmt.Errorf("failed to init validator %v", err)
	}
	return validator, nil
}
