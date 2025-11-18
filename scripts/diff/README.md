# PVM Trace Matching by Service ID

This tool properly matches jamDuna and JavaJAM PVM execution traces by actual service ID, enabling accurate comparison of gas accounting and implementation differences.

## Problem Statement

When comparing PVM traces from jamDuna (Go implementation) and JavaJAM (Java reference implementation), naive comparison fails because:

1. **Different execution order**: jamDuna executes services sequentially (sorted by ID), JavaJAM executes them in parallel (interleaved)
2. **No service markers in JavaJAM**: JavaJAM's trace doesn't include service ID annotations
3. **Interleaved logs**: Multiple services' instructions are mixed together in JavaJAM's output

This makes it impossible to directly compare "service 1" from both traces, as they might be completely different services.

## Solution

This tool performs a 2-step matching process:

### Step 1: Extract jamDuna Services
Uses existing `[service: X]` markers in jamDuna's PVM trace output. These markers are automatically included when jamDuna calls host functions (FETCH, TRANSFER, etc.) and already contain the service ID.

No code modifications required - jamDuna already outputs service markers in the trace.

### Step 2: Match JavaJAM by PC Sequence
Since JavaJAM doesn't log service IDs, we match by Program Counter (PC) sequences. The bytecode execution is deterministic, so the same service will have the same PC sequence in both implementations.

## Usage

### Step 1: Generate Traces

```bash
cd /path/to/jam
go test -v -run TestSingleFuzzTrace ./statedb 2>&1 | tee /tmp/jamduna_test.log
```

This generates:
- PVM trace: `/Users/michael/Downloads/XXXXXXXXXX_jamduna_trace.log`

You also need the corresponding JavaJAM trace:
- `/Users/michael/Downloads/XXXXXXXXXX_javajam_trace.log`

### Step 2: Run the Matcher

```bash
python3 scripts/diff/match_pvm_traces_by_service_id.py \
    --jamduna-trace /Users/michael/Downloads/1763371127_jamduna_trace.log \
    --javajam-trace /Users/michael/Downloads/1763371127_javajam_trace.log \
    --output /tmp/services_by_id
```

### Output

Creates matched service pairs in the output directory:

```
/tmp/services_by_id/
├── jamduna_service_0.log           ← Service ID 0
├── javajam_service_0.log           ← Service ID 0
├── jamduna_service_1011448405.log  ← Service ID 1011448405
├── javajam_service_1011448405.log  ← Service ID 1011448405
├── jamduna_service_2122968541.log  ← Service ID 2122968541
├── javajam_service_2122968541.log  ← Service ID 2122968541
└── ...
```

Each pair contains the **exact same service** execution, allowing direct comparison.