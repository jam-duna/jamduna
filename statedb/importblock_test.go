package statedb

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/google/go-github/v58/github"
	"github.com/nsf/jsondiff"
)

var update_from_git = false

func getGithubDirFile(owner string, repo string, branch string, folderPath string) (filenames []string, contents []string, err error) {
	client := github.NewClient(nil)
	localDir := fmt.Sprintf("testdata/%s/%s", owner, repo)

	if update_from_git {
		// Get all files in the folder
		_, dircontents, _, err := client.Repositories.GetContents(context.Background(), owner, repo, folderPath, &github.RepositoryContentGetOptions{Ref: branch})
		if err != nil {
			panic(err) // "Error fetching folder contents: %v", err
		}

		// Create local directory if it doesn't exist
		if _, err := os.Stat(localDir); os.IsNotExist(err) {
			err = os.MkdirAll(localDir, os.ModePerm)
			if err != nil {
				return nil, nil, fmt.Errorf("Failed to create directory %s: %v", localDir, err)
			}
		}

		for _, file := range dircontents {
			// Ensure this is a file and not a folder
			if file.GetType() == "file" && file.GetName()[len(file.GetName())-5:] == ".json" {
				fmt.Printf("📂 Reading file: %s\n", file.GetName())

				// Get file content
				fileContent, _, _, err := client.Repositories.GetContents(context.Background(), owner, repo, file.GetPath(), &github.RepositoryContentGetOptions{Ref: branch})
				if err != nil {
					return nil, nil, fmt.Errorf("Error fetching file content: %v", err)
				}

				// Decode content
				content, err := fileContent.GetContent()
				if err != nil {
					return nil, nil, fmt.Errorf("Error decoding file content: %v", err)
				}

				// Write content to local file
				localFilePath := fmt.Sprintf("%s/%s", localDir, file.GetName())
				err = os.WriteFile(localFilePath, []byte(content), 0644)
				if err != nil {
					return nil, nil, fmt.Errorf("Error writing file %s: %v", localFilePath, err)
				}

				fmt.Printf("✅ Successfully read and stored: %s\n", file.GetName())

				filenames = append(filenames, file.GetName())
				contents = append(contents, content)
			}
		}
	} else {
		// Read files from local directory
		files, err := os.ReadDir(localDir)
		if err != nil {
			return nil, nil, fmt.Errorf("Error reading local directory %s: %v", localDir, err)
		}

		for _, file := range files {
			if !file.IsDir() && file.Name()[len(file.Name())-5:] == ".json" {
				localFilePath := fmt.Sprintf("%s/%s", localDir, file.Name())
				content, err := os.ReadFile(localFilePath)
				if err != nil {
					return nil, nil, fmt.Errorf("Error reading file %s: %v", localFilePath, err)
				}

				fmt.Printf("✅ Successfully read from local: %s\n", file.Name())

				filenames = append(filenames, file.Name())
				contents = append(contents, string(content))
			}
		}
	}
	return filenames, contents, nil
}
func initStorage(testDir string) (*storage.StateDBStorage, error) {
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("Failed to create directory /tmp/fuzz: %v", err)
		}
	}

	sdb_storage, err := storage.NewStateDBStorage(testDir)
	if err != nil {
		return nil, fmt.Errorf("Error with storage: %v", err)
	}
	return sdb_storage, nil

}
func testSTF(t *testing.T, filename string, content string) {
	t.Helper()
	testDir := "/tmp/test_locala"
	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return
	}
	defer test_storage.Close()

	fmt.Printf("🔍 Testing file: %s\n", filename)
	fmt.Println("---------------------------------")

	var stf StateTransition
	err = json.Unmarshal([]byte(content), &stf)
	if err != nil {
		t.Errorf("❌ [%s] Failed to read JSON file: %v", filename, err)
		return
	}

	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil)
	if err != nil {
		for key, value := range diffs {
			// so the key return will be c3|
			// want to be C3
			state_key := key[:len(key)-1]
			fmt.Printf("========================================\n")
			fmt.Printf("file:%s\n", filename)
			fmt.Printf("\033[34mState Key:%s\033[0m\n", state_key)
			fmt.Printf("Block:%s\n", stf.Block.String())
			fmt.Printf("Val0 (PreState):%x\n", value.Prestate)
			fmt.Printf("Val0 (our):%x\n", value.PoststateCompared)
			fmt.Printf("Val1 (their):%x\n", value.Poststate)
			pre_state_json, err := StateDecodeToJson(value.Prestate, state_key)
			if err != nil {
				t.Errorf("❌ [%s] Failed to decode JSON file: %v", filename, err)
				return
			}
			fmt.Printf("PreState JSON:%s\n", pre_state_json)
			val_0_json, err := StateDecodeToJson(value.PoststateCompared, state_key)
			if err != nil {
				t.Errorf("❌ [%s] Failed to decode JSON file: %v", filename, err)
				return
			}
			val_1_json, err := StateDecodeToJson(value.Poststate, state_key)
			if err != nil {
				t.Errorf("❌ [%s] Failed to decode JSON file: %v", filename, err)
				return
			}
			opts := jsondiff.DefaultJSONOptions()
			diff, diffStr := jsondiff.Compare([]byte(val_0_json), []byte(val_1_json), &opts)
			if diff != jsondiff.FullMatch {
				fmt.Printf("Diff: %s\n", diffStr)
			}
			fmt.Printf("========================================\n")
		}
		t.Errorf("❌ [%s] Test failed: %v", filename, err)
	}
}

func TestStateTransitionSingle(t *testing.T) {
	filename := "4_005.json"
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("debug")
	testSTF(t, filename, string(content))
}

func TestCompareJson(t *testing.T) {
	var testdata1 types.Validator
	var testdata2 types.Validator
	testdata1 = types.Validator{
		Ed25519: types.HexToEd25519Key("0x1"),
	}
	testdata2 = types.Validator{
		Ed25519: types.HexToEd25519Key("0x2"),
	}
	diff := CompareJSON(testdata1, testdata2)
	fmt.Print(diff)
}

func TestStateTranistionForPublish(t *testing.T) {
	testDir := "/tmp/test_local"
	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ Error initializing storage: %v", err)
		return
	}
	defer test_storage.Close()
	//../cmd/importblocks/data/orderedaccumulation/state_transitions
	// read til ../cmd/importblocks/data
	// and get the mode "orderedaccumulation"
	// and get the state_transitions and read all the json file

	data_dir := "../cmd/importblocks/data"
	mode_dirs, err := os.ReadDir(data_dir)
	if err != nil {
		t.Errorf("❌ Error reading directory: %v", err)
		return
	}
	for _, mode_dir := range mode_dirs {
		if !mode_dir.IsDir() {
			continue
		}
		mode_dir_path := fmt.Sprintf("%s/%s", data_dir, mode_dir.Name())
		mode_dir_path = fmt.Sprintf("%s/state_transitions", mode_dir_path)
		data_files, err := os.ReadDir(mode_dir_path)
		if err != nil {
			t.Errorf("❌ Error reading directory: %v", err)
			return
		}

		for _, file := range data_files {
			// check if the file is a json file
			if file.IsDir() || file.Name()[len(file.Name())-5:] != ".json" {
				continue
			}

			filepath := fmt.Sprintf("%s/%s", mode_dir_path, file.Name())
			StateTransitionCheckForFile(t, filepath, test_storage)
		}
	}
}

func StateTransitionCheckForFile(t *testing.T, file string, storage *storage.StateDBStorage) {
	t.Helper()
	var stf StateTransition
	data, err := os.ReadFile(file)
	if err != nil {
		t.Errorf("❌ Failed to read JSON file: %v", err)
		return
	}
	err = json.Unmarshal(data, &stf)
	if err != nil {
		t.Errorf("❌ Failed to read JSON file: %v", err)
		return
	}
	s0, err := NewStateDBFromSnapshotRaw(storage, &(stf.PreState))
	if err != nil {
		t.Errorf("❌ Failed to create StateDB from snapshot: %v", err)
		return
	}
	s1, err := NewStateDBFromSnapshotRaw(storage, &(stf.PostState))
	if err != nil {
		t.Errorf("❌ Failed to create StateDB from snapshot: %v", err)
		return
	}
	errornum, diffs := ValidateSTF(s0, stf.Block, s1)
	if errornum > 0 {
		t.Errorf("❌ [%s] Test failed: %v", file, errornum)
		fmt.Printf("ErrorNum:%d\n", errornum)
	}
	for key, value := range diffs {
		fmt.Printf("File %s,error %s =========\n", file, key)
		fmt.Printf("%s", value)
	}
}
