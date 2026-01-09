package rpc

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type parityVector struct {
	Name          string   `json:"name"`
	Script        string   `json:"script"`
	InitialStack  []string `json:"initial_stack"`
	ExpectedStack []string `json:"expected_stack"`
	ExpectedError string   `json:"expected_error"`
}

func loadParityVectors(t *testing.T) []parityVector {
	path := filepath.Join("..", "..", "..", "test_vectors", "transparent_script_parity.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read parity vectors: %v", err)
	}
	var vectors []parityVector
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatalf("parse parity vectors: %v", err)
	}
	return vectors
}

func decodeHex(t *testing.T, name, value string) []byte {
	if value == "" {
		return []byte{}
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("vector %s hex decode failed: %v", name, err)
	}
	return data
}

func mapScriptError(err error) string {
	if err == nil {
		return "ok"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "OP_RETURN"):
		return "op_return"
	case strings.Contains(msg, "stack underflow"):
		return "stack_underflow"
	case strings.Contains(msg, "VERIFY"):
		return "verify_failed"
	case strings.Contains(msg, "unsupported opcode") || strings.Contains(msg, "unknown opcode") || strings.Contains(msg, "disabled"):
		return "invalid_opcode"
	default:
		return "error"
	}
}

func TestTransparentScriptParityVectors(t *testing.T) {
	vectors := loadParityVectors(t)

	for _, vector := range vectors {
		vector := vector
		t.Run(vector.Name, func(t *testing.T) {
			engine := NewScriptEngine()
			for _, item := range vector.InitialStack {
				data := decodeHex(t, vector.Name, item)
				if err := engine.stack.Push(data); err != nil {
					t.Fatalf("vector %s stack push failed: %v", vector.Name, err)
				}
			}

			script := decodeHex(t, vector.Name, vector.Script)
			err := engine.ExecuteScript(script)
			code := mapScriptError(err)
			if code != vector.ExpectedError {
				t.Fatalf("vector %s error mismatch: got %s want %s", vector.Name, code, vector.ExpectedError)
			}

			if code == "ok" {
				if len(engine.stack.data) != len(vector.ExpectedStack) {
					t.Fatalf("vector %s stack length mismatch: got %d want %d", vector.Name, len(engine.stack.data), len(vector.ExpectedStack))
				}
				for i, item := range engine.stack.data {
					got := hex.EncodeToString(item)
					if got != vector.ExpectedStack[i] {
						t.Fatalf("vector %s stack mismatch at %d: got %s want %s", vector.Name, i, got, vector.ExpectedStack[i])
					}
				}
			}
		})
	}
}
