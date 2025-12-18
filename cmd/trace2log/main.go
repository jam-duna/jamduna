// trace2log converts split PVM binary trace files (.gz) to PvmLogging text format
//
// Usage:
//
//	trace2log <trace_directory>              # Auto-write logs to trace directory
//	trace2log -v <trace_directory>           # Show logs in terminal (verbose)
//	trace2log -o <output_dir> <trace_dir>    # Write logs to specified output directory
//
// For nested trace structures (e.g., 0/516569628, 0/1985398958):
//   - Creates 0_all_traces.log (combined) and 0_516569628.log, 0_1985398958.log (per service)
//
// Directory structure:
//   - First level (0, 1, 2...): Work item index / core index
//   - Second level (516569628, etc.): Service ID
package main

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var opcodeMap = map[byte]string{
	0:   "TRAP",
	1:   "FALLTHROUGH",
	10:  "ECALLI",
	20:  "LOAD_IMM_64",
	30:  "STORE_IMM_U8",
	31:  "STORE_IMM_U16",
	32:  "STORE_IMM_U32",
	33:  "STORE_IMM_U64",
	40:  "JUMP",
	50:  "JUMP_IND",
	51:  "LOAD_IMM",
	52:  "LOAD_U8",
	53:  "LOAD_I8",
	54:  "LOAD_U16",
	55:  "LOAD_I16",
	56:  "LOAD_U32",
	57:  "LOAD_I32",
	58:  "LOAD_U64",
	59:  "STORE_U8",
	60:  "STORE_U16",
	61:  "STORE_U32",
	62:  "STORE_U64",
	70:  "STORE_IMM_IND_U8",
	71:  "STORE_IMM_IND_U16",
	72:  "STORE_IMM_IND_U32",
	73:  "STORE_IMM_IND_U64",
	80:  "LOAD_IMM_JUMP",
	81:  "BRANCH_EQ_IMM",
	82:  "BRANCH_NE_IMM",
	83:  "BRANCH_LT_U_IMM",
	84:  "BRANCH_LE_U_IMM",
	85:  "BRANCH_GE_U_IMM",
	86:  "BRANCH_GT_U_IMM",
	87:  "BRANCH_LT_S_IMM",
	88:  "BRANCH_LE_S_IMM",
	89:  "BRANCH_GE_S_IMM",
	90:  "BRANCH_GT_S_IMM",
	100: "MOVE_REG",
	101: "SBRK",
	102: "COUNT_SET_BITS_64",
	103: "COUNT_SET_BITS_32",
	104: "LEADING_ZERO_BITS_64",
	105: "LEADING_ZERO_BITS_32",
	106: "TRAILING_ZERO_BITS_64",
	107: "TRAILING_ZERO_BITS_32",
	108: "SIGN_EXTEND_8",
	109: "SIGN_EXTEND_16",
	110: "ZERO_EXTEND_16",
	111: "REVERSE_BYTES",
	120: "STORE_IND_U8",
	121: "STORE_IND_U16",
	122: "STORE_IND_U32",
	123: "STORE_IND_U64",
	124: "LOAD_IND_U8",
	125: "LOAD_IND_I8",
	126: "LOAD_IND_U16",
	127: "LOAD_IND_I16",
	128: "LOAD_IND_U32",
	129: "LOAD_IND_I32",
	130: "LOAD_IND_U64",
	131: "ADD_IMM_32",
	132: "AND_IMM",
	133: "XOR_IMM",
	134: "OR_IMM",
	135: "MUL_IMM_32",
	136: "SET_LT_U_IMM",
	137: "SET_LT_S_IMM",
	138: "SHLO_L_IMM_32",
	139: "SHLO_R_IMM_32",
	140: "SHAR_R_IMM_32",
	141: "NEG_ADD_IMM_32",
	142: "SET_GT_U_IMM",
	143: "SET_GT_S_IMM",
	144: "SHLO_L_IMM_ALT_32",
	145: "SHLO_R_IMM_ALT_32",
	146: "SHAR_R_IMM_ALT_32",
	147: "CMOV_IZ_IMM",
	148: "CMOV_NZ_IMM",
	149: "ADD_IMM_64",
	150: "MUL_IMM_64",
	151: "SHLO_L_IMM_64",
	152: "SHLO_R_IMM_64",
	153: "SHAR_R_IMM_64",
	154: "NEG_ADD_IMM_64",
	155: "SHLO_L_IMM_ALT_64",
	156: "SHLO_R_IMM_ALT_64",
	157: "SHAR_R_IMM_ALT_64",
	158: "ROT_R_64_IMM",
	159: "ROT_R_64_IMM_ALT",
	160: "ROT_R_32_IMM",
	161: "ROT_R_32_IMM_ALT",
	170: "BRANCH_EQ",
	171: "BRANCH_NE",
	172: "BRANCH_LT_U",
	173: "BRANCH_LT_S",
	174: "BRANCH_GE_U",
	175: "BRANCH_GE_S",
	180: "LOAD_IMM_JUMP_IND",
	200: "ADD_32",
	201: "SUB_32",
	202: "MUL_32",
	203: "DIV_U_32",
	204: "DIV_S_32",
	205: "REM_U_32",
	206: "REM_S_32",
	207: "SHLO_L_32",
	208: "SHLO_R_32",
	209: "SHAR_R_32",
	210: "ADD_64",
	211: "SUB_64",
	212: "MUL_64",
	213: "DIV_U_64",
	214: "DIV_S_64",
	215: "REM_U_64",
	216: "REM_S_64",
	217: "SHLO_L_64",
	218: "SHLO_R_64",
	219: "SHAR_R_64",
	220: "AND",
	221: "XOR",
	222: "OR",
	223: "MUL_UPPER_S_S",
	224: "MUL_UPPER_U_U",
	225: "MUL_UPPER_S_U",
	226: "SET_LT_U",
	227: "SET_LT_S",
	228: "CMOV_IZ",
	229: "CMOV_NZ",
	230: "ROT_L_64",
	231: "ROT_L_32",
	232: "ROT_R_64",
	233: "ROT_R_32",
	234: "AND_INV",
	235: "OR_INV",
	236: "XOR_INV",
	237: "MAX_U",
	238: "MAX_S",
	239: "MIN_U",
	240: "MIN_S",
}

func opcodeStr(opcode byte) string {
	if name, ok := opcodeMap[opcode]; ok {
		return name
	}
	return fmt.Sprintf("UNKNOWN_%d", opcode)
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	verbose := false
	outputDir := ""
	var baseDir string

	// Parse arguments
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-v", "--verbose":
			verbose = true
		case "-o", "--output":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Error: -o requires an output directory argument\n")
				os.Exit(1)
			}
			i++
			outputDir = args[i]
		case "-h", "--help":
			printUsage()
			os.Exit(0)
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "Error: unknown option %s\n", args[i])
				printUsage()
				os.Exit(1)
			}
			baseDir = args[i]
		}
	}

	if baseDir == "" {
		fmt.Fprintf(os.Stderr, "Error: no trace directory specified\n")
		printUsage()
		os.Exit(1)
	}

	// Verify directory exists
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: directory does not exist: %s\n", baseDir)
		os.Exit(1)
	}

	// Default output directory is the input directory
	if outputDir == "" {
		outputDir = baseDir
	}

	// Check if this is a trace directory or a parent directory
	if isTraceDir(baseDir) {
		// Single trace directory
		logPath := filepath.Join(outputDir, "traces.log")
		if err := convertToFileAndStdout(baseDir, logPath, "", verbose); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Written: %s\n", logPath)
	} else {
		// Parent directory - process nested structure
		if err := processNestedTraces(baseDir, outputDir, verbose); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options] <trace_directory>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nConverts split PVM binary trace files (.gz) to PvmLogging text format.\n")
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "  -v, --verbose      Show logs in terminal (in addition to writing files)\n")
	fmt.Fprintf(os.Stderr, "  -o, --output DIR   Write logs to specified directory (default: input directory)\n")
	fmt.Fprintf(os.Stderr, "  -h, --help         Show this help message\n")
	fmt.Fprintf(os.Stderr, "\nOutput files for nested structures (e.g., 0/516569628, 0/1985398958):\n")
	fmt.Fprintf(os.Stderr, "  - 0_all_traces.log        Combined log for work item 0\n")
	fmt.Fprintf(os.Stderr, "  - 0_516569628.log         Individual service log\n")
	fmt.Fprintf(os.Stderr, "  - 0_1985398958.log        Individual service log\n")
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  %s fuzzy/00000083/0/516569628    # Single trace, writes traces.log\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -v fuzzy/00000083/0/516569628 # Single trace, show in terminal\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s fuzzy/00000083                # Nested, writes grouped logs\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -v fuzzy/00000083             # Nested, show all in terminal\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s -o /tmp/logs fuzzy/00000083   # Write logs to /tmp/logs\n", os.Args[0])
}

// isTraceDir checks if a directory contains trace files (gas.gz, pc.gz, etc.)
func isTraceDir(dir string) bool {
	gasPath := filepath.Join(dir, "gas.gz")
	_, err := os.Stat(gasPath)
	return err == nil
}

// traceGroup represents a group of traces under a common work item index
type traceGroup struct {
	workItemIndex string            // e.g., "0"
	services      map[string]string // service ID -> trace directory path
}

// processNestedTraces handles the nested trace structure
// Structure: baseDir/workItemIndex/serviceID/[trace files]
func processNestedTraces(baseDir, outputDir string, verbose bool) error {
	// Find all trace directories and group them
	traceDirs, err := findTraceDirs(baseDir)
	if err != nil {
		return fmt.Errorf("failed to find trace directories: %v", err)
	}

	if len(traceDirs) == 0 {
		return fmt.Errorf("no trace directories found in %s", baseDir)
	}

	// Group traces by work item index (first level directory)
	groups := make(map[string]*traceGroup)

	for _, traceDir := range traceDirs {
		relPath, err := filepath.Rel(baseDir, traceDir)
		if err != nil {
			continue
		}

		parts := strings.Split(relPath, string(os.PathSeparator))
		if len(parts) >= 2 {
			// Nested structure: workItemIndex/serviceID
			workItemIndex := parts[0]
			serviceID := parts[1]

			if groups[workItemIndex] == nil {
				groups[workItemIndex] = &traceGroup{
					workItemIndex: workItemIndex,
					services:      make(map[string]string),
				}
			}
			groups[workItemIndex].services[serviceID] = traceDir
		} else if len(parts) == 1 {
			// Flat structure: just serviceID or single trace
			serviceID := parts[0]
			if groups[""] == nil {
				groups[""] = &traceGroup{
					workItemIndex: "",
					services:      make(map[string]string),
				}
			}
			groups[""].services[serviceID] = traceDir
		}
	}

	// Create output directory if needed
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Process each group
	for _, group := range groups {
		if err := processGroup(baseDir, outputDir, group, verbose); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}

	fmt.Fprintf(os.Stderr, "Done.\n")
	return nil
}

// processGroup writes logs for a single work item group
func processGroup(baseDir, outputDir string, group *traceGroup, verbose bool) error {
	prefix := group.workItemIndex
	if prefix == "" {
		prefix = "traces"
	}

	// Sort service IDs for consistent output
	var serviceIDs []string
	for id := range group.services {
		serviceIDs = append(serviceIDs, id)
	}
	sort.Strings(serviceIDs)

	// Write individual service logs
	for _, serviceID := range serviceIDs {
		traceDir := group.services[serviceID]
		var logName string
		if group.workItemIndex == "" {
			logName = fmt.Sprintf("%s.log", serviceID)
		} else {
			logName = fmt.Sprintf("%s_%s.log", prefix, serviceID)
		}
		logPath := filepath.Join(outputDir, logName)

		relPath, _ := filepath.Rel(baseDir, traceDir)
		fmt.Fprintf(os.Stderr, "Converting %s -> %s\n", relPath, logName)

		if err := convertToFileAndStdout(traceDir, logPath, relPath, verbose); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
		}
	}

	// Write combined log if there are multiple services
	if len(serviceIDs) > 1 {
		allLogName := fmt.Sprintf("%s_all_traces.log", prefix)
		allLogPath := filepath.Join(outputDir, allLogName)

		fmt.Fprintf(os.Stderr, "Creating combined log: %s\n", allLogName)

		allFile, err := os.Create(allLogPath)
		if err != nil {
			return fmt.Errorf("failed to create %s: %v", allLogPath, err)
		}
		defer allFile.Close()

		// Combined log only writes to file (individual logs already output to stdout if verbose)
		bw := bufio.NewWriter(allFile)
		defer bw.Flush()

		for _, serviceID := range serviceIDs {
			traceDir := group.services[serviceID]
			relPath, _ := filepath.Rel(baseDir, traceDir)

			fmt.Fprintf(bw, "\n# Trace: %s\n", relPath)
			if err := convertTrace(traceDir, bw); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning converting %s: %v\n", relPath, err)
			}
		}
	}

	return nil
}

// convertToFileAndStdout converts a trace directory to a log file, optionally also to stdout
func convertToFileAndStdout(traceDir, logPath, header string, verbose bool) error {
	outFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %v", logPath, err)
	}
	defer outFile.Close()

	var w io.Writer = outFile
	if verbose {
		// Write to both file and stdout
		w = io.MultiWriter(outFile, os.Stdout)
	}

	bw := bufio.NewWriter(w)
	defer bw.Flush()

	if header != "" {
		fmt.Fprintf(bw, "# Trace: %s\n", header)
	}

	return convertTrace(traceDir, bw)
}

// findTraceDirs recursively finds all directories containing trace files
func findTraceDirs(baseDir string) ([]string, error) {
	var traceDirs []string

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && isTraceDir(path) {
			traceDirs = append(traceDirs, path)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by numeric order if possible
	sort.Slice(traceDirs, func(i, j int) bool {
		relI, _ := filepath.Rel(baseDir, traceDirs[i])
		relJ, _ := filepath.Rel(baseDir, traceDirs[j])
		return comparePathsNumeric(relI, relJ)
	})

	return traceDirs, nil
}

// comparePathsNumeric compares paths with numeric sorting for each component
func comparePathsNumeric(a, b string) bool {
	partsA := strings.Split(a, string(os.PathSeparator))
	partsB := strings.Split(b, string(os.PathSeparator))

	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}

	for i := 0; i < minLen; i++ {
		numA, errA := strconv.ParseInt(partsA[i], 10, 64)
		numB, errB := strconv.ParseInt(partsB[i], 10, 64)

		if errA == nil && errB == nil {
			if numA != numB {
				return numA < numB
			}
		} else {
			if partsA[i] != partsB[i] {
				return partsA[i] < partsB[i]
			}
		}
	}

	return len(partsA) < len(partsB)
}

func convertTrace(baseDir string, w io.Writer) error {
	// Open all trace files
	readers := make([]*gzip.Reader, 16)
	files := make([]*os.File, 16)
	filenames := []string{
		"r0.gz", "r1.gz", "r2.gz", "r3.gz", "r4.gz", "r5.gz", "r6.gz",
		"r7.gz", "r8.gz", "r9.gz", "r10.gz", "r11.gz", "r12.gz",
		"opcode.gz", "gas.gz", "pc.gz",
	}

	// Open all files
	for i, name := range filenames {
		path := filepath.Join(baseDir, name)
		f, err := os.Open(path)
		if err != nil {
			// Clean up already opened files
			for j := 0; j < i; j++ {
				readers[j].Close()
				files[j].Close()
			}
			return fmt.Errorf("failed to open %s: %v", path, err)
		}
		files[i] = f

		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			for j := 0; j < i; j++ {
				readers[j].Close()
				files[j].Close()
			}
			return fmt.Errorf("failed to create gzip reader for %s: %v", path, err)
		}
		readers[i] = gz
	}

	// Cleanup function
	defer func() {
		for i := range readers {
			if readers[i] != nil {
				readers[i].Close()
			}
			if files[i] != nil {
				files[i].Close()
			}
		}
	}()

	// Buffered readers for each trace
	bufReaders := make([]*bufio.Reader, 16)
	for i := range readers {
		bufReaders[i] = bufio.NewReader(readers[i])
	}

	// Buffered writer for output
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	// Read and convert
	stepn := int64(1)
	for {
		// Read 13 registers (int64 each)
		var registers [13]uint64
		for i := 0; i < 13; i++ {
			err := binary.Read(bufReaders[i], binary.LittleEndian, &registers[i])
			if err == io.EOF {
				return nil // Done
			}
			if err != nil {
				return fmt.Errorf("failed to read register %d at step %d: %v", i, stepn, err)
			}
		}

		// Read opcode (1 byte)
		var opcode byte
		err := binary.Read(bufReaders[13], binary.LittleEndian, &opcode)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to read opcode at step %d: %v", stepn, err)
		}

		// Read gas (int64)
		var gas int64
		err = binary.Read(bufReaders[14], binary.LittleEndian, &gas)
		if err != nil {
			return fmt.Errorf("failed to read gas at step %d: %v", stepn, err)
		}

		// Read pc (int64)
		var pc int64
		err = binary.Read(bufReaders[15], binary.LittleEndian, &pc)
		if err != nil {
			return fmt.Errorf("failed to read pc at step %d: %v", stepn, err)
		}

		// Format output like PvmLogging
		// Format: OPCODE_NAME step_number pc Gas: gas_value Registers:[r0, r1, ..., r12]
		fmt.Fprintf(bw, "%s %d %d Gas: %d Registers:[%d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d, %d]\n",
			opcodeStr(opcode), stepn, pc, gas,
			registers[0], registers[1], registers[2], registers[3],
			registers[4], registers[5], registers[6], registers[7],
			registers[8], registers[9], registers[10], registers[11], registers[12])

		stepn++
	}
}
