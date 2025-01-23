## Service Test Vectors

This "services" directory has **64-bit** PVM services compiled with polkatool:

* `bootstrap` - included in tiny genesis state (confirmed working with "make fib")
* `fib` - used in `mode=assurances` (confirmed working with "make fib")
* `tribonacci`, `megatron` - used in `mode=orderedaccumulation` (not confirmed yet)
* `megatron`
* `xor` - good for testing invoke (not confirmed yet)
* `balances` 

At present, these are all built with `polkatool`, where we maintain our own fork of `koute/pvm` in the `colorfulnotion/polkavm` repo.  

Style guidelines:
* put the JAM-ready blobs in the top level services directory, and use `common.GetFilePath("/services/xyz_blob.pvm")` to reference the blob.  
* put the disassembled code a txt file and a xyz_blob.pvm file (with magic bytes) within each services directory
* keep a public copy in `colorfulnotion/polkavm`

