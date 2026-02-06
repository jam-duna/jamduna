package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/jam-duna/jamduna/common"
	"github.com/jam-duna/jamduna/statedb"
	"github.com/jam-duna/jamduna/types"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <stf_file1.json> <stf_file2.json>\n", os.Args[0])
		os.Exit(1)
	}

	file1 := os.Args[1]
	file2 := os.Args[2]

	fmt.Printf("🔍 Comparing StateTransition files:\n")
	fmt.Printf("  File 1: %s\n", file1)
	fmt.Printf("  File 2: %s\n", file2)
	fmt.Printf("\n")

	// Read and parse file 1
	data1, err := os.ReadFile(file1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file 1: %v\n", err)
		os.Exit(1)
	}

	var stf1 statedb.StateTransition
	if err := json.Unmarshal(data1, &stf1); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing file 1: %v\n", err)
		os.Exit(1)
	}

	// Read and parse file 2
	data2, err := os.ReadFile(file2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file 2: %v\n", err)
		os.Exit(1)
	}

	var stf2 statedb.StateTransition
	if err := json.Unmarshal(data2, &stf2); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing file 2: %v\n", err)
		os.Exit(1)
	}

	// Compare the state transitions
	differences := compareStateTransitions(&stf1, &stf2)

	if len(differences) == 0 {
		fmt.Printf("✅ Files are identical!\n")
		os.Exit(0)
	}

	// Categorize differences
	var stateRootDiffs []string
	var lengthDiffs []string
	var onlyInFile1 []string
	var onlyInFile2 []string
	var valueDiffs []string
	var blockDiffs []string

	for _, diff := range differences {
		if strings.Contains(diff, "StateRoot:") {
			stateRootDiffs = append(stateRootDiffs, diff)
		} else if strings.Contains(diff, "length mismatch") {
			lengthDiffs = append(lengthDiffs, diff)
		} else if strings.Contains(diff, "only in file1") {
			onlyInFile1 = append(onlyInFile1, diff)
		} else if strings.Contains(diff, "only in file2") {
			onlyInFile2 = append(onlyInFile2, diff)
		} else if strings.Contains(diff, "Block.") {
			blockDiffs = append(blockDiffs, diff)
		} else {
			valueDiffs = append(valueDiffs, diff)
		}
	}

	fmt.Printf("❌ Found %d difference(s)\n\n", len(differences))
	fmt.Printf("=" + strings.Repeat("=", 79) + "\n")
	fmt.Printf("SUMMARY\n")
	fmt.Printf("=" + strings.Repeat("=", 79) + "\n\n")

	if len(stateRootDiffs) > 0 {
		fmt.Printf("🔴 STATE ROOT MISMATCHES: %d\n", len(stateRootDiffs))
		for _, diff := range stateRootDiffs {
			fmt.Printf("  • %s\n", diff)
		}
		fmt.Printf("\n")
	}

	if len(lengthDiffs) > 0 {
		fmt.Printf("📊 LENGTH MISMATCHES: %d\n", len(lengthDiffs))
		for _, diff := range lengthDiffs {
			fmt.Printf("  • %s\n", diff)
		}
		fmt.Printf("\n")
	}

	if len(onlyInFile1) > 0 {
		fmt.Printf("➕ KEYS ONLY IN FILE1: %d\n", len(onlyInFile1))
		for i, diff := range onlyInFile1 {
			fmt.Printf("  %d. %s\n", i+1, diff)
		}
		fmt.Printf("\n")
	}

	if len(onlyInFile2) > 0 {
		fmt.Printf("➖ KEYS ONLY IN FILE2: %d\n", len(onlyInFile2))
		for i, diff := range onlyInFile2 {
			fmt.Printf("  %d. %s\n", i+1, diff)
		}
		fmt.Printf("\n")
	}

	if len(blockDiffs) > 0 {
		fmt.Printf("📦 BLOCK DIFFERENCES: %d\n", len(blockDiffs))
		for i, diff := range blockDiffs {
			fmt.Printf("  %d. %s\n", i+1, diff)
		}
		fmt.Printf("\n")
	}

	if len(valueDiffs) > 0 {
		fmt.Printf("=" + strings.Repeat("=", 79) + "\n")
		fmt.Printf("🔍 VALUE MISMATCHES: %d\n", len(valueDiffs))
		fmt.Printf("=" + strings.Repeat("=", 79) + "\n\n")
		for i, diff := range valueDiffs {
			fmt.Printf("%d. %s\n\n", i+1, diff)
		}
	}

	os.Exit(1)
}

func compareStateTransitions(stf1, stf2 *statedb.StateTransition) []string {
	var diffs []string

	// Compare PreState
	preStateDiffs := compareStateSnapshots("PreState", &stf1.PreState, &stf2.PreState)
	diffs = append(diffs, preStateDiffs...)

	// Compare Block
	blockDiffs := compareBlocks("Block", &stf1.Block, &stf2.Block)
	diffs = append(diffs, blockDiffs...)

	// Compare PostState
	postStateDiffs := compareStateSnapshots("PostState", &stf1.PostState, &stf2.PostState)
	diffs = append(diffs, postStateDiffs...)

	return diffs
}

func compareStateSnapshots(prefix string, s1, s2 *statedb.StateSnapshotRaw) []string {
	var diffs []string

	// Compare StateRoot
	if s1.StateRoot.Hex() != s2.StateRoot.Hex() {
		diffs = append(diffs, fmt.Sprintf("%s.StateRoot: %s != %s",
			prefix, s1.StateRoot.Hex(), s2.StateRoot.Hex()))
	}

	// Compare KeyVals length
	if len(s1.KeyVals) != len(s2.KeyVals) {
		diffs = append(diffs, fmt.Sprintf("%s.KeyVals: length mismatch (%d != %d)",
			prefix, len(s1.KeyVals), len(s2.KeyVals)))
	}

	// Compare KeyVals contents
	keyMap1 := make(map[[31]byte][]byte)
	keyMap2 := make(map[[31]byte][]byte)

	for _, kv := range s1.KeyVals {
		keyMap1[kv.Key] = kv.Value
	}

	for _, kv := range s2.KeyVals {
		keyMap2[kv.Key] = kv.Value
	}

	// Find keys only in file1
	for key := range keyMap1 {
		if _, exists := keyMap2[key]; !exists {
			diffs = append(diffs, fmt.Sprintf("%s.KeyVals: key '0x%x' only in file1 (value: 0x%x)",
				prefix, truncateBytes(key[:], 20), truncateBytes(keyMap1[key], 20)))
		}
	}

	// Find keys only in file2
	for key := range keyMap2 {
		if _, exists := keyMap1[key]; !exists {
			diffs = append(diffs, fmt.Sprintf("%s.KeyVals: key '0x%x' only in file2 (value: 0x%x)",
				prefix, truncateBytes(key[:], 20), truncateBytes(keyMap2[key], 20)))
		}
	}

	// Compare values for common keys
	for key, val1 := range keyMap1 {
		if val2, exists := keyMap2[key]; exists {
			if !bytesEqual(val1, val2) {
				diffDetail := findBytesDifference(val1, val2)
				highlightedVal1 := highlightDifferences(val1, val2)
				highlightedVal2 := highlightDifferences(val2, val1)

				// Check if this is a core authorization key (0x01-0x16 for C1-C16)
				jsonRepresentation := tryDecodeAsJSON(key[:], val1, val2)

				diffs = append(diffs, fmt.Sprintf("%s.KeyVals[0x%x]: value mismatch (len=%d vs %d)\n    File1: %s\n    File2: %s\n    %s%s",
					prefix, key[:], len(val1), len(val2), highlightedVal1, highlightedVal2, diffDetail, jsonRepresentation))
			}
		}
	}

	return diffs
}

func compareBlocks(prefix string, b1, b2 *types.Block) []string {
	var diffs []string

	// Use reflection to compare all fields
	v1 := reflect.ValueOf(b1).Elem()
	v2 := reflect.ValueOf(b2).Elem()
	t := v1.Type()

	for i := 0; i < v1.NumField(); i++ {
		field := t.Field(i)
		fieldName := field.Name

		val1 := v1.Field(i)
		val2 := v2.Field(i)

		if !deepEqual(val1.Interface(), val2.Interface()) {
			diffs = append(diffs, fmt.Sprintf("%s.%s: mismatch\n    File1: %v\n    File2: %v",
				prefix, fieldName, formatValue(val1.Interface()), formatValue(val2.Interface())))
		}
	}

	return diffs
}

func deepEqual(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case []byte:
		if len(val) > 32 {
			return fmt.Sprintf("0x%x... (len=%d)", val[:32], len(val))
		}
		return fmt.Sprintf("0x%x", val)
	case string:
		return truncate(val, 80)
	default:
		s := fmt.Sprintf("%v", v)
		return truncate(s, 80)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if strings.HasPrefix(s, "0x") && maxLen > 10 {
		// For hex strings, show beginning and end
		half := (maxLen - 5) / 2
		return s[:2+half] + "..." + s[len(s)-half:]
	}
	return s[:maxLen-3] + "..."
}

func truncateBytes(b []byte, maxLen int) []byte {
	if len(b) <= maxLen {
		return b
	}
	return b[:maxLen]
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// highlightDifferences creates a hex string with differing bytes highlighted in red
func highlightDifferences(a, b []byte) string {
	const (
		colorRed   = "\033[31m"
		colorReset = "\033[0m"
	)

	var result strings.Builder
	result.WriteString("0x")

	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	// Compare bytes up to minLen
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			// Highlight differing byte in red
			result.WriteString(colorRed)
			result.WriteString(fmt.Sprintf("%02x", a[i]))
			result.WriteString(colorReset)
		} else {
			// Normal byte
			result.WriteString(fmt.Sprintf("%02x", a[i]))
		}
	}

	// If a is longer than b, highlight remaining bytes in red
	if len(a) > minLen {
		result.WriteString(colorRed)
		for i := minLen; i < len(a); i++ {
			result.WriteString(fmt.Sprintf("%02x", a[i]))
		}
		result.WriteString(colorReset)
	}

	return result.String()
}

func findBytesDifference(a, b []byte) string {
	const (
		colorRed   = "\033[31m"
		colorReset = "\033[0m"
	)

	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	var diffs []string
	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			diffs = append(diffs, fmt.Sprintf("byte[%d]: %s0x%02x%s != %s0x%02x%s",
				i, colorRed, a[i], colorReset, colorRed, b[i], colorReset))
			if len(diffs) >= 5 {
				diffs = append(diffs, "...")
				break
			}
		}
	}

	if len(a) != len(b) {
		diffs = append(diffs, fmt.Sprintf("length differs: %s%d%s vs %s%d%s",
			colorRed, len(a), colorReset, colorRed, len(b), colorReset))
	}

	if len(diffs) == 0 {
		return "Values appear identical (possible encoding issue)"
	}

	return "Differences: " + strings.Join(diffs, "; ")
}

// tryDecodeAsJSON attempts to decode SCALE-encoded values and display as JSON
// for core state keys (C1-C16) and other structured data
func tryDecodeAsJSON(key []byte, val1, val2 []byte) string {
	// Check if rest of key is zeros (state keys have first byte as identifier, rest zeros)
	allZeros := true
	for i := 1; i < len(key); i++ {
		if key[i] != 0 {
			allZeros = false
			break
		}
	}

	if !allZeros {
		return "" // Not a state key
	}

	keyByte := int(key[0])

	// Map of key types for display
	var stateNames = map[int]string{
		0x01: "C1: CoreAuthPool",
		0x02: "C2: AuthQueue",
		0x03: "C3: RecentBlocks",
		0x04: "C4: SafroleState",
		0x05: "C5: PastJudgements",
		0x06: "C6: Entropy",
		0x07: "C7: NextEpochValidatorKeys",
		0x08: "C8: CurrentValidatorKeys",
		0x09: "C9: PriorEpochValidatorKeys",
		0x0a: "C10: PendingReports",
		0x0b: "C11: MostRecentBlockTimeslot",
		0x0c: "C12: PrivilegedServiceIndices",
		0x0d: "C13: ActiveValidator",
		0x0e: "C14: AccumulationQueue",
		0x0f: "C15: AccumulationHistory",
		0x10: "C16: RecentBlocks",
	}

	keyType, isStateKey := stateNames[keyByte]
	if !isStateKey {
		return ""
	}

	const (
		colorRed   = "\033[31m"
		colorReset = "\033[0m"
	)

	var result strings.Builder
	result.WriteString("\n    Key Type: ")
	result.WriteString(keyType)
	result.WriteString("\n")

	// Decode SCALE-encoded values based on key type
	var decoded1, decoded2 interface{}
	var err1, err2 error

	switch keyByte {
	case 0x01: // C1: CoreAuthPool
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf([types.TotalCores][]common.Hash{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf([types.TotalCores][]common.Hash{}))
	case 0x02: // C2: AuthQueue
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(types.AuthorizationQueue{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(types.AuthorizationQueue{}))
	case 0x03: // C3: RecentBlocks
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(statedb.RecentBlocks{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(statedb.RecentBlocks{}))
	case 0x04: // C4: SafroleState
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(statedb.SafroleBasicState{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(statedb.SafroleBasicState{}))
	case 0x05: // C5: PastJudgements
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(statedb.DisputeState{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(statedb.DisputeState{}))
	case 0x06: // C6: Entropy
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(statedb.Entropy{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(statedb.Entropy{}))
	case 0x07: // C7: NextEpochValidatorKeys
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(types.Validators{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(types.Validators{}))
	case 0x08: // C8: CurrentValidatorKeys
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(types.Validators{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(types.Validators{}))
	case 0x09: // C9: PriorEpochValidatorKeys
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(types.Validators{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(types.Validators{}))
	case 0x0a: // C10: PendingReports
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(statedb.AvailabilityAssignments{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(statedb.AvailabilityAssignments{}))
	case 0x0b: // C11: MostRecentBlockTimeslot
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(uint32(0)))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(uint32(0)))
	case 0x0c: // C12: PrivilegedServiceIndices
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(types.PrivilegedServiceState{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(types.PrivilegedServiceState{}))
	case 0x0d: // C13: ActiveValidator
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(types.ValidatorStatistics{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(types.ValidatorStatistics{}))
	case 0x0e: // C14: AccumulationQueue
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf([types.EpochLength][]types.AccumulationQueue{}))
	case 0x0f: // C15: AccumulationHistory
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf([types.EpochLength]types.AccumulationHistory{}))
	case 0x10: // C16: RecentBlocks
		decoded1, _, err1 = types.Decode(val1, reflect.TypeOf(statedb.RecentBlocks{}))
		decoded2, _, err2 = types.Decode(val2, reflect.TypeOf(statedb.RecentBlocks{}))
	}

	if err1 != nil || err2 != nil {
		result.WriteString(fmt.Sprintf("    %sError decoding SCALE: file1=%v, file2=%v%s\n", colorRed, err1, err2, colorReset))
		return result.String()
	}

	// Convert to JSON
	json1, err1 := json.MarshalIndent(decoded1, "      ", "  ")
	json2, err2 := json.MarshalIndent(decoded2, "      ", "  ")

	if err1 != nil || err2 != nil {
		result.WriteString(fmt.Sprintf("    %sError marshaling JSON: file1=%v, file2=%v%s\n", colorRed, err1, err2, colorReset))
		return result.String()
	}

	// Check if they're identical
	if string(json1) == string(json2) {
		result.WriteString("    JSON representations are identical\n")
		return result.String()
	}

	// Show highlighted JSON diff
	result.WriteString("    File1 JSON:\n")
	result.WriteString(highlightJSONDifferences(string(json1), string(json2)))
	result.WriteString("\n\n    File2 JSON:\n")
	result.WriteString(highlightJSONDifferences(string(json2), string(json1)))
	result.WriteString("\n")

	return result.String()
}

// highlightJSONDifferences highlights differing parts of JSON strings
func highlightJSONDifferences(json1, json2 string) string {
	const (
		colorRed   = "\033[31m"
		colorReset = "\033[0m"
	)

	var result strings.Builder
	lines1 := strings.Split(json1, "\n")
	lines2 := strings.Split(json2, "\n")

	minLen := len(lines1)
	if len(lines2) < minLen {
		minLen = len(lines2)
	}

	for i := 0; i < minLen; i++ {
		if lines1[i] != lines2[i] {
			result.WriteString(colorRed)
			result.WriteString(lines1[i])
			result.WriteString(colorReset)
		} else {
			result.WriteString(lines1[i])
		}
		if i < len(lines1)-1 {
			result.WriteString("\n")
		}
	}

	// Add remaining lines if json1 is longer
	if len(lines1) > minLen {
		result.WriteString(colorRed)
		for i := minLen; i < len(lines1); i++ {
			result.WriteString("\n")
			result.WriteString(lines1[i])
		}
		result.WriteString(colorReset)
	}

	return result.String()
}
