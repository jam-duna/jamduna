package storage

import "github.com/ethereum/go-ethereum/crypto"

// ChunkedCode represents packed code chunks (1 metadata byte + 31 bytes of code).
type ChunkedCode []byte

const (
	push1  = byte(0x60)
	push32 = byte(0x7f)
)

// ChunkifyCode splits bytecode into 31-byte chunks with a 1-byte push offset.
func ChunkifyCode(code []byte) ChunkedCode {
	var (
		chunkOffset = 0
		chunkCount  = len(code) / 31
		codeOffset  = 0
	)
	if len(code)%31 != 0 {
		chunkCount++
	}
	chunks := make([]byte, chunkCount*32)
	for i := 0; i < chunkCount; i++ {
		end := 31 * (i + 1)
		if len(code) < end {
			end = len(code)
		}
		copy(chunks[i*32+1:], code[31*i:end])

		if chunkOffset > 31 {
			chunks[i*32] = 31
			chunkOffset = 1
			continue
		}
		chunks[32*i] = byte(chunkOffset)
		chunkOffset = 0

		for ; codeOffset < end; codeOffset++ {
			if code[codeOffset] >= push1 && code[codeOffset] <= push32 {
				codeOffset += int(code[codeOffset] - push1 + 1)
				if codeOffset+1 >= 31*(i+1) {
					codeOffset++
					chunkOffset = codeOffset - 31*(i+1)
					break
				}
			}
		}
	}
	return chunks
}

func emptyCodeHash() [32]byte {
	var out [32]byte
	hash := crypto.Keccak256(nil)
	copy(out[:], hash)
	return out
}
