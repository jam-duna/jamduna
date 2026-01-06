package performance

import "time"

// BackendBenchResult captures timing stats for a backend at a specific iteration count.
type BackendBenchResult struct {
	Backend        string  `json:"backend"`
	Iterations     uint64  `json:"iterations"`
	RunDurationsNs []int64 `json:"run_durations_ns"`
	AvgNs          int64   `json:"avg_ns"`
	MinNs          int64   `json:"min_ns"`
	MaxNs          int64   `json:"max_ns"`
}

// BackendBenchReport stores the full benchmark run for interpreter vs recompiler.
type BackendBenchReport struct {
	Program          string               `json:"program"`
	ProgramPath      string               `json:"program_path"`
	InitialGas       uint64               `json:"initial_gas"`
	RunsPerIteration int                  `json:"runs_per_iteration"`
	Iterations       []uint64             `json:"iterations"`
	Backends         []string             `json:"backends"`
	Results          []BackendBenchResult `json:"results"`
	GeneratedAt      time.Time            `json:"generated_at"`
}
