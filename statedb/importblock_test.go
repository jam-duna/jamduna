package statedb

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/colorfulnotion/jam/log"
	"github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"

	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"
)

var update_from_git = false

const (
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
)

// printHexDiff prints two byte slices as hex, highlighting any mismatched byte in red.
func printHexDiff(label string, exp, act []byte) {
	// Print the “Expected” line
	fmt.Printf("%-10s | Expected: 0x", label)
	max := len(exp)
	if len(act) > max {
		max = len(act)
	}
	for i := 0; i < max; i++ {
		var b byte
		var match bool
		if i < len(exp) {
			b = exp[i]
			if i < len(act) && exp[i] == act[i] {
				match = true
			}
		}
		hex := fmt.Sprintf("%02x", b)
		if !match {
			fmt.Print(colorRed, hex, colorReset)
		} else {
			fmt.Print(hex)
		}
	}
	fmt.Println()

	// Print the “Actual” line
	fmt.Printf("%-10s | Actual:   0x", label)
	for i := 0; i < max; i++ {
		var b byte
		var match bool
		if i < len(act) {
			b = act[i]
			if i < len(exp) && exp[i] == act[i] {
				match = true
			}
		}
		hex := fmt.Sprintf("%02x", b)
		if !match {
			fmt.Print(colorRed, hex, colorReset)
		} else {
			fmt.Print(hex)
		}
	}
	fmt.Println()
}

// func printColoredJSONDiff(diffStr string) {
// 	for _, line := range strings.Split(diffStr, "\n") {
// 		switch {
// 		case strings.HasPrefix(line, "-"):
// 			fmt.Println(colorRed + line + colorReset)
// 		case strings.HasPrefix(line, "+"):
// 			fmt.Println(colorGreen + line + colorReset)
// 		default:
// 			fmt.Println(line)
// 		}
// 	}
// }

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
func SortDiffKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		strip := func(k string) string {
			return strings.TrimSuffix(k, "|")
		}

		ti := strip(keys[i])
		tj := strip(keys[j])

		// attempt to parse c<digits>
		var (
			ni, nj     int
			errI, errJ error
		)
		if len(ti) > 1 && ti[0] == 'c' {
			ni, errI = strconv.Atoi(ti[1:])
		}
		if len(tj) > 1 && tj[0] == 'c' {
			nj, errJ = strconv.Atoi(tj[1:])
		}

		switch {
		// both are c<integer>: compare numerically
		case errI == nil && errJ == nil:
			return ni < nj
		// only i is c<integer>: i comes first
		case errI == nil:
			return true
		// only j is c<integer>: j comes first
		case errJ == nil:
			return false
		// neither: fallback to lexical on full key
		default:
			return keys[i] < keys[j]
		}
	})
}

func testSTF(t *testing.T, filename string, content string) {
	t.Helper()
	fmt.Printf("file: %s\n", filename)
	// 1) setup
	testDir := "/tmp/test_locala"
	test_storage, err := initStorage(testDir)
	if err != nil {
		t.Errorf("❌ [%s] Error initializing storage: %v", filename, err)
		return
	}
	defer test_storage.Close()

	// 2) parse the STF
	var stf StateTransition
	if strings.Contains(filename, ".bin") {
		stf0, _, _ := types.Decode([]byte(content), reflect.TypeOf(StateTransition{}))
		stf = stf0.(StateTransition)
	} else {
		if err := json.Unmarshal([]byte(content), &stf); err != nil {
			t.Errorf("❌ [%s] Failed to read JSON file: %v", filename, err)
			return
		}
	}
	//fmt.Printf("STF: %s\n", stf.String())

	// 3) do the state transition check
	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil)
	if err == nil {
		fmt.Printf("PostState.StateRoot %s matches\n", stf.PostState.StateRoot)
		return
	}

	// 4) collect & custom-sort the keys
	keys := make([]string, 0, len(diffs))
	for k := range diffs {
		keys = append(keys, k)
	}
	SortDiffKeys(keys) // your helper that orders c1…cN first, then the rest
	fmt.Printf("Diff on %d keys: %v\n", len(keys), keys)

	// 5) walk each diff
	for _, key := range keys {
		val := diffs[key]

		// human‐friendly name
		stateType := "unknown"
		if m := strings.TrimSuffix(val.ActualMeta, "|"); m != "" {
			stateType = m
		}

		fmt.Println(strings.Repeat("=", 40))
		fmt.Printf("\033[34mState Key: %s (%s)\033[0m\n", stateType, key)

		// raw‐byte diff
		fmt.Printf("%-10s | PreState : 0x%x\n", stateType, val.Prestate)
		printHexDiff(stateType, val.ExpectedPostState, val.ActualPostState)

		// JSON diff, if we know the struct type
		if stateType != "unknown" {

			expJSON, _ := StateDecodeToJson(val.ExpectedPostState, stateType)
			actJSON, _ := StateDecodeToJson(val.ActualPostState, stateType)

			differ := gojsondiff.New()
			delta, err := differ.Compare([]byte(expJSON), []byte(actJSON))
			if err != nil {
				fmt.Printf("  (error diffing JSON: %v)\n", err)
			} else if delta.Modified() {
				// unmarshal for the formatter
				var leftObj, rightObj interface{}
				_ = json.Unmarshal([]byte(expJSON), &leftObj)
				_ = json.Unmarshal([]byte(actJSON), &rightObj)

				cfg := formatter.AsciiFormatterConfig{
					ShowArrayIndex: true,
					Coloring:       true, // ANSI red/green text only
				}
				asciiFmt := formatter.NewAsciiFormatter(leftObj, cfg)
				asciiDiff, err := asciiFmt.Format(delta)
				if err != nil {
					fmt.Printf("  (error formatting diff: %v)\n", err)
				} else {
					fmt.Println(asciiDiff)
				}
				//			} else {
				//fmt.Printf("  %s JSON fully matched ✅\n", stateType)
			}

			fmt.Printf("------ %s JSON DONE ------\n", stateType)
		}

		fmt.Println(strings.Repeat("=", 40))
	}
	// finally fail the test
	t.Errorf("❌ [%s] Test failed: %v", filename, err)
}

func TestStateTransitionSingle(t *testing.T) {
	filename := "1_007.json"
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("failed to read file %s: %v", filename, err)
	}
	log.InitLogger("trace")
	log.EnableModule(log.PvmAuthoring)
	log.EnableModule(log.GeneralAuthoring)
	log.EnableModule(log.StateDBMonitoring)

	testSTF(t, filename, string(content))
}

// demonstrate random STF testing
func testSTFDir(t *testing.T, dir string) {
	t.Logf("Testing directory: %s\n", dir)
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir %s: %v", dir, err)
	}

	// filter for the JSON files we care about
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}

		if name != "00000000.json" && name != "00000000.bin" {
			files = append(files, filepath.Join(dir, name))
		}
	}
	if false {
		rand.Shuffle(len(files), func(i, j int) {
			files[i], files[j] = files[j], files[i]
		})
	}
	// run testSTF
	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", path, err)
		}
		testSTF(t, filepath.Base(path), string(content))
	}
}
func TestTraces(t *testing.T) {
	log.InitLogger("info")
	//log.EnableModule(log.PvmAuthoring)
	//log.EnableModule(log.GeneralAuthoring)
	//log.EnableModule(log.StateDBMonitoring)

	//testSTFDir(t, "/root/go/src/github.com/jam-duna/jamtestnet/data/assurances/state_transitions")
	testSTFDir(t, "../jamtestvectors/traces/fallback")
	testSTFDir(t, "../jamtestvectors/traces/safrole")
	testSTFDir(t, "../jamtestvectors/traces/reports-l0")
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
