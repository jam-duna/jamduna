# trace2log

Converts split PVM binary trace files (`.gz`) to human-readable PvmLogging text format.

## Building

```bash
# Build for macOS (arm64) and Linux (amd64)
make -C cmd/trace2log

# Or build for current platform only
make -C cmd/trace2log local
```

## Usage

```bash
trace2log [options] <trace_directory>
```

### Options

| Flag | Description |
|------|-------------|
| `-v, --verbose` | Show logs in terminal (in addition to writing files) |
| `-o, --output DIR` | Write logs to specified directory (default: input directory) |
| `-h, --help` | Show help message |

### Examples

```bash
# Single trace directory - writes traces.log to the trace directory
trace2log /path/to/traces/0/516569628

# Single trace directory with verbose output
trace2log -v /path/to/traces/0/516569628

# Nested trace structure - writes grouped log files
trace2log /path/to/traces

# Write logs to a different output directory
trace2log -o /tmp/logs /path/to/traces
```

## Trace Directory Structure

The tool handles two types of directory structures:

### Single Trace Directory

A directory containing the raw trace files:
```
traces/0/516569628/
├── gas.gz
├── pc.gz
├── opcode.gz
├── r0.gz
├── r1.gz
...
└── r12.gz
```

Output: `traces.log` in the trace directory.

### Nested Trace Structure

A parent directory with work item indices and service IDs:
```
traces/
├── 0/                      # Work item index
│   ├── 516569628/          # Service ID
│   │   ├── gas.gz
│   │   └── ...
│   └── 1985398958/         # Service ID
│       ├── gas.gz
│       └── ...
└── 1/                      # Work item index
    └── 516569628/
        ├── gas.gz
        └── ...
```

Output files:
- `0_516569628.log` - Individual service log
- `0_1985398958.log` - Individual service log
- `0_all_traces.log` - Combined log for work item 0
- `1_516569628.log` - Individual service log

## Output Format

The output follows the PvmLogging format:

```
OPCODE_NAME step_number pc Gas: gas_value Registers:[r0, r1, r2, r3, r4, r5, r6, r7, r8, r9, r10, r11, r12]
```

Example:
```
# Trace: 0/516569628
LOAD_IMM 1 0 Gas: 10000000 Registers:[0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
ADD_64 2 5 Gas: 9999999 Registers:[42, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
STORE_U64 3 10 Gas: 9999998 Registers:[42, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]
```

## Generating Traces

From the jam repo root:

```bash
# 1. Generate traces
go test -run TestTracesInterpreter/fuzzy/00000083 -v ./statedb
# Output: statedb/fuzzy/00000083/

# 2. Build trace2log
make -C cmd/trace2log local
# Output:
# Built trace2log
```

### Converting Traces

```bash
# Convert all traces in nested structure
./cmd/trace2log/trace2log statedb/fuzzy/00000083
```

Expected output files:

```text
statedb/fuzzy/00000083/
├── 0/                         # Input trace directories (unchanged)
├── 0_0.log                    # service 0
├── 0_516569628.log            # service 516569628
├── 0_1985398958.log           # service 1985398958
├── 0_2647367265.log           # service 2647367265
├── 0_3953987607.log           # service 3953987607
├── 0_3953987649.log           # service 3953987649
└── 0_all_traces.log           # combined log for work item 0
```

Terminal output:

```text
Converting 0/0 -> 0_0.log
Converting 0/516569628 -> 0_516569628.log
Converting 0/1985398958 -> 0_1985398958.log
Converting 0/2647367265 -> 0_2647367265.log
Converting 0/3953987607 -> 0_3953987607.log
Converting 0/3953987649 -> 0_3953987649.log
Creating combined log: 0_all_traces.log
Done.
```

### Converting a Single Trace

```bash
./cmd/trace2log/trace2log statedb/fuzzy/00000083/0/516569628
```

Expected output files:

```text
statedb/fuzzy/00000083/0/516569628/
├── gas.gz, pc.gz, opcode.gz, ...   # Input trace files (unchanged)
└── traces.log                       # Converted PvmLogging output
```

Terminal output:

```text
Written: statedb/fuzzy/00000083/0/516569628/traces.log
```

### Other Options

```bash
# With verbose (also prints trace content to terminal)
./cmd/trace2log/trace2log -v statedb/fuzzy/00000083

# Output to different directory
./cmd/trace2log/trace2log -o /tmp/logs statedb/fuzzy/00000083
# Output: /tmp/logs/0_*.log files
```
