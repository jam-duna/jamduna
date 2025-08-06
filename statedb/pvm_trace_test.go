package statedb

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

type LogEntry struct {
	Opcode    string
	Step      int
	PC        uint64
	Registers []uint64
}

// Parse lines like "12:33:59.421 JUMP 0 57368 Registers:[4294901760, 4278059008, ...]"
func parseExpectedLog(path string) ([]LogEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 1: opcode, 2: step, 3: pc, 4: comma-separated registers
	re := regexp.MustCompile(`\S+\s+(\S+)\s+(\d+)\s+(\d+)\s+Registers:\[([0-9, ]+)\]`)
	var out []LogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		op := m[1]
		step, _ := strconv.Atoi(m[2])
		pc, _ := strconv.ParseUint(m[3], 10, 64)
		// Split registers
		regsStr := strings.Split(m[4], ",")
		regs := make([]uint64, len(regsStr))
		for i, s := range regsStr {
			s = strings.TrimSpace(s)
			regs[i], _ = strconv.ParseUint(s, 10, 64)
		}
		out = append(out, LogEntry{Opcode: op, Step: step, PC: pc, Registers: regs})
	}
	return out, scanner.Err()
}

// Parse lines like "accumulate: ADD_IMM_64  step:     1 pc: 57371 gas:9999992 Registers:[4294901760 4278058944 ...]"
func parseActualLog(path string) ([]LogEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// 1: opcode, 2: step, 3: pc, 4: space-separated registers
	re := regexp.MustCompile(`accumulate:\s+(\S+)\s+step:\s*(\d+)\s+pc:\s*(\d+)\s+gas:\d+\s+Registers:\[([0-9 ]+)\]`)
	var out []LogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		op := m[1]
		step, _ := strconv.Atoi(m[2])
		pc, _ := strconv.ParseUint(m[3], 10, 64)
		regsStr := strings.Fields(m[4])
		regs := make([]uint64, len(regsStr))
		for i, s := range regsStr {
			regs[i], _ = strconv.ParseUint(s, 10, 64)
		}
		out = append(out, LogEntry{Opcode: op, Step: step, PC: pc, Registers: regs})
	}
	return out, scanner.Err()
}

func TestInterpreterLogsMatch(t *testing.T) {
	exp, err := parseExpectedLog("javajam.log")
	if err != nil {
		t.Fatalf("failed to parse expected log: %v", err)
	}
	act, err := parseActualLog("jamduna.log")
	if err != nil {
		t.Fatalf("failed to parse actual log: %v", err)
	}

	for i := range exp {
		e, a := exp[i], act[i]
		if e.Opcode != a.Opcode {
			t.Fatalf("entry %d opcode: expected %q, got %q", i, e.Opcode, a.Opcode)
		}
		if e.Step != a.Step {
			t.Fatalf("entry %d step: expected %d, got %d", i, e.Step, a.Step)
		}
		if e.PC != a.PC {
			t.Fatalf("entry %d pc: expected %d, got %d", i, e.PC, a.PC)
		}
		if len(e.Registers) != len(a.Registers) {
			t.Fatalf("entry %d register count: expected %d, got %d", i, len(e.Registers), len(a.Registers))
			continue
		}
		for j := range e.Registers {
			if e.Registers[j] != a.Registers[j] {
				t.Fatalf("entry %d [%s], step %d, reg[%d]: expected %d, got %d", i, e.Opcode, e.Step, j, e.Registers[j], a.Registers[j])
			}
		}
	}

	if len(exp) != len(act) {
		t.Fatalf("entry count mismatch: expected %d entries, got %d", len(exp), len(act))
	}
}
