# UBT Hash Specification (Blake3)

This document pins the hash semantics for the Go UBT implementation. The hash
function is **Blake3-256** (`github.com/zeebo/blake3`). SHA256 and other
functions are **not supported**.

## Definitions

- `ZERO32` = 32-byte zero array.
- `Stem` = 31-byte prefix of a `TreeKey`.
- `TreeKey` = `Stem` (31 bytes) + `Subindex` (1 byte).

## Shared Rule (All Profiles)

**Zero-hash special case**:
- `Hash64(ZERO32, ZERO32) = ZERO32`.
- This is required for the stem-subtree cache initialization to be correct.
- `Hash32` does **not** have a zero-hash rule.

## EIP Profile (Untagged Blake3)

### Hash32 (leaf)
```
Hash32(value[32]) = blake3(value)
```

### Hash64 (internal)
```
if left == ZERO32 && right == ZERO32:
    return ZERO32
else:
    return blake3(left || right)  // 64 bytes
```

### HashStemNode
```
HashStemNode(stem[31], subtreeRoot[32]) =
    blake3(stem || 0x00 || subtreeRoot)  // 64 bytes
```

## JAM Profile (Tagged Blake3)

### Hash32 (leaf)
```
Hash32(value[32]) = blake3(0x00 || value)  // 33 bytes
```

### Hash64 (internal)
```
if left == ZERO32 && right == ZERO32:
    return ZERO32
else:
    return blake3(0x01 || left || right)  // 65 bytes
```

### HashStemNode
```
HashStemNode(stem[31], subtreeRoot[32]) =
    blake3(0x02 || stem || 0x00 || subtreeRoot)  // 65 bytes
```

## Notes

- `HashRaw(input)` is a direct Blake3 hash of the input bytes and is not used
  for node hashing.
- Proof compatibility claims only apply when the profile matches the reference
  semantics.
