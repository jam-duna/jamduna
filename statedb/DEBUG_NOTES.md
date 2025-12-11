# Debug Notes: TestBundleExecution Result Corruption

## Issue Summary

**Test**: `TestBundleExecution` in `refine_test.go`
**Status**: FAILING (both Go and C interpreters - never worked for C interpreter)

The first 2 bytes of the work result are corrupted:
- **Expected**: `0x7b 0x72` (ASCII "{r")
- **Actual**: `0x69 0x6f` (ASCII "io")

---

## Investigation Findings

### Corruption Chain Traced

1. **Final corruption**: STORE_IND_U8 instructions at PC=0x29eb and PC=0x29ed write `0x69` and `0x6f` to result buffer at `0x3ad40`

2. **Instruction details**:
   - PC=0x29eb: `args=0x86` → regA=6 (data), regB=8 (addr base), imm=0
   - PC=0x29ed: `args=0x8a01` → regA=10 (data), regB=8 (addr base), imm=1

3. **Register values at corruption time**:
   - Register 6: `0x7ffffffff05a6f69` (low byte = `0x69`)
   - Register 10: `0x7ffffffff05a6f` (low byte = `0x6f`)
   - Register 8: `0x3ad40` (result buffer address)

4. **Source of corrupted values**: These are **gas values** from child VM execution via `hostInvoke`

### Data Flow

```
hostInvoke writes gas 0x7ffffffff05a6f69 to parent memory at 0xfefdfa38
    ↓
Parent VM code copies value through stack
    ↓
STORE_IND_U64 at PC=0x332a7 stores to stack 0xfefdfb30
    ↓
LOAD_IND_U64 at PC=0x299f loads from 0xfefdfb30 to register 6
    ↓
STORE_IND_U8 at PC=0x29eb stores low byte (0x69) to result 0x3ad40
```

### Key Observations

1. **Same gas value stored twice** to stack address `0xfefdfb30` at PC=0x332a7 - suspicious

2. **Multiple INVOKE calls** produce gas values ending in `0x69`:
   ```
   postGas=0x7fffffffffc70469
   postGas=0x7ffffffff4faf069
   postGas=0x7ffffffff4faa269
   postGas=0x7ffffffff3ed9469
   postGas=0x7ffffffff183db69
   postGas=0x7ffffffff05a6f69  ← This one corrupts the result
   ```

3. **PEEK writes different data**: Last PEEK before corruption writes `0x90 0xf2` to result buffer (not `0x7b 0x72` either)

4. **Instruction encoding verified correct**: The STORE_IND_U8 instruction handling correctly extracts registers from args byte

---

## Current Status

### What We Know

- The STORE_IND_U8 instruction implementation is correct
- The register extraction (low/high nibble) is correct
- The corruption happens because register 6 contains a gas value instead of result data
- This issue affects **both Go and C interpreters** (never worked for C interpreter)

### What's Unclear

1. Why does the parent VM code store gas values to the result buffer location?
2. Is this a bug in both interpreters, or is the test expectation wrong?

### Gas Value Analysis

Checked all gas values from hostInvoke - **none have low bytes `0x7b 0x72`**:

- Searched for gas values ending in `0x727b` (little-endian) - none found
- The expected `0x7b 0x72` is NOT coming from gas values
- The expected result is from the bundle file test data, confirmed to be `0x7b725af0ffffff7f...`

### Result Buffer Confirmation

- Result buffer address: `0x3ad40`
- Result buffer length: 16919 bytes
- Register 7 (output address): `0x3ad40`
- Register 8 (output length): `16919`
- First 2 bytes returned: `0x69 0x6f` (wrong)
- Expected first 2 bytes: `0x7b 0x72`

---

## Files Modified During Investigation

### Functional fixes kept:
- `hostfunctions.go`: Fixed `hostHistoricalLookup` slice to `v[f:f+l]`
- `pvmgo.go`: 16/32-bit masking (`&0xFFFF`, `&0xFFFFFFFF`) and address wrap in `HandleTwoRegsOneImm`
- `pvmgo-instructions.go`: Nibble-packed decode for two-reg/immediate/offset/two-imms

### Debug code removed:
- All `fmt.Printf` debug statements were gated by env vars or removed
- No always-on debug logging remains in hot paths

---

## Next Steps

1. Verify if the expected result (`0x7b 0x72`) is from a known-correct reference implementation
2. Compare execution traces between our interpreter and reference
3. Check if the parent VM code is intentionally outputting gas-related data
4. Investigate why PEEK writes `0x90 0xf2` instead of `0x7b 0x72`

---

## Debug Environment Variables

For targeted debugging, use:
- `PVM_DEBUG_RESULT=1` - Enable result buffer tracing (currently no debug code uses this)

---

## Related Files

- `statedb/refine_test.go` - Test file
- `statedb/pvmgo.go` - Go interpreter
- `statedb/hostfunctions.go` - Host function implementations
- `statedb/pvm.go` - VM orchestration
- `statedb/TAINT_TRACKING.md` - Taint tracking documentation
