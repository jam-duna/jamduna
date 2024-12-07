## Service Test Vectors

This "services" directory has **64-bit** PVM services compiled with polkadot:

* `bootstrap` - included in tiny genesis state (confirmed working with "make fib")
* `fib` - used in `mode=assurances` (confirmed working with "make fib")
* `tribonacci`, `megatron` - used in `mode=orderedaccumulation` (not confirmed yet)
* `xor` - good for testing invoke (not confirmed yet)

At present, these are all built with `polkatool`, where we maintain our own fork of `koute/pvm` in the `colorfulnotion/polkavm` repo.  

These are additional services to be developed:
* `sp1verifier` -- actually useful service for ethproofs.org competitor
* `keccak256` - good for testing invoke
* `add` - simplest service, best for learning polkatool
* `pell`, `racaman` - additional services useful to test more complex megatron and multi-segment cases

Style guidelines:
* put the JAM-ready blobs in the top level services directory, and use `common.GetFilePath("/services/xyz_blob.pvm")` to reference the blob.  
* put the disassembled code a txt file and a xyz_blob.pvm file (with magic bytes) within each services directory
* keep a public copy in `colorfulnotion/polkavm`

# WIP

* [William+Shawn] make megatron: update {megatron,tribonacci}.txt, {megatron,tribonacci}.pvm, {megatron,trib}/blob.pvm
* [Sourabh] make sp1verifier: update sp1 using README successfully

At present, `polkatool` uses the old opcodes and there are no 64-bit test vectors, but we only need to make these 12-13 opcodes work to achieve the above
```
const (
	LOAD_IND_U32 = 1       // 0x01 TEMP ==> 118
	ADD_IMM_32 = 2         // 0x02 TEMP ==> 121
	STORE_IND_U32 = 3      // 0x03 TEMP ==> 112
	LOAD_IMM  = 4          // 0x04 TEMP ==> 51 NOTE: 64-bit twin = load_imm_64
	JUMP = 5               // 0x05 TEMP ==> 40

	BRANCH_EQ_IMM   = 7    // 0x07 TEMP ==> 81
	ADD_32  = 8            // 0x08 TEMP ==> 170 (previously ADD_REG)
	FALLTHROUGH = 17       // 0x11 TEMP ==> 1
	JUMP_IND  = 19         // 0x13 TEMP ==> 50
	STORE_IMM_IND_U8  = 26 // 0x1a TEMP ==> 70

	ECALLI = 78            // 0x4e TEMP ==> 10
	MOVE_REG = 82          // 0x52 TEMP ==> 100
  // LOAD_IMM_64 = 20       // might need to be 4
)
```
