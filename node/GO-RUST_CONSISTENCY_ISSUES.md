#  Review

**Block witnesses fetched with the wrong API.**  The newly introduced `ReadStateWitnessRaw` documents that EVM block objects are stored as raw `EvmBlockPayload` blobs without an `ObjectRef`.

BUG: both `readBlockByHash` and `readBlockNumberIndex` still call `ReadStateWitness`, which assumes the first 64 bytes contain an `ObjectRef` and will attempt to fetch payload segments from DA using bogus metadata. This breaks block retrieval and diverges from the Rust contract that exposes raw payloads for block objects. The fix is to use `ReadStateWitnessRaw` for block object IDs and bypass the `ObjectRef` path.【F:statedb/statedb_hostenv.go†L368-L399】【F:node/node_evm_block.go†L136-L185】