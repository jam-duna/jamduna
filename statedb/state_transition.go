package statedb

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"

	storage "github.com/colorfulnotion/jam/storage"
	"github.com/colorfulnotion/jam/types"
	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"
)

const (
	colorReset = "\033[0m"
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
)

func InitStorage(testDir string) (*storage.StateDBStorage, error) {
	return initStorage(testDir)
}

func initStorage(testDir string) (*storage.StateDBStorage, error) {
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		err = os.MkdirAll(testDir, os.ModePerm)
		if err != nil {
			return nil, fmt.Errorf("failed to create directory /tmp/fuzz: %v", err)
		}
	}

	sdb_storage, err := storage.NewStateDBStorage(testDir, storage.NewMockJAMDA(), nil)
	if err != nil {
		return nil, fmt.Errorf("error with storage: %v", err)
	}
	return sdb_storage, nil

}

// printHexDiff prints two byte slices as hex, highlighting mismatched bytes in red.
// If data exceeds 1KB, truncates output with "..."
func printHexDiff(label string, exp, act []byte) {
	const maxDisplay = 1024 // 1KB limit

	// Check for missing keys
	if len(exp) == 0 && len(act) > 0 {
		fmt.Printf("%-10s | Expected: \033[33m<MISSING IN EXPECTED>\033[0m\n", label)
		fmt.Printf("%-10s | Actual:   0x%x\n", label, act)
		return
	}
	if len(act) == 0 && len(exp) > 0 {
		fmt.Printf("%-10s | Expected: 0x%x\n", label, exp)
		fmt.Printf("%-10s | Actual:   \033[33m<MISSING IN ACTUAL>\033[0m\n", label)
		return
	}

	// Helper function to print hex line
	printHexLine := func(prefix string, a, b []byte) {
		fmt.Printf("%-10s | %s0x", label, prefix)

		max := len(a)
		if len(b) > max {
			max = len(b)
		}
		if max > maxDisplay {
			max = maxDisplay
		}

		for i := 0; i < max; i++ {
			var val byte
			var match bool
			if i < len(a) {
				val = a[i]
				if i < len(b) && a[i] == b[i] {
					match = true
				}
			}
			hex := fmt.Sprintf("%02x", val)
			if !match {
				fmt.Print(colorRed, hex, colorReset)
			} else {
				fmt.Print(hex)
			}
		}

		if len(a) > maxDisplay || len(b) > maxDisplay {
			fmt.Print("...")
		}
		fmt.Println()
	}

	// Print both Expected and Actual
	printHexLine("Expected: ", exp, act)
	printHexLine("Actual:   ", act, exp)
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

func ValidateStateTransitionFile(filename string, storageDir string, outputDir string) (bool, error) {
	test_storage, err := initStorage(storageDir)
	if err != nil {

	}
	defer test_storage.Close()

	// 1) read the file
	content, err := os.ReadFile(filename)
	if err != nil {
		return false, fmt.Errorf(" [%s] Error reading file: %v", filename, err)
	}

	// 2) parse the STF
	var stf StateTransition
	if strings.Contains(filename, ".bin") {
		stf0, _, _ := types.Decode([]byte(content), reflect.TypeOf(StateTransition{}))
		stf = stf0.(StateTransition)
	} else {
		if err := json.Unmarshal([]byte(content), &stf); err != nil {

			return false, err
		}
	}
	//fmt.Printf("STF: %s\n", stf.String())

	// 3) do the state transition check
	diffs, err := CheckStateTransitionWithOutput(test_storage, &stf, nil, outputDir, false)
	if err == nil {
		fmt.Printf("✅ [%s] State transition succeeded with no diffs\n", filename)
		return false, err
	}

	fmt.Printf("QQQ⚠️ [%s] State transition failed with diffs: %v\n", filename, err.Error())

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
		fmt.Printf("\033[34mState Key: %s (%s)\033[0m\n", stateType, key[:64])

		// Special handling for c7 (validator keys) - parse and display validators
		if stateType == "c7" {
			parseAndLogValidators("PreState", val.Prestate)
			parseAndLogValidators("Expected", val.ExpectedPostState)
			parseAndLogValidators("Actual", val.ActualPostState)
		} else {
			// raw‐byte diff
			fmt.Printf("%-10s | PreState : 0x%x\n", stateType, val.Prestate)
			printHexDiff(stateType, val.ExpectedPostState, val.ActualPostState)
		}

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
	if len(diffs) > 0 {
		return true, fmt.Errorf("[%s] State transition failed with %d diffs", filename, len(diffs))
	}
	return false, nil
}
