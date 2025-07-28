## Services

At present, these are all built with `polkatool`, where we maintain our own fork of `koute/pvm` in the `colorfulnotion/polkavm` repo.  

Style guidelines:
* put the JAM-ready blobs in the top level services directory, and use `common.GetFilePath("/services/xyz_blob.pvm")` to reference the blob.  
* put the disassembled code a txt file and a xyz_blob.pvm file (with magic bytes) within each services directory
* keep a public copy in `colorfulnotion/polkavm`

