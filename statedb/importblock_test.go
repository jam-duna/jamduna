package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/common"
	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/types"

	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"
)

var update_from_git = false

//	func printColoredJSONDiff(diffStr string) {
//		for _, line := range strings.Split(diffStr, "\n") {
//			switch {
//			case strings.HasPrefix(line, "-"):
//				fmt.Println(colorRed + line + colorReset)
//			case strings.HasPrefix(line, "+"):
//				fmt.Println(colorGreen + line + colorReset)
//			default:
//				fmt.Println(line)
//			}
//		}
//	}
func runSingleSTFTest(t *testing.T, filename string, content string) {
	t.Helper()

	testDir := "/tmp/test_locala"
	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return
	}
	defer test_storage.Close()

	stf, err := parseSTFFile(filename, content)
	if err != nil {
		t.Errorf("❌ [%s] Failed to parse STF: %v", filename, err)
		return
	}

	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil)
	if err == nil {
		fmt.Printf("✅ [%s] PostState.StateRoot %s matches\n", filename, stf.PostState.StateRoot)
		return
	}

	handleDiffs(filename, diffs)
	t.Errorf("❌ [%s] Test failed: %v", filename, err)
}

func parseSTFFile(filename, content string) (StateTransition, error) {
	var stf StateTransition
	var err error
	if strings.HasSuffix(filename, ".bin") {
		stf0, _, err := types.Decode([]byte(content), reflect.TypeOf(StateTransition{}))
		if err == nil {
			stf = stf0.(StateTransition)
		}
	} else {
		err = json.Unmarshal([]byte(content), &stf)
	}
	return stf, err
}

func handleDiffs(filename string, diffs map[string]DiffState) {
	keys := make([]string, 0, len(diffs))
	for k := range diffs {
		keys = append(keys, k)
	}
	SortDiffKeys(keys)

	fmt.Printf("Diff on %d keys: %v\n", len(keys), keys)
	for _, key := range keys {
		val := diffs[key]

		stateType := "unknown"
		if m := strings.TrimSuffix(val.ActualMeta, "|"); m != "" {
			stateType = m
		} else {
			keyFirstByte := common.FromHex(key)[0]
			if tmp, ok := StateKeyMap[keyFirstByte]; ok {
				stateType = tmp
			}
		}

		fmt.Println(strings.Repeat("=", 40))
		fmt.Printf("\033[34mState Key: %s (%s)\033[0m\n", stateType, key)
		fmt.Printf("%-10s | PreState : 0x%x\n", stateType, val.Prestate)
		printHexDiff(stateType, val.ExpectedPostState, val.ActualPostState)

		if stateType != "unknown" {
			expJSON, _ := StateDecodeToJson(val.ExpectedPostState, stateType)
			actJSON, _ := StateDecodeToJson(val.ActualPostState, stateType)

			differ := gojsondiff.New()
			delta, err := differ.Compare([]byte(expJSON), []byte(actJSON))
			if err == nil && delta.Modified() {
				var leftObj, rightObj interface{}
				_ = json.Unmarshal([]byte(expJSON), &leftObj)
				_ = json.Unmarshal([]byte(actJSON), &rightObj)

				cfg := formatter.AsciiFormatterConfig{ShowArrayIndex: true, Coloring: true}
				asciiFmt := formatter.NewAsciiFormatter(leftObj, cfg)
				asciiDiff, _ := asciiFmt.Format(delta)
				fmt.Println(asciiDiff)
			}
			fmt.Printf("------ %s JSON DONE ------\n", stateType)
		}
		fmt.Println(strings.Repeat("=", 40))
	}
}

func TestStateTransitionSingle(t *testing.T) {
	filename := "../jamtestvectors/traces/reports-l0/00000013.json"
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	t.Run(filepath.Base(filename), func(t *testing.T) {
		runSingleSTFTest(t, filename, string(content))
	})
}
func TestTraces(t *testing.T) {
	//testSTFDir(t, "/root/go/src/github.com/jam-duna/jamtestnet/data/assurances/state_transitions")
	//testSTFDir(t, "../jamtestvectors/traces/fallback")
	//testSTFDir(t, "../jamtestvectors/traces/safrole")
	dir := "../jamtestvectors/traces/reports-l0"
	log.InitLogger("info")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir %s: %v", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || e.Name() == "00000000.json" {
			continue
		}
		filename := filepath.Join(dir, e.Name())
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Errorf("failed to read file %s: %v", filename, err)
			continue
		}

		// ensure inner test failure bubbles up
		t.Run(e.Name(), func(t *testing.T) {
			runSingleSTFTest(t, filename, string(content))
		})
	}
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
