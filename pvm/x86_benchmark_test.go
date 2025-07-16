package pvm

import (
	"encoding/json"
	"os"
	"testing"
)

func BenchmarkInstRetHalt(b *testing.B) {
	// Enable any global tracing/logging you want *outside* measured loops:
	PvmLogging = true
	PvmTrace = true
	debugRecompiler = true

	// 1) Load & parse test case exactly once:
	const name = "inst_ret_halt"
	filePath := "../jamtestvectors/pvm/programs/" + name + ".json"

	data, err := os.ReadFile(filePath)
	if err != nil {
		b.Fatalf("Failed to read file %s: %v", filePath, err)
	}
	var tc TestCase
	if err := json.Unmarshal(data, &tc); err != nil {
		b.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// 2) Define your three modes:
	benchmarks := []struct {
		label string
		mode  string
		runFn func(TestCase) error
	}{
		{"pvm", "", pvm_test},
		{"emulator", "recompiler_sandbox", recompiler_sandbox_test},
		{"recompiler", "recompiler", recompiler_test},
	}

	// 3) Run each sub-benchmark:
	for _, bm := range benchmarks {
		b.Run(name+"_"+bm.label, func(b *testing.B) {
			// switch VM_MODE only if needed:
			orig := VM_MODE
			if bm.mode != "" {
				VM_MODE = bm.mode
			}
			defer func() { VM_MODE = orig }()

			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := bm.runFn(tc); err != nil {
					b.Fatalf("❌ [%s] failed: %v", bm.label, err)
				}
			}
		})
	}
}
