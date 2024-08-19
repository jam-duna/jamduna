package trie

import (
	"github.com/colorfulnotion/jam/storage"
	"encoding/hex"
	"fmt"
)

func Initial_bpt(db *storage.StateDBStorage) ([]byte, *MerkleTree, error) {
	// Test data
	data := [][2]string{
		{"0000000000000000000000000000000000000000000000000000000000000000", "abcdef"},
		{"1111111111111111111111111111111111111111111111111111111111111111", "123456789987654321"},
	}

	decodedData := make([][2][]byte, len(data))
	for i, item := range data {
		key, err := hex.DecodeString(item[0])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode key: %v", err)
		}
		value, err := hex.DecodeString(item[1])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to decode value: %v", err)
		}
		decodedData[i] = [2][]byte{key, value}
	}

	// Create a new Merkle Tree
	tree := NewMerkleTree(nil, db)
	// defer tree.Close()

	for _, item := range decodedData {
		tree.Insert(item[0], item[1])
	}

	service_account_info := []string{
		/*
			ac: Service Accout code, same as Service Accout Primage value, random generate
			ab: Service Accout balance, random generate
			ag: Service Accout gas for accumulate funtion, random generate
			am: Service Accout gas for on_transfer funtion, random generate
			al: In GP(95), An index of an Service Accout
			ai: In GP(95), An index of an Service Accout
		*/

		// Follow GP(292):
		/*   "|-----------------------------value----------------------------||------ab------||------ag------||------am------||------al------||--ai--|"   */
		/**/ "0000000000000000000000000000000000000000000000000000000000000000a0860100000000000a000000000000000a00000000000000960000000000000003000000", // s=0
		/**/ "1111111111111111111111111111111111111111111111111111111111111111b77a0000000000000a000000000000000a00000000000000960000000000000003000000", // s=1
		/**/ "222222222222222222222222222222222222222222222222222222222222222239300000000000000a000000000000000a00000000000000960000000000000003000000", // s=2
		/**/ "333333333333333333333333333333333333333333333333333333333333333306120f00000000000a000000000000000a00000000000000960000000000000003000000", // s=3
		/**/ "44444444444444444444444444444444444444444444444444444444444444449f860100000000000a000000000000000a00000000000000960000000000000003000000", // s=4
	}

	service_account_storage := [][2]string{
		/*
			The way to generate "Service Account Storage Dict":

			func bhash(data []byte) common.Hash {
				hash := blake2b.Sum256(data)
				return common.BytesToHash(hash[:])
			}

			sbytes := make([]byte, 4)
			binary.LittleEndian.PutUint32(sbytes, 0) // s = 0,1,2,3,4
			vBytes := []byte{18, 52, 86, 120} // initial RAM
			k := bhash(append(sbytes, vBytes...))
			fmt.Println(k)

			value : Random generate
		*/

		// Follow GP(292):
		/*    "|----------------------------hash------------------------------|"  "|-value-|"*/
		/**/ {"e6f0db7107765905cfdc1f19af6eb8ff07d89626f47429556d9a52b4e8b001d7", "0123456789"}, /* s=0 */
		/**/ {"b0d9726bb896edffa662a893c33ecfd228237736dc999f3484ce07ef101d1326", "9876543210"}, /* s=1 */
		/**/ {"10d33da886f4650e774a8fa7749b56266acdd32a894588ba94bb9cfa966cad8a", "aabbccddee"}, /* s=2 */
		/**/ {"7cedd57efd9c8e2211d7d149d046ba9e8f9173659e6339dbda93a7192fe081f2", "0011223344"}, /* s=3 */
		/**/ {"089208209619955d3467790628a955b80582e4ea0a4af264b000de85df79ed32", "5566778899"}, /* s=4 */
	}

	service_account_preimage := [][2]string{
		/*
			The way to generate "Service Account Preimage Dict":

			func bhash(data []byte) common.Hash {
				hash := blake2b.Sum256(data)
				return common.BytesToHash(hash[:])
			}

			hash := bhash(value)
			value : Same as ac in service_account_info, represent code
		*/

		// Follow GP(292):
		/*    "|----------------------------hash------------------------------|"  "|-----------------------------value----------------------------|"*/
		/**/ {"89eb0d6a8a691dae2cd15ed0369931ce0a949ecafa5c3f93f8121833646e15c3", "0000000000000000000000000000000000000000000000000000000000000000"}, /* s=0 */
		/**/ {"d4ffaeeac45aa41825e0bc3f875570af061acbf0b950ad752ff0f9463fe13ad5", "1111111111111111111111111111111111111111111111111111111111111111"}, /* s=1 */
		/**/ {"e2a94e18647fe0c6283a31e40c46ae1cc5f0867650f6834e4f01e34284adc9c7", "2222222222222222222222222222222222222222222222222222222222222222"}, /* s=2 */
		/**/ {"510c7466f2a90281df576a765517adfc6a4c8f89fee3e14b8eae3a574f442c37", "3333333333333333333333333333333333333333333333333333333333333333"}, /* s=3 */
		/**/ {"0395256ce5d90f07504b614b9e70e29a06fdd69cef6b01f6018615164125a5c5", "4444444444444444444444444444444444444444444444444444444444444444"}, /* s=4 */
	}

	service_account_preimage_l := [][2]string{

		/*
			The way to generate "Service Account Preimage_l Dict":

			hash := preimage_value_length + false(preimage_key[4:])
			value : Service account preimage_l value, belongs to time slots, random generate
		*/

		// Follow GP(292):
		/*    "|----------------------------hash------------------------------|"  "|----------value---------|"*/
		/**/ {"200000007596e251d32ea12fc966ce31f56b613505a3c06c07ede7cc9b91ea3c", "0101000000"}, /* 				s=0 */
		/**/ {"200000003ba55be7da1f43c078aa8f50f9e5340f46af528ad00f06b9c01ec52a", "020100000002000000"}, /* 		s=1 */
		/**/ {"200000009b801f39d7c5ce1bf3b951e33a0f7989af097cb1b0fe1cbd7b523638", "03010000000200000003000000"}, /* s=2 */
		/**/ {"200000000d56fd7e20a89589aae8520395b37076011c1eb47151c5a8b0bbd3c8", "020200000003000000"}, /* 		s=3 */
		/**/ {"200000001a26f0f8afb49eb4618f1d65f90229631094fe09fe79eae9beda5a3a", "0103000000"}, /* 				s=4 */

	}

	// Insert service index and service account info
	for s, hexString := range service_account_info {
		service_account_info_byte, err := hex.DecodeString(hexString)
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		tree.SetService(255, uint32(s), service_account_info_byte)
	}
	// Insert service index, hash, storage value
	for s, hexString := range service_account_storage {
		service_account_storage_hash_byte, err := hex.DecodeString(hexString[0])
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		service_account_storage_value_byte, err := hex.DecodeString(hexString[1])
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		tree.SetPreImage(uint32(s), service_account_storage_hash_byte, service_account_storage_value_byte)
	}
	// Insert service index, hash, primage value
	for s, hexString := range service_account_preimage {
		service_account_primage_hash_byte, err := hex.DecodeString(hexString[0])
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		service_account_primage_value_byte, err := hex.DecodeString(hexString[1])
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		tree.SetPreImage(uint32(s), service_account_primage_hash_byte, service_account_primage_value_byte)
	}

	// Insert service index, (preimage_length, hash), timeslots
	for s, hexString := range service_account_preimage_l {
		service_account_primage_l_hash_byte, err := hex.DecodeString(hexString[0])
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		service_account_primage_l_value_byte, err := hex.DecodeString(hexString[1])
		if err != nil {
			fmt.Errorf("Failed to decode service value %s: %v", hexString, err)
		}
		tree.SetPreImage(uint32(s), service_account_primage_l_hash_byte, service_account_primage_l_value_byte)
	}

	// Get the root hash of the tree
	rootHash := tree.GetRootHash()
	return rootHash, tree, nil
}
